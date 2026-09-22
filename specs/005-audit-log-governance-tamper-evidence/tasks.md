# Tasks: Audit Log Governance, Export & Cryptographic Tamper-Evidence (Spec 005)

**Status**: Complete  
**Spec**: `specs/005-audit-log-governance-tamper-evidence/spec.md`  
**Plan**: `specs/005-audit-log-governance-tamper-evidence/plan.md`  
**Created**: 2026-09-22  

## Phase 1: Domain & Model Layer (TDD)
- [x] T1: Update `AuditLogEntry` struct in `pkg/domain/flag.go` with `PrevHash` and `EntryHash`.
- [x] T2: Add `ComputeAuditEntryHash` and `AuditIntegrityResult` model in `pkg/domain/flag.go`.
- [x] T3: Add unit tests for `ComputeAuditEntryHash` in `pkg/domain/flag_test.go`.

## Phase 2: Store Layer & Hash Chaining (TDD)
- [x] T4: Update `store.Store` interface in `pkg/store/store.go` with audit verification, export, and purge methods.
- [x] T5: Update database schema in `supabase/schema.sql` (`prev_hash`, `entry_hash` columns and index).
- [x] T6: Implement and test hash chaining, verification, export, and purge in `pkg/store/memory.go` & `memory_test.go`.
- [x] T7: Implement and test hash chaining, verification, export, and purge in `pkg/store/sqlite.go` & `sqlite_test.go`.
- [x] T8: Implement and test hash chaining, verification, export, and purge in `pkg/store/postgres.go` & `postgres_test.go`.

## Phase 3: API Handlers & Endpoints (TDD)
- [x] T9: Author test suite in `pkg/api/handlers_audit_test.go` covering chain verification, export streaming (CSV/JSON/NDJSON), tampering detection, and RBAC purge.
- [x] T10: Implement `handleVerifyAuditChain`, `handleExportAuditLogs`, and `handlePurgeAuditLogs` in `pkg/api/handlers_audit.go`.
- [x] T11: Register routes in `pkg/api/server.go`.

## Phase 4: UI & Template Integration
- [x] T12: Update `web/views/audit_modal.templ` with integrity badge, re-verify trigger, export buttons, and hash display.
- [x] T13: Update `web/static/js/app.js` with `auditViewComponent` verification and export methods.
- [x] T14: Run `templ generate` and verify template tests in `web/views/views_test.go`.

## Phase 5: Quality Gate & Verification
- [x] T15: Run `go test ./... -count=1` — verify 100% pass across all packages.
- [x] T16: Run `gosec -exclude-dir=web/views ./...` — verify 0 security issues.
- [x] T17: Verify server compilation with `go build ./cmd/server/main.go`.
