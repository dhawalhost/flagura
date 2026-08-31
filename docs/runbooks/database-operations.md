# 🗄️ Database Operations & Disaster Recovery Runbook

This runbook documents database management, migration procedures, performance tuning, and backup/recovery strategies for Flagura on Supabase PostgreSQL.

---

## 1. Database Architecture & Tables

Flagura relies on relational tables with JSONB document columns in PostgreSQL:

| Table | Primary Key | Purpose | Key Indexes |
| :--- | :--- | :--- | :--- |
| `organizations` | `id` (TEXT) | Tenant organizations workspace | `idx_organizations_slug` (UNIQUE) |
| `projects` | `id` (TEXT) | Isolated project hierarchies | `idx_projects_org`, `idx_projects_slug` |
| `users` | `id` (TEXT) | System administrators and developers | `idx_users_email` (UNIQUE) |
| `sessions` | `token` (TEXT) | Active 7-day session tokens | `idx_sessions_user_id`, `idx_sessions_expires_at` |
| `feature_flags`| `id` (TEXT) | Flag metadata, environment JSON configs | `idx_feature_flags_key`, `idx_feature_flags_proj` |
| `audit_logs` | `id` (TEXT) | Immutable historical audit trails | `idx_audit_logs_timestamp`, `idx_audit_logs_proj` |
| `api_keys` | `id` (TEXT) | SDK and server authorization keys | `idx_api_keys_hash` (UNIQUE) |
| `change_requests` | `id` (TEXT) | 4-Eyes governance change proposals | `idx_change_requests_status` |
| `experiment_events` | `id` (TEXT) | A/B experimentation telemetry stream | `idx_exp_events_flag` |

---

## 2. Initial Setup & Multi-Database Support

Flagura uses standard ANSI SQL and the official PostgreSQL driver (`github.com/lib/pq`). It runs seamlessly on any standard PostgreSQL instance (version 14+).

### Method 1: Local PostgreSQL via Docker Compose (1-Click)
```bash
# Starts local PostgreSQL + Flagura with persistent storage and auto-migrations
docker compose up -d
```

### Method 2: Supabase Dashboard
1. Log in to [Supabase Console](https://supabase.com/dashboard).
2. Select your Project -> **SQL Editor**.
3. Copy and run the contents of [`supabase/schema.sql`](../../supabase/schema.sql).

### Method 3: Any Cloud PostgreSQL (AWS RDS, Neon, Cloud SQL, Railway, Render)
```bash
# Connect and initialize schema on any remote or local database:
psql "$DATABASE_URL" -f supabase/schema.sql
```

---

## 3. Connection Pooling Configuration

When deploying Flagura at scale or on serverless platforms (Vercel, Cloud Run):

| Mode | Port | Best Used For | Notes |
| :--- | :---: | :--- | :--- |
| **Transaction Pooler** | **`6543`** | **Production Vercel / Serverless** | Recommended. Handles 10,000+ client connections with minimal Postgres backend load. |
| **Session Pooler** | `5432` | Dedicated VM / Long-running Containers | Direct connection with prepared statement support. |

### Go Connection Pooler Tuning in [postgres.go](../../internal/store/postgres.go):
```go
db.SetMaxOpenConns(25)              // Maximum active connections
db.SetMaxIdleConns(10)              // Idle pool size
db.SetConnMaxLifetime(5 * time.Minute) // Recycle stale connections
```

---

## 4. Backup & Disaster Recovery (PITR)

### Automated Daily Backups (Supabase Pro)
- Supabase automatically takes daily physical backups with Point-In-Time-Recovery (PITR) up to 7–30 days.
- In case of accidental data truncation or catastrophic drop:
  1. Open **Supabase Dashboard** -> **Database** -> **Backups**.
  2. Choose the restore point timestamp prior to the incident.
  3. Click **Restore Database**.

### Manual Snapshot Backup (CLI)
```bash
# Export all feature flags and users
pg_dump "$DATABASE_URL" \
  --table=users \
  --table=sessions \
  --table=feature_flags \
  --table=audit_logs \
  -Fc > flagura_backup_$(date +%Y%m%d_%H%M%S).dump

# Restore from dump file:
pg_restore -d "$DATABASE_URL" --clean --if-exists flagura_backup_*.dump
```

---

## 5. Controlled Database Reset Protocol

In staging or QA environments, to reset the database to clean seed flags:

```bash
# Requires RoleAdmin session
curl -X POST "https://flagura.yourdomain.com/api/v1/reset" \
  -H "Authorization: Bearer ${ADMIN_SESSION_TOKEN}"
```

> [!CAUTION]
> The `/api/v1/reset` endpoint truncates `feature_flags` and `audit_logs`, re-seeding the default feature set. This endpoint is strictly restricted to authenticated users with `RoleAdmin` (`403 Forbidden` for non-admins).
