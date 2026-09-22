# Spec 001 — SaaS Org-Scoped Role Model

**Status**: Implemented  
**Created**: 2026-09-15  
**Updated**: 2026-09-22

---

## Background

Flagura operates as a multi-tenant SaaS feature-flagging platform. Every registered user
belongs to at least one Organization and at minimum one Project within it.

The database already models two distinct role dimensions:

| Dimension                     | Table         | Column | Semantics                                                                          |
| ----------------------------- | ------------- | ------ | ---------------------------------------------------------------------------------- |
| **Global platform role**      | `users`       | `role` | `admin` = Flagura platform superadmin (staff only). All customers are `developer`. |
| **Org-scoped workspace role** | `org_members` | `role` | `owner`, `admin`, `developer`, `viewer` within a specific tenant org.              |

When a user self-registers, the signup handler correctly provisions:

1. A personal Organization.
2. An `org_members` row with `role = "owner"` for that org.
3. A default Project inside it.

**The problem is not in signup logic — it is in how the role is displayed and enforced.**

---

## Problem Statement

The UI header displays `user.Role` (always `"developer"`) as the user's role badge, instead
of their org-scoped `org_members.role` (`"owner"` for self-signup users). This causes
confusion: an org owner sees "Role: developer" and believes they lack admin privileges.

Additionally, several authorization checks compare `user.Role == domain.RoleAdmin` to gate
privileged actions (e.g. creating admin-role API keys, cross-environment tokens). Because no
self-signup customer ever holds `domain.RoleAdmin` globally, these actions are currently
unreachable for workspace owners — which breaks core product functionality.

---

## User Stories

### US-1 — Workspace Owner Self-Signup

**As** a user who self-registers,  
**I want** to be recognized as the owner/administrator of my own workspace,  
**so that** I can create flags, admin API keys, manage environments, and invite colleagues.

**Acceptance Criteria:**

- AC-1.1: Header role badge shows `"Owner"` (org-scoped role), not `"developer"`.
- AC-1.2: Newly registered user can create an `admin`-role API key without a 403.
- AC-1.3: Newly registered user can generate `all`-environment tokens without downgrade.
- AC-1.4: Newly registered user can approve change requests within their own org.

### US-2 — Invited Collaborator

**As** an org owner,  
**I want** to invite colleagues with specific workspace roles (`developer`, `admin`, `viewer`),  
**so that** they have appropriate access scoped strictly to my organization.

**Acceptance Criteria:**

- AC-2.1: Invited `developer` cannot create `admin`-role API keys.
- AC-2.2: Invited `admin` can create `admin`-role API keys within the org.
- AC-2.3: Invited `viewer` has read-only access.
- AC-2.4: No invited user can see or act on another tenant's resources.

### US-3 — Platform Superadmin (Flagura Staff)

**As** a Flagura platform operator,  
**I want** a `global_role = admin` account to operate the platform across all tenants.

**Acceptance Criteria:**

- AC-3.1: `global_role = admin` is never assignable via public signup or invite flow.
- AC-3.2: Platform admins bypass org-member checks to access any tenant's resources.
- AC-3.3: `/api/v1/reset` remains guarded by `global_role = admin` exclusively.

### US-4 — Signup Form UX

**As** a new user filling out the registration form,  
**I want** no "Your Role" dropdown that has no effect on my actual role.

**Acceptance Criteria:**

- AC-4.1: The "Your Role" dropdown is removed from the signup form.
- AC-4.2: Server continues to reject `role` escalation in signup payload (existing auth_test.go security test must pass).

---

## Functional Requirements

### FR-1: Effective Role Resolution Helper

The system MUST expose a server-side helper `resolveEffectiveOrgRole(ctx, userID, projectID) string` returning the caller's `org_members.role` for the org that owns the given project.

Precedence:

1. `user.Role == domain.RoleAdmin` → effective role is `"admin"` (platform superadmin bypass).
2. Else look up `org_members.role` for the org owning the project.
3. Not a member → `""` (no access).

### FR-2: Org-Scoped Authorization Gates

The following MUST check effective role ∈ {`owner`, `admin`}:

- Creating an `admin`-role API key (`POST /api/v1/api-keys`).
- Creating an `all`-environment API key.
- Approving/rejecting change requests (`PATCH /api/v1/change-requests/:id`).
- Deleting a project or organization.

### FR-3: Header Role Display

The header role badge MUST display the user's `org_members.role` for their active project's org. Platform admins display `"Platform Admin"`.

### FR-4: Signup Role Dropdown Removal

`auth.templ` MUST NOT render a "Your Role" select. Workspace role is determined by signup path (self → owner) or invitation role.

### FR-5: No Change to Global Role Assignment

`user.Role = domain.RoleDeveloper` on all public signups. Existing `auth_test.go` security assertion MUST continue to pass.

---

## Non-Functional Requirements

- **Performance**: `resolveEffectiveOrgRole` adds ≤1 DB query per privileged request; result is cacheable in request context.
- **Security**: No public endpoint may elevate a user's effective role.
- **Observability**: Authorization failures MUST log `security_event` with `org_role`, `required_role`, `user_id`, `project_id`.
- **Backward Compatibility**: No migration of `users.role` values needed.

---

## Out of Scope

- SAML/SSO role mapping (future).
- Org-level audit log of role changes (future).

---

## Open Questions

None. Diagnosis confirmed by direct DB query.
