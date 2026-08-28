# 💻 SDKs & REST API Developer Guide

This guide details how to integrate Flagura into your applications using the **Official Go SDK**, TypeScript / React, Python, or standard REST API endpoints.

---

## 1. Official Go SDK (`pkg/client`)

The Go client is designed for high-concurrency microservices, providing both **local in-memory synchronized evaluations** (< 1 µs) and remote API evaluations.

### Installation
```bash
go get github.com/dhawalhost/flagura/pkg/client
```

### High-Performance Local Evaluation (Recommended)
```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/dhawalhost/flagura/pkg/client"
)

func main() {
	// Initialize client with local evaluation enabled
	c := client.New("https://flagura.dhawalhost.com",
		client.WithLocalEvaluation(true),
		client.WithSyncInterval(30*time.Second),
		client.WithAPIKey("your-api-key"), // optional
	)
	defer c.Close()

	// Evaluate flag locally in ~400 nanoseconds
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

## 3. Python (FastAPI, Django, AI Agents)

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

# Usage
if evaluate_flag("ai-smart-search", user_id="usr_dhawal_01", email="dhawal@flagura.dev"):
    print("AI Smart Search enabled")
```

---

## 4. REST API Reference

### Evaluate Flags (`POST /api/v1/evaluate`)
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

**Response (`200 OK`)**:
```json
{
  "results": {
    "ai-smart-search": {
      "flag_key": "ai-smart-search",
      "enabled": true,
      "variant": "treatment",
      "value": true,
      "reason": "TARGETING_RULE_MATCH",
      "matched_rule_id": "rule_staff_domain",
      "matched_rule_name": "Staff & Internal Testing Domain",
      "evaluation_latency_ns": 4152,
      "evaluation_latency_us": 4.152
    }
  },
  "total_flags": 1,
  "environment": "production"
}
```
