# Tasks: Enterprise Production-Readiness Phase 0 (Spec 002)

**Status**: Completed  
**Spec**: `specs/002-enterprise-readiness-phase0/spec.md`  
**Updated**: 2026-09-22  

## Phase 1: Store Abstraction & Migrations (TDD)
- [x] T1: Add `SaveCanarySchedule`, `GetCanarySchedule`, `ListActiveCanarySchedules`, and `DeleteCanarySchedule` to `store.Store` interface in `pkg/store/store.go`.
- [x] T2: Add migration for `canary_schedules` table in `supabase/schema.sql`.
- [x] T3: Implement and test canary schedule storage in `pkg/store/memory.go` with unit tests in `pkg/store/memory_test.go`.
- [x] T4: Implement and test canary schedule storage in `pkg/store/sqlite.go` with unit tests in `pkg/store/sqlite_test.go`.
- [x] T5: Implement and test canary schedule storage in `pkg/store/postgres.go` with unit tests in `pkg/store/postgres_test.go`.

## Phase 2: Canary Scheduler State Externalization
- [x] T6: Update `pkg/canary/scheduler.go` to persist and load schedules via `store.Store`.
- [x] T7: Write unit tests in `pkg/canary/scheduler_test.go` verifying schedule persistence across simulated scheduler instances (multi-replica simulation).

## Phase 3: Telemetry Deduplication & Experiment Persistence
- [x] T8: Update `handleIngestTelemetry` in `pkg/api/handlers_telemetry.go` to persist incoming track/exposure events to `s.store.RecordExperimentEvents`.
- [x] T9: Add regression tests in `pkg/api/handlers_telemetry_test.go` verifying that ingested events are written to the store and read back in experiment reports.

## Phase 4: UI & Template Updates
- [x] T10: Add `starts_with`, `greater_than_or_equal`, and `less_than_or_equal` operators to the rule selector in `web/views/flag_editor.templ`.
- [x] T11: Add statistical deduplication note / disclaimer to `web/views/experiment_modal.templ`.
- [x] T12: Run `templ generate` and verify template unit tests in `web/views/views_test.go`.

## Phase 5: Documentation & Operations
- [x] T13: Author `docs/scaling-and-ha.md` covering supported deployment topologies (single-instance vs multi-replica), rate-limiting per-replica considerations, and database requirements.
- [x] T14: Reference `docs/scaling-and-ha.md` in `README.md`.

## Phase 6: Quality Gate Verification
- [x] T15: Run `go test ./... -count=1` — verify 100% pass across all packages.
- [x] T16: Run `gosec -exclude-dir=web/views ./...` — verify 0 security issues found.
- [x] T17: Run `go build ./cmd/server/main.go` — verify clean build.
