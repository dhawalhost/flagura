# Architecture & Technical Plan: SDK Hardening & Protocol Resilience (Spec 006)

**Status**: Draft  
**Spec**: `specs/006-sdk-hardening-protocol-resilience/spec.md`  
**Created**: 2026-09-22  

---

## 1. Architectural Design

```
+----------------------------------------------------------------------------------+
|                                FLAGURA GO SDK                                    |
|                                                                                  |
|  +---------------------------+       +----------------------------------------+  |
|  |    Connection Manager     |       |          Telemetry Buffer              |  |
|  |                           |       |                                        |  |
|  |  +---------------------+  |       |  - Max 1,000 distinct flags            |  |
|  |  | SSE Stream Listener |  |       |  - Max 50 variants per flag            |  |
|  |  |  (Buffer <= 10MB)   |  |       |  - Atomic DroppedEvents Counter        |  |
|  |  +----------+----------+  |       |  - Periodic Flush with Timeout         |  |
|  |             | (fails)     |       +----------------------------------------+  |
|  |             v             |                                                   |
|  |  +---------------------+  |       +----------------------------------------+  |
|  |  |  Fallback Polling   |  |       |         Local In-Memory Cache          |  |
|  |  |  (Interval: 5s)     |  | <---> |                                        |  |
|  |  +----------+----------+  |       |  - Atomic sync with RWMutex            |  |
|  |             | (retries)   |       |  - Update listener notifications       |  |
|  |             v             |       |  - Offline disk snapshot backing       |  |
|  |  +---------------------+  |       +----------------------------------------+  |
|  |  | Jittered Backoff    |  |                                                   |
|  |  | min(30s, base*2^n)  |  |                                                   |
|  |  +---------------------+  |                                                   |
|  +---------------------------+                                                   |
+----------------------------------------------------------------------------------+
```

---

## 2. Key Component Changes

### 1. ConnectionState Enum & Listeners (`pkg/client/client.go`)
```go
type ConnectionState string

const (
    StateDisconnected      ConnectionState = "DISCONNECTED"
    StateConnecting        ConnectionState = "CONNECTING"
    StateConnectedSSE      ConnectionState = "CONNECTED_SSE"
    StateConnectedPolling  ConnectionState = "CONNECTED_POLLING"
)
```
- Client tracks `connectionState ConnectionState` with `stateMu sync.RWMutex`.
- Expose `ConnectionState() ConnectionState`.
- Expose `OnConnectionStateChange(func(ConnectionState))` and thread-safe listener dispatch.

### 2. Stream Scanner Buffer Scaling (`pkg/client/stream.go`)
- Replace standard `scanner := bufio.NewScanner(resp.Body)` with:
```go
const (
    InitialStreamBufferSize = 64 * 1024         // 64 KB initial allocation
    MaxStreamPayloadSize    = 10 * 1024 * 1024  // 10 MB maximum token size
)

buf := make([]byte, InitialStreamBufferSize)
scanner := bufio.NewScanner(resp.Body)
scanner.Buffer(buf, MaxStreamPayloadSize)
```
This safely accommodates enterprise flag updates up to 10MB while guarding against unbounded allocations.

### 3. Jittered Exponential Backoff (`pkg/client/stream.go`)
```go
func computeJitteredBackoff(attempt int, base, max time.Duration) time.Duration {
    multiplier := 1 << attempt
    if multiplier <= 0 || multiplier > 1024 {
        multiplier = 1024
    }
    backoff := base * time.Duration(multiplier)
    if backoff > max {
        backoff = max
    }
    // Full jitter: add random duration in [0, base)
    jitter := time.Duration(rand.Int63n(int64(base)))
    return backoff + jitter
}
```

### 4. Transparent Fallback Polling (`pkg/client/stream.go`)
- When `listenSSEStream` fails, transition state to `StateConnectedPolling` (if previous flags cached) or `StateConnecting`.
- Trigger immediate one-shot poll `c.syncFlags(ctx)` to ensure flag freshness during disconnection.
- While waiting for jittered reconnect, poll periodically at `c.config.FallbackPollInterval` (default: 5s).
- When SSE stream reconnects and delivers `flags_init`, transition state back to `StateConnectedSSE` and cancel the fallback polling ticker.

### 5. Bounded Telemetry Buffer (`pkg/client/telemetry.go`)
```go
const (
    DefaultMaxBufferedFlags    = 1000
    DefaultMaxVariantsPerFlag  = 50
)
```
- Add `maxFlags int`, `maxVariants int`, and `droppedEvents uint64` to `TelemetryBuffer`.
- In `Record(flagKey, variant)`:
  - If `len(tb.metrics) >= tb.maxFlags` and key is not already in map:
    increment `tb.droppedEvents++`, return early.
  - If `len(m.Variants) >= tb.maxVariants` and variant is not in map:
    increment `tb.droppedEvents++`, return early.
- Include `dropped_events: uint64` in `TelemetryPayload` flushed to Flagura.

---

## 3. Verification Plan

1. **Unit Tests**:
   - `stream_test.go`:
     - Test scanner buffer handling a 256KB and 1MB flag payload (exceeding standard 64KB).
     - Test fallback polling activation on SSE server disconnection.
     - Test backoff jitter distribution (non-deterministic spread).
     - Test connection state transition callbacks.
   - `telemetry_test.go`:
     - Test `MaxBufferedFlags` limit enforcement and verify dropped count increment.
     - Test `MaxVariantsPerFlag` limit enforcement.
2. **Quality Gates**:
   - `go test -race ./pkg/client/...`
   - `go test ./...`
   - `gosec -exclude-dir=web/views ./...` (0 issues).
