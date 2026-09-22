# Spec 003 — Enterprise Identity & Access: SSO via OIDC

**Status**: Complete  
**Created**: 2026-09-22  
**Completed**: 2026-09-22  
**Author**: Flagura Team / SDD Engine  

---

## 1. Background & Scope

Enterprises require centralized authentication through OpenID Connect (OIDC) identity providers such as Okta, Microsoft Entra ID (Azure AD), Google Workspace, Ping Identity, or Keycloak. 

Flagura currently supports local username/password authentication with bcrypt hashing. Under **REQ-E01 (Enterprise SSO via OIDC)**, Flagura will allow organizations to configure their own enterprise OIDC identity provider as an addition to the native password login flow.

Key architectural requirements:
1. **Per-Organization Multi-Tenancy**: OIDC identity configurations are strictly isolated per tenant organization.
2. **Standard Session Reuse**: Post-OIDC authentication, the user receives the exact same hardened `flagura_session` and `flagura_project` cookies, seamlessly integrating with the existing session lifecycle, RBAC, and multi-tenant scoping.
3. **Just-In-Time (JIT) Provisioning**: Employees authenticating via their organization's IdP for the first time are automatically provisioned with a Flagura user account and joined to their organization with a configured default role (e.g., `developer`).
4. **Self-Service Organization Settings**: Organization owners/admins can configure and test their OIDC connection directly from the Flagura dashboard.

---

## 2. User Stories & Acceptance Criteria

### US-1: Organization SSO Login Initiation (REQ-E01)
**As** an enterprise employee,  
**I want** to sign into Flagura using my company's single sign-on provider,  
**so that** I do not need a separate password and my access is governed by company identity policies.

**Acceptance Criteria:**
- **AC-1.1**: The login view allows initiating SSO either by entering a work email (auto-discovering the organization via domain) or clicking an organization SSO button / link.
- **AC-1.2**: Flagura redirects the browser to the IdP's `/authorize` endpoint with `response_type=code`, `scope=openid profile email`, valid `client_id`, `redirect_uri`, and a cryptographically random `state` token stored in a secure cookie to prevent CSRF attacks.
- **AC-1.3**: The `state` parameter expires after 10 minutes and is single-use.

### US-2: OIDC Callback Verification & Session Issuance (REQ-E01)
**As** an authenticated enterprise user returning from the IdP,  
**I want** Flagura to verify my identity and log me in,  
**so that** I can access my organization's dashboard immediately.

**Acceptance Criteria:**
- **AC-2.1**: `/api/v1/auth/oidc/callback` validates the `state` parameter against the stored state cookie using constant-time comparison.
- **AC-2.2**: The server exchanges the authorization `code` with the IdP's token endpoint for an `id_token` and `access_token`.
- **AC-2.3**: The server validates the `id_token` (issuer, audience = `client_id`, expiry, and signature via JWKS or userinfo endpoint).
- **AC-2.4**: If the user already exists in Flagura, the session is issued for that user. If the user is new, JIT provisioning creates the user account and adds them as an `org_member` with the org's configured `default_role`.
- **AC-2.5**: If the organization has configured `allowed_domains` (e.g. `@acme.com`), any authenticated identity with a non-matching domain is rejected with a 403 Forbidden.
- **AC-2.6**: Issues standard `flagura_session` cookie (HttpOnly, SameSite=Lax, Secure) and redirects to `/dashboard`.

### US-3: Self-Service OIDC Configuration by Org Owners (REQ-E01)
**As** an organization owner or administrator,  
**I want** to configure my organization's OIDC connection in the dashboard,  
**so that** my team can authenticate through our corporate identity provider.

**Acceptance Criteria:**
- **AC-3.1**: Org owners and admins can configure `issuer_url`, `client_id`, `client_secret`, `allowed_domains`, and `default_role`.
- **AC-3.2**: Non-privileged users (`developer`, `viewer`) cannot view or modify OIDC configurations (403 Forbidden).
- **AC-3.3**: `client_secret` is stored securely and masked in API responses (`••••••••`).
- **AC-3.4**: Disabling or deleting an OIDC configuration immediately prevents further SSO logins for that organization without affecting existing password users.

---

## 3. Data Model & Storage Schema

Table: `oidc_configs`
```sql
CREATE TABLE IF NOT EXISTS oidc_configs (
    organization_id TEXT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT false,
    issuer_url TEXT NOT NULL,
    client_id TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    allowed_domains TEXT NOT NULL DEFAULT '',
    default_role TEXT NOT NULL DEFAULT 'developer',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_oidc_configs_enabled ON oidc_configs(enabled);
```

---

## 4. API Endpoints

1. `GET /api/v1/auth/oidc/login?org_id=...` or `?domain=...` — Initiates OIDC flow, redirects to IdP.
2. `GET /api/v1/auth/oidc/callback` — Handles authorization code exchange, JIT provisioning, and cookie issuance.
3. `GET /api/v1/organizations/:id/oidc` — Retrieves OIDC configuration for org (Owner/Admin only).
4. `PUT /api/v1/organizations/:id/oidc` — Creates or updates OIDC configuration for org (Owner/Admin only).
5. `DELETE /api/v1/organizations/:id/oidc` — Deletes/disables OIDC configuration for org (Owner/Admin only).

---

## 5. Security & Non-Functional Requirements

- **CSRF & State Security**: State tokens must contain ≥128 bits of cryptographic randomness (`crypto/rand`) and be verified with `subtle.ConstantTimeCompare`.
- **Open Redirect Prevention**: Post-login redirect targets are strictly validated to prevent open redirect vulnerabilities. Only internal relative paths (`/dashboard`, `/settings`) are permitted.
- **Multi-Tenant Isolation**: An OIDC configuration belonging to Org A cannot be used to authenticate into Org B.
- **Fast Evaluation Invariant**: OIDC authentication logic touches only login/callback endpoints and does not affect the hot flag evaluation path `/api/v1/evaluate`.
- **Zero SAST Issues**: Must pass `gosec -exclude-dir=web/views ./...` with 0 issues.
