# Feature Plan: Session & Cookie Lifecycle Hardening

**Feature**: `004-session-cookie-hardening`  
**Status**: Complete  
**Date**: 2026-09-22

---

## 1. Technical Architecture & Component Changes

```
┌─────────────────────────────────────────────────────────────────┐
│                      Client Browser                             │
│ Cookie: flagura_session=OLD_STALE; flagura_session=VALID_TOKEN │
└──────────────────────────────┬──────────────────────────────────┘
                               │ PATCH /api/v1/flags/:key/toggle
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                 pkg/api/session.go                              │
│   getUserFromRequest: Loops all matching cookies                │
│   - Tests OLD_STALE -> fails (not found)                        │
│   - Tests VALID_TOKEN -> succeeds -> User Context               │
│   - Overwrites / cleans stale cookies in response               │
└──────────────────────────────┬──────────────────────────────────┘
                               │ Queries session via pool
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│              pkg/store/postgres.go (PostgresStore)              │
│   - MaxOpenConns = 4, MaxIdleConns = 2                          │
│   - Cold-start Timeout = 10s                                    │
│   - Sliding expiration for active sessions                      │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. File Modification Blueprint

- `api/index.go`: Eliminate silent fallback to in-memory store; return 503 on unrecoverable DB failures.
- `pkg/store/postgres.go`: Adjust connection pool parameters for serverless safety and implement sliding expiration.
- `pkg/api/session.go`: Multi-cookie candidate loop in `getUserFromRequest`, domain-aware `setSessionCookie`/`setProjectCookie`, and dual-eviction in `clearSessionCookie`/`clearProjectCookie`.
- `pkg/api/server.go`: Comma-separated origin parsing in `ServeHTTP`.
- `web/static/js/app.js`: Explicit `credentials: 'same-origin'` on `submitLogin`, add `toggleFlagEnvStatus` to `flagMatrixEnterpriseComponent`.

---

## 3. Verification & Results

- **Session Hardening Tests**: `TestMultiCookieSessionResolution`, `TestCookieDomainAndEviction`, `TestCORSMultiOriginSupport` in `pkg/api/session_hardening_test.go` — **PASS**.
- **Serverless DB Resilience Tests**: `TestServerlessDatabaseConnectionUnavailable`, `TestServerlessDatabaseConnectionFallbackAllowed` in `api/index_test.go` — **PASS**.
- **Full Test Suite**: `go test -count=1 ./...` — **100% PASS**.
- **SAST Security Audit**: `gosec -exclude-dir=web/views ./...` — **0 Issues**.
