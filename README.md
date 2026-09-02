# ⚡ Flagura — Open-Source Feature Control Plane & Evaluation Engine

<div align="center">

![Flagura Banner](https://img.shields.io/badge/Flagura-Feature%20Control%20Plane-2563eb?style=for-the-badge&logo=go&logoColor=white)
[![Go Report Card](https://goreportcard.com/badge/github.com/dhawalhost/flagura)](https://goreportcard.com/report/github.com/dhawalhost/flagura)
![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go&logoColor=white)
![Evaluation](https://img.shields.io/badge/In--Process%20Evaluation-~85ns-emerald?style=flat&logo=speedtest&logoColor=white)
![Build & Tests](<https://img.shields.io/badge/Tests-Passing%20(100%25)-emerald?style=flat&logo=githubactions&logoColor=white>)
![OpenFeature](https://img.shields.io/badge/OpenFeature-Native-7c3aed?style=flat&logo=openfeature&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-blue?style=flat)

**Persisted in PostgreSQL. Evaluated in CPU Cache.**  
_Sub-microsecond local evaluations (~85ns), automated flag debt hygiene, 4-Eyes governance, and zero PII leakage — without the per-MAU billing trap._

[Live Demo & Console](#-quickstart) • [Architecture](#-architecture) • [OpenFeature](#-openfeature-go-provider) • [API Reference](#-rest-api-reference) • [Go SDK](sdks/go) • [Deployment](#-deployment-options) • [Runbooks](docs/runbooks/README.md)

</div>

---

## 📖 Overview

**Flagura** is an open-source, developer-first feature management platform engineered around a fundamental separation of concerns:

- **Control Plane (Durable ACID Persistence)**: Governs environments, organizations, projects, API keys, RBAC, 4-Eyes change requests, and immutable audit trails backed by PostgreSQL or SQLite.
- **Data Plane (In-Memory SDKs)**: Evaluates targeting rules and percentage rollouts locally in-process using **deterministic 64-bit FNV-1a sticky hashing**, executing in **nanoseconds (~85ns)** with zero network hops, zero database I/O, and zero customer PII egress.

---

## ⚡ Core Architectural Pillars & Wedges

- **⚡ Local-First In-Process Evaluation:** Connected SDKs evaluate flags in-memory with deterministic sticky bucketing and zero database I/O on evaluation hot paths.
- **🧹 Zero Flag Debt & Active Hygiene:** Automated detection of 100% rolled-out flags, longevity tracking, and safe deprecation workflows to eliminate dead code rot.
- **🛡️ 4-Eyes Change Governance:** Configurable environment protection requiring peer review and dual authorization before production flag mutations are applied.
- **🔒 Zero Customer PII Egress:** Targeting rules are compiled and distributed to SDKs; customer emails, IDs, and IP addresses never leave process memory (GDPR/HIPAA ready).
- **💰 Predictable Infrastructure (No MAU Penalties):** Single 15MB Go binary with embedded SQLite or PostgreSQL. Evaluate millions of flags at flat cost without punitive monthly active user (MAU) billing tiers.
- **🌐 OpenFeature Native:** Built-in standard OpenFeature providers for Go, TypeScript, Python, and Rust—switch or adopt Flagura with zero proprietary code lock-in.
- **🏢 Strict Multi-Tenant Isolation:** Complete organization and project-level separation across storage, API credentials, and real-time SSE streaming channels.
- **🔄 Versioned Configuration Synchronization:** Monotonic `config_version` stream protocol with real-time SSE push updates, cold-start disk snapshots, and automatic gap reconciliation.
- **🚨 Authenticated Webhook Kill-Switch:** Secure automated circuit breakers (`X-Webhook-Secret` or Bearer token) for APM alerts (Datadog/Sentry) to shut down failing features instantly.

---

## ⚖️ Capability Matrix

| Feature / Capability              | ⚡ Flagura (`v2.0`)                | OpenFeature Native | Self-Hosted Support |
| :-------------------------------- | :--------------------------------- | :----------------: | :-----------------: |
| **Local In-Process Evaluation**   | ✅ Sub-microsecond (~85ns)         |       ✅ Yes       |       ✅ Yes        |
| **Durable ACID Persistence**      | ✅ PostgreSQL & SQLite Embedded    |        N/A         |       ✅ Yes        |
| **Zero Flag Debt & Hygiene**      | ✅ Stale Flag Detection & Auditing |        N/A         |       ✅ Yes        |
| **Zero Customer PII Egress**      | ✅ 100% In-Process Context (GDPR)  |       ✅ Yes       |       ✅ Yes        |
| **Standard OpenFeature SDKs**     | ✅ Go, TypeScript, Python, Rust    |       ✅ Yes       |       ✅ Yes        |
| **Flat Predictable Cost**         | ✅ No Per-MAU or Seat Penalties    |        N/A         |       ✅ Yes        |
| **Multi-Tenant Organizations**    | ✅ Isolated Projects & Keys        |        N/A         |       ✅ Yes        |
| **Real-Time Streaming Sync**      | ✅ Project-Scoped SSE Channels     |       ✅ Yes       |       ✅ Yes        |
| **Offline Snapshot Resilience**   | ✅ Local Cold-Start Disk Cache     |       ✅ Yes       |       ✅ Yes        |
| **4-Eyes Change Governance**      | ✅ Peer Approval Pipeline          |        N/A         |       ✅ Yes        |
| **Config Version Reconciliation** | ✅ Monotonic `config_version`      |        N/A         |       ✅ Yes        |
| **Automated Webhook Kill-Switch** | ✅ Token-Authenticated             |        N/A         |       ✅ Yes        |
| **A/B Experiment Statistics**     | ✅ Two-Tailed Z-Score & P-Values   |       ✅ Yes       |       ✅ Yes        |
| **Native Prometheus Metrics**     | ✅ `/metrics` Standard Exporter    |        N/A         |       ✅ Yes        |

---

## 🏗️ Architecture

```
┌────────────────────────────────────────────────────────────────────────┐
│                        FLAGURA CONTROL PLANE                           │
├────────────────────────────────────────────────────────────────────────┤
│  • Go Core              ── High-concurrency control API & Web Console │
│  • Storage Layer        ── Multi-tenant PostgreSQL / In-Memory Edge    │
│  • Governance Engine    ── 4-Eyes review pipeline & Change Requests   │
│  • Stream Hub           ── Project & environment-scoped SSE broadcast │
│  • Audit & Telemetry    ── Append-only audit logs & experiment events │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ Config Stream (SSE + Versioning)
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                      FLAGURA DATA PLANE (SDKs)                         │
├────────────────────────────────────────────────────────────────────────┤
│  1. Check Environment Master Kill-Switch                               │
│  2. Evaluate Specific Attribute Targeting Rules                        │
│  3. Compute Deterministic FNV-1a 64-bit Sticky Hash Bucket (0-99.99%)  │
│  4. Resolve Value / Variant locally in nanoseconds (~100ns)            │
└────────────────────────────────────────────────────────────────────────┘
```

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
| `GET`    | `/api/v1/organizations`             | List all organizations (Admin only)                                   |
| `POST`   | `/api/v1/organizations`             | Create new organization (Admin only)                                  |
| `GET`    | `/api/v1/projects`                  | List projects in active or queried organization                       |
| `POST`   | `/api/v1/projects`                  | Create new project in organization                                    |
| `POST`   | `/api/v1/projects/active`           | Switch active project session context                                 |
| `GET`    | `/api/v1/flags/stream`              | Real-time HTTP/2 Server-Sent Events (SSE) flag synchronization stream |
| `GET`    | `/api/v1/flags`                     | List feature flags in active project scope                            |
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
| `GET`    | `/api/v1/audit-logs`                | Fetch immutable audit trail history for active project                |

> **Multi-Tenancy Note:** All evaluation, flag, and audit endpoints accept the `X-Project-ID` request header or `?project_id=...` parameter to scope operations to a specific project (defaults to `proj_default`).

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
	// 1. Initialize Flagura client with Real-Time Streaming, Offline Snapshot, and Project Scoping
	flaguraClient := client.New("https://flagura.dhawalhost.com",
		client.WithProject("proj_default"),                  // Project scope (optional)
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

# Generate a new programmatic API key
flagura api-key create --name="Production Kubernetes SDK" --role=admin

# List active API keys and their last used timestamp
flagura api-key list

# Revoke a compromised API key
flagura api-key revoke <key-id>

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
   - `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`: (Optional SMTP credentials for password reset and invite emails)
4. Click **Deploy**. Vercel compiles the serverless handler via [`api/index.go`](api/index.go) and [`vercel.json`](vercel.json).

### Option B: Docker Container / Self-Hosted

```bash
# Copy and configure environment variables
cp .env.example .env

# Build Docker image
docker build -t flagura:latest .

# Run container
docker run -p 3000:3000 --env-file .env flagura:latest
```

---

## ⚙️ Environment Variables Reference

See [`.env.example`](.env.example) for a fully annotated template:

| Variable                          | Default                 | Purpose                                                                           |
| :-------------------------------- | :---------------------- | :-------------------------------------------------------------------------------- |
| `PORT`                            | `3000`                  | HTTP listening port                                                               |
| `DATABASE_URL`                    | _(empty)_               | PostgreSQL connection string (uses In-Memory Edge Store if empty)                 |
| `FLAGURA_APP_URL`                 | `http://localhost:3000` | Base URL used for recovery links and redirects                                    |
| `ENABLE_LANDING_PAGE`             | `false`                 | `false` redirects directly to `/auth`, `true` displays public product showcase    |
| `SMTP_HOST`                       | _(empty)_               | SMTP hostname for password reset & welcome emails (uses terminal logger if empty) |
| `SMTP_PORT`                       | `587`                   | SMTP port (`587` STARTTLS, `465` SMTPS)                                           |
| `SMTP_USERNAME` / `SMTP_PASSWORD` | _(empty)_               | SMTP authentication credentials                                                   |
| `SMTP_FROM`                       | `no-reply@localhost`    | Sender address on all outbound transactional emails                               |
| `FLAGURA_BRAND_NAME`              | `Flagura`               | Custom brand title rendered in email headers & greetings                          |
| `FLAGURA_SUPPORT_EMAIL`           | _(empty)_               | Support address shown in footers (defaults to internal admin notice if empty)     |
| `FLAGURA_GOVERNANCE_EMAILS`       | _(empty)_               | Comma-separated reviewer emails (auto-resolves DB admins if empty)                |

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
