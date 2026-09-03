# ⚖️ Flagura vs. Other Feature Flag Platforms

When choosing a feature flagging and release management engine, engineering teams evaluate **latency, infrastructure resource footprint, governance controls, experimentation fidelity, deployment complexity, and cost**.

This document provides an objective architectural comparison of Flagura (`v1.5.0`) against major open-source and commercial enterprise solutions (LaunchDarkly, Unleash, Flagsmith, and GrowthBook).

---

## 📊 Comprehensive Comparison Matrix

| Feature / Capability | ⚡ Flagura (`v1.5.0`) | LaunchDarkly | Unleash | Flagsmith | GrowthBook |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **Core Architecture** | **Go (Native Binary)** | Cloud SaaS / Daemon | Node.js / TypeScript | Python (Django) | TypeScript / Next.js |
| **P50 In-Memory Evaluation Latency** | **`~13 ns – 220 ns`** | ~25 µs (SDK) / ~35ms | ~25 µs | ~50 µs | ~30 µs |
| **Data Sovereignty & Privacy** | **✅ 100% Private (Your DB/VPC)** | ❌ 3rd-Party Cloud SaaS | ✅ Self-Hostable | ✅ Self-Hostable | ✅ Self-Hostable |
| **Zero Vendor Lock-in** | **✅ Native OpenFeature Provider** | ⚠️ Proprietary SDKs | ⚠️ Proprietary SDKs | ⚠️ Proprietary SDKs | ⚠️ Proprietary SDKs |
| **4-Eyes Governance (Dual Approver)** | **✅ Included (100% Free)** | 🔒 Enterprise ($$$) | 🔒 Enterprise ($$$) | 🔒 Enterprise ($$$) | 🔒 Enterprise ($$$) |
| **Built-in Statistical A/B Engine** | **✅ Native ($Z$, $P$-Value, Lift)** | 🔒 Experimentation Addon | ❌ Basic Variants Only | ❌ Basic Variants Only | ✅ Python / SQL Engine |
| **Automated Canary with Auto-Rollback** | **✅ Native Stage Ramp** | 🔒 Enterprise Guardrails | ❌ Manual Ramp | ❌ Manual Ramp | ❌ Manual Ramp |
| **Native Prometheus Metrics (`/metrics`)** | **✅ Built-in Exporter** | ⚠️ Requires Sidecar | ⚠️ Requires Addon | ⚠️ Requires Plugin | ⚠️ Requires Addon |
| **Stale Flag Codebase Auditor** | **✅ `flagura audit` CLI** | 🔒 Code References Addon | ❌ Manual | ❌ Manual | ❌ Manual |
| **Memory Footprint (Idle / Load)** | **`~25 MB – 40 MB`** | High (Relay Daemon) | `~450 MB – 800 MB` | `~400 MB – 1 GB` | `~350 MB – 600 MB` |
| **Pricing / Cost** | **100% Free & Open Source (MIT)** | **$500 – $5,000+/mo** (Per MAU) | Freemium ($80+/mo) | Freemium ($45+/mo) | Freemium ($20+/seat) |
| **Zero Database I/O on Evaluation** | **✅ Yes** (`atomic.Pointer` CoW) | ⚠️ Requires Relay Proxy | ⚠️ Requires Redis/Proxy | ❌ Queries DB / Cache | ⚠️ Requires Cache |
| **Deployment Model** | **Single Binary / Docker / Vercel** | Cloud SaaS Only | Multi-tier (Node + Redis + DB) | Multi-tier (Django + DB) | Multi-tier (Next + DB) |
| **Compiled Web UI** | **✅ Yes (`templ` + Tailwind)** | Proprietary Web Portal | React SPA | React SPA | React SPA |
| **Deterministic Sticky Rollout Math** | **64-bit FNV-1a (0 Allocations)** | SHA1 / Murmur3 | Murmur3 | MD5 | Murmur3 |

---

## 🔬 Benchmark Methodology & Latency Breakdown

Evaluated on Apple Silicon M3 Pro using standard Go microbenchmarking (`go test -bench=. -benchmem ./pkg/engine/...`):

| Evaluation Scenario | Flagura Latency | Memory / Allocations | Throughput (Per CPU Core) |
| :--- | :---: | :---: | :---: |
| **FNV-1a 64-bit Hashing** | **13.30 ns** | 0 B / 0 allocs | **~75,000,000 ops/sec** |
| **Master Kill-Switch / Boolean** | **~35.00 ns** | 0 B / 0 allocs | **~28,000,000 ops/sec** |
| **Sticky Bucket (User + Salt)** | **52.61 ns** | 16 B / 1 alloc | **~19,000,000 ops/sec** |
| **Targeting Rule Match (`country == 'US'`)** | **91.02 ns** | 16 B / 1 alloc | **~11,000,000 ops/sec** |
| **Percentage Rollout (`0–100%`)** | **111.50 ns** | 32 B / 2 allocs | **~9,000,000 ops/sec** |
| **Regex Rule Matching (LRU-Cached)** | **221.60 ns** | 32 B / 2 allocs | **~4,500,000 ops/sec** |

---

## 🏆 Why Teams Choose Flagura

### 1. ⚡ Extreme Sub-Microsecond Performance (`< 0.25 µs`)
Traditional Node.js and Python engines evaluate rules by parsing dynamic JSON trees, compiling regular expressions dynamically, and generating significant garbage collection churn. 

Flagura uses **lock-free `atomic.Pointer` snapshots**, **zero-allocation FNV-1a hashing**, and a thread-safe pre-compiled regex cache. Even complex targeting evaluations finish in under **`220 nanoseconds`**, executing completely within L1 CPU cache.

---

### 2. 🛡️ 4-Eyes Change Governance Included by Default
In enterprise environments, accidental production flag toggles can cause severe outages. Competitors lock dual-approver change requests (Maker-Checker principle) behind high-tier enterprise contracts.

Flagura provides **4-Eyes Governance out of the box**:
- Authors submit proposed flag changes as `ChangeRequests`.
- Peer engineers inspect side-by-side configuration diffs.
- Self-approvals are strictly prohibited at the API and store layers.
- Once approved, reviewers apply the change directly to the live cluster.

---

### 3. 📊 Built-in Statistical Experimentation
Rather than requiring separate integrations with third-party analytics platforms, Flagura features an embedded statistical engine:
- Computes standard error, $Z$-scores, two-tailed $P$-values, and relative percentage lift.
- Automatically flags treatments as **WINNING (Statistically Significant)**, **LOSING**, or **NEED SAMPLES** with 95% confidence intervals.

---

### 4. 💰 Elimination of MAU & Event Bill Shock
Commercial SaaS providers charge per **Monthly Active User (MAU)** or per **Evaluation Event**. As your user base scales from 100,000 to 10,000,000 users, SaaS bills can skyrocket to thousands of dollars per month.

Flagura is **100% free and open-source (MIT License)**. You can evaluate **100 million requests on a $5/mo VPS or free Vercel Edge tier** without paying a penny in license fees.

---

### 5. 📦 Single-Binary Operational Simplicity
Other self-hosted systems require running multiple moving parts:
- Node.js runtime + Unleash Proxy + Redis cache + PostgreSQL database.
- Python Django app + Celery worker + Redis + PostgreSQL.

Flagura compiles into a **single standalone Go binary (`flagura`)** containing the core engine, API, database layer, and compiled UI views (`templ`). It boots in **`< 15 milliseconds`** and runs on anything from a Raspberry Pi to a cloud Kubernetes cluster.
