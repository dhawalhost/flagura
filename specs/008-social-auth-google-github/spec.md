# Feature Specification: Social Authentication (Google & GitHub OAuth)

**Feature Slug**: `008-social-auth-google-github`  
**Status**: Specified  
**Created**: 2026-09-23  
**Owner**: Flagura Team / SDD Engine  

---

## 1. Objective & Motivation

Developers and operators adopting Flagura expect seamless, 1-click authentication without having to create and remember a separate password. While Spec 003 implemented enterprise-grade OIDC SSO for B2B organizations, self-service developers and open-source users require standard platform-level **OAuth 2.0 / OpenID Connect social sign-in** via **Google** and **GitHub**.

This specification defines platform-wide social authentication:
1. Environment-driven provider enablement (`GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`).
2. 1-click "Continue with Google" and "Continue with GitHub" on the `/auth` page for both Sign In and Sign Up tabs.
3. Automatic Just-In-Time account creation, avatar ingestion, and personal workspace auto-provisioning (Organization + Default Project) on first login.
4. Seamless email-based account linking when an existing user signs in via OAuth with the same verified email.
5. Strict CSRF protection with cryptographic state/nonce verification and dynamic TLS-enforced cookies.

---

## 2. User Stories

### US-1: One-Click Registration & Login with Google
**As a** developer visiting Flagura,  
**I want to** click "Continue with Google" to sign in or register with my Google account,  
**so that** I can access the feature flag dashboard instantly without setting a password.

### US-2: One-Click Registration & Login with GitHub
**As an** open-source developer or engineer,  
**I want to** click "Continue with GitHub" to authenticate using my GitHub identity,  
**so that** I can onboard directly using my developer profile and avatar.

### US-3: Automatic Personal Workspace Provisioning
**As a** new user registering through Google or GitHub for the first time,  
**I want** Flagura to automatically provision my personal Organization, assign me the `owner` role, and create a default Project with standard environments,  
**so that** I am immediately redirected to a fully functional dashboard without manual setup wizard friction.

### US-4: Automatic Account Linking by Verified Email
**As an** existing user who previously registered with email/password,  
**I want to** sign in using Google or GitHub matching my registered email,  
**so that** my social login authenticates my existing account and preserves all my existing flags, organizations, and permissions.

### US-5: Dynamic Provider Discovery in Auth UI
**As a** self-hosted administrator who may only configure GitHub (or neither),  
**I want** the auth page to only display social login buttons for providers that have valid credentials configured,  
**so that** users are never presented with broken or unconfigured authentication options.

---

## 3. Acceptance Criteria

### A. Provider Configuration & Discovery
- **AC-1.1**: The server checks for `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GITHUB_CLIENT_ID`, and `GITHUB_CLIENT_SECRET` at startup.
- **AC-1.2**: `GET /api/v1/auth/oauth/providers` returns a public JSON status indicating which providers are active:
  ```json
  {
    "google": true,
    "github": true
  }
  ```
- **AC-1.3**: The `/auth` page dynamically displays "Continue with Google" and "Continue with GitHub" buttons only when their respective flags are `true`. If neither is configured, the social login section is cleanly omitted.

### B. OAuth Initiation & CSRF Defense
- **AC-2.1**: `GET /api/v1/auth/oauth/{provider}/login` initiates authorization:
  - Generates a 32-byte cryptographically secure random `state` and `nonce` (`crypto/rand`).
  - Sets an `HttpOnly`, `SameSite=Lax`, dynamically secure (`isCookieSecure(r)`) cookie `flagura_oauth_state` with a 10-minute time-to-live.
  - Redirects to provider authorization endpoint with required scopes:
    - Google: `openid email profile`
    - GitHub: `read:user user:email`
- **AC-2.2**: If the requested provider is not enabled on the server, `GET /api/v1/auth/oauth/{provider}/login` returns `404 Not Found` with a structured `AppError`.

### C. OAuth Callback & Identity Verification
- **AC-3.1**: `GET /api/v1/auth/oauth/{provider}/callback` validates incoming query parameters:
  - Validates `state` against `flagura_oauth_state` cookie using constant-time comparison (`subtle.ConstantTimeCompare`).
  - Clears `flagura_oauth_state` cookie upon evaluation.
  - On error or user cancellation from provider, redirects to `/auth?error=<description>`.
- **AC-3.2**: Server exchanges authorization `code` for an access token via provider's token endpoint over HTTPS.
- **AC-3.3**: Server retrieves verified user profile:
  - **Google**: Parses ID token claims or queries `https://www.googleapis.com/oauth2/v3/userinfo` for `email`, `name`, `picture`, `sub`.
  - **GitHub**: Queries `https://api.github.com/user` for `name`, `avatar_url`, and `https://api.github.com/user/emails` to find the primary verified email (`primary == true && verified == true`). Unverified emails are rejected with HTTP 400.

### D. User Provisioning, Account Linking & Session Issuance
- **AC-4.1**: Account resolution:
  - If a user with the verified email exists: Link avatar if empty, update `UpdatedAt`, and authenticate the existing user.
  - If no user exists: Create a new `domain.User` with `RoleDeveloper`, generate a secure random unguessable password hash, populate `AvatarURL`, and persist to the store.
- **AC-4.2**: New user provisioning:
  - If a new user is created and no invite token is present: Automatically provision a dedicated multi-tenant Organization (`<Name>'s Workspace`), assign the user `owner` in `org_members`, and create a default Project with `development`, `staging`, `production` environments.
- **AC-4.3**: Session issuance:
  - Issue standard `domain.Session` with a 7-day expiration.
  - Set `flagura_session` (HttpOnly, SameSite=Lax, dynamic Secure) and `flagura_project_id` cookies.
  - Redirect user with HTTP 302 Found to `/dashboard`.

---

## 4. Edge Cases & Constraints

1. **Unverified Email on GitHub**: GitHub allows users to have unverified emails or multiple private emails. The callback handler MUST explicitly query `/user/emails` and require a verified email address. If no verified email exists, reject with a user-friendly error message.
2. **Missing Name from Provider**: If a user does not have a display name configured on GitHub or Google, default `Name` to the local-part of their email (e.g. `alex` from `alex@example.com`).
3. **State Cookie Tampering or Expiry**: If `state` in the callback does not match the cookie, or if the cookie is expired (> 10 minutes), fail with `Invalid or expired authorization state`.
4. **Self-Hosted Air-Gapped Deployments**: When neither Google nor GitHub credentials are provided, social auth remains dormant, zero outbound OAuth network calls are attempted, and the UI remains purely email/password + enterprise OIDC.
5. **SAST Security Compliance**: All code must pass `gosec -exclude-dir=web/views ./...` with 0 issues (CWE-798 credential heuristics, CSRF cookie security, and constant-time string comparisons).
