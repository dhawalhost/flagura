# 💻 SDKs & REST API Developer Guide

This guide details how to integrate Flagura into your applications using the **Official Go SDK**, TypeScript / React, Python, or standard REST API endpoints.

---

## 1. Official Go SDK (`pkg/client`)

The Go client is designed for high-concurrency microservices, providing both **local in-memory synchronized evaluations** (< 1 µs) and remote API evaluations.

### Installation
```bash
go get github.com/dhawalhost/flagura/pkg/client
```

### High-Performance In-Memory Local Evaluation with Real-Time Streaming (Recommended)
```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/dhawalhost/flagura/pkg/client"
	"github.com/dhawalhost/flagura/pkg/domain"
)

func main() {
	// Initialize client with SSE streaming, offline snapshot disk cache, project scoping, and circuit breaking
	c := client.New("https://flagura.dev",
		client.WithProject("proj_default"),                  // scope to project (optional)
		client.WithEnvironment(domain.EnvProduction),        // default environment scope
		client.WithLocalEvaluation(30*time.Second),
		client.WithStreaming(true),                          // <5ms instant updates via SSE
		client.WithSnapshotFile("/tmp/flagura-cache.json"),  // 0ms offline cold-boot
		client.WithCircuitBreaker(5, 10*time.Second),        // 3-state failure circuit breaker
		client.WithTelemetryFlushInterval(60*time.Second),   // evaluation metrics push
		client.WithAPIKey("flg_live_..."),                   // environment-scoped API key
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

### Register Real-Time Change Listeners
```go
c.RegisterUpdateListener(func(flags map[string]domain.FeatureFlag, changedKeys []string) {
	fmt.Printf("Real-time flag change detected for keys: %v\n", changedKeys)
})
```

### Evaluation Helpers
```go
// Returns boolean status safely
isEnabled := c.IsEnabled(ctx, "ai-smart-search", client.Context{
	UserID: "usr_123",
})

// Returns variant or fallback
variant := c.GetVariant(ctx, "checkout-v2", client.Context{
	UserID: "usr_123",
}, "control")
```

---

## 2. TypeScript / JavaScript & React

Zero-dependency standard `fetch` integration for Node.js, Next.js, and Browser:

### TypeScript Function
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
  context: { userId: string; email?: string; country?: string; role?: string; tier?: string; environment?: string },
  endpoint = "https://flagura.dev"
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
        tier: context.tier,
        environment: context.environment || "production",
      },
    }),
  });

  if (!res.ok) throw new Error(`Flagura API error: ${res.statusText}`);
  const data = await res.json();
  return data.results[flagKey];
}
```

### React Hook Example
```tsx
import React, { useState, useEffect } from "react";
import { evaluateFlag } from "./flagura";

export function useFeatureFlag(flagKey: string, context: { userId: string; email?: string }) {
  const [enabled, setEnabled] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    evaluateFlag(flagKey, context)
      .then((res) => setEnabled(res?.enabled ?? false))
      .catch(() => setEnabled(false))
      .finally(() => setLoading(false));
  }, [flagKey, context.userId]);

  return { enabled, loading };
}

// In your React Component
export function SmartSearchBanner() {
  const { enabled, loading } = useFeatureFlag("ai-smart-search", { userId: "usr_123" });
  if (loading) return null;
  return enabled ? <div className="banner">AI Search is Active!</div> : null;
}
```

---

## 3. Python SDK (`flagura`)

The official Python client supports batch evaluations, real-time SSE streaming, and thread-safe callbacks:

```python
from flagura import FlaguraClient, EvaluationContext

# Initialize client with real-time SSE streaming
client = FlaguraClient(
    endpoint="https://flagura.dev",
    api_key="your-api-key",
    enable_streaming=True, # <5ms live flag updates
)

# Register real-time change listener
client.on_update(lambda flags: print(f"Flags updated in real-time! Count: {len(flags)}"))

# Evaluate flag
context = EvaluationContext(
    user_id="usr_dhawal_01",
    email="dhawal@flagura.dev",
    tier="enterprise",
)

if client.is_enabled("ai-smart-search", context):
    variant = client.get_variant("ai-smart-search", context)
    print(f"AI Smart Search is ON! Variant: {variant}")
```

---

## 4. OpenFeature Universal Standard

Adopt Flagura with zero vendor lock-in across Go, TypeScript, and Python:

```go
// Go OpenFeature
_ = of.SetProviderAndWait(flaguraOF.NewProvider(flaguraClient))
```

```typescript
// TypeScript OpenFeature
await OpenFeature.setProviderAndWait(new FlaguraOpenFeatureProvider({ endpoint: "https://flagura.dev", enableStreaming: true }));
```

```python
# Python OpenFeature
api.set_provider(FlaguraOpenFeatureProvider(endpoint="https://flagura.dev", enable_streaming=True))
```

---

## 5. Native Rust SDK (`flagura-rs`)

Add `flagura` to `Cargo.toml`:
```toml
[dependencies]
flagura = "1.3.0"
tokio = { version = "1", features = ["full"] }
```

```rust
use flagura::{FlaguraClient, EvaluationContext};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let client = FlaguraClient::new("https://flagura.dev");
    let ctx = EvaluationContext::new("usr_alex_42").with_email("alex@company.com");

    if client.is_enabled("ai-smart-search", &ctx).await {
        println!("✨ AI Smart Search enabled!");
    }

    // Track conversion event for A/B experiment
    client.track("checkout-v2", "treatment", "purchase", 49.99, "usr_alex_42").await?;
    Ok(())
}
```

---

## 6. CNCF OpenFeature Standard Providers

Flagura officially implements the **Cloud Native Computing Foundation (CNCF) OpenFeature** specification across multiple runtimes, ensuring complete portability without proprietary vendor lock-in.

### Go OpenFeature Provider (`sdks/go/openfeature`)
```go
import (
    of "github.com/open-feature/go-sdk/openfeature"
    flaguraOF "github.com/dhawalhost/flagura/sdks/go/openfeature"
)

provider := flaguraOF.NewProvider(client)
_ = of.SetProviderAndWait(provider)
ofClient := of.NewClient("app-service")

val, _ := ofClient.BooleanValue(ctx, "ai-smart-search", false, of.NewEvaluationContext("usr_123", nil))
```

### TypeScript / Node.js OpenFeature Provider (`sdks/js`)
```typescript
import { OpenFeature } from '@openfeature/server-sdk';
import { FlaguraOpenFeatureProvider } from 'flagura-sdk';

const provider = new FlaguraOpenFeatureProvider({
  endpoint: 'http://localhost:3000',
  apiKey: process.env.FLAGURA_KEY
});
await OpenFeature.setProviderAndWait(provider);
const client = OpenFeature.getClient();

const enabled = await client.getBooleanValue('ai-smart-search', false, { targetingKey: 'usr_123' });
```

### Python OpenFeature Provider (`sdks/python`)
```python
from openfeature import api
from openfeature.evaluation_context import EvaluationContext
from flagura.openfeature_provider import FlaguraOpenFeatureProvider

api.set_provider(FlaguraOpenFeatureProvider(endpoint="http://localhost:3000", api_key="..."))
client = api.get_client()

ctx = EvaluationContext(targeting_key="usr_123")
enabled = client.get_boolean_value("ai-smart-search", False, ctx)
```

👉 **[Deep-Dive OpenFeature Architecture Guide](../integrations/openfeature.md)**  
👉 **[Runnable Cross-Language Code Examples (`examples/`)](../../examples/README.md)**

---

## 7. REST API Reference

### 1. Evaluate Flags with Execution Trace (`POST /api/v1/evaluate?trace=true`)
```bash
curl -X POST "https://flagura.dev/api/v1/evaluate?trace=true" \
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

### 2. A/B Experiment Metric Ingestion (`POST /api/v1/events`)
```bash
curl -X POST "https://flagura.dev/api/v1/events" \
  -H "Content-Type: application/json" \
  -d '{
    "event": {
      "flag_key": "checkout-v2",
      "variant": "treatment",
      "metric_name": "purchase_completed",
      "value": 49.99,
      "user_id": "usr_alex_42",
      "environment": "production"
    }
  }'
```

### 3. A/B Experiment Statistical Significance Report (`GET /api/v1/experiments/:key`)
```bash
curl "https://flagura.dev/api/v1/experiments/checkout-v2?metric=purchase_completed&control=control"
```

### 4. Progressive Canary Auto-Ramp (`POST /api/v1/flags/:key/canary`)
```bash
curl -X POST "https://flagura.dev/api/v1/flags/search-v2/canary" \
  -H "Content-Type: application/json" \
  -H "Cookie: flagura_session=..." \
  -d '{
    "stages": [
      { "index": 0, "target_percentage": 5, "duration_sec": 300 },
      { "index": 1, "target_percentage": 25, "duration_sec": 1800 },
      { "index": 2, "target_percentage": 50, "duration_sec": 3600 },
      { "index": 3, "target_percentage": 100, "duration_sec": 0 }
    ],
    "guardrails": {
      "max_error_rate_pct": 1.0,
      "auto_rollback": true
    }
  }'
```

### 5. 4-Eyes Governance Change Request (`POST /api/v1/change-requests`)
```bash
curl -X POST "https://flagura.dev/api/v1/change-requests" \
  -H "Content-Type: application/json" \
  -H "Cookie: flagura_session=..." \
  -d '{
    "flag_key": "db-failover",
    "environment": "production",
    "title": "Scheduled maintenance window failover",
    "proposed_config": {
      "enabled": true,
      "strategy": "boolean",
      "percentage": 100
    }
  }'
```

### 6. Automated Webhook Kill-Switch (`POST /api/v1/webhooks/kill-switch/:key`)
```bash
curl -X POST "https://flagura.dev/api/v1/webhooks/kill-switch/ai-smart-search?env=production"
```

### 7. API Key & Service Account Management (`POST /api/v1/api-keys`, `GET /api/v1/api-keys`, `DELETE /api/v1/api-keys/:id`)

Flagura supports cryptographically secure (`2^256` bits CSPRNG) API service account tokens for microservices, SDKs, and CI/CD pipelines.

#### Create API Key:
```bash
curl -X POST "https://flagura.dev/api/v1/api-keys" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <session-or-admin-token>" \
  -d '{
    "name": "Production Kubernetes Worker",
    "role": "developer"
  }'
```

**Response:**
```json
{
  "api_key": {
    "id": "key_1788085710350701000_b3f8c993",
    "key": "flg_live_eae851c8e9b3af4cfdaa692645ac146128e81fb5c02f2854d7c9ca43f6c3b5cc",
    "key_prefix": "flg_live_eae851c8...****",
    "name": "Production Kubernetes Worker",
    "role": "developer",
    "created_by": "admin@flagura.dev",
    "created_at": "2026-08-30T16:00:00Z",
    "revoked": false
  },
  "message": "API key generated successfully. Copy this secret key now; it will not be shown again."
}
```

#### List API Keys (Tokens are redacted for security):
```bash
curl "https://flagura.dev/api/v1/api-keys" \
  -H "Authorization: Bearer <token>"
```

#### Revoke API Key:
```bash
curl -X DELETE "https://flagura.dev/api/v1/api-keys/key_1788085710350701000_b3f8c993" \
  -H "Authorization: Bearer <token>"
```

---

## 8. Layered Error Codes & Error Response Envelope

All API errors return standard HTTP status codes accompanied by a structured JSON error envelope containing unit-incrementing internal integer codes categorized by architectural layer:

```json
{
  "error": {
    "code": 1006,
    "type": "ENVIRONMENT_RESTRICTED",
    "layer": "SecurityLayer",
    "message": "API key is scoped to environment 'staging' and cannot access 'production'",
    "status": 403,
    "request_id": "req_1788156313537006000_7946c832"
  }
}
```

| Layer Range | Architectural Subsystem | Example Code | Description |
|:---|:---|:---|:---|
| **`1000–1999`** | Security, Auth & RBAC | `1001` (Unauthorized), `1006` (Environment Restricted) | Authentication, permission tokens, environment boundaries |
| **`2000–2999`** | Multi-Tenancy & Workspaces | `2001` (Org Not Found), `2003` (Project Not Found) | Tenant isolation, organization/project management |
| **`3000–3999`** | Feature Flag Engine | `3001` (Flag Not Found), `3004` (Invalid Rollout) | Flag resolution, targeting rules, rollout percentages |
| **`4000–4999`** | Governance & 4-Eyes Approvals | `4001` (CR Not Found), `4002` (Four-Eyes Violation) | Change reviews, author self-approval violations |
| **`5000–5999`** | Storage & Database Layer | `5001` (DB Connection), `5002` (DB Query Error) | SQL constraints, connection failures |
| **`6000–6999`** | Transport & Network Layer | `6002` (Circuit Breaker Open), `6003` (Rate Limit) | SSE streams, rate limiting, request validation |
| **`9000–9999`** | Internal Server Layer | `9001` (Internal Error) | Unhandled panics, system runtime exceptions |

## 9. Observability & Health Probes

Flagura provides dedicated endpoints for Kubernetes, container orchestrators, and Prometheus:

- **`GET /livez`**: Liveness probe returning `200 OK` `{ "status": "alive" }`.
- **`GET /readyz`**: Deep readiness probe testing database connection pool (`store.Ping`). Returns `503 Service Unavailable` if database is down.
- **`GET /metrics`**: Prometheus-formatted metrics (evaluation counts, latency histograms, error rates).
- **`GET /healthz`**: Summary status endpoint for internal monitoring.

---

## 10. SDK Publishing & Package Registries

For maintainers and contributors wishing to release new versions of the Flagura SDKs to public registries (**Go Modules**, **NPM**, **PyPI**, **Crates.io**), see the complete operational guide:

👉 **[SDK Release & Publishing Runbook](../runbooks/sdk-publishing.md)**


