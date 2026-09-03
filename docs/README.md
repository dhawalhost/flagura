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
│   ├── code-hygiene-and-flag-debt.md  # Automated stale flag cleanup & CLI static scanner
│   └── sdks-and-api.md                # Developer guide for Go SDK, TS/React, Python & REST
│
├── architecture/                      # Technical Architecture for Engineers & Architects
│   ├── system-design.md               # Component decomposition, data flows & sequence diagrams
│   ├── evaluation-algorithm.md        # Mathematical FNV-1a 64-bit hashing & collision analysis
│   ├── data-models-and-storage.md     # SQLite, PostgreSQL & In-Memory storage drivers
│   ├── security-architecture.md       # STRIDE threat model, crypto primitives & security headers
│   └── performance-and-benchmarking.md# P99 latency percentiles, memory profiling & scaling
│
├── integrations/                      # Cloud, Platform & Standard Setup Guides
│   ├── openfeature.md                 # CNCF OpenFeature provider specification & event bus
│   └── supabase-vercel-setup.md       # Complete guide for Supabase & Vercel deployment
│
└── runbooks/                          # SRE & DevOps Runbooks
    ├── README.md                      # Runbook index & operational principles
    ├── deployment.md                  # Release management & deployment procedures
    ├── sdk-publishing.md              # Multi-language SDK publishing (Go, NPM, PyPI, Crates)
    ├── database-operations.md         # SQLite & PostgreSQL operations, pooler & backups
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
- **[Code Hygiene & Technical Debt](product/code-hygiene-and-flag-debt.md)** — Eliminating stale flags, interactive refactoring diffs, and CLI static repository scanning.
- **[Feature Flags Deep Dive](product/feature-flags.md)** — Creating and managing Boolean, Percentage Rollout, and Multivariate flags.
- **[Targeting Rules & Segmentation](product/targeting-rules.md)** — Setting up user rules based on email, country, role, tier, and custom context.
- **[Environments & Workflows](product/environments.md)** — Promoting features safely from Development to Staging and Production.
- **[Analytics & Audit Logs](product/analytics-and-audit.md)** — Monitoring rollout health, P99 evaluation latencies, and tracking change history.

---

### 3. 💻 Developer & SDK Guides
- **[SDKs & API Reference](product/sdks-and-api.md)** — Complete code examples for the **Official Go SDK (`pkg/client`)**, TypeScript / React hooks, Python, and REST API.
- **[CNCF OpenFeature Guide](integrations/openfeature.md)** — Standards-based evaluation with OpenFeature providers for Go, TypeScript, and Python.
- **[Runnable Examples Directory](../examples/README.md)** — Working code examples across Go, TypeScript, Python, and Rust.
- **[REST API Quickstart](../README.md#-rest-api-reference)** — Endpoints for evaluation, flag creation, rollouts, and kill-switches.

---

### 4. 🚢 Deployment & Integrations
- **[Embedded SQLite Operations](runbooks/database-operations.md)** — Zero-dependency, single-binary production deployment with embedded SQLite.
- **[Supabase & Vercel Integration Guide](integrations/supabase-vercel-setup.md)** — Step-by-step instructions for configuring Supabase PostgreSQL and deploying to Vercel Serverless Edge.
- **[Docker & Local Deployment](../README.md#-deployment-options)** — Running Flagura via Docker Compose and standalone binary.

---

### 5. 🛡️ Operations & SRE Runbooks
- **[Deployment Runbook](runbooks/deployment.md)** — CI/CD approval workflows, manual triggers, and GoReleaser tagging.
- **[SDK Release & Publishing Runbook](runbooks/sdk-publishing.md)** — Publishing Go submodule, NPM `@flagura/sdk`, PyPI `flagura-sdk`, Crates.io `flagura`, and OpenFeature catalog.
- **[Database Operations](runbooks/database-operations.md)** — SQLite WAL mode, PostgreSQL connection pooling, migrations, and health checks.
- **[Security & Access Management](runbooks/security-and-access.md)** — Admin roles, session cookies, and security audit logs.
- **[Incident Response](runbooks/incident-response.md)** — 1-click master kill-switch circuit breakers and emergency rollbacks.
