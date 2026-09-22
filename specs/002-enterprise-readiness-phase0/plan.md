# Plan 002 — Enterprise Production-Readiness Phase 0: Architecture & State Externalization

**Spec**: `specs/002-enterprise-readiness-phase0/spec.md`  
**Status**: Implemented  
**Date**: 2026-09-22  
**Updated**: 2026-09-22  

---

## 1. Technical Architecture

### 1.1 Store Abstraction for Canary Schedules (`store.Store`)
To ensure canary schedule state survives process restarts and remains consistent across multiple replicas:
1. Extend `store.Store` with:
   - `SaveCanarySchedule(ctx context.Context, sched domain.CanarySchedule) error`
   - `GetCanarySchedule(ctx context.Context, projectID, flagKey string) (*domain.CanarySchedule, error)`
   - `ListActiveCanarySchedules(ctx context.Context) ([]domain.CanarySchedule, error)`
   - `DeleteCanarySchedule(ctx context.Context, projectID, flagKey string) error`
2. Update implementations:
   - `pkg/store/memory.go`: In-memory map with read-write mutex lock.
   - `pkg/store/sqlite.go`: Table `canary_schedules (project_id, flag_key, environment, status, current_stage_idx, stages_json, rollback_reason, created_at, updated_at, PRIMARY KEY (project_id, flag_key))`.
   - `pkg/store/postgres.go`: Same table structure, with SQL queries and JSON serialization for stages.
   - `supabase/schema.sql`: DDL migration for table `canary_schedules`.

### 1.2 Canary Scheduler Externalization (`pkg/canary/scheduler.go`)
1. In `CanaryScheduler`:
   - Replace or back `cs.schedules` with calls to `cs.store.SaveCanarySchedule`, `cs.store.GetCanarySchedule`, `cs.store.ListActiveCanarySchedules`, and `cs.store.DeleteCanarySchedule`.
   - On startup / evaluation tick, fetch active schedules via `ListActiveCanarySchedules`.
   - In `TriggerHealthRollback` and `EvaluateSchedules`: persist stage advancements and rollback statuses directly to `cs.store.SaveCanarySchedule`.
   - If store write fails, schedule enters `NeedsAttention` and does not advance.

### 1.3 Telemetry & Experiment Deduplication Persistence (`pkg/api/handlers_telemetry.go`)
1. In `handleIngestTelemetry`:
   - When batch of events with `user_id`, `flag_key`, `variant` is received:
     - Record in memory aggregator for fast metrics rendering.
     - Convert track events to `domain.ExperimentEvent{ProjectID: projectID, FlagKey: ev.FlagKey, Variant: ev.Variant, UserID: ev.UserID, EventType: domain.EventTypeExposure, Timestamp: ...}`.
     - Call `s.store.RecordExperimentEvents(r.Context(), events)` to persist to SQLite/Postgres.
   - In `handleGetExperimentReport`:
     - Reads events from `s.store.GetExperimentEventsByProject(r.Context(), projectID, flagKey, 10000)`.
     - Computes distinct exposed users per variant.

### 1.4 UI Enhancements
1. `web/views/flag_editor.templ`:
   - Add `<option value="starts_with">starts_with</option>`
   - Add `<option value="greater_than_or_equal">&gt;= (greater_than_or_equal)</option>`
   - Add `<option value="less_than_or_equal">&lt;= (less_than_or_equal)</option>`
2. `web/views/experiment_modal.templ`:
   - Add statistical sample deduplication banner and disclaimer note:
     `"Sample sizes represent distinct exposed user IDs. Repeated evaluations from the same user ID are deduplicated to ensure valid Z-test significance."`
3. Regenerate templates with `templ generate`.

### 1.5 Documentation & Operations Guide
1. Create `docs/scaling-and-ha.md`:
   - Document single-instance vs. multi-replica deployment topologies.
   - Explain shared PostgreSQL storage requirement for horizontal scaling.
   - Detail rate limiting per-replica sizing formula (`replica_limit = target_cluster_limit / replica_count`).
   - Describe canary schedule persistence and background evaluation behavior across replicas.
2. Update `README.md` to link to `docs/scaling-and-ha.md`.

---

## 2. Risk & Constitution Assessment
- **Article I (Architecture)**: Clean interface implementation in `pkg/store`.
- **Article II (Multi-Tenancy)**: All canary queries strictly filtered by `project_id`.
- **Article IV (Fast Path)**: Flag evaluation `/api/v1/evaluate` remains 100% in-memory without database queries; canary evaluation is in the background ticker loop.
- **Article V (Security & SAST)**: All SQL queries use parameterized placeholders (`?` for SQLite, `$1` for Postgres). 0 gosec issues.
- **Article VI (Observability)**: Structured logging on canary schedule state transitions.
