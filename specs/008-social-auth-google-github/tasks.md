# Tasks: Social Authentication (Google & GitHub OAuth) (Spec 008)

**Status**: Specified  
**Spec**: `specs/008-social-auth-google-github/spec.md`  
**Plan**: `specs/008-social-auth-google-github/plan.md`  
**Created**: 2026-09-23  

---

## Phase 1: Configuration & Domain Constants
- [x] T1: Add OAuth credential fields (`GoogleClientID`, `GoogleClientSecret`, `GitHubClientID`, `GitHubClientSecret`) and helper methods (`IsGoogleOAuthEnabled`, `IsGitHubOAuthEnabled`) in `pkg/config/config.go`.
- [x] T2: Add `CookieOAuthStateName` in `pkg/domain/constants.go`.
- [x] T3: Add OAuth route constants (`RouteAuthOAuthProviders`, `RouteAuthOAuthGoogleLogin`, etc.) in `pkg/api/routes.go`.

## Phase 2: OAuth Handlers & Logic (TDD)
- [x] T4: Write unit tests in `pkg/api/handlers_oauth_test.go` covering provider discovery, login initiation, CSRF state cookie verification, mock Google callback, and mock GitHub callback.
- [x] T5: Implement `handleOAuthProviders`, `handleOAuthLogin`, and `handleOAuthCallback` in `pkg/api/handlers_oauth.go`.
- [x] T6: Register OAuth routes in `pkg/api/server.go` using `s.handle` / `s.handleMethods`.
- [x] T7: Run `go test -v ./pkg/api -run "TestOAuth"` and confirm tests pass.

## Phase 3: Auth Page UI & Alpine.js Integration
- [x] T8: Update `web/views/auth.templ` with branded Google and GitHub "Continue with..." buttons for Sign In and Sign Up tabs.
- [x] T9: Update `web/static/js/app.js` (or inline component) to query `/api/v1/auth/oauth/providers` and conditionally display active buttons.
- [x] T10: Compile Templ templates via `templ generate` and verify UI layout rendering in `pkg/api/handlers_ui_test.go`.

## Phase 4: Documentation & Environment Updates
- [x] T11: Update `.env.example` with `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GITHUB_CLIENT_ID`, and `GITHUB_CLIENT_SECRET`.
- [x] T12: Update `docs/product/sdks-and-api.md` and `README.md` with social login setup guides.

## Phase 5: Verification & Quality Gates
- [x] T13: Run race detector `go test -race ./...`.
- [x] T14: Run security gate `gosec -exclude-dir=web/views ./...` (0 issues).
- [x] T15: Verify binary builds `go build ./cmd/server` and `go build ./cmd/cli`.
