# 📚 Flagura Operations & SRE Runbooks

This directory contains the operational runbooks and standard operating procedures (SOPs) for maintaining, deploying, troubleshooting, and securing the **Flagura** feature flagging platform in production.

---

## ⚡ Emergency Quick Reference (P0 / P1 Incidents)

| Incident Scenario | Severity | Immediate Action | Runbook Link |
| :--- | :---: | :--- | :--- |
| **Buggy Feature in Production** | **P0** | Trigger Flag Kill-Switch via UI Console or `PATCH /api/v1/flags/{id}/toggle` | [Emergency Kill Switch](file:///Users/dhawal.dyavanpalli/go/src/flagura/docs/runbooks/incident-response.md#1-emergency-feature-flag-kill-switch) |
| **PostgreSQL Outage / Connection Drop** | **P1** | Flagura automatically falls back to In-Memory Edge Store. Check Supabase Pooler status | [Database Incident Response](file:///Users/dhawal.dyavanpalli/go/src/flagura/docs/runbooks/incident-response.md#2-database-connectivity-outage--edge-fallback) |
| **High Latency (> 10ms on Evaluation)** | **P1** | Check network latency / switch client SDK to Local In-Memory Evaluation mode | [Latency & Performance Runbook](file:///Users/dhawal.dyavanpalli/go/src/flagura/docs/runbooks/incident-response.md#3-high-evaluation-latency-troubleshooting) |
| **Compromised Admin Credentials** | **P0** | Revoke sessions in `sessions` table, update bcrypt hash, rotate API keys | [Security & Credential Revocation](file:///Users/dhawal.dyavanpalli/go/src/flagura/docs/runbooks/security-and-access.md#2-compromised-credentials--session-revocation) |
| **Failed Deployment / Build Error** | **P2** | Rollback Vercel / Docker deployment to previous stable release SHA | [Deployment & Rollback Runbook](file:///Users/dhawal.dyavanpalli/go/src/flagura/docs/runbooks/deployment.md#4-rollback-procedures) |

---

## 📑 Runbook Directory

1. **[Deployment & Release Management](file:///Users/dhawal.dyavanpalli/go/src/flagura/docs/runbooks/deployment.md)**
   - Standalone Binary & Systemd Service
   - Docker Containerization
   - Vercel Serverless Function Deployment
   - Health Check Verification & Smoke Tests
   - Rollback Procedures

2. **[Incident Response & Emergency Operations](file:///Users/dhawal.dyavanpalli/go/src/flagura/docs/runbooks/incident-response.md)**
   - 1-Click Kill Switch Activation (UI & REST API)
   - PostgreSQL Outage & Edge Fallback Handling
   - High Latency & Slow Evaluation Debugging
   - Audit Log Forensic Inspection

3. **[Database Operations & Disaster Recovery](file:///Users/dhawal.dyavanpalli/go/src/flagura/docs/runbooks/database-operations.md)**
   - Supabase PostgreSQL Schema Setup & Indexes
   - Connection Pooling & Port 6543 Configuration
   - Backup & Point-In-Time Recovery (PITR)
   - Database Reset & Controlled Migration Steps

4. **[Security, Access & Credential Management](file:///Users/dhawal.dyavanpalli/go/src/flagura/docs/runbooks/security-and-access.md)**
   - User Provisioning & Role-Based Access Control (RBAC)
   - Session & API Key Rotation
   - SAST / SCA Vulnerability Triage (Gosec, Govulncheck, Dependabot)
   - HTTP Security Headers & DoS Defense Verification
