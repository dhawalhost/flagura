# Tasks: Session & Cookie Lifecycle Hardening (004-session-cookie-hardening)

## Phase 1: Test Scaffolding (TDD)
- [x] Task 1.1: Write failing unit test `TestMultiCookieSessionResolution` in `pkg/api/session_hardening_test.go` verifying multiple `flagura_session` cookies are handled and valid ones selected (satisfies AC-1.1).
- [x] Task 1.2: Write failing unit test `TestCookieDomainAndEviction` in `pkg/api/session_hardening_test.go` verifying domain resolution and dual eviction headers (satisfies AC-3.1, AC-3.2).
- [x] Task 1.3: Write failing unit test in `api/index_test.go` verifying serverless database initialization retry and 503 behavior when `DATABASE_URL` fails (satisfies AC-2.1).

## Phase 2: Core Store & Serverless Connection Pool Tuning
- [x] Task 2.1: Update `NewPostgresStore` in `pkg/store/postgres.go` with serverless-tuned connection limits (`MaxOpenConns=4`, `MaxIdleConns=2`, `ConnMaxLifetime=10m`, `ConnMaxIdleTime=1m`, timeout=10s) and sliding expiration in `GetSession` (satisfies AC-1.3, AC-2.2, AC-2.3).
- [x] Task 2.2: Refactor `api/index.go` to eliminate silent in-memory fallback when `DATABASE_URL` is configured (satisfies AC-2.1).

## Phase 3: Session Resolution & Cookie Hardening
- [x] Task 3.1: Implement `resolveCookieDomain` and update `setSessionCookie`/`setProjectCookie` in `pkg/api/session.go` (satisfies AC-3.1).
- [x] Task 3.2: Update `clearSessionCookie` and `clearProjectCookie` in `pkg/api/session.go` for dual-scope (host-only & domain) eviction (satisfies AC-3.2).
- [x] Task 3.3: Refactor `getUserFromRequest` in `pkg/api/session.go` to iterate through all `flagura_session` cookies and accept any valid session token (satisfies AC-1.1, AC-1.2).
- [x] Task 3.4: Update `ServeHTTP` in `pkg/api/server.go` to support comma-separated origins in `FLAGURA_ALLOWED_ORIGIN` (satisfies AC-3.4).

## Phase 4: Frontend Client Fetch Hardening
- [x] Task 4.1: Update `web/static/js/app.js` with explicit `credentials: 'same-origin'` on `submitLogin` and add `toggleFlagEnvStatus` delegation in `flagMatrixEnterpriseComponent` (satisfies AC-3.3).

## Phase 5: Verification & Quality Gates
- [x] Task 5.1: Run `go test ./...` and confirm 100% pass across all packages.
- [x] Task 5.2: Run `gosec -exclude-dir=web/views ./...` and confirm 0 security vulnerabilities.
