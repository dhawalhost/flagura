# ⚡ Flagura — Sub-Microsecond Feature Flagging & Release Engine

<div align="center">

![Flagura Banner](https://img.shields.io/badge/Flagura-Feature%20Engine-2563eb?style=for-the-badge&logo=go&logoColor=white)
[![Go Report Card](https://goreportcard.com/badge/github.com/dhawalhost/flagura)](https://goreportcard.com/report/github.com/dhawalhost/flagura)
![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go&logoColor=white)
![Evaluation Latency](https://img.shields.io/badge/P50%20Latency-~109ns-emerald?style=flat&logo=speedtest&logoColor=white)
![Build & Tests](<https://img.shields.io/badge/Tests-Passing%20(100%25)-emerald?style=flat&logo=githubactions&logoColor=white>)
![OpenFeature](https://img.shields.io/badge/OpenFeature-Compatible-7c3aed?style=flat&logo=openfeature&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-blue?style=flat)

**Feature flags that disappear from your critical path.**  
_In-process deterministic in-memory evaluation (~100ns), OpenFeature native providers, gradual rollouts, authenticated webhook kill-switches, and 4-Eyes change governance._

[Live Demo & Console](#-quickstart) • [Architecture](#-architecture) • [OpenFeature](#-openfeature-go-provider) • [API Reference](#-rest-api-reference) • [SDK Integration](#-polyglot-sdk-quickstart) • [Deployment](#-deployment-options) • [Runbooks](docs/runbooks/README.md)

</div>

---

## 📖 Overview

**Flagura** is a modern, high-performance feature flag and experimentation engine engineered for in-process flag evaluation. Traditional SaaS feature flagging services introduce remote HTTP network hops (~20–80ms) that slow down user interactions and degrade Core Web Vitals.

Flagura evaluates rules and percentage rollouts locally in-memory using **deterministic 64-bit FNV-1a hashing** and cached targeting rule matchers, executing in **sub-microsecond time (~90ns – 220ns)** with zero database I/O on evaluation hot paths.

---

## ⚡ Key Highlights

- **⚡ Sub-Microsecond Local Evaluation:** In-memory deterministic rule engine resolving flags in ~90ns – 250ns with zero remote network I/O.
- **🌐 Native OpenFeature Provider:** Drop-in vendor-neutral `open-feature/go-sdk` integration—switch or adopt Flagura with zero code lock-in.
- **🛡️ 4-Eyes Change Governance & RBAC:** Protected API endpoints, non-self-assignable roles, and multi-approver pull requests for production flag mutations.
- **🎯 Deterministic Sticky Percentage Bucketing:** Mathematical FNV-1a 64-bit hashing ensures users consistently land in the exact same rollout bucket across sessions and platforms.
- **🚨 Authenticated Webhook Kill-Switch:** Secure circuit breaker (`X-Webhook-Secret` or Bearer token) to immediately shut down broken flags in production.
- **📊 A/B Testing & Statistical Engine:** Ingestion API (`/api/v1/events`) computing two-tailed pooled Z-scores, p-values, confidence intervals, and variant lift in sub-35µs.
- **💎 Clean Developer Console:** Built with Go, Templ, and Tailwind CSS featuring an interactive switchboard, live evaluator sandbox, and append-only audit trail.

---

## ⚖️ How Flagura Compares

| Feature / Capability             |             ⚡ Flagura (`v1.4.0`)     |      LaunchDarkly       |            Unleash             |        Flagsmith         |       GrowthBook       |
| :------------------------------- | :---------------------------------: | :---------------------: | :----------------------------: | :----------------------: | :--------------------: |
| **Core Architecture**            |       **Go (Native Binary)**        |   Cloud SaaS / Daemon   |      Node.js / TypeScript      |     Python (Django)      |  TypeScript / Next.js  |
| **P50 Local Evaluation Latency** |      **`~13 ns – 220 ns`**          | ~25 µs (In-Process SDK) |       ~25 µs (Proxy SDK)       |          ~50 µs          |         ~30 µs         |
| **Data Sovereignty & Privacy**   |   **✅ 100% Private (Your DB/VPC)** |    ❌ 3rd-Party Cloud   |             ✅ Yes             |          ✅ Yes          |         ✅ Yes         |
| **Zero Vendor Lock-in**          |  **✅ Native OpenFeature Provider** | ⚠️ Proprietary SDKs     | ⚠️ Proprietary SDKs            | ⚠️ Proprietary SDKs      | ⚠️ Proprietary SDKs    |
| **4-Eyes Change Governance**     |     **✅ Included (100% Free)**     |   🔒 Enterprise ($$$)   |      🔒 Enterprise ($$$)       |   🔒 Enterprise ($$$)    |  🔒 Enterprise ($$$)   |
| **Built-in Statistical A/B**     | **✅ Native ($Z$, $P$-Value, Lift)**| 🔒 Experimentation Addon|     ❌ Basic Variants Only     |  ❌ Basic Variants Only  |  ✅ Python / SQL Engine|
| **Automated Canary + Guardrails**|     **✅ Native Stage Ramp**        | 🔒 Enterprise Guardrails|         ❌ Manual Ramp         |      ❌ Manual Ramp      |     ❌ Manual Ramp     |
| **Prometheus Metrics (`/metrics`)** | **✅ Native Exporter**          |    ⚠️ Requires Sidecar  |        ⚠️ Requires Addon       |    ⚠️ Requires Plugin    |   ⚠️ Requires Addon    |
| **Stale Flag Codebase Auditor**  |       **✅ `flagura audit` CLI**    | 🔒 Code References Addon|            ❌ Manual           |        ❌ Manual         |        ❌ Manual       |
| **Memory Footprint**             |            **`~25 MB – 40 MB`**     |   High (Relay Daemon)   |       `~450 MB – 800 MB`       |     `~400 MB – 1 GB`     |   `~350 MB – 600 MB`   |
| **Pricing Model**                |     **100% Free & Open Source**     |  **$500 – $5,000+/mo**  |       Freemium ($80+/mo)       |    Freemium ($45+/mo)    |   Freemium ($20+/mo)   |
| **Zero DB I/O on Evaluation**    |    **✅ Yes (`atomic.Pointer` CoW)**|    ⚠️ Requires Proxy    |    ⚠️ Requires Redis/Proxy     |   ❌ Queries DB/Cache    |   ⚠️ Requires Cache    |
| **Deployment Model**             | **Single Binary / Docker / Vercel** |     Cloud SaaS Only     | Multi-tier (Node + Redis + DB) | Multi-tier (Django + DB) | Multi-tier (Next + DB) |

> **Benchmark Methodology:** Evaluated on Apple M3 Pro using standard Go benchmarks (`go test -bench=. -benchmem ./pkg/engine/...`): Percentage Rollout ~111 ns (32 B/op), Rule Match ~91 ns (16 B/op), Bare FNV-1a Hash ~13 ns (0 B/op), Regex Match (Cached) ~221 ns (32 B/op). Competitor metrics reflect typical client SDK in-memory evaluation / relay daemon documentation.

[👉 Read full architectural comparison in documentation](docs/product/comparison.md)

---

## 🏗️ Architecture

Flagura is engineered for maximum execution speed, zero runtime overhead, and simple deployment:

```
┌────────────────────────────────────────────────────────────────────────┐
│                              FLAGURA ENGINE                            │
├────────────────────────────────────────────────────────────────────────┤
│  • Go 1.26+ Core          ── High-concurrency engine & FNV-1a hashing   │
│  • Direct SQL Store       ── Pure database/sql + lib/pq for Supabase   │
│  • API-First Design       ── Ultra-fast batch & single evaluation API  │
│  • Compiled Views         ── Type-safe compiled HTML views & CSS      │
│  • Reactive UI Components ── Fast, zero-page-reload console components │
└────────────────────────────────────────────────────────────────────────┘
```

```
                     ┌──────────────────────────┐
                     │ Client Application / SDK │
                     └─────────────┬────────────┘
                                   │ (In-Memory / HTTP)
                                   ▼
        ┌─────────────────────────────────────────────────────┐
        │               Flagura Evaluation Engine             │
        ├─────────────────────────────────────────────────────┤
        │  1. Check Environment Master Kill-Switch            │
        │  2. Evaluate Specific Attribute Rules (Email/Tier)  │
        │  3. Compute FNV-1a 64-bit Sticky Hash Bucket (0-99) │
        │  4. Resolve Treatment vs Control Group              │
        └──────────────────────────┬──────────────────────────┘
                                   │ (< 80ns execution)
                                   ▼
                     ┌──────────────────────────┐
                     │  Boolean / Variant Value │
                     └──────────────────────────┘
```

---

## 🚀 Quickstart

### 1. Run Locally (In-Memory Edge Mode)

No database installation required to get started. Flagura includes a built-in in-memory edge store pre-seeded with sample flags:

```bash
# Clone the repository
git clone https://github.com/dhawalhost/flagura.git
cd flagura

# Run with Make (auto-generates templates & starts server)
make dev

# Or build both server and developer CLI binaries into bin/
make build
```

> **Tip:** Run `make help` to see all available local workflows (testing, benchmarks, docker, code coverage).

Open your browser to:

- **Storytelling Landing Page:** [http://localhost:3000](http://localhost:3000)
- **Developer Console:** [http://localhost:3000/dashboard](http://localhost:3000/dashboard)

---

### 2. Connect to Supabase PostgreSQL

1. Open your **Supabase Dashboard** -> **SQL Editor**.
2. Run the migration script in [`supabase/schema.sql`](supabase/schema.sql):

   ```sql
   CREATE TABLE IF NOT EXISTS feature_flags (
       id TEXT PRIMARY KEY,
       key TEXT UNIQUE NOT NULL,
       name TEXT NOT NULL,
       description TEXT,
       type TEXT NOT NULL DEFAULT 'boolean',
       tags TEXT[] DEFAULT '{}',
       variants JSONB DEFAULT '[]'::jsonb,
       environments JSONB NOT NULL DEFAULT '{}'::jsonb,
       created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
       updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
   );

   CREATE TABLE IF NOT EXISTS audit_logs (
       id TEXT PRIMARY KEY,
       flag_key TEXT NOT NULL,
       action TEXT NOT NULL,
       environment TEXT NOT NULL,
       actor TEXT NOT NULL,
       details TEXT,
       timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
   );
   ```

3. Set your connection string and launch:
   ```bash
   export DATABASE_URL="postgres://postgres.[PROJECT_REF]:[PASSWORD]@aws-0-[REGION].pooler.supabase.com:6543/postgres?sslmode=require"
   go run main.go
   ```

---

## 🔌 REST API Reference

### 1. Batch / Single Flag Evaluation

**Endpoint:** `POST /api/v1/evaluate`

Evaluates requested feature flags against a user context in sub-millisecond time.

#### Request:

```bash
curl -X POST http://localhost:3000/api/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flags": ["ai-smart-search", "new-checkout-flow"],
    "context": {
      "user_id": "usr_dhawal_01",
      "email": "dhawal@flagship.dev",
      "country": "US",
      "role": "admin",
      "tier": "enterprise",
      "environment": "production"
    }
  }'
```

#### Response:

```json
{
  "results": {
    "ai-smart-search": {
      "flag_key": "ai-smart-search",
      "enabled": true,
      "variant": "treatment",
      "value": true,
      "reason": "TARGETING_RULE_MATCH",
      "matched_rule_id": "rule_staff_whitelist",
      "bucket_val": 42,
      "evaluation_latency_ns": 4820,
      "evaluation_latency_us": 4.82
    },
    "new-checkout-flow": {
      "flag_key": "new-checkout-flow",
      "enabled": true,
      "variant": "true",
      "value": true,
      "reason": "PERCENTAGE_ROLLOUT",
      "bucket_val": 35,
      "evaluation_latency_ns": 120,
      "evaluation_latency_us": 0.12
    }
  },
  "total_flags": 2,
  "environment": "production",
  "total_duration_us": 22.4
}
```

---

### 2. Flag Management & API Endpoints

| Method   | Endpoint                            | Description                                                           |
| :------- | :---------------------------------- | :-------------------------------------------------------------------- |
| `GET`    | `/healthz` / `/livez`               | Kubernetes liveness health probe                                      |
| `GET`    | `/readyz`                           | Kubernetes readiness probe (checks storage availability)              |
| `GET`    | `/metrics`                          | Prometheus metrics exposition (`flagura_evaluations_total`, etc.)     |
| `GET`    | `/api/v1/flags/stream`              | Real-time HTTP/2 Server-Sent Events (SSE) flag synchronization stream |
| `GET`    | `/api/v1/flags`                     | List all feature flags across environments                            |
| `GET`    | `/api/v1/flags/:key`                | Retrieve configuration and rules for a specific flag                  |
| `POST`   | `/api/v1/flags`                     | Create or update a feature flag configuration                         |
| `PATCH`  | `/api/v1/flags/:key/toggle`         | Instant 1-click toggle for master kill-switch                         |
| `PATCH`  | `/api/v1/flags/:key/rollout`        | Dynamically update percentage rollout (0–100%)                        |
| `POST`   | `/api/v1/flags/:key/promote`        | Promote flag configuration (e.g. `?from=staging&to=production`)       |
| `POST`   | `/api/v1/webhooks/kill-switch/:key` | Automated kill-switch endpoint for APM alerts (Datadog/Sentry)        |
| `POST`   | `/api/v1/telemetry/events`          | Ingest batched evaluation counts from client SDKs                     |
| `GET`    | `/api/v1/telemetry/stats`           | Query 24h evaluation velocity and variant distribution                |
| `DELETE` | `/api/v1/flags/:key`                | Permanently remove a feature flag (Requires Admin role)               |
| `POST`   | `/api/v1/evaluate`                  | Evaluate flags (`?trace=true` returns visual execution trace)         |
| `POST`   | `/api/v1/benchmark`                 | Execute live in-process latency stress test                           |
| `GET`    | `/api/v1/audit-logs`                | Fetch immutable audit trail history                                   |

---

## 🌐 OpenFeature Polyglot Ecosystem

Flagura provides drop-in **OpenFeature Providers** across Go, TypeScript/JavaScript, and Python, ensuring zero vendor lock-in:

### 1. Go OpenFeature Provider (`pkg/openfeature`)

```bash
go get github.com/dhawalhost/flagura/pkg/openfeature
go get github.com/open-feature/go-sdk
```

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/dhawalhost/flagura/pkg/client"
	flaguraOF "github.com/dhawalhost/flagura/pkg/openfeature"
	of "github.com/open-feature/go-sdk/openfeature"
)

func main() {
	// 1. Initialize Flagura client with Real-Time Streaming and Offline Snapshot
	flaguraClient := client.New("https://flagura.dhawalhost.com",
		client.WithLocalEvaluation(30*time.Second),
		client.WithStreaming(true),                          // <5ms instant updates via SSE
		client.WithSnapshotFile("/tmp/flagura-cache.json"),  // offline resilience
		client.WithCircuitBreaker(5, 10*time.Second),        // 3-state failure circuit breaker
	)
	defer flaguraClient.Close()

	// 2. Register Flagura as global OpenFeature Provider (includes live eventing)
	_ = of.SetProviderAndWait(flaguraOF.NewProvider(flaguraClient))

	// 3. Evaluate flags using standard OpenFeature APIs
	ofClient := of.NewClient("checkout-service")
	evalCtx := of.NewEvaluationContext("usr_alex_42", map[string]interface{}{
		"email": "alex@company.com",
		"tier":  "enterprise",
	})

	enabled, _ := ofClient.BooleanValue(context.Background(), "ai-smart-search", false, evalCtx)
	log.Printf("Flag Status: %v", enabled)
}
```

### 2. TypeScript / Node.js OpenFeature Provider (`sdks/js`)

```typescript
import { OpenFeature } from "@openfeature/server-sdk";
import { FlaguraOpenFeatureProvider } from "flagura-sdk";

// 1. Initialize and register Flagura OpenFeature provider
const provider = new FlaguraOpenFeatureProvider({
  endpoint: "https://flagura.dhawalhost.com",
  apiKey: process.env.FLAGURA_API_KEY,
  enableStreaming: true, // <5ms live flag sync
});

await OpenFeature.setProviderAndWait(provider);
const client = OpenFeature.getClient();

// 2. Evaluate with OpenFeature context
const isEnabled = await client.getBooleanValue("ai-smart-search", false, {
  targetingKey: "usr_123",
  email: "dev@flagura.dev",
  tier: "enterprise",
});
```

### 3. Python OpenFeature Provider (`sdks/python`)

```python
from openfeature import api
from openfeature.evaluation_context import EvaluationContext
from flagura.openfeature_provider import FlaguraOpenFeatureProvider

# 1. Register Flagura OpenFeature provider
api.set_provider(FlaguraOpenFeatureProvider(
    endpoint="https://flagura.dhawalhost.com",
    api_key="your-api-key",
    enable_streaming=True, # <5ms live flag sync
))
client = api.get_client()

# 2. Evaluate flags
ctx = EvaluationContext(targeting_key="usr_123", attributes={"email": "alice@flagura.dev"})
is_enabled = client.get_boolean_value("ai-smart-search", False, ctx)
```

---

## ⚡ Real-Time Streaming & Resilience

### 1. Sub-5ms SSE Flag Streaming (`GET /api/v1/flags/stream`)

Whenever a flag is toggled, rolled out, or promoted in the Flagura console or via API, the control plane broadcasts an instant Server-Sent Event (SSE) to all connected microservices worldwide. Your in-memory cache synchronizes in **`< 5ms`** without polling delays.

### 2. Offline Snapshot Persistence & 3-State Circuit Breaker

- **Offline Snapshot (`WithSnapshotFile`)**: Writes atomic snapshots to disk. If your database or control plane goes down, your application cold-boots in **0ms** from disk cache with zero evaluation downtime.
- **3-State Circuit Breaker (`CLOSED` ↔ `OPEN` ↔ `HALF_OPEN`)**: Automatically fast-fails remote network requests in `< 20µs` during server interruptions, shielding your application from latency spikes.

---

## 🚨 Automated Alert Webhook Kill-Switches

Connect Flagura to your APM and monitoring infrastructure (**Datadog, Sentry, Prometheus Alertmanager, PagerDuty**):

```bash
# Example: Automated rollback webhook from Datadog alert or CI/CD
curl -X POST "https://flagura.dhawalhost.com/api/v1/webhooks/kill-switch/ai-smart-search?env=production" \
  -H "Content-Type: application/json"
```

When an alert triggers, Flagura instantly flips the master kill-switch and broadcasts the disable event to all microservices over SSE in **`< 5ms`**.

---

## 🔍 Visual Rule Execution Trace ("Why Did I Get This Variant?")

Debug complex targeting rules and multivariate splits using the on-demand trace engine:

```bash
curl -X POST "https://flagura.dhawalhost.com/api/v1/evaluate?trace=true" \
  -H "Content-Type: application/json" \
  -d '{
    "flags": ["ai-smart-search"],
    "context": {
      "user_id": "usr_alex_42",
      "email": "alex@acme.com",
      "tier": "enterprise",
      "environment": "production"
    }
  }'
```

**Trace Output:**

```json
{
  "traces": {
    "ai-smart-search": {
      "steps": [
        {
          "step_index": 1,
          "name": "Master Kill-Switch Check",
          "passed": true,
          "detail": "Flag is active in production."
        },
        {
          "step_index": 2,
          "name": "Targeting Rule Match: Enterprise VIP",
          "passed": true,
          "detail": "Condition matched (tier equals [enterprise]). Action: force_enabled."
        }
      ],
      "final_reason": "TARGETING_RULE_MATCH",
      "final_variant": "treatment",
      "final_enabled": true,
      "elapsed_ns": 78
    }
  }
}
```

---

## 🔀 Environment Promotion (Staging ➔ Production)

Promote verified rules and multivariate traffic allocations from Staging to Production in 1-click via the Dashboard or API:

```bash
curl -X POST "https://flagura.dhawalhost.com/api/v1/flags/ai-smart-search/promote?from=staging&to=production" \
  -H "Cookie: flagura_session=..."
```

---

## 💻 Flagura Developer CLI (`cmd/flagura`)

Flagura includes a single-binary CLI tool for developers, terminal evaluation, and CI/CD automated rollouts:

```bash
# Build / install CLI
go build -o /usr/local/bin/flagura ./cmd/flagura
```

### CLI Commands:

```bash
# List all feature flags and their current status
flagura list

# Instant master kill-switch toggle
flagura toggle ai-smart-search --env=production

# Update percentage rollout
flagura rollout ai-smart-search 25% --env=production

# Evaluate flag in terminal with visual execution trace
flagura evaluate ai-smart-search --user=usr_alex_42 --trace

# Promote staging configuration to production
flagura promote ai-smart-search --from=staging --to=production

# Scan codebase technical debt and stale flags
flagura clean-up
```

---

## 💻 Polyglot SDK Quickstart

### 1. Official Go Native SDK (`pkg/client`)

The built-in Go client supports **sub-microsecond in-memory evaluations** (cached in-process) and remote evaluations:

```bash
go get github.com/dhawalhost/flagura/pkg/client
```

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/dhawalhost/flagura/pkg/client"
)

func main() {
	// Initialize Flagura client with in-memory caching and offline snapshot
	c := client.New("https://flagura.dhawalhost.com",
		client.WithLocalEvaluation(30*time.Second),
		client.WithSnapshotFile("/tmp/flagura-cache.json"),
	)
	defer c.Close()

	// Evaluate flag locally in < 80 nanoseconds
	result, err := c.Evaluate(context.Background(), "ai-smart-search", client.Context{
		UserID:  "usr_dhawal_01",
		Email:   "dhawal@flagura.dev",
		Country: "US",
		Role:    "admin",
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("Flag: %s | Enabled: %v | Variant: %s (in %d ns)\n",
		result.FlagKey, result.Enabled, result.Variant, result.EvaluationLatencyNs)
}
```

---

### 2. TypeScript / JavaScript (Node.js, Next.js, Browser)

Zero-dependency standard `fetch` helper for full-stack JavaScript:

```typescript
interface FlagEvaluation {
  flag_key: string;
  enabled: boolean;
  variant: string;
  value: any;
  reason: string;
}

export async function evaluateFlag(
  flagKey: string,
  context: {
    userId: string;
    email?: string;
    country?: string;
    role?: string;
    environment?: string;
  },
  endpoint = "https://flagura.dhawalhost.com",
): Promise<FlagEvaluation> {
  const res = await fetch(`${endpoint}/api/v1/evaluate`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      flags: [flagKey],
      context: {
        user_id: context.userId,
        email: context.email,
        country: context.country,
        role: context.role,
        environment: context.environment || "production",
      },
    }),
  });

  if (!res.ok) throw new Error(`Flagura API error: ${res.statusText}`);
  const data = await res.json();
  return data.results[flagKey];
}

// Example Usage
const flag = await evaluateFlag("ai-smart-search", {
  userId: "usr_123",
  email: "user@flagura.dev",
});
if (flag.enabled) {
  console.log("Feature active! Variant:", flag.variant);
}
```

---

### 3. Python (FastAPI, Django, AI Agents)

```python
import requests

def evaluate_flag(flag_key: str, user_id: str, email: str = "", endpoint: str = "https://flagura.dhawalhost.com") -> bool:
    try:
        response = requests.post(f"{endpoint}/api/v1/evaluate", json={
            "flags": [flag_key],
            "context": {
                "user_id": user_id,
                "email": email,
                "environment": "production"
            }
        }, timeout=2.0)
        response.raise_for_status()
        data = response.json()
        return data.get("results", {}).get(flag_key, {}).get("enabled", False)
    except Exception as e:
        print(f"[WARN] Flag evaluation fallback to false: {e}")
        return False

# Example Usage
if evaluate_flag("ai-smart-search", user_id="usr_dhawal_01", email="dhawal@flagura.dev"):
    print("AI Smart Search is enabled!")
```

---

### 4. cURL / REST API

```bash
curl -X POST https://flagura.dhawalhost.com/api/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flags": ["ai-smart-search"],
    "context": {
      "user_id": "usr_dhawal_01",
      "email": "dhawal@flagura.dev",
      "environment": "production"
    }
  }'
```

---

## 🚢 Deployment Options

### Option A: Vercel Serverless (Zero Config)

1. Push your code to GitHub.
2. Import the project into **Vercel**.
3. Under **Settings -> Environment Variables**, add:
   - `DATABASE_URL`: `postgres://...` (Your Supabase connection string)
4. Click **Deploy**. Vercel compiles the serverless handler via [`api/index.go`](api/index.go) and [`vercel.json`](vercel.json).

### Option B: Docker Container

```bash
# Build Docker image
docker build -t flagura:latest .

# Run container
docker run -p 3000:3000 -e DATABASE_URL="postgres://..." flagura:latest
```

---

## 🧪 Testing & Verification

Run the comprehensive unit and integration test suite:

```bash
# Run all Go tests
go test -v ./...

# Recompile Templ components
templ generate

# Build production binary
go build -o bin/flagura main.go
```

---

## 📚 Operations & Runbooks

Comprehensive SRE, emergency response, and operational runbooks are available in the [`docs/runbooks/`](docs/runbooks/README.md) directory:

- 🚀 **[Deployment & Release Management](docs/runbooks/deployment.md)**: Standalone binary, Docker, Vercel serverless, and rollback procedures.
- 🌐 **[Supabase & Vercel Integration Guide](docs/integrations/supabase-vercel-setup.md)**: Step-by-step cloud setup, connection pooling, and continuous deployment.
- 🚨 **[Incident Response & Emergency Operations](docs/runbooks/incident-response.md)**: 1-click kill switch, database outage fallback, latency debugging, and audit forensics.
- 🗄️ **[Database Operations & Disaster Recovery](docs/runbooks/database-operations.md)**: Supabase PostgreSQL schema, connection pool tuning, and PITR backup/recovery.
- 🛡️ **[Security & Access Management](docs/runbooks/security-and-access.md)**: RBAC provisioning, credential rotation, and automated vulnerability triage.

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).
