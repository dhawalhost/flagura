# Plan 003 — Enterprise Identity & Access: SSO via OIDC

**Spec**: `specs/003-enterprise-sso-oidc/spec.md`  
**Status**: Complete  
**Date**: 2026-09-22  
**Completed**: 2026-09-22  

---

## 1. Architecture & Flow

### 1.1 OIDC Protocol Flow

```
[User Browser]           [Flagura Server]                     [OIDC IdP (Okta/Entra/Google)]
     |                           |                                         |
     |--- 1. Click SSO --------->|                                         |
     |    GET /auth/oidc/login   |                                         |
     |                           |-- Generate random state & nonce ------->|
     |                           |-- Set secure transient state cookie --->|
     |<-- 2. 302 Redirect -------|                                         |
     |       to IdP /authorize   |                                         |
     |                                                                     |
     |--- 3. User Authenticates with IdP Credentials --------------------->|
     |<-- 4. 302 Redirect with ?code=XYZ&state=ABC ------------------------|
     |                                                                     |
     |--- 5. GET /auth/oidc/callback?code=XYZ&state=ABC ------------------>|
     |                           |                                         |
     |                           |-- Verify state cookie (ConstantTime) -->|
     |                           |-- POST /token (code exchange) --------->|
     |                           |<-- Returns id_token & access_token -----|
     |                           |-- Validate id_token & parse claims ---->|
     |                           |-- Check allowed_domains --------------->|
     |                           |-- JIT User & OrgMember Provisioning --->|
     |                           |-- Issue flagura_session cookie -------->|
     |<-- 6. 302 Redirect -------|                                         |
     |       to /dashboard       |                                         |
```

### 1.2 Storage Abstraction (`store.Store`)
Add to `pkg/store/store.go`:
```go
SaveOIDCConfig(ctx context.Context, cfg domain.OIDCConfig) error
GetOIDCConfig(ctx context.Context, organizationID string) (*domain.OIDCConfig, error)
GetOIDCConfigByDomain(ctx context.Context, domain string) (*domain.OIDCConfig, error)
DeleteOIDCConfig(ctx context.Context, organizationID string) error
```

Implement in:
- `pkg/store/memory.go`: In-memory thread-safe map with `RWMutex`.
- `pkg/store/sqlite.go`: Table `oidc_configs` with parameterized SQL.
- `pkg/store/postgres.go`: Table `oidc_configs` with parameterized SQL and conflict upsert.
- `supabase/schema.sql`: Section 12 DDL migration.

### 1.3 HTTP Handlers & Endpoints (`pkg/api/handlers_oidc.go`)
1. **`handleOIDCLogin(w http.ResponseWriter, r *http.Request)`**:
   - Accepts `org_id` or `domain` query param.
   - Loads OIDC config for the organization. If not found or disabled, returns error.
   - Generates 32-byte cryptographic random state.
   - Stores encrypted/signed state payload in a short-lived cookie (`flagura_oidc_state`, HttpOnly, SameSite=Lax, MaxAge=600s).
   - Redirects to IdP authorization endpoint with `client_id`, `redirect_uri`, `scope`, `state`.
2. **`handleOIDCCallback(w http.ResponseWriter, r *http.Request)`**:
   - Reads `state` from URL and verifies against `flagura_oidc_state` cookie using `subtle.ConstantTimeCompare`.
   - Reads `code` from URL.
   - Exchanges code with IdP token endpoint using `http.Client` with timeout.
   - Validates `id_token` claims (`sub`, `email`, `name`).
   - Verifies email matches `allowed_domains` if configured.
   - JIT Provisions or links `domain.User` in store:
     - If user exists, retrieves user.
     - If user doesn't exist, creates user with `Role: domain.RoleDeveloper`.
     - Ensures user is an `org_member` of the tenant org with configured `default_role`.
   - Creates session via `s.store.CreateSession` and writes `flagura_session` cookie.
   - Clears `flagura_oidc_state` cookie.
   - Redirects to `/dashboard`.
3. **`handleGetOIDCConfig` / `handleSaveOIDCConfig` / `handleDeleteOIDCConfig`**:
   - Gated by `s.resolveAndAuthorizeProjectID` + `user.IsPrivileged()`.
   - Mask `client_secret` in GET responses.

---

## 2. Test Strategy

1. **Unit Tests (`pkg/store/`)**:
   - Test `SaveOIDCConfig`, `GetOIDCConfig`, `GetOIDCConfigByDomain`, and `DeleteOIDCConfig` on `MemoryStore`, `SQLiteStore`, and `PostgresStore`.
2. **Mock IdP Server (`pkg/api/handlers_oidc_test.go`)**:
   - Spin up `httptest.Server` simulating standard OIDC endpoints (`/authorize`, `/token`, `/userinfo`).
   - Test full login initiation -> callback -> JIT user creation -> session cookie validation.
   - Test CSRF state mismatch rejection (400 Bad Request).
   - Test domain restriction filtering: allowed `@company.com` succeeds, foreign `@external.org` receives 403 Forbidden.
   - Test org isolation: user authenticating via Org A cannot access Org B's projects.
   - Test non-privileged developer blocked from modifying OIDC settings.
3. **SAST Scan**:
   - `gosec -exclude-dir=web/views ./...` with 0 issues.
