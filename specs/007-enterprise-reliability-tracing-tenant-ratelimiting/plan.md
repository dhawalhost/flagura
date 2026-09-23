# Architecture & Technical Plan: Enterprise Reliability, Tracing & Tenant Rate Limiting (Spec 007)

**Status**: Draft  
**Spec**: `specs/007-enterprise-reliability-tracing-tenant-ratelimiting/spec.md`  
**Created**: 2026-09-22  

---

## 1. Architecture Overview

```
+----------------------------------------------------------------------------------+
|                                FLAGURA SERVER                                    |
|                                                                                  |
|   +--------------------------------------------------------------------------+   |
|   |                       HTTP Transport Layer                               |   |
|   |                                                                          |   |
|   |   +---------------------------+       +------------------------------+   |   |
|   |   | W3C Tracing Middleware    |       | Role-Based Rate Limiter      |   |   |
|   |   | - Extract 'traceparent'   |       | - Anonymous (120/m, b 30)    |   |   |
|   |   | - Inject context & slog   | ----> | - Authenticated (1200/m)     |   |   |
|   |   | - Header 'X-Trace-ID'     |       | - System (12000/m, b 500)    |   |   |
|   |   +---------------------------+       +------------------------------+   |   |
|   +--------------------------------------------------------------------------+   |
|                                     |                                            |
|                                     v                                            |
|   +--------------------------------------------------------------------------+   |
|   |                     API Handlers & Core Engine                           |   |
|   |   - Evaluation span: 'flagura.evaluate'                                  |   |
|   |   - Mutation span: 'flagura.mutation'                                    |   |
|   +--------------------------------------------------------------------------+   |
|                                     |                                            |
|                                     v                                            |
|   +--------------------------------------------------------------------------+   |
|   |                     Store Layer & Backup Tooling                         |   |
|   |   - Snapshot Engine: Multi-tenant relational JSON/gzip stream            |   |
|   |   - SQLite Hot Backup: VACUUM INTO online snapshot                       |   |
|   |   - Transactional Restore Drill: Integrity & hash-chain validation       |   |
|   +--------------------------------------------------------------------------+   |
+----------------------------------------------------------------------------------+
```

---

## 2. Key Component Specifications

### 1. Backup & Restore Engine (`pkg/store/backup.go`)
- `SnapshotPayload` model:
  ```go
  type SnapshotPayload struct {
      Version       string                   `json:"version"`
      CreatedAt     time.Time                `json:"created_at"`
      Organizations []domain.Organization    `json:"organizations"`
      Projects      []domain.Project         `json:"projects"`
      Flags         []domain.FeatureFlag     `json:"flags"`
      Users         []domain.User            `json:"users"`
      APIKeys       []domain.APIKey          `json:"api_keys"`
      AuditLogs     []domain.AuditLogEntry   `json:"audit_logs"`
      AuditAnchors  map[string]string        `json:"audit_anchors"`
  }
  ```
- Methods:
  - `CreateSnapshot(ctx context.Context, st store.Store, w io.Writer) error`
  - `RestoreSnapshot(ctx context.Context, st store.Store, r io.Reader) error`
  - `BackupSQLite(dbPath, destPath string) error`

### 2. CLI Backup Commands (`cmd/cli/backup.go`)
- `flagura backup create --file <path>`
- `flagura backup restore --file <path>`

### 3. OpenTelemetry Distributed Tracing (`pkg/telemetry/tracer.go` & `pkg/api/middleware_trace.go`)
- Integrates `go.opentelemetry.io/otel` with `propagation.TraceContext`.
- Middleware extracts W3C `traceparent` headers, sets `X-Trace-ID` on responses, and attaches `trace_id` to logger context.
- Instrument spans on `/api/v1/evaluate` and `/api/v1/flags/*`.
- SDK client propagates W3C traceparent headers on outbound HTTP requests when trace is present.

### 4. Role-Based Traffic Rate Limiting (`pkg/api/ratelimit.go`)
- `RateLimitTier` enum: `TierAnonymous` (120 req/min, burst 30), `TierAuthenticated` (1,200 req/min, burst 100), `TierSystem` (12,000 req/min, burst 500).
- Caller identification: unauthenticated IP -> Anonymous; authenticated developer sessions & service tokens -> Authenticated; admin tokens & production clusters -> System.
- Standard headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, `Retry-After`.

---

## 3. Verification Plan

1. **Unit & Integration Tests**:
   - `tests/e2e/backup_restore_test.go`: Seed database, backup, drop tables, restore, and verify all records + cryptographic audit log integrity.
   - `pkg/telemetry/tracer_test.go`: Verify W3C trace context extraction, span creation, and attribute propagation.
   - `pkg/api/middleware_trace_test.go`: Verify `traceparent` header handling and `X-Trace-ID` response headers.
   - `pkg/api/ratelimit_test.go`: Verify per-tenant isolation, tiered quotas, and rate limit headers.
2. **Quality Gates**:
   - `go test -race ./...`
   - `gosec -exclude-dir=web/views ./...` (0 issues)
   - `go build ./cmd/server` and `go build ./cmd/cli`
