# Tasks: SaaS Org-Scoped Role Model (Spec 001)

**Status**: Completed  
**Spec**: `specs/001-saas-org-scoped-role-model/spec.md`  
**Updated**: 2026-09-22  

## Phase 1: Test Scaffolding (TDD)
- [x] T1a: Write failing tests in `handlers_apikey_test.go` for org-owner creating admin key
- [x] T1b: Write failing tests in `handlers_apikey_test.go` for invited developer being downgraded
- [x] T1c: Write failing tests in `handlers_apikey_test.go` for org-owner creating all-env key
- [x] T1d: Write failing tests in `handlers_changerequest_test.go` for org-owner approving CR
- [x] T1e: Write failing tests in `handlers_changerequest_test.go` for invited developer blocked from approving CR

## Phase 2: Middleware & Privileges Implementation
- [x] T1f: Implement `resolveEffectiveOrgRole` and `isOrgPrivileged` in `pkg/api/middleware.go`
- [x] T2: Fix `pkg/api/handlers_apikey.go` auth gates to use `isOrgPrivileged`
- [x] T3: Fix `pkg/api/handlers_changerequest.go` approval gate to use `isOrgPrivileged`

## Phase 3: UI & Templates Integration
- [x] T4: Update `web/views/header.templ` to accept `effectiveOrgRole string` parameter and render dynamic role badge ("Owner" / "Admin" / "Developer")
- [x] T5: Update `pkg/api/handlers_ui.go` to resolve and pass effective org role to `views.Dashboard` and `views.HeaderBar`
- [x] T6: Remove deceptive "Your Role" dropdown from `web/views/auth.templ` signup form
- [x] T7: Regenerate templ Go files (`templ generate`) and update render tests

## Phase 4: Quality Gate Verification
- [x] T8: Run full test suite `go test ./... -count=1` — 100% pass across all packages
- [x] T9: Run SAST scanner `gosec -exclude-dir=web/views ./...` — 0 issues found
- [x] T10: Verify build with `go build ./cmd/server/main.go` — clean build

## Phase 5: Architecture Unification (ActiveMembership & Clean Signatures)
- [x] T11: Add `ActiveMembership *OrgMember` and `EffectiveRole()`, `IsPrivileged()` methods to `domain.User`
- [x] T12: Add `GetOrgMember` to `store.Store` interface and implementations (Memory, SQLite, Postgres)
- [x] T13: Single-pass enrichment in `resolveAndAuthorizeProjectID` via `enrichUserOrgMembership`
- [x] T14: Clean up `HeaderBar` and `Dashboard` signatures, removing `effectiveOrgRole` parameter
- [x] T15: Run `templ generate` and verify all package tests (`go test ./...` 100% pass, `gosec` 0 issues)
