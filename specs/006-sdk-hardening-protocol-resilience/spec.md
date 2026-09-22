# Spec 006: SDK Hardening & Protocol Resilience

**Status**: Draft  
**Owner**: Dhawal Dyavanpalli  
**Created**: 2026-09-22  

---

## 1. Background & Problem Statement

Flagura's client SDK (`pkg/client`) provides both remote HTTP evaluation and local in-memory evaluation synchronized via Server-Sent Events (SSE). While this provides sub-microsecond local evaluations in ideal environments, production deployments in enterprise networks face real-world failure modes:
1. **Middlebox & Proxy Drops**: Enterprise load balancers, HTTP/1.1 proxies, or corporate firewalls often terminate long-lived SSE connections after 30–60 seconds of inactivity or block chunked streaming entirely. When SSE fails, client instances without fallback fail to receive flag updates until restart.
2. **Thundering Herd on Reconnection**: Fixed or deterministic backoff (`backoff *= 2`) causes thousands of client instances to reconnect in lockstep after a deployment or server restart, overwhelming the control plane.
3. **Scanner Buffer Limits**: Go's standard `bufio.Scanner` imposes a 64KB token ceiling (`MaxScanTokenSize`). As enterprise customers add flags and complex targeting rules, payloads exceed 64KB, causing silent stream failure with `bufio.ErrTooLong`.
4. **Unbounded Telemetry Memory**: The client-side telemetry buffer aggregates evaluation counts in memory. Dynamic flag keys or runaway variant names under high evaluation traffic can cause memory growth.

This specification addresses these vulnerabilities through client-side protocol hardening, jittered backoff, bounded buffer allocations, and transparent polling fallback.

---

## 2. Requirements & User Stories

### User Story 1: Resilient Flag Updates During SSE Disruption (REQ-S01)
As an application engineer running Flagura SDK in production,  
I want the client to seamlessly fall back to HTTP polling if the SSE stream drops or is blocked,  
So that my application continues to receive flag updates without interruption.

#### Acceptance Criteria:
- When SSE stream disconnects or fails to establish, the client automatically starts a background fallback polling loop against `/api/v1/flags` at `FallbackPollInterval` (default: 5s, configurable).
- When SSE stream successfully reconnects, fallback polling is paused to prevent redundant control plane load.
- Flag listeners registered via `RegisterUpdateListener` receive change notifications regardless of whether updates arrived via SSE or polling fallback.

### User Story 2: Thundering Herd Prevention on Control Plane (REQ-S02)
As a platform operator maintaining Flagura server clusters,  
I want reconnecting SDK instances to randomize their reconnection backoff with jitter,  
So that restarting a server or edge node does not cause a traffic stampede.

#### Acceptance Criteria:
- Reconnect backoff implements full jitter: `backoff = min(maxBackoff, base * 2^attempt) + rand(0, base)`.
- Base backoff defaults to 500ms, max backoff defaults to 30s.
- Backoff resets cleanly upon successful stream establishment and initial payload receipt.

### User Story 3: Large Enterprise Payload Handling & Bounds (REQ-S03)
As an enterprise team with thousands of feature flags and complex rules,  
I want the SDK to handle flag definition payloads larger than 64KB without buffer overflow,  
While rejecting payloads exceeding a safe maximum (10MB) to prevent memory exhaustion.

#### Acceptance Criteria:
- `bufio.Scanner` in `listenSSEStream` is configured with a custom buffer scaling up to `MaxStreamPayloadSize = 10 * 1024 * 1024` (10MB).
- Incoming events larger than 10MB are rejected with structured warning logs without crashing or halting the stream scanner.

### User Story 4: Bounded Client-Side Telemetry Memory (REQ-S04)
As a service owner running high-throughput microservices,  
I want the client-side telemetry buffer to enforce strict memory bounds and drop policies,  
So that rogue or unbounded flag keys cannot cause out-of-memory conditions.

#### Acceptance Criteria:
- Telemetry buffer limits maximum distinct tracked flag keys to `MaxBufferedFlags` (default: 1,000).
- Telemetry buffer limits maximum distinct variants per flag to `MaxVariantsPerFlag` (default: 50).
- When buffer limits are reached, excess evaluations are dropped, and an atomic `DroppedEvents` counter is incremented and included in the telemetry flush payload.

### User Story 5: Connection State Observability (REQ-S05)
As an observability engineer monitoring application health,  
I want to query the SDK's real-time connection state and register status listeners,  
So that our health probes can detect when the client is operating in degraded or fallback mode.

#### Acceptance Criteria:
- Client exposes `ConnectionState()` returning one of: `StateDisconnected`, `StateConnecting`, `StateConnectedSSE`, `StateConnectedPolling`.
- Callers can register callbacks via `OnConnectionStateChange(func(state ConnectionState))`.
- The OpenFeature provider translates connection state transitions to OpenFeature provider events (`ProviderReady`, `ProviderConfigurationChanged`, `ProviderError`).

---

## 3. Non-Functional Requirements

- **Zero Allocation Evaluation**: Local flag evaluation hot path must remain zero allocation and sub-microsecond.
- **Thread Safety**: All state transitions, buffer flushes, and fallback switches must be safe under concurrent execution (`go test -race`).
- **Clean Teardown**: Calling `client.Close()` must immediately terminate all SSE streams, polling routines, and flush background goroutines without goroutine leaks.
