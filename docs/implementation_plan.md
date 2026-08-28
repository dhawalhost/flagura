# Security Guardrails Implementation Plan

Establish comprehensive, production-grade security guardrails for the Flagura feature flag platform across CI/CD pipelines, API authorization, HTTP/network security headers, cookie hardening, and DoS mitigation.

---

## User Review Required

> [!IMPORTANT]
> **API Route Protection**: Once authentication middleware is enforced on mutation endpoints (`POST/PUT/PATCH/DELETE /api/v1/flags*` and `POST /api/v1/reset`), requests from external scripts or unauthorized callers without a valid session cookie or `Authorization: Bearer <token/key>` will receive `401 Unauthorized` (or `403 Forbidden` for non-admin resets). The UI dashboard already uses session cookies and will authenticate seamlessly.

> [!NOTE]
> Read endpoints used by SDKs (`GET /api/v1/flags` and `POST /api/v1/evaluate`) will allow both public read/evaluation and authenticated SDK access with optional Bearer API keys.

---

## Proposed Changes

Grouped by component layer:

### 1. HTTP Security, Authentication & Middleware Layer

#### [NEW] [middleware.go](file:///Users/dhawal.dyavanpalli/go/src/flagura/internal/api/middleware.go)
- **Security Headers Middleware**:
  - `X-Content-Type-Options: nosniff`
  - `X-Frame-Options: SAMEORIGIN`
  - `X-XSS-Protection: 1; mode=block`
  - `Referrer-Policy: strict-origin-when-cross-origin`
  - `Permissions-Policy: geolocation=(), camera=(), microphone=()`
  - Content Security Policy (CSP) allowing Tailwind, Lucide, Chart.js, Three.js, and Google Fonts.
- **Request Body Size Limit Middleware**:
  - Wrap incoming JSON request bodies with `http.MaxBytesReader(w, r.Body, 1<<20)` (1 MB limit) to prevent memory exhaustion and DoS attacks.
- **Authentication & Authorization Middleware**:
  - `RequireAuth(next http.HandlerFunc)`: Validates session token from cookie or `Authorization: Bearer <token>`, looks up user, injects authenticated `*domain.User` into request context. Rejects unauthenticated requests with `401 Unauthorized`.
  - `RequireRole(role domain.UserRole, next http.HandlerFunc)`: Enforces role permissions (e.g. `RoleAdmin` required for database reset `/api/v1/reset` and flag deletion).
  - Context helpers `UserFromContext(ctx)` to securely retrieve authenticated user and prevent spoofed `X-Actor` headers.

#### [MODIFY] [server.go](file:///Users/dhawal.dyavanpalli/go/src/flagura/internal/api/server.go)
- Wrap server routes with security headers, request body size limiter, and CORS middleware.
- Protect mutation routes:
  - `POST /api/v1/flags` -> Protected by `RequireAuth`
  - `PUT/PATCH /api/v1/flags/*` -> Protected by `RequireAuth`
  - `PATCH /api/v1/flags/*/toggle` -> Protected by `RequireAuth`
  - `PATCH /api/v1/flags/*/rollout` -> Protected by `RequireAuth`
  - `DELETE /api/v1/flags/*` -> Protected by `RequireAuth` + Role check (`Admin`/`Developer`)
  - `POST /api/v1/reset` -> Protected by `RequireAuth` + `RequireRole(domain.RoleAdmin)`

#### [MODIFY] [handlers_auth.go](file:///Users/dhawal.dyavanpalli/go/src/flagura/internal/api/handlers_auth.go)
- Dynamically set `Secure: true` on cookies when request is over HTTPS or `ENVIRONMENT=production`.

#### [MODIFY] [handlers_api.go](file:///Users/dhawal.dyavanpalli/go/src/flagura/internal/api/handlers_api.go)
- Replace unverified `r.Header.Get("X-Actor")` with authenticated user email from context (`UserFromContext(r.Context()).Email`), ensuring audit log integrity.

---

### 2. CI/CD & Repository Security Guardrails

#### [MODIFY] [.github/workflows/ci.yml](file:///Users/dhawal.dyavanpalli/go/src/flagura/.github/workflows/ci.yml)
- Add explicit least-privilege workflow permissions: `permissions: contents: read`.
- Add **SAST**: Run `gosec` (Go Security Checker) on all Go packages.
- Add **SCA**: Run `govulncheck` to detect known CVEs in Go dependencies.
- Add **Secret Scanning**: Run Gitleaks action or scanner to detect accidental credential commits.

#### [NEW] [.github/dependabot.yml](file:///Users/dhawal.dyavanpalli/.gemini/antigravity-ide/brain/79264107-667c-440c-b123-aeac9a36e16b/.github/dependabot.yml) -> [.github/dependabot.yml](file:///Users/dhawal.dyavanpalli/go/src/flagura/.github/dependabot.yml)
- Configure automated weekly dependency security updates for Go modules (`gomod`) and GitHub Actions (`github-actions`).

---

### 3. Automated Security Testing Suite

#### [NEW] [security_test.go](file:///Users/dhawal.dyavanpalli/go/src/flagura/internal/api/security_test.go)
- Test security response headers on all endpoints.
- Test rejection of unauthenticated mutation calls to `/api/v1/flags` and `/api/v1/reset` (401 Unauthorized).
- Test role enforcement on `/api/v1/reset` (403 Forbidden for non-admin, 200 OK for admin).
- Test audit log actor verification (ensuring authenticated user is recorded in audit logs).
- Test request body size limit enforcement (rejecting oversized payloads).

---

## Verification Plan

### Automated Tests
```bash
# 1. Run all existing and new security unit tests
go test -v -race ./...

# 2. Run Go vulnerability check
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...

# 3. Run Gosec security scanner
go install github.com/securego/gosec/v2/cmd/gosec@latest
gosec ./...
```

### Manual Verification
1. Verify web UI dashboard loads with all security headers.
2. Verify authenticated user can toggle/edit flags and the audit log records their actual authenticated email.
3. Verify unauthenticated `curl -X POST http://localhost:3000/api/v1/flags` receives `401 Unauthorized`.
