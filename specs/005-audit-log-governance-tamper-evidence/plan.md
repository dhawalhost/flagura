# Technical Plan: Audit Log Governance, Export & Cryptographic Tamper-Evidence

**Spec**: `specs/005-audit-log-governance-tamper-evidence/spec.md`  
**Status**: Planned  
**Date**: 2026-09-22  

---

## 1. Architectural Design

```
┌────────────────────────────────────────────────────────────────────────┐
│                        Flag Mutation Request                           │
│  (Toggle / Rollout / Create / Delete Flag / Apply Change Request)       │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                     Audit Chain Appender (Store)                       │
│  1. Resolve Latest Project Entry Hash:                                 │
│     SELECT entry_hash FROM audit_logs                                  │
│     WHERE project_id = $1 ORDER BY timestamp DESC LIMIT 1             │
│     (If none -> prev_hash = "GENESIS")                                 │
│  2. Compute SHA-256 Entry Hash:                                        │
│     entry_hash = SHA256(prev_hash|id|proj|key|act|env|actor|det|ts)   │
│  3. Persist Log Entry with [prev_hash, entry_hash]                     │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
          ┌─────────────────────────┼─────────────────────────┐
          ▼                         ▼                         ▼
┌──────────────────┐      ┌──────────────────┐      ┌──────────────────┐
│ Verify Endpoint  │      │ Export Endpoint  │      │  Purge Endpoint  │
│ /audit/verify    │      │ /audit/export    │      │  /audit/purge    │
│ Traverses chain  │      │ Streams CSV or   │      │  Prunes older    │
│ Recalculates all │      │ SIEM JSON/NDJSON │      │  Preserves last  │
│ Hashes in order  │      │ with proper MIME │      │  anchor hash     │
└──────────────────┘      └──────────────────┘      └──────────────────┘
```

---

## 2. Hash Chaining Specification

### Canonical Hash String
For each log entry, the canonical preimage string is:
```
{prev_hash}|{id}|{project_id}|{flag_key}|{action}|{environment}|{actor}|{details}|{timestamp_rfc3339nano}
```
The resulting `entry_hash` is encoded as a 64-character lowercase hexadecimal string:
`hex.EncodeToString(sha256Sum)`.

If no prior entry exists for the project, `prev_hash` is initialized to `"GENESIS"`.

---

## 3. Database Schema Modifications (`supabase/schema.sql`)

```sql
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS prev_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS entry_hash TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_audit_logs_chain ON audit_logs(project_id, timestamp ASC);
```

---

## 4. API Surface

| Method | Endpoint | Authorization | Description |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/v1/audit/verify` | Authenticated (Project Scoped) | Verifies the cryptographic chain integrity for the project |
| `GET` | `/api/v1/audit/export` | Authenticated (Project Scoped) | Streams audit events in `csv`, `json`, or `ndjson` format |
| `POST` | `/api/v1/audit/purge` | Org Owner / Admin Only | Purges audit logs older than retention window |

---

## 5. Security & Multi-Tenancy

- All audit operations are scoped via `resolveAndAuthorizeProjectID`.
- CSV export applies sanitization to prevent CSV/Formula injection (cells starting with `=`, `+`, `-`, `@` are prepended with single quotes).
- Strict MIME types and headers (`Content-Disposition: attachment; filename="flagura-audit-{project}-{date}.csv"`).
- SAST security audit: 0 gosec issues.
