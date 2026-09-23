# Spec 007: Enterprise Reliability, Distributed Tracing & Tenant Rate Limiting

**Status**: Draft  
**Owner**: Dhawal Dyavanpalli  
**Created**: 2026-09-22  

---

## 1. Background & Problem Statement

Flagura's core architecture supports multi-tenancy and in-memory sub-microsecond flag evaluations. However, operating Flagura at enterprise scale requires production-grade reliability, telemetry, and tenant isolation:
1. **No Standard Backup/Restore Procedures (REQ-O01)**: While SQLite and PostgreSQL schemas exist, there are no automated scripts or CLI utilities to create consistent hot snapshots, export data, and execute point-in-time restores. Without tested restore drills, disaster recovery is unverified.
2. **Lack of Distributed Tracing (REQ-O02)**: Microservice architectures rely on distributed tracing to diagnose cross-service latency anomalies. Without OpenTelemetry (OTel) and W3C Trace Context propagation, operators cannot correlate an application flag check with Flagura control plane or database execution times.
3. **Global Un-Tiered Rate Limiting (REQ-O03)**: Rate limiting is currently fixed globally per IP without awareness of organization, project, or subscription tier. A single noisy or rogue tenant can exhaust global limits and degrade service for all other tenants sharing the cluster.

---

## 2. Requirements & User Stories

### User Story 1: Enterprise Backup, Restore & Disaster Recovery (REQ-O01 & REQ-O04)
As a platform site reliability engineer (SRE),  
I want automated, verifiable backup and restore commands for both SQLite and PostgreSQL backends,  
So that we can meet enterprise RTO (< 15 minutes) and RPO (< 1 hour) disaster recovery objectives.

#### Acceptance Criteria:
- CLI provides `flagura backup create --backend [sqlite|postgres] --file [path]` and `flagura backup restore --backend [sqlite|postgres] --file [path]`.
- SQLite backup utilizes SQLite online backup API (`VACUUM INTO` or transaction lock snapshot) to prevent lock contention with active readers/writers.
- PostgreSQL backup utilizes streaming `pg_dump` format with transactional consistency.
- Automated end-to-end integration test (`backup_restore_test.go`) seeds records, takes a snapshot, truncates/mutates data, restores from snapshot, and verifies full data consistency (flags, audit chain, users, API keys).
- Disaster recovery runbook in `docs/runbooks/disaster-recovery.md` defines RTO/RPO targets and concrete step-by-step restoration procedures.

### User Story 2: OpenTelemetry Distributed Tracing (REQ-O02)
As an enterprise DevOps engineer,  
I want Flagura HTTP requests and evaluation paths to propagate and emit OpenTelemetry spans,  
So that I can visualize full trace timelines across microservices and Flagura API/store interactions.

#### Acceptance Criteria:
- Server initializes OpenTelemetry tracer provider configurable via environment variables (`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME=flagura`).
- Inbound HTTP requests extract and propagate W3C Trace Context (`traceparent`, `tracestate`).
- Key hot paths emit structured spans with tenant attributes:
  - Evaluation (`flagura.evaluate`, attributes: `flag.key`, `project.id`, `eval.variant`, `eval.enabled`)
  - Mutations (`flagura.flag_save`, `flagura.flag_toggle`)
  - Storage operations (`flagura.store_query`, `store.backend`)
- Zero performance degradation on hot path when tracing is disabled or sampled.

### User Story 3: Per-Tenant & Tiered Rate Limiting (REQ-O03)
As a platform administrator running a multi-tenant Flagura installation,  
I want rate limits to be scoped per organization/project with tier-based thresholds,  
So that one tenant's burst traffic cannot starve other tenants, and higher tiers receive higher throughput limits.

#### Acceptance Criteria:
- Rate limiter scopes quotas by Tenant (`ProjectID` or `OrgID`) when authenticated, falling back to client IP for unauthenticated routes.
- Role-based traffic tier configuration:
  - `Anonymous` (unauthenticated / public IP): 120 req/minute, burst 30
  - `Authenticated` (authenticated developers & service tokens): 1,200 req/minute, burst 100
  - `System` (admin tokens & production clusters): 12,000 req/minute, burst 500
- Custom rate limits can be configured per project or through environment variables.
- When limit is exceeded, HTTP 429 Too Many Requests is returned with:
  - `Retry-After: <seconds>`
  - `X-RateLimit-Limit: <limit>`
  - `X-RateLimit-Remaining: 0`
  - `X-RateLimit-Reset: <unix_timestamp>`

---

## 3. Non-Functional Requirements
- **Security**: Backup files must preserve strict permissions (`0600`). API tokens and sensitive credentials must never be exported in plaintext traces or error logs.
- **Performance**: In-memory rate limiting must execute in < 5 microseconds per request without global lock contention.
- **Compatibility**: W3C Trace Context implementation must be compliant with W3C Trace Context Level 1 specifications.
