# 📚 Flagura Documentation Portal

Welcome to the official Flagura documentation. This portal covers everything from core concepts and product features to technical architecture, SDK integrations, deployment options, and operational runbooks.

---

## 🧭 Documentation Map

```
docs/
├── README.md                          # Master Documentation Portal (You are here)
│
├── product/                           # Product Guides & Conceptual Documentation
│   ├── overview.md                    # Core concepts, mental model & architecture
│   ├── feature-flags.md               # Flag types: Boolean, Percentage & Multivariate
│   ├── targeting-rules.md             # Attribute-based targeting & segmentation
│   ├── environments.md                # Multi-environment workflows (Prod, Staging, Dev)
│   ├── analytics-and-audit.md         # Real-time metrics, rollout gauges & audit trails
│   └── sdks-and-api.md                # Developer guide for Go SDK, TS/React, Python & REST
│
├── architecture/                      # Technical Architecture for Engineers & Architects
│   ├── system-design.md               # Component decomposition, data flows & sequence diagrams
│   ├── evaluation-algorithm.md        # Mathematical FNV-1a 64-bit hashing & collision analysis
│   ├── data-models-and-storage.md     # Relational schema, JSONB modeling & connection pooling
│   ├── security-architecture.md       # STRIDE threat model, crypto primitives & security headers
│   └── performance-and-benchmarking.md# P99 latency percentiles, memory profiling & scaling
│
├── integrations/                      # Cloud & Platform Setup Guides
│   └── supabase-vercel-setup.md       # Complete guide for Supabase & Vercel deployment
│
└── runbooks/                          # SRE & DevOps Runbooks
    ├── README.md                      # Runbook index & operational principles
    ├── deployment.md                  # Release management & deployment procedures
    ├── database-operations.md         # PostgreSQL migrations, pooler setup & backups
    ├── security-and-access.md         # RBAC, session management & credentials
    └── incident-response.md           # Master kill-switches, rollbacks & troubleshooting
```

---

## ⚡ Quick Navigation

### 1. 🏗️ Technical Architecture & System Design
- **[System Architecture & Design](architecture/system-design.md)** — Architectural components, data flows, and sequence diagrams.
- **[Evaluation Algorithm & Math](architecture/evaluation-algorithm.md)** — Mathematical FNV-1a 64-bit deterministic sticky bucketing and complexity proofs.
- **[Data Models & Storage](architecture/data-models-and-storage.md)** — ERD, PostgreSQL schema, JSONB environment modeling, and connection poolers.
- **[Security Architecture & Threat Model](architecture/security-architecture.md)** — STRIDE analysis, cryptographic primitives, and security headers.
- **[Performance & Latency Profiling](architecture/performance-and-benchmarking.md)** — Sub-microsecond P99 benchmarks, memory profiling, and horizontal scaling.

---

### 2. 📖 Product & Feature Guides
- **[Product Overview](product/overview.md)** — Why sub-microsecond feature flagging matters and how Flagura's deterministic engine works.
- **[Flagura vs. Other Platforms](product/comparison.md)** — Architectural, latency, and cost comparison with LaunchDarkly, Unleash, Flagsmith, and GrowthBook.
- **[Feature Flags Deep Dive](product/feature-flags.md)** — Creating and managing Boolean, Percentage Rollout, and Multivariate flags.
- **[Targeting Rules & Segmentation](product/targeting-rules.md)** — Setting up user rules based on email, country, role, tier, and custom context.
- **[Environments & Workflows](product/environments.md)** — Promoting features safely from Development to Staging and Production.
- **[Analytics & Audit Logs](product/analytics-and-audit.md)** — Monitoring rollout health, P99 evaluation latencies, and tracking change history.

---

### 3. 💻 Developer & SDK Guides
- **[SDKs & API Reference](product/sdks-and-api.md)** — Complete code examples for the **Official Go SDK (`pkg/client`)**, TypeScript / React hooks, Python, and REST API.
- **[REST API Quickstart](../README.md#-rest-api-reference)** — Endpoints for evaluation, flag creation, rollouts, and kill-switches.

---

### 4. 🚢 Deployment & Integrations
- **[Supabase & Vercel Integration Guide](integrations/supabase-vercel-setup.md)** — Step-by-step instructions for configuring Supabase PostgreSQL and deploying to Vercel Serverless Edge.
- **[Docker & Local Deployment](../README.md#-deployment-options)** — Running Flagura via Docker Compose and standalone binary.

---

### 5. 🛡️ Operations & SRE Runbooks
- **[Deployment Runbook](runbooks/deployment.md)** — CI/CD approval workflows, manual triggers, and GoReleaser tagging.
- **[Database Operations](runbooks/database-operations.md)** — PostgreSQL connection pooling, migrations, and health checks.
- **[Security & Access Management](runbooks/security-and-access.md)** — Admin roles, session cookies, and security audit logs.
- **[Incident Response](runbooks/incident-response.md)** — 1-click master kill-switch circuit breakers and emergency rollbacks.
