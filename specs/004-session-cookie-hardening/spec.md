# Feature Spec: Session & Cookie Lifecycle Hardening

**Feature Slug**: `004-session-cookie-hardening`  
**Status**: Complete  
**Created**: 2026-09-22  
**Owner**: Dhawal Dyavanpalli

---

## 1. Problem Statement & Motivation

Users accessing `flagura.dev` experience intermittent authentication failures (HTTP 401 Unauthorized, "Session expired or access denied") immediately after logging in when toggling feature flags, but observe it functioning normally after waiting or refreshing the page.

Root causes identified:

1. **Serverless Split-Brain (`api/index.go`)**: Cold start connection timeouts against Supabase pooler cause instances to silently fall back to `NewMemoryStore()`. Since memory stores are isolated per container, requests hitting a degraded container cannot resolve sessions created by a PostgreSQL-connected container.
2. **Cookie Shadowing (`pkg/api/session.go`)**: Go's `r.Cookie(name)` returns only the first matching cookie. When clients hold multiple `flagura_session` cookies (e.g. host-only vs domain-scoped or legacy paths), an expired cookie can shadow a valid one.
3. **Incomplete Eviction (`clearSessionCookie`)**: Eviction headers only clear host-only cookies at `/`, leaving domain-scoped cookies active.
4. **Serverless Pooler Exhaustion (`pkg/store/postgres.go`)**: `MaxOpenConns = 25` per lambda instance oversubscribes Supabase connection limits during traffic spikes.
5. **Client Fetch Credentials (`web/static/js/app.js`)**: Inconsistent `credentials` configurations across fetch calls and missing delegation in `flagMatrixEnterpriseComponent`.

---

## 2. User Stories & Acceptance Criteria

### User Story 1: Reliable Session Recognition

As a developer or administrator logging into `flagura.dev`,
I want my session and active project selection to be immediately recognized on all subsequent API and dashboard actions,
So that I never get unexpected 401 Unauthorized errors when modifying flags immediately after login.

- **AC-1.1**: When a client presents multiple `flagura_session` cookies, `getUserFromRequest` iterates through all candidates and succeeds if any token maps to an active, non-expired session in the store.
- **AC-1.2**: When a valid session token is identified from a secondary cookie, the server automatically updates the response `Set-Cookie` header to overwrite and clear redundant/stale duplicate cookies.
- **AC-1.3**: Session expiration dynamically slides when an active session is within 48 hours of expiration.

### User Story 2: Serverless Database Resilience

As the platform operator,
I want serverless instances to maintain database continuity and never silently degrade to an isolated in-memory store in production,
So that state and sessions remain unified across all serverless lambda containers.

- **AC-2.1**: When `DATABASE_URL` is set, `initServer` never permanently locks an instance into `MemoryStore`. If a connection attempt fails, it retries with exponential backoff and returns a 503 error rather than corrupting session state.
- **AC-2.2**: `PostgresStore` connection pool settings are optimized for serverless deployments (`MaxOpenConns = 4`, `MaxIdleConns = 2`, `ConnMaxLifetime = 10m`, `ConnMaxIdleTime = 1m`).
- **AC-2.3**: Connection timeout for Postgres ping and auto-migration is increased from 5s to 10s to tolerate pooler cold-starts.

### User Story 3: Clean Cookie Lifecycle & Multi-Domain Support

As a user navigating between `flagura.dev` and `www.flagura.dev`,
I want cookie domains and clearing operations to cleanly evict all variations upon logout,
So that no zombie cookies persist in the browser.

- **AC-3.1**: `setSessionCookie` and `setProjectCookie` support configurable domain scoping (`COOKIE_DOMAIN` or apex domain matching for production domains).
- **AC-3.2**: `clearSessionCookie` and `clearProjectCookie` issue clearing directives for both host-only (`Domain=""`) and domain-scoped (`Domain=".flagura.dev"`) cookies.
- **AC-3.3**: All frontend `fetch()` calls in `web/static/js/app.js` explicitly transmit credentials (`credentials: 'same-origin'`).
- **AC-3.4**: `FLAGURA_ALLOWED_ORIGIN` supports comma-separated origin lists to support both apex and `www` origins simultaneously.
