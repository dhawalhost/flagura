# Flagura Go SDK (`sdks/go`)

The official, lightweight Go SDK and OpenFeature Provider for **Flagura** — featuring zero server-side dependencies, sub-microsecond local evaluations, real-time Server-Sent Events (SSE) streaming sync, in-memory circuit breaking, and offline snapshot resilience.

---

## Installation

### Native SDK
```bash
go get github.com/dhawalhost/flagura/sdks/go
```

### OpenFeature Provider
```bash
go get github.com/dhawalhost/flagura/sdks/go/openfeature
```

---

## Quickstart

### 1. Native Go Client

```go
package main

import (
	"context"
	"fmt"
	"time"

	flagura "github.com/dhawalhost/flagura/sdks/go"
)

func main() {
	ctx := context.Background()

	// Initialize the Flagura client
	client := flagura.NewClient(
		"https://flagura.yourdomain.com", // Base URL
		"flg_live_your_api_key",          // API Key
		flagura.WithProjectID("prod-workspace-1"),
		flagura.WithEnvironment(flagura.EnvProduction),
		flagura.WithLocalEvaluation(true),                 // Sub-microsecond local evaluation
		flagura.WithSnapshotFile("/tmp/flagura_snap.json"), // Offline bootstrap cache
	)
	defer client.Close()

	// User targeting context
	evalCtx := flagura.Context{
		UserID:  "usr_1048",
		Email:   "alex@company.com",
		Role:    "admin",
		Tier:    "enterprise",
		Country: "US",
	}

	// Evaluate boolean flag
	enabled, err := client.IsEnabled(ctx, "fast-checkout-v2", evalCtx)
	if err != nil {
		fmt.Printf("Evaluation fallback: %v\n", err)
	}
	if enabled {
		fmt.Println("🚀 Fast checkout v2 is ENABLED")
	}

	// Evaluate multivariate flag
	variant, _ := client.GetVariant(ctx, "search-algorithm", evalCtx)
	fmt.Printf("Active search variant: %s\n", variant)

	// Track custom conversion metric
	client.Track("fast-checkout-v2", "checkout_completed", 149.99, "usr_1048")
}
```

---

### 2. Standard OpenFeature Provider

Flagura is 100% compliant with the vendor-neutral **OpenFeature** standard.

```go
package main

import (
	"context"
	"fmt"

	flagura "github.com/dhawalhost/flagura/sdks/go"
	"github.com/dhawalhost/flagura/sdks/go/openfeature"
	of "github.com/open-feature/go-sdk/openfeature"
)

func main() {
	ctx := context.Background()

	// 1. Initialize native client
	flaguraClient := flagura.NewClient(
		"https://flagura.yourdomain.com",
		"flg_live_your_api_key",
		flagura.WithProjectID("prod-workspace-1"),
		flagura.WithLocalEvaluation(true),
	)

	// 2. Register Flagura as global OpenFeature provider
	provider := openfeature.NewProvider(flaguraClient)
	_ = of.SetProviderAndWait(provider)

	// 3. Create OpenFeature client
	ofClient := of.NewClient("checkout-service")

	evalCtx := of.NewEvaluationContext("usr_1048", map[string]interface{}{
		"email": "alex@company.com",
		"tier":  "enterprise",
	})

	// 4. Standard OpenFeature evaluations
	isNewUI, _ := ofClient.BooleanValue(ctx, "new-dashboard", false, evalCtx)
	theme, _ := ofClient.StringValue(ctx, "theme-color", "dark", evalCtx)

	fmt.Printf("New UI: %v, Theme: %s\n", isNewUI, theme)
}
```

---

## Configuration Options

| Option | Description | Default |
| :--- | :--- | :--- |
| `WithProjectID(id)` | Scopes evaluations to an isolated project | `""` |
| `WithEnvironment(env)` | Default targeting environment (`production`, `staging`, `development`) | `production` |
| `WithLocalEvaluation(bool)` | Enables local in-memory sub-microsecond evaluation and SSE synchronization | `false` |
| `WithSnapshotFile(path)` | Persists flag definitions to disk for offline crash resilience | `""` |
| `WithSyncInterval(duration)` | Polling refresh rate for flag definitions | `30s` |
| `WithCircuitBreaker(threshold, cooldown)` | Configures consecutive failure limit and cooldown before fast-failing | `5 failures`, `10s` |
| `WithLogger(logger)` | Custom structured logger implementation | `nopLogger` |
| `WithHTTPClient(client)` | Custom `*http.Client` with custom timeouts or transports | `5s timeout` |

---

## Testing

```bash
# Run unit and integration tests
go test -v ./...

# Run concurrency race detector
go test -race ./...
```
