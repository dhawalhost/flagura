# Tasks: SDK Hardening & Protocol Resilience (Spec 006)

**Status**: Completed  
**Spec**: `specs/006-sdk-hardening-protocol-resilience/spec.md`  
**Plan**: `specs/006-sdk-hardening-protocol-resilience/plan.md`  
**Created**: 2026-09-22  
**Completed**: 2026-09-22  

## Phase 1: Connection State & Configuration (TDD)
- [x] T1: Define `ConnectionState` enum and `FallbackPollInterval` option in `pkg/client/client.go`.
- [x] T2: Add connection state management and `OnConnectionStateChange` listeners in `pkg/client/client.go`.
- [x] T3: Add unit tests in `pkg/client/client_test.go` verifying connection state transitions.

## Phase 2: Stream Scanner Buffer & Jittered Backoff (TDD)
- [x] T4: Implement `computeJitteredBackoff` with full jitter in `pkg/client/stream.go`.
- [x] T5: Configure scaled stream scanner buffer (`MaxStreamPayloadSize = 10MB`) in `pkg/client/stream.go`.
- [x] T6: Add unit tests in `pkg/client/stream_test.go` for large payload (>64KB) handling and backoff jitter.

## Phase 3: Polling Fallback on Stream Disconnection (TDD)
- [x] T7: Implement transparent polling fallback routine that activates upon SSE disconnection and suspends on reconnection.
- [x] T8: Add unit tests in `pkg/client/stream_test.go` verifying polling fallback keeps flags fresh when SSE is broken.

## Phase 4: Bounded Telemetry Buffer (TDD)
- [x] T9: Add capacity bounds (`MaxBufferedFlags`, `MaxVariantsPerFlag`) and `DroppedEvents` counter to `TelemetryBuffer` in `pkg/client/telemetry.go`.
- [x] T10: Add unit tests in `pkg/client/telemetry_test.go` verifying boundary enforcement and dropped events tracking.

## Phase 5: OpenFeature Provider Integration & Quality Gates
- [x] T11: Update `pkg/openfeature/provider.go` to emit OpenFeature events on connection state transitions.
- [x] T12: Run race detector `go test -race ./pkg/client/... ./pkg/openfeature/...`.
- [x] T13: Run repository-wide test suite `go test ./...`.
- [x] T14: Run security gate `gosec -exclude-dir=web/views ./...` (0 issues).
- [x] T15: Verify binary builds `go build ./cmd/server` and `go build ./cmd/cli`.
