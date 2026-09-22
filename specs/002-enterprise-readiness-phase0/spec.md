# Spec 002 — Enterprise Production-Readiness Phase 0: Defects Closure & Architecture State Externalization

**Status**: Implemented  
**Created**: 2026-09-22  
**Updated**: 2026-09-22  
**Author**: Flagura Team / SDD Engine  

---

## 1. Background & Scope

The "Flagura Enterprise Production-Readiness Requirements" roadmap sets a strict phased progression:
Phase 0 (P0) must close all known correctness defects (Section 1: REQ-D01 to REQ-D08) and resolve externalized state architecture for horizontal scaling (Section 2: REQ-A01 to REQ-A04).
No Phase 1+ enterprise features (SSO, SCIM, Audit retention, GDPR deletion) may be started before Phase 0 is fully closed, because enterprise features depend directly on replica-safe persistence, shared rate-limiting guarantees, and accurate deduplicated analytics.

Auditing the current codebase reveals:
1. **Defects Status (REQ-D01 to REQ-D08)**:
   - Backend implementations for REQ-D02 (canary rollback status `NeedsAttention` on store error), REQ-D03 (stale change request conflict detection via `ConfigVersion`), REQ-D04 (canary stage advance only after write success), REQ-D05 (14-day flag hygiene threshold), REQ-D06 (multivariate weights sum to 100% validation), and REQ-D07 (raw attribute casing for regex evaluation) are already implemented and tested in the engine/backend.
   - **UI & Telemetry Gaps to Close**:
     - **REQ-D01**: In `web/views/experiment_modal.templ`, display an explicit statistical disclaimer regarding user deduplication and sample size inflation prevention. In `pkg/api/handlers_telemetry.go`, persist exposure events to `store.RecordExperimentEvents` so that experiment exposures survive process restarts and are visible across all cluster replicas.
     - **REQ-D08**: In `web/views/flag_editor.templ`, add `starts_with`, `greater_than_or_equal`, and `less_than_or_equal` to the UI operator dropdown `<select x-model="rule.operator">`.
2. **Architecture State Externalization (REQ-A01 to REQ-A04)**:
   - **REQ-A01**: Canary schedule state (`CanarySchedule`) is currently held in an in-memory Go map in `CanaryScheduler`. It must be backed by `store.Store` (`canary_schedules` table in SQLite/Postgres and map in MemoryStore) so any replica can read/write schedule state.
   - **REQ-A02**: Document the rate-limiting architecture and topology considerations. In high-availability multi-replica deployments behind an L4/L7 load balancer, token bucket limits are per-replica; provide clear operational documentation and formulas (divide intended global limit by replica count).
   - **REQ-A03**: Telemetry and experiment exposure counts must aggregate across replicas. By persisting incoming track events to the backing store (`RecordExperimentEvents`), experiment reports compute from shared store data rather than single-replica memory.
   - **REQ-A04**: Document the supported deployment topologies (single-instance vs. multi-replica with shared DB) in a dedicated "Scaling and High Availability" guide (`docs/scaling-and-ha.md` and linked in `README.md`).

---

## 2. User Stories & Acceptance Criteria

### US-1: Reliable Cross-Replica Canary Schedules (REQ-A01, REQ-D02, REQ-D04)
**As** a Platform Operator deploying Flagura across multiple replicas,  
**I want** canary rollout schedules to be persisted in the shared database and coordinated safely,  
**so that** rolling deployments and background evaluations maintain a consistent view without desynchronization or race conditions.

**Acceptance Criteria:**
- AC-1.1: `store.Store` defines methods to save, fetch, list active, and delete canary schedules (`SaveCanarySchedule`, `GetCanarySchedule`, `ListActiveCanarySchedules`, `DeleteCanarySchedule`).
- AC-1.2: Implementations exist for `MemoryStore`, `SQLiteStore`, and `PostgresStore`.
- AC-1.3: `CanaryScheduler` queries the store for active schedules and updates schedule state via the store.
- AC-1.4: Background evaluation only advances or rolls back a schedule when the underlying flag rollout write is confirmed.

### US-2: Deduplicated Experiment Telemetry Across Replicas (REQ-A03, REQ-D01)
**As** an Engineer running an A/B experiment on Flagura,  
**I want** evaluation and exposure counts to be persisted in the shared store and deduplicated by user identifier,  
**so that** experiment statistical significance reflects distinct exposed users regardless of which replica received the telemetry batch.

**Acceptance Criteria:**
- AC-2.1: `handleIngestTelemetry` converts received track events into `domain.ExperimentEvent` with `UserID` and persists them to `s.store.RecordExperimentEvents`.
- AC-2.2: `handleGetExperimentReport` reads experiment events from the store and computes unique exposed users per variant.
- AC-2.3: `web/views/experiment_modal.templ` contains an explicit statistical notice informing operators that sample size deduplication is enforced by distinct user IDs.

### US-3: Complete Targeting Operators in Flag Editor UI (REQ-D08)
**As** a Product Manager creating targeting rules in the dashboard,  
**I want** the operator dropdown in the Flag Editor to include `starts_with`, `greater_than_or_equal`, and `less_than_or_equal`,  
**so that** I can configure prefix matching and numeric inequality rules directly in the UI without resorting to regex workarounds.

**Acceptance Criteria:**
- AC-3.1: The operator `<select>` in `web/views/flag_editor.templ` contains options for `starts_with`, `greater_than_or_equal` (>=), and `less_than_or_equal` (<=).
- AC-3.2: Flags saved with these operators persist correctly and evaluate accurately in the engine.

### US-4: High Availability & Scaling Deployment Guide (REQ-A02, REQ-A04)
**As** an Enterprise Site Reliability Engineer (SRE),  
**I want** authoritative documentation on supported deployment topologies, multi-replica behaviors, and rate limit sizing,  
**so that** I can deploy Flagura in Kubernetes or cloud containers with predictable operational characteristics.

**Acceptance Criteria:**
- AC-4.1: `docs/scaling-and-ha.md` covers single-node vs. multi-replica topologies, database requirements (PostgreSQL for multi-replica), sticky canary behaviors, rate-limiting per-replica mechanics, and health check integration.
- AC-4.2: `README.md` references the Scaling & HA guide.

---

## 3. Constitution & Quality Gate Compliance
- Article I (Architecture): All store changes strictly implement `store.Store`.
- Article II (Multi-Tenancy): All canary and telemetry queries are scoped by `project_id`.
- Article III (Test-First): Unit and integration tests for store implementations, canary scheduler, and telemetry ingestion.
- Article V (Security): 0 gosec issues; strict CSP adherence in templates.
- Article VI (Observability): Structured logs on canary mutations and telemetry persistence.
