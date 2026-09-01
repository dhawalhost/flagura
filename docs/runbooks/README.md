# 📚 Flagura Operations & SRE Runbooks

This directory contains the operational runbooks and standard operating procedures (SOPs) for maintaining, deploying, troubleshooting, and securing the **Flagura** feature flagging platform in production.

---

## ⚡ Emergency Quick Reference (P0 / P1 Incidents)

| Incident Scenario | Severity | Immediate Action | Runbook Link |
| :--- | :---: | :--- | :--- |
| **Buggy Feature in Production** | **P0** | Trigger Flag Kill-Switch via UI Console or `PATCH /api/v1/flags/{id}/toggle` | [Emergency Kill Switch](incident-response.md#1-emergency-feature-flag-kill-switch-p0) |
| **PostgreSQL Outage / Connection Drop** | **P1** | Flagura automatically falls back to In-Memory Edge Store. Check Supabase Pooler status | [Database Incident Response](incident-response.md#2-database-connectivity-outage--edge-fallback-p1) |
| **High Latency (> 10ms on Evaluation)** | **P1** | Check network latency / switch client SDK to Local In-Memory Evaluation mode | [Latency & Performance Runbook](incident-response.md#3-high-evaluation-latency-troubleshooting-p1) |
| **Compromised Admin Credentials** | **P0** | Revoke sessions in `sessions` table, update bcrypt hash, rotate API keys | [Security & Credential Revocation](security-and-access.md#4-compromised-credentials--session-revocation) |
| **Email / Password Reset Outage** | **P2** | Check SMTP logs or perform manual database password reset via SQL | [Email Delivery Remediation](incident-response.md#5-email-delivery--password-reset-failures-p2) |
| **Failed Deployment / Build Error** | **P2** | Rollback Vercel / Docker deployment to previous stable release SHA | [Deployment & Rollback Runbook](deployment.md#4-rollback-procedures) |

---

## 📑 Runbook Directory

1. **[Deployment & Release Management](deployment.md)**
   - Complete Environment Variables Reference
   - Standalone Binary & Systemd Service
   - Docker Containerization
   - Vercel Serverless Function Deployment
   - Health Check Verification & Smoke Tests
   - Rollback Procedures

2. **[Incident Response & Emergency Operations](incident-response.md)**
   - 1-Click Kill Switch Activation (UI & REST API)
   - PostgreSQL Outage & Edge Fallback Handling
   - High Latency & Slow Evaluation Debugging
   - Audit Log Forensic Inspection
   - Transactional Email Delivery & Reset Troubleshooting

3. **[Database Operations & Disaster Recovery](database-operations.md)**
   - Supabase PostgreSQL Schema Setup & Indexes
   - Connection Pooling & Port 6543 Configuration
   - Backup & Point-In-Time Recovery (PITR)
   - Database Reset & Controlled Migration Steps

4. **[Security, Access & Credential Management](security-and-access.md)**
   - User Provisioning & Role-Based Access Control (RBAC)
   - Password Security Policy & Real-Time Client Validation
   - Self-Service & Admin Password Recovery Flows
   - Session & Credential Revocation
   - Transactional Email & 4-Eyes Governance Configuration
   - SAST / SCA Vulnerability Triage (Gosec, Govulncheck, Dependabot)
   - HTTP Security Headers & DoS Defense Verification

5. **[SDK Release & Publishing Procedures](sdk-publishing.md)**
   - Go Submodule publishing (`proxy.golang.org`, `pkg.go.dev`)
   - NPM package release (`@flagura/sdk` for JS/TS/React)
   - PyPI package release (`flagura-sdk` for Python)
   - Crates.io package release (`flagura` for Rust)
   - OpenFeature Ecosystem Catalog submission

6. **[Supabase & Vercel Cloud Integration Guide](../integrations/supabase-vercel-setup.md)**
   - End-to-end Supabase project creation & connection pooler setup
   - Vercel Serverless Function import and environment variables
   - GitHub Actions automated continuous deployment pipeline
   - Self-hosting and local PostgreSQL Docker Compose guide
