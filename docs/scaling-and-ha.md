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

## 3. Rate Limiting Across Replicas (REQ-A02)

Flagura uses an in-process, high-performance token-bucket rate limiter (`golang.org/x/time/rate`) per replica to maintain sub-millisecond API response times without adding an external Redis network hop to the hot request path.

### 3.1 Operator Sizing Formula
Because token buckets are tracked per replica process, running $N$ replicas behind a load balancer scales the aggregate cluster capacity:

$$\text{Cluster Capacity} = \text{Per-Replica Limit} \times N$$

To enforce a specific target aggregate limit across your cluster (e.g., maximum 1,000 auth requests/minute across 4 replicas):

$$\text{Configured Per-Replica Limit} = \frac{\text{Target Global Limit}}{N}$$

| Target Cluster Limit | Replicas ($N$) | Recommended Config Per Replica |
| :--- | :--- | :--- |
| 100 req/sec Auth | 2 replicas | 50 req/sec (burst: 100) |
| 1,000 req/sec API | 4 replicas | 250 req/sec (burst: 500) |
| 10,000 req/sec API | 10 replicas | 1,000 req/sec (burst: 2,000) |

### 3.2 Load Balancer Recommendations
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

## 5. Summary Checklist for Production Readiness

- [x] Storage engine configured to PostgreSQL (`STORE_ENGINE=POSTGRES`).
- [x] Connection pool configured with adequate max connections (`POSTGRES_MAX_OPEN_CONNS`).
- [x] Minimum 2 replicas running across distinct availability zones.
- [x] Ingress load balancer configured with `/livez` and `/readyz` health checks.
- [x] Per-replica rate limits sized according to replica count ($N$).
- [x] Backing database automated backups and point-in-time recovery (PITR) enabled.
