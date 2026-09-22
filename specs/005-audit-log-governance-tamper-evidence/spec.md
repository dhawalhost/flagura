# Spec 005 — Enterprise Audit Log Governance, Export & Cryptographic Tamper-Evidence

**Feature Slug**: `005-audit-log-governance-tamper-evidence`  
**Status**: Specified  
**Created**: 2026-09-22  
**Owner**: Flagura Team / SDD Engine  
**Requirements**: REQ-C01 (Retention), REQ-C02 (Export), REQ-C03 (Tamper-Evidence)  

---

## 1. Objective & Motivation

Enterprise customers and SOC 2 / ISO 27001 auditors require proof that change management logs cannot be modified, deleted, or falsified by unauthorized parties or database administrators. Furthermore, enterprises require configurable retention schedules and structured export capabilities to centralize security event monitoring in external SIEMs (Splunk, Datadog, Elastic, AWS CloudWatch).

Flagura currently writes audit logs upon flag creation, deletion, toggling, and rollout changes, but:
1. Entries have no cryptographic link or verification mechanism (a direct database update or row deletion goes undetected).
2. No automated or configurable retention policy exists to purge expired logs according to regulatory mandates.
3. No structured export endpoint (CSV / SIEM-compatible JSON stream) exists to ingest logs into external compliance platforms.

This specification implements cryptographic hash-chaining (SHA-256), a configurable retention and purge engine with anchor preservation, and multi-format export endpoints.

---

## 2. User Stories & Acceptance Criteria

### US-1: Cryptographic Tamper-Evidence via Hash Chaining (REQ-C03)
**As** an Enterprise Security Officer and Compliance Auditor,  
**I want** each audit log entry to be cryptographically linked to its predecessor via SHA-256 hash chaining,  
**so that** any post-hoc modification, row deletion, or insertion of fake audit records is immediately detectable.

**Acceptance Criteria:**
- **AC-1.1**: Every `AuditLogEntry` includes `PrevHash` and `EntryHash` fields.
- **AC-1.2**: `EntryHash` is computed deterministically as `SHA256(PrevHash + "|" + ID + "|" + ProjectID + "|" + FlagKey + "|" + Action + "|" + Environment + "|" + Actor + "|" + Details + "|" + TimestampUTC)`.
- **AC-1.3**: The first entry in a project chain (or anchor) uses `GENESIS` as its `PrevHash`.
- **AC-1.4**: All storage drivers (`MemoryStore`, `SQLiteStore`, `PostgresStore`) atomically resolve the latest `EntryHash` for the project and link the new entry when persisting audit logs.
- **AC-1.5**: An endpoint `GET /api/v1/audit/verify` traverses the chain from oldest to newest, recalculates hashes, and returns `{ "valid": true, "total_verified": N, "head_hash": "...", "verified_at": "..." }` or pinpointed error if broken.

### US-2: Structured SIEM & Compliance Export (REQ-C02)
**As** a Security Operations Engineer,  
**I want** to export audit logs in structured JSON (NDJSON) or CSV format via API and dashboard,  
**so that** our enterprise SIEM or data warehouse can ingest immutable feature flag audit trails.

**Acceptance Criteria:**
- **AC-2.1**: Endpoint `GET /api/v1/audit/export` accepts query parameters: `format=json|csv|ndjson`, `from`, `to`, and `limit`.
- **AC-2.2**: CSV export provides RFC 4180 compliant headers: `id,timestamp,project_id,flag_key,environment,action,actor,details,prev_hash,entry_hash`.
- **AC-2.3**: JSON/NDJSON formats output standard structured SIEM event objects with ISO 8601 timestamps.
- **AC-2.4**: Streaming response prevents high memory allocation for large export windows.

### US-3: Configurable Retention & Anchor Purging (REQ-C01)
**As** a Platform Administrator,  
**I want** to configure audit log retention periods (e.g. 90 days, 365 days, 7 years) and purge expired records without breaking the cryptographic integrity of surviving records,  
**so that** storage costs and regulatory retention limits are enforced cleanly.

**Acceptance Criteria:**
- **AC-3.1**: Configurable retention via environment variable `FLAGURA_AUDIT_RETENTION_DAYS` (default: 365) and API parameters.
- **AC-3.2**: `PurgeAuditLogs(ctx, before)` removes entries older than the retention timestamp while preserving the last purged entry's hash as an anchor so subsequent chain verification remains valid.
- **AC-3.3**: Protected endpoint `POST /api/v1/audit/purge` restricted to organization owners and admins.

### US-4: Dashboard Verification & Export Experience
**As** a Team Lead or Auditor reviewing the Flagura web console,  
**I want** to click "Verify Audit Integrity" and "Export Trail" directly within the Audit Modal,  
**so that** I have immediate visibility into compliance state and can download reports on demand.

**Acceptance Criteria:**
- **AC-4.1**: `web/views/audit_modal.templ` displays an "Integrity Verified" status badge with a one-click re-verify button.
- **AC-4.2**: Direct download buttons for CSV and JSON exports.
- **AC-4.3**: Each log entry displays its truncated `entry_hash` with hover-to-copy capability.

---

## 3. Edge Cases & Non-Functional Constraints
1. **Concurrency**: Multiple concurrent flag mutations in the same project must lock or order audit log insertion to prevent branching chains in `SQLiteStore` and `PostgresStore`.
2. **Zero Evaluation Overhead**: Feature flag evaluation (`/api/v1/evaluate`) writes no audit logs and remains 100% in-memory with sub-millisecond latency.
3. **Multi-Tenancy**: Audit logs, hash chains, verification, and exports are strictly isolated per `project_id` via `resolveAndAuthorizeProjectID`.
4. **Security & Gosec**: CSV and file downloads must set strict `Content-Disposition: attachment` and prevent formula injection. SAST must maintain 0 gosec issues.
