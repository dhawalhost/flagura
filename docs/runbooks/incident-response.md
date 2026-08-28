# 🚨 Incident Response & Emergency Operations Runbook

This runbook outlines immediate remediation procedures for operational emergencies, feature regressions, database disruptions, and latency degradation in Flagura.

---

## 1. Emergency Feature Flag Kill-Switch (P0)

If a newly launched feature causes production exceptions, API degradation, or security concerns, disable it immediately with zero code deployment.

### Method A: Web Console (Fastest — 1 Click)
1. Navigate to `https://flagura.yourdomain.com/dashboard`.
2. Find the problematic flag in the list (or search by key).
3. Toggle the **Kill-Switch switch** to `OFF` for `Production`.
4. The circuit breaker is now active; all subsequent evaluations will instantly receive `false` / `off`.

### Method B: REST API CLI (Automated / PagerDuty / On-Call)
```bash
# Obtain your admin/developer session token or pass Authorization header
API_URL="https://flagura.yourdomain.com"
AUTH_TOKEN="your_session_token_or_api_key"
FLAG_KEY="problematic-feature-key"

# Instant Kill-Switch Disable in Production
curl -X PATCH "${API_URL}/api/v1/flags/${FLAG_KEY}/toggle" \
  -H "Authorization: Bearer ${AUTH_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "environment": "production",
    "enabled": false
  }'
```

---

## 2. Database Connectivity Outage & Edge Fallback (P1)

### Behavior
- Flagura is engineered with **Edge Fallback Resilience**.
- If the Supabase PostgreSQL database becomes unreachable or suffers a connection drop, Flagura automatically logs `[WARN] Failed to connect to Supabase PostgreSQL: ... Falling back to in-memory store.`
- Flagura will continue serving evaluations in **In-Memory Edge Store mode** without crashing.

### Diagnosis Steps:
```bash
# 1. Inspect Service Logs
# For Systemd:
journalctl -u flagura -n 50 --no-pager

# For Docker:
docker logs --tail 50 flagura-prod

# 2. Check Database Health via API
curl -s "${API_URL}/api/health" | jq .
# If "driver" reports "In-Memory Edge Store", the database connection has dropped.

# 3. Test Direct Supabase Connectivity from Host
pg_isready -d "$DATABASE_URL"
```

### Remediation:
1. Verify Supabase Project status in [Supabase Status Page](https://status.supabase.com).
2. If connection pool was exhausted, ensure the connection string uses the **Pooler URI on Port `6543`**:
   ```
   postgres://postgres.[REF]:[PW]@aws-0-[REGION].pooler.supabase.com:6543/postgres?sslmode=require
   ```
3. Restart Flagura to re-establish the pool:
   ```bash
   sudo systemctl restart flagura
   # or
   docker restart flagura-prod
   ```

---

## 3. High Evaluation Latency Troubleshooting (P1)

Flagura evaluation runs in **< 80 nanoseconds in-memory** and **< 1–4 milliseconds over local HTTP**. If latencies exceed 15ms, follow this troubleshooting tree:

### Step 1: Diagnose Where the Latency Occurs
- Run the built-in benchmark endpoint to verify the in-memory engine:
  ```bash
  curl -s -X POST "${API_URL}/api/v1/benchmark" \
    -H "Content-Type: application/json" \
    -d '{"iterations": 50000, "environment": "production"}' | jq .
  ```
  - If `p99_latency_ns` is `< 500ns`, the core evaluation engine is performing normally.
  - Latency is occurring in network transport, reverse proxy, or TLS termination.

### Step 2: Switch Client SDK to Local In-Memory Evaluation
For zero-latency high-throughput applications, switch your client SDK to **Local Evaluation mode**:

```go
// Go SDK: Synchronizes flags every 30s and evaluates in-process (0 network hops)
client, err := flagura.NewClient(
    flagura.WithEndpoint("https://flagura.yourdomain.com"),
    flagura.WithLocalEvaluation(30 * time.Second),
)
```

---

## 4. Audit Log Forensic Inspection

When investigating who made a change or when a flag was toggled:

```bash
# Fetch recent 50 audit log events
curl -s "${API_URL}/api/v1/audit-logs?limit=50" \
  -H "Authorization: Bearer ${AUTH_TOKEN}" | jq '.logs[] | {timestamp, flag_key, action, environment, actor, details}'
```

Sample Output:
```json
{
  "timestamp": "2026-08-28T10:15:30Z",
  "flag_key": "checkout_v2",
  "action": "KILL_SWITCH_TOGGLED",
  "environment": "production",
  "actor": "admin@flagura.dev",
  "details": "Disabled (Kill Switch) flag for production environment."
}
```
