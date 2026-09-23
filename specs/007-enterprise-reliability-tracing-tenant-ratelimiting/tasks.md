# Tasks: Enterprise Reliability, Tracing & Tenant Rate Limiting (Spec 007)

**Status**: Complete  
**Spec**: `specs/007-enterprise-reliability-tracing-tenant-ratelimiting/spec.md`  
**Plan**: `specs/007-enterprise-reliability-tracing-tenant-ratelimiting/plan.md`  
**Created**: 2026-09-22  

## Phase 1: Backup & Restore Engine (REQ-O01 & REQ-O04)
- [x] T1: Implement `CreateSnapshot` and `RestoreSnapshot` in `pkg/store/backup.go`.
- [x] T2: Add CLI commands `flagura backup create` and `flagura backup restore` in `cmd/cli/backup.go`.
- [x] T3: Add automated end-to-end integration test drill in `tests/e2e/backup_restore_test.go` verifying zero-loss restoration and audit chain integrity.
- [x] T4: Author Disaster Recovery & RTO/RPO Runbook in `docs/runbooks/disaster-recovery.md`.

## Phase 2: OpenTelemetry Distributed Tracing (REQ-O02)
- [x] T5: Implement OpenTelemetry tracer and W3C context propagator in `pkg/telemetry/tracer.go`.
- [x] T6: Implement W3C tracing middleware and logger correlation in `pkg/api/middleware_trace.go`.
- [x] T7: Instrument spans on evaluation and flag mutation handlers in `pkg/api/handlers_eval.go` and `pkg/api/handlers_flags.go`.
- [x] T8: Add W3C traceparent propagation to SDK client in `pkg/client/client.go`.
- [x] T9: Add unit and integration tests in `pkg/telemetry/tracer_test.go` and `pkg/api/middleware_trace_test.go`.

## Phase 3: Per-Tenant & Tiered Rate Limiting (REQ-O03)
- [x] T10: Implement tenant-scoped tiered rate limiting and quota headers in `pkg/api/ratelimit.go`.
- [x] T11: Add unit tests in `pkg/api/ratelimit_test.go` verifying tier isolation, header compliance, and 429 response formatting.

## Phase 4: Quality Gates & Verification
- [x] T12: Run race detector `go test -race ./...`.
- [x] T13: Run repository-wide test suite `go test ./...`.
- [x] T14: Run security gate `gosec -exclude-dir=web/views ./...` (0 issues).
- [x] T15: Verify binary builds `go build ./cmd/server` and `go build ./cmd/cli`.
