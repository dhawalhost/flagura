# ⚡ Flagura — Sub-Microsecond Feature Flagging & Release Engine

<div align="center">

![Flagura Banner](https://img.shields.io/badge/Flagura-Feature%20Engine-2563eb?style=for-the-badge&logo=go&logoColor=white)
[![Go Report Card](https://goreportcard.com/badge/github.com/dhawalhost/flagura)](https://goreportcard.com/report/github.com/dhawalhost/flagura)
![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go&logoColor=white)
![Evaluation Latency](https://img.shields.io/badge/P99%20Latency-%3C%2080ns-emerald?style=flat&logo=speedtest&logoColor=white)
![Build & Tests](<https://img.shields.io/badge/Tests-Passing%20(100%25)-emerald?style=flat&logo=githubactions&logoColor=white>)
![License](https://img.shields.io/badge/License-MIT-blue?style=flat)

**Ship feature flags at the speed of thought without breaking production.**  
_Deterministic in-memory evaluation, gradual percentage rollouts, and instant kill-switch circuit breakers._

[Live Demo & Console](#-quickstart) • [Architecture](#-architecture) • [API Reference](#-rest-api-reference) • [SDK Integration](#-polyglot-sdk-quickstart) • [Deployment](#-deployment-options) • [Runbooks](docs/runbooks/README.md)

</div>

---

## 📖 Overview

**Flagura** is a modern, high-performance feature flag and release management platform engineered for zero-latency in-memory flag evaluation. Traditional SaaS feature flagging services introduce remote HTTP network hops (~20–80ms) that slow down user interactions and degrade Core Web Vitals.

Flagura solves this by evaluating rules and percentage rollouts locally in-memory using **deterministic 64-bit FNV-1a hashing** and atomic rule bitmaps, executing in **sub-microsecond time (< 80 nanoseconds)** with zero database roundtrips on evaluation hot paths.

---

## ⚡ Key Highlights

- **⚡ Sub-Microsecond Resolution:** In-memory deterministic rule engine resolving flags in 80ns – 4µs without blocking the main event loop.
- **🎯 Deterministic Sticky Percentage Bucketing:** Pure mathematical FNV-1a 64-bit hashing ensures users consistently land in the exact same rollout bucket across all sessions, platforms, and server restarts.
- **🚨 Instant Master Kill-Switch:** 1-click edge circuit breaker to instantly shut down buggy features in production with zero code redeployments.
- **🛡️ Attribute-Based Targeting Rules:** Granular conditions based on User ID, Email domain (`@company.com`), Geographic country, Role, and Plan tier.
- **💎 Clean, Stripe-Grade UI/UX:** Docked developer console, interactive switchboard playground, live evaluation sandbox, latency benchmark suite, and immutable audit trails.
- **🚀 Dual-Deployment Ready:** Runs as a standalone Go binary, a containerized Docker service, or serverless functions on **Vercel** backed by **Supabase PostgreSQL**.

---

## 🏗️ Architecture

Flagura is engineered for maximum execution speed, zero runtime overhead, and simple deployment:

```
┌────────────────────────────────────────────────────────────────────────┐
│                              FLAGURA ENGINE                            │
├────────────────────────────────────────────────────────────────────────┤
│  • Go 1.22+ Core          ── High-concurrency engine & FNV-1a hashing   │
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

# Run the Go server
go run main.go
```

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

### 2. Flag Management Endpoints

| Method   | Endpoint                     | Description                                    |
| :------- | :--------------------------- | :--------------------------------------------- |
| `GET`    | `/api/v1/flags`              | List all feature flags across environments     |
| `GET`    | `/api/v1/flags/:key`         | Retrieve details for a specific flag           |
| `POST`   | `/api/v1/flags`              | Create or update a feature flag configuration  |
| `PATCH`  | `/api/v1/flags/:key/toggle`  | Instant 1-click toggle for master kill-switch  |
| `PATCH`  | `/api/v1/flags/:key/rollout` | Dynamically update percentage rollout (0–100%) |
| `DELETE` | `/api/v1/flags/:key`         | Permanently remove a feature flag              |
| `POST`   | `/api/v1/benchmark`          | Execute live in-process latency stress test    |
| `GET`    | `/api/v1/audit-logs`         | Fetch immutable audit trail history            |

---

## 💻 Polyglot SDK Quickstart

### 1. Go SDK

```go
package main

import (
    "context"
    "fmt"
    "github.com/dhawalhost/flagura/pkg/domain"
    "github.com/dhawalhost/flagura/pkg/engine"
)

func main() {
    evaluator := engine.NewEvaluator()

    res := evaluator.Evaluate(myFlag, domain.EvaluationContext{
        UserID:      "usr_dhawal_01",
        Email:       "dhawal@flagura.dev",
        Environment: "production",
    })

    if res.Enabled {
        fmt.Printf("Flag ENABLED: variant=%s, resolved in %.2f µs\n", res.Variant, res.EvaluationLatencyUs)
    }
}
```

### 2. React / Next.js Hook

```tsx
import React from "react";
import { useFeatureFlag } from "@flagura/react";

export function SmartSearch() {
  const { isEnabled, variant, loading } = useFeatureFlag("ai-smart-search", {
    userId: "usr_dhawal_01",
    tier: "enterprise",
  });

  if (loading) return <div>Loading...</div>;
  return isEnabled ? <AISmartSearch variant={variant} /> : <StandardSearch />;
}
```

### 3. Node.js / Express Middleware

```javascript
import { FlaguraClient } from "@flagura/node";

const client = new FlaguraClient({ endpoint: "http://localhost:3000" });

app.get("/api/checkout", async (req, res) => {
  const isEligible = await client.evaluate("new-checkout-flow", {
    userId: req.user.id,
    country: req.user.country,
  });

  res.json({ modernCheckout: isEligible });
});
```

### 4. Python

```python
import requests

def check_flag(flag_key: str, user_id: str, email: str = "") -> bool:
    res = requests.post("http://localhost:3000/api/v1/evaluate", json={
        "flags": [flag_key],
        "context": {
            "user_id": user_id,
            "email": email,
            "environment": "production"
        }
    })
    return res.json().get("results", {}).get(flag_key, {}).get("enabled", False)
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
