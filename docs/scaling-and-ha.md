# Scaling and High Availability Guide

This guide details Flagura's supported deployment topologies, state externalization guarantees, and operational procedures for running Flagura in production environments ranging from single-node instances to highly available multi-replica clusters.

---

## 1. Supported Deployment Topologies

Flagura supports two production deployment topologies:

```
                  [ L4 / L7 Load Balancer (ALB / Nginx / Envoy) ]
                                    │
               ┌────────────────────┼────────────────────┐
               ▼                    ▼                    ▼
       ┌───────────────┐    ┌───────────────┐    ┌───────────────┐
       │ Flagura App   │    │ Flagura App   │    │ Flagura App   │
       │ Replica 1     │    │ Replica 2     │    │ Replica N     │
       └───────┬───────┘    └───────┬───────┘    └───────┬───────┘
               │                    │                    │
               └────────────────────┼────────────────────┘
                                    ▼
                      ┌───────────────────────────┐
                      │ Shared PostgreSQL Backend │
                      │ (Supabase / RDS / Aurora) │
                      └───────────────────────────┘
```

### Topology A: Hardened Single-Instance
- **Target Environments**: Edge nodes, internal development, staging, or resource-constrained single-server deployments (< 5,000 req/sec).
- **Supported Storage Engines**: In-Memory (`MEM`), SQLite (`SQLITE`), or PostgreSQL (`POSTGRES`).
- **Persistence**: Local SQLite disk volume (`flagura.db`) or dedicated single database instance.
- **Failover / HA**: Handled via container restart policies (e.g. systemd, Docker restart, Kubernetes single-replica deployment).

### Topology B: Horizontally Scaled Multi-Replica Cluster
- **Target Environments**: Enterprise production, multi-zone High Availability, fault-tolerant deployments (> 5,000 req/sec).
- **Required Storage Engine**: **PostgreSQL** (`POSTGRES_URL` connection string pointing to Supabase, AWS RDS, Neon, Google Cloud SQL, or self-hosted Postgres cluster).
- **SQLite Restriction**: SQLite is **NOT supported** for multi-replica deployments due to file-locking and disk synchronization constraints across container replicas.

---

## 2. Externalized State Architecture

In a horizontally scaled cluster, replicas must hold a consistent view of configurations, governance, rollout schedules, and analytics without depending on single-process memory.

### 2.1 Feature Flags & Evaluation Path
- **Evaluation Latency**: Local zero-allocation, lock-free evaluation via 64-bit FNV-1a sticky hashing (`pkg/engine/evaluator.go`).
- **State Synchronization**: Replicas load the active flag configurations into local in-memory read-optimized snapshots (`FlagSnapshot`) and subscribe to updates. Flag evaluations (`/api/v1/evaluate`) **never make blocking database calls**.

### 2.2 Progressive Canary Rollouts (`canary_schedules`)
- **Shared Persistence**: Canary schedules (`domain.CanarySchedule`) are stored in the shared database table `canary_schedules` keyed by `(project_id, flag_key)`.
- **Cross-Replica Consistency**:
  - Submitting a canary rollout on Replica A writes the schedule and initial rollout percentage to PostgreSQL.
  - Replica B discovers the active schedule during its background evaluation cycle or API queries.
  - Stage advancements and automated health rollbacks commit to PostgreSQL first before advancing state. If the underlying database write fails, the schedule enters `NEEDS_ATTENTION` rather than reporting false progress or silent desynchronization.

### 2.3 Experimentation & Telemetry Aggregation
- **Shared Event Storage**: Telemetry track events (`/api/v1/telemetry/events`) persist to `experiment_events` in PostgreSQL.
- **User Deduplication (REQ-D01 / REQ-A03)**: Experiment exposure and conversion metrics are calculated by distinct user identifiers (`user_id`), ensuring that repeated evaluations from the same user or session count as a single exposure sample regardless of which replica served the evaluation.

---

## 3. Role-Based Tenant Rate Limiting (REQ-O03)

Flagura enforces in-process, high-performance token-bucket rate limiting (`golang.org/x/time/rate`) per replica, maintaining sub-millisecond API response times without adding an external Redis network hop to the hot request path.

### 3.1 Role-Based Infrastructure Tiers
Traffic is classified by caller authentication status, ensuring unauthenticated public traffic cannot exhaust quotas needed by production microservice clusters:

| Tier | Default Quota | Default Burst | Resolved From | Environment Override |
| :--- | :--- | :--- | :--- | :--- |
| **`Anonymous`** | **120 req/min** (2/s) | 30 | Unauthenticated IP | `FLAGURA_RATE_LIMIT_ANONYMOUS` |
| **`Authenticated`** | **1,200 req/min** (20/s) | 100 | User Session / Developer Token | `FLAGURA_RATE_LIMIT_AUTHENTICATED` |
| **`System`** | **12,000 req/min** (200/s) | 500 | Admin Key / High-Throughput Service | `FLAGURA_RATE_LIMIT_SYSTEM` |

### 3.2 Standard Rate Limit Headers
Every response includes standard RFC rate limiting headers:
- `X-RateLimit-Limit`: Maximum requests permitted per minute in caller tier.
- `X-RateLimit-Remaining`: Remaining request tokens in the active window.
- `X-RateLimit-Reset`: UTC epoch timestamp when token bucket replenishes.
- On HTTP `429 Too Many Requests`: `Retry-After: <seconds>` indicating the backoff duration.

### 3.3 Multi-Replica Cluster Operator Sizing Formula
Because token buckets are tracked per replica process, running $N$ replicas behind a load balancer scales aggregate cluster capacity:

$$\text{Cluster Capacity} = \text{Per-Replica Limit} \times N$$

To enforce a target aggregate cluster quota across $N$ replicas:

$$\text{Configured Per-Replica Limit} = \frac{\text{Target Global Limit}}{N}$$

| Target Cluster Limit (System Tier) | Replicas ($N$) | Config Per Replica (`FLAGURA_RATE_LIMIT_SYSTEM`) |
| :--- | :--- | :--- |
| 24,000 req/min (400 req/s) | 2 replicas | 12,000 req/min (burst: 500) |
| 48,000 req/min (800 req/s) | 4 replicas | 12,000 req/min (burst: 500) |
| 120,000 req/min (2,000 req/s) | 10 replicas | 12,000 req/min (burst: 500) |

### 3.4 Load Balancer Recommendations
- **Even Distribution**: Use **Round-Robin** or **Least Connections** algorithms on your ingress/ALB.
- **Sticky Client Limiting (Optional)**: If strict per-IP rate limiting is required across cluster nodes, enable IP-hash stickiness at your ingress controller or edge CDN (Cloudflare, Fastly, AWS CloudFront).

---

## 4. Health Probes & Orchestration

Flagura exposes standard orchestration endpoints ready for Kubernetes, AWS ECS, Docker Swarm, and Nomad:

### Liveness Probe (`/livez`)
- **HTTP Path**: `GET /livez`
- **Port**: App HTTP port (default `:8080`)
- **Expected Status**: `200 OK`
- **Semantics**: Confirms the process HTTP server is alive and accepting connections.

### Readiness Probe (`/readyz`)
- **HTTP Path**: `GET /readyz`
- **Expected Status**: `200 OK`
- **Failure Status**: `503 Service Unavailable`
- **Semantics**: Executes a store `Ping()` against the PostgreSQL database. If database connectivity is interrupted, traffic is removed from the replica until reconnection.

### Kubernetes Deployment Example
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: flagura
  labels:
    app: flagura
spec:
  replicas: 3
  selector:
    matchLabels:
      app: flagura
  template:
    metadata:
      labels:
        app: flagura
    spec:
      containers:
      - name: flagura
        image: dhawalhost/flagura:latest
        ports:
        - containerPort: 8080
        env:
        - name: STORE_ENGINE
          value: "POSTGRES"
        - name: POSTGRES_URL
          valueFrom:
            secretKeyRef:
              name: flagura-secrets
              key: database-url
        livenessProbe:
          httpGet:
            path: /livez
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
```

---

## 5. OpenTelemetry Distributed Tracing (REQ-O02)

In high-concurrency multi-service architectures, Flagura correlates evaluations and control plane operations across upstream microservices and downstream storage calls:

### 5.1 Propagation & Header Injection
- **W3C Standards**: Extracts and injects standard W3C `traceparent` and `tracestate` headers across incoming and outgoing HTTP/gRPC requests.
- **Trace ID Injection**: Populates `X-Trace-ID` in HTTP response headers for rapid triage during production incidents.
- **Span Hierarchy**:
  - `HTTP <METHOD> <PATH>`: Server root span capturing HTTP metrics, status codes, and remote caller IP.
  - `flagura.evaluate`: Child span on the evaluation hot path capturing tenant context (`project.id`, `eval.environment`, `eval.requested_flags_count`) with zero allocations.

### 5.2 Environment Variables
```bash
FLAGURA_TRACING_ENABLED=true
OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4318"
FLAGURA_TRACING_SAMPLE_RATE=1.0  # 100% in staging, dial back to 0.05 (5%) in extreme-scale production
```

---

## 6. Disaster Recovery & Automated Snapshot Drills (REQ-O01 & REQ-O04)

Flagura separates ephemeral process caches from durable configuration snapshots:

### 6.1 Atomic Snapshot Backups
- Complete tenant configurations (flags, environments, targeting rules, change requests, API keys, and audit logs) are captured atomically via `flagura backup create --file <path> --compress`.
- Export snapshots support both raw JSON and `.json.gz` compression for compact S3/GCS archival.

### 6.2 Reverse-Chronological Cryptographic Audit Chain Restoration
- During disaster recovery restoration (`flagura backup restore`), audit log entries are replayed from oldest to newest to preserve SHA-256 tamper-evident hash chains.
- Restorations execute under a synthetic `actor == "snapshot_restore"` context to prevent artificial audit mutations from desynchronizing history.

### 6.3 Automated Kubernetes CronJob Drill
See the complete operational guide and ready-to-deploy manifest:
👉 **[Disaster Recovery & Hot Backups Runbook](runbooks/disaster-recovery.md)**

---

## 7. Summary Checklist for Production Readiness

- [x] Storage engine configured to PostgreSQL (`STORE_ENGINE=POSTGRES`).
- [x] Connection pool configured with adequate max connections (`POSTGRES_MAX_OPEN_CONNS`).
- [x] Minimum 2 replicas running across distinct availability zones.
- [x] Ingress load balancer configured with `/livez` and `/readyz` health checks.
- [x] Per-replica rate limits sized according to replica count ($N$) using role tiers (`FLAGURA_RATE_LIMIT_*`).
- [x] OpenTelemetry distributed tracing enabled (`FLAGURA_TRACING_ENABLED=true`).
- [x] Automated daily snapshot backup CronJob verified with automated restore drills (RTO < 15 min, RPO < 1 hr).
