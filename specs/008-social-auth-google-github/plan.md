# Technical Implementation Plan: Social Authentication (Google & GitHub OAuth)

**Feature Slug**: `008-social-auth-google-github`  
**Spec**: `specs/008-social-auth-google-github/spec.md`  
**Status**: Planned  
**Created**: 2026-09-23  

---

## 1. Architectural Approach

```mermaid
sequenceDiagram
    autonumber
    actor User as Developer Browser
    participant UI as Flagura UI (/auth)
    participant Server as Flagura Server (pkg/api)
    participant IdP as Google / GitHub OAuth
    participant DB as Flagura Store (SQLite/PostgreSQL)

    UI->>Server: GET /api/v1/auth/oauth/providers
    Server-->>UI: { "google": true, "github": true }
    UI->>UI: Render "Continue with Google" & "Continue with GitHub"

    User->>UI: Click "Continue with Google"
    UI->>Server: GET /api/v1/auth/oauth/google/login
    Server->>Server: Generate secure state & nonce; Set flagura_oauth_state cookie
    Server-->>User: 302 Redirect to IdP Authorize URL

    User->>IdP: Authorize Flagura Application
    IdP-->>User: 302 Redirect to /api/v1/auth/oauth/google/callback?code=...&state=...
    User->>Server: GET /api/v1/auth/oauth/google/callback
    Server->>Server: Verify state against flagura_oauth_state cookie (ConstantTimeCompare)
    Server->>IdP: POST /token (Exchange code for access_token)
    IdP-->>Server: { "access_token": "...", "id_token": "..." }
    Server->>IdP: Query User Profile (verified email, name, avatar)
    IdP-->>Server: Profile Payload
    
    alt User with verified email does not exist
        Server->>DB: CreateUser(name, email, avatar, random_password_hash)
        Server->>DB: CreateOrganization(name: "<Name>'s Workspace")
        Server->>DB: AddOrgMember(user_id, org_id, role="owner")
        Server->>DB: CreateProject(name: "Default Project", org_id)
    else User exists
        Server->>DB: Link Avatar & Touch UpdatedAt
    end

    Server->>DB: CreateSession(user_id, token, 7 days)
    Server-->>User: Set-Cookie: flagura_session & flagura_project_id; 302 Found -> /dashboard
```

---

## 2. Configuration & Data Model

### 2.1 Server Configuration (`pkg/config/config.go`)
Extend `Config` with platform-level OAuth client settings:
```go
type Config struct {
    // ... existing fields ...
    GoogleClientID     string `env:"GOOGLE_CLIENT_ID"`
    GoogleClientSecret string `env:"GOOGLE_CLIENT_SECRET"` // #nosec G101
    GitHubClientID     string `env:"GITHUB_CLIENT_ID"`
    GitHubClientSecret string `env:"GITHUB_CLIENT_SECRET"` // #nosec G101
}

func (c *Config) IsGoogleOAuthEnabled() bool {
    return c.GoogleClientID != "" && c.GoogleClientSecret != ""
}

func (c *Config) IsGitHubOAuthEnabled() bool {
    return c.GitHubClientID != "" && c.GitHubClientSecret != ""
}
```

### 2.2 Domain Constants (`pkg/domain/constants.go`)
```go
const (
    CookieOAuthStateName = "flagura_oauth_state"
)
```

### 2.3 Route Constants (`pkg/api/routes.go`)
```go
const (
    RouteAuthOAuthProviders = "/api/v1/auth/oauth/providers"
    RouteAuthOAuthGoogleLogin = "/api/v1/auth/oauth/google/login"
    RouteAuthOAuthGoogleCallback = "/api/v1/auth/oauth/google/callback"
    RouteAuthOAuthGitHubLogin = "/api/v1/auth/oauth/github/login"
    RouteAuthOAuthGitHubCallback = "/api/v1/auth/oauth/github/callback"
)
```

---

## 3. API Contracts

### 3.1 Public Provider Availability: `GET /api/v1/auth/oauth/providers`
- **Response**: `200 OK`
```json
{
  "google": true,
  "github": true
}
```

### 3.2 Provider Login Initiation: `GET /api/v1/auth/oauth/{provider}/login`
- **Supported Providers**: `google`, `github`.
- **Response**: `302 Found` with `Location` header pointing to provider authorize URL and `Set-Cookie: flagura_oauth_state=<base64-claims>; Path=/; HttpOnly; SameSite=Lax`.
- **404 Response**: If requested provider is not enabled on server.

### 3.3 Provider Callback: `GET /api/v1/auth/oauth/{provider}/callback`
- **Query Params**: `code`, `state` (or `error`, `error_description`).
- **Response**:
  - On Success: `302 Found` with `Location: /dashboard`, `Set-Cookie: flagura_session=...`, `Set-Cookie: flagura_project_id=...`.
  - On Error: `302 Found` with `Location: /auth?error=<url_encoded_message>`.

---

## 4. Multi-Tenancy & Security Invariants

1. **CSRF State Token**: 32-byte cryptographic random token generated with `crypto/rand`, stored in a base64-encoded JSON cookie containing `provider`, `state`, `nonce`, and `created_at`.
2. **Constant-Time Verification**: `subtle.ConstantTimeCompare([]byte(stateParam), []byte(cookieClaims.State)) == 1`.
3. **Verified Email Enforcement**:
   - Google: Requires verified email claim in ID token or userinfo.
   - GitHub: Queries `/user/emails` endpoint and filters for `verified == true && primary == true`.
4. **Isolated Personal Tenancy**: New users receive a brand new `domain.Organization` and `domain.Project`, maintaining strict multi-tenancy isolation (`resolveAndAuthorizeProjectID`).
5. **Gosec SAST Protection**:
   - `// #nosec G101` on OAuth credential field definitions.
   - Dynamic cookie TLS enforcement (`isCookieSecure(r)`).
   - Zero SQL concatenation.

---

## 5. Testing & Verification Plan

### Automated Unit Tests (`pkg/api/handlers_oauth_test.go`)
1. `TestOAuthProvidersEndpoint`: Asserts returned JSON accurately reflects server `Config`.
2. `TestOAuthLogin_RedirectsAndSetsStateCookie`: Verifies Google and GitHub 302 redirects, URL query params, and `flagura_oauth_state` cookie structure.
3. `TestOAuthCallback_StateMismatch_Fails`: Verifies CSRF failure when cookie state differs from callback query param.
4. `TestOAuthCallback_Google_NewUserProvisioning`: Tests mock Google token exchange and userinfo, asserting that user, organization, project, and session are properly created.
5. `TestOAuthCallback_GitHub_ExistingUserAccountLinking`: Tests mock GitHub token exchange and email verification, asserting existing user is authenticated without duplicate account creation.
6. `TestOAuthCallback_GitHub_UnverifiedEmail_Rejected`: Verifies rejection if GitHub primary email is unverified.

### Quality Gates
- `go test -race ./...` (0 data races)
- `gosec -exclude-dir=web/views ./...` (0 security issues)
- `go build ./cmd/server` & `go build ./cmd/cli`
