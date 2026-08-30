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
)

func main() {
	// Initialize client with SSE streaming, offline snapshot disk cache, and circuit breaking
	c := client.New("https://flagura.dhawalhost.com",
		client.WithLocalEvaluation(30*time.Second),
		client.WithStreaming(true),                          // <5ms instant updates via SSE
		client.WithSnapshotFile("/tmp/flagura-cache.json"),  // 0ms offline cold-boot
		client.WithCircuitBreaker(5, 10*time.Second),        // 3-state failure circuit breaker
		client.WithTelemetryFlushInterval(60*time.Second),   // evaluation metrics push
		client.WithAPIKey("your-api-key"),                   // optional
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

### Boolean Evaluation Helper
```go
// Returns fallback value if flag is missing or network fails
isEnabled := c.EvaluateBool(ctx, "ai-smart-search", false, client.Context{
	UserID: "usr_123",
})
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
  endpoint = "https://flagura.dhawalhost.com"
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
    endpoint="https://flagura.dhawalhost.com",
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
await OpenFeature.setProviderAndWait(new FlaguraOpenFeatureProvider({ endpoint: "https://flagura.dhawalhost.com", enableStreaming: true }));
```

```python
# Python OpenFeature
api.set_provider(FlaguraOpenFeatureProvider(endpoint="https://flagura.dhawalhost.com", enable_streaming=True))
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
    let client = FlaguraClient::new("https://flagura.dhawalhost.com");
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

## 6. REST API Reference

### 1. Evaluate Flags with Execution Trace (`POST /api/v1/evaluate?trace=true`)
```bash
curl -X POST "https://flagura.dhawalhost.com/api/v1/evaluate?trace=true" \
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
curl -X POST "https://flagura.dhawalhost.com/api/v1/events" \
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
curl "https://flagura.dhawalhost.com/api/v1/experiments/checkout-v2?metric=purchase_completed&control=control"
```

### 4. Progressive Canary Auto-Ramp (`POST /api/v1/flags/:key/canary`)
```bash
curl -X POST "https://flagura.dhawalhost.com/api/v1/flags/search-v2/canary" \
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
curl -X POST "https://flagura.dhawalhost.com/api/v1/change-requests" \
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
curl -X POST "https://flagura.dhawalhost.com/api/v1/webhooks/kill-switch/ai-smart-search?env=production"
```
