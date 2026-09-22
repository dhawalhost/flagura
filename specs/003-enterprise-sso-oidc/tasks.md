# Tasks: Enterprise SSO via OIDC (Spec 003)

**Status**: Complete  
**Spec**: `specs/003-enterprise-sso-oidc/spec.md`  
**Plan**: `specs/003-enterprise-sso-oidc/plan.md`  
**Created**: 2026-09-22  
**Completed**: 2026-09-22  

## Phase 1: Domain & Store Layer (TDD)
- [x] T1: Define `OIDCConfig` model in `pkg/domain/oidc.go`.
- [x] T2: Add OIDC persistence methods to `store.Store` interface in `pkg/store/store.go`.
- [x] T3: Add `oidc_configs` table DDL migration in `supabase/schema.sql`.
- [x] T4: Implement and test OIDC persistence in `pkg/store/memory.go` and `pkg/store/memory_test.go`.
- [x] T5: Implement and test OIDC persistence in `pkg/store/sqlite.go` and `pkg/store/sqlite_test.go`.
- [x] T6: Implement and test OIDC persistence in `pkg/store/postgres.go` and `pkg/store/postgres_test.go`.

## Phase 2: OIDC Handlers & Flow (TDD)
- [x] T7: Write mock OIDC IdP test helper in `pkg/api/handlers_oidc_test.go`.
- [x] T8: Implement `handleOIDCLogin` (flow initiation, state generation, transient cookie) in `pkg/api/handlers_oidc.go`.
- [x] T9: Implement `handleOIDCCallback` (code exchange, token validation, JIT user provisioning, session cookie issuance) in `pkg/api/handlers_oidc.go`.
- [x] T10: Write tests verifying CSRF state validation and token validation in `handlers_oidc_test.go`.
- [x] T11: Write tests verifying domain restrictions and JIT user provisioning in `handlers_oidc_test.go`.

## Phase 3: Admin Configuration Endpoints & RBAC
- [x] T12: Implement `handleGetOIDCConfig`, `handleSaveOIDCConfig`, and `handleDeleteOIDCConfig` in `pkg/api/handlers_oidc.go`.
- [x] T13: Write tests verifying that only org owners/admins can configure OIDC in `handlers_oidc_test.go`.

## Phase 4: UI & Template Integration
- [x] T14: Add "Sign in with SSO" button / modal to `web/views/auth.templ`.
- [x] T15: Add OIDC SSO configuration panel to organization settings in `web/views/profile_settings.templ`.
- [x] T16: Run `templ generate` and verify template tests in `web/views/views_test.go`.

## Phase 5: Quality Gate & Verification
- [x] T17: Run `go test ./... -count=1` — verify 100% pass across all packages.
- [x] T18: Run `gosec -exclude-dir=web/views ./...` — verify 0 security issues.
- [x] T19: Verify server compilation with `go build ./cmd/server/main.go`.
