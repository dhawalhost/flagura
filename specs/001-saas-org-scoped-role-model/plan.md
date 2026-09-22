# Plan 001 — SaaS Org-Scoped Role Model

**Spec**: specs/001-saas-org-scoped-role-model/spec.md  
**Status**: Implemented  
**Updated**: 2026-09-22

---

## Architecture

Two role dimensions exist in the DB. The plan is:

```
users.role         → "developer" for all customers (unchanged)
                      "admin"    for Flagura platform staff (DB-only, never via UI)

org_members.role   → "owner"     self-signup auto-provisioned
                      "admin"    invited with elevated access
                      "developer" invited standard access
                      "viewer"   invited read-only access
```

All org-scoped privilege checks must use `org_members.role`, not `users.role`.

---

## New Helper: `resolveEffectiveOrgRole`

Location: `pkg/api/middleware.go` (or `pkg/api/handlers_flags.go` near `resolveAndAuthorizeProjectID`)

```go
// resolveEffectiveOrgRole returns the caller's org-scoped role for the project's organization.
// Platform admins (user.Role == domain.RoleAdmin) always get "admin" bypass.
// Returns "" if the user is not a member of the project's org.
func (s *Server) resolveEffectiveOrgRole(ctx context.Context, userID string, projectID string) (string, error) {
    // 1. Get the project to find its org
    proj, err := s.store.GetProject(ctx, projectID)
    if err != nil { return "", err }
    // 2. Look up org membership
    members, err := s.store.ListOrgMembers(ctx, proj.OrganizationID)
    if err != nil { return "", err }
    for _, m := range members {
        if m.UserID == userID {
            return m.Role, nil
        }
    }
    return "", nil // not a member
}
```

---

## Changes Required

### 1. `pkg/api/middleware.go` — New helper
- Add `resolveEffectiveOrgRole(ctx, userID, projectID) (string, error)`.
- Add `isOrgPrivileged(role string) bool` → returns true for `"owner"` or `"admin"`.

### 2. `pkg/api/handlers_apikey.go` — Fix API key creation gates
**Before** (line 79):
```go
if role == domain.RoleAdmin && (user == nil || user.Role != domain.RoleAdmin) {
    role = domain.RoleDeveloper
}
if env == "all" || env == "*" {
    if user == nil || user.Role != domain.RoleAdmin {
        env = string(domain.EnvProduction)
    }
}
```
**After**:
```go
// Resolve effective org role
projectID := s.resolveProjectID(r)
effectiveRole, _ := s.resolveEffectiveOrgRole(r.Context(), user.ID, projectID)
isPrivileged := s.isOrgPrivileged(effectiveRole) || user.Role == domain.RoleAdmin

if role == domain.RoleAdmin && !isPrivileged {
    role = domain.RoleDeveloper
}
if (env == "all" || env == "*") && !isPrivileged {
    env = string(domain.EnvProduction)
}
```

### 3. `pkg/api/handlers_changerequest.go` — Fix change request approval gate
**Before** (line 212):
```go
if user.Role == domain.RoleAdmin { ... }
```
**After**: check `resolveEffectiveOrgRole` → `isOrgPrivileged`.

### 4. `web/views/header.templ` — Role badge display via `user.EffectiveRole()`
The header displays the user's role badge based on their active organization membership.

Instead of polluting `views.Dashboard` and `views.HeaderBar` signatures with an ad-hoc `effectiveOrgRole string` parameter, `domain.User` has `ActiveMembership *domain.OrgMember` and helper methods:
- `user.EffectiveRole()`: dynamically returns `"owner"`, `"admin"`, `"developer"`, or `"viewer"` based on the active org.
- `user.IsPrivileged()`: returns true if the user is `"owner"` or `"admin"`.

`HeaderBar(user *domain.User, flags []domain.FeatureFlag, currentEnv string)` keeps its clean signature, and calls `user.EffectiveRole()` directly.

### 5. `web/views/auth.templ` — Remove "Your Role" dropdown
Remove lines 267–278 (the `<div>` containing the `Your Role` label and `<select>`).
The server-side `userRole := domain.RoleDeveloper` assignment stays unchanged.

---

## Store Interface

Added direct $O(1)$ lookup to `store.Store`:
```go
GetOrgMember(ctx context.Context, organizationID, userID string) (*domain.OrgMember, error)
```
Implemented across `MemoryStore`, `SQLiteStore`, and `PostgresStore`.

---

## Test Plan

### Existing tests that must continue to pass
- `auth_test.go`: "SECURITY VULNERABILITY" assertion (user cannot self-assign RoleAdmin).
- `handlers_apikey_test.go`: existing API key creation tests.

### New tests to add
- `TestEffectiveOrgRole_Owner`: self-signup user resolves to "owner" for their project.
- `TestEffectiveOrgRole_NotMember`: user not in org resolves to "".
- `TestAPIKeyCreation_OwnerCanCreateAdminKey`: org owner can create admin-role API key.
- `TestAPIKeyCreation_DeveloperCannotCreateAdminKey`: invited developer gets downgraded to developer role.
- `TestChangeRequestApproval_OwnerCanApprove`: org owner can approve CRs.
- `TestChangeRequestApproval_DeveloperCannotApprove`: invited developer gets 403.

---

## Migration

No DB migration needed. All `org_members` rows for self-signup users already have `role = "owner"`.

---

## Rollback

All changes are additive. The `resolveEffectiveOrgRole` helper can be disabled by reverting
the three handler call sites. The header template change is purely UI.
