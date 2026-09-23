# Disaster Recovery & RTO/RPO Runbook

**Document Owner**: Flagura Reliability & Infrastructure Team  
**Last Verified Drill**: 2026-09-22  
**Target Architecture**: Single-Instance HA, Kubernetes, and Cloud Database Backends  

---

## 1. Disaster Recovery Objectives

| Metric | Target | Rationale & Verification |
| :--- | :--- | :--- |
| **RTO** (Recovery Time Objective) | **< 15 Minutes** | Time to spin up replacement store instance and restore relational snapshot via CLI or API. |
| **RPO** (Recovery Point Objective) | **< 1 Hour** | Maximum acceptable data loss window during catastrophic storage failure. Supported by hourly automated snapshots. |
| **Integrity Assurance** | **100% Cryptographic Verification** | Every restored audit log entry is validated against its SHA-256 predecessor hash chain. |

---

## 2. Failure Scenarios & Recovery Strategies

### Scenario A: Storage Volume Corruption or Accidental Data Deletion
- **Impact**: Database container/host is online, but tables or files are corrupted or truncated.
- **Action**: Execute transactional snapshot restore from the latest verified backup archive.

### Scenario B: Primary Database Node Outage (PostgreSQL / SQLite Host Failure)
- **Impact**: Database server is completely unavailable.
- **Action**:
  1. Provision standby database or mount backup persistent volume (PVC).
  2. Apply latest snapshot via `flagura backup restore --file <backup_path>`.
  3. Verify application health check `/readyz` returns HTTP 200.

---

## 3. Creating Backups

### Method 1: Using the Flagura CLI
```bash
# Export uncompressed JSON snapshot
flagura backup create --file /var/backups/flagura-backup-$(date +%Y%m%d%H%M%S).json

# Export Gzip-compressed snapshot (recommended for production)
flagura backup create --compress --file /var/backups/flagura-backup-$(date +%Y%m%d%H%M%S).json.gz
```

### Method 2: Kubernetes CronJob Automation
```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: flagura-hourly-backup
  namespace: flagura
spec:
  schedule: "0 * * * *" # Every hour
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: flagura/cli:latest
            command: ["/bin/sh", "-c"]
            args:
            - flagura backup create --compress --file /backups/flagura-$(date +%s).json.gz
            env:
            - name: FLAGURA_ENDPOINT
              value: "http://flagura-service.flagura.svc.cluster.local:3000"
            - name: FLAGURA_API_KEY
              valueFrom:
                secretKeyRef:
                  name: flagura-admin-credentials
                  key: api-key
            volumeMounts:
            - mountPath: /backups
              name: backup-storage
          restartPolicy: OnFailure
          volumes:
          - name: backup-storage
            persistentVolumeClaim:
              claimName: flagura-backup-pvc
```

---

## 4. Step-by-Step Restoration Drill

### Step 1: Prepare Clean Target Database
Ensure target PostgreSQL or SQLite database is reachable and migrations have completed:
```bash
# Check connectivity
flagura health --endpoint http://localhost:3000
```

### Step 2: Execute Restoration
```bash
# Restore from compressed or uncompressed snapshot
flagura backup restore --file /var/backups/flagura-backup-latest.json.gz
```

### Step 3: Verify Cryptographic Audit Log Integrity
Ensure that all historical audit logs, rollouts, and toggle records survived without tampering or chain breakage:
```bash
curl -s -H "Authorization: Bearer $FLAGURA_API_KEY" \
  http://localhost:3000/api/v1/audit/verify | jq .
```
**Expected Output**:
```json
{
  "valid": true,
  "total_verified": 42,
  "verified_at": "2026-09-22T20:25:00Z"
}
```

### Step 4: Validate Flag Evaluations & Health Probes
```bash
# 1. Check health probe
curl -f http://localhost:3000/readyz

# 2. Evaluate sample flag
flagura evaluate critical-checkout-v3 --env=production
```

---

## 5. Hot SQLite File Backup
For edge deployments using embedded SQLite, use the atomic hot backup engine:
```bash
# Go SDK helper
err := store.BackupSQLite("/data/flagura.db", "/data/backups/flagura-hot.db")
```
This preserves strict `0600` POSIX file permissions and executes safely alongside concurrent reader threads.
