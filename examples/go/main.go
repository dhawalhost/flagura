package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	flagura "github.com/dhawalhost/flagura/sdks/go"
	flaguraOF "github.com/dhawalhost/flagura/sdks/go/openfeature"
	of "github.com/open-feature/go-sdk/openfeature"
)

func main() {
	endpoint := os.Getenv("FLAGURA_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:3000"
	}

	apiKey := os.Getenv("FLAGURA_API_KEY")
	if apiKey == "" {
		apiKey = "flg_live_demo_key_example"
	}

	fmt.Println("🚀 Flagura Go Integration Example (Native + OpenFeature)")
	fmt.Printf("   Endpoint: %s\n\n", endpoint)

	// =========================================================================
	// 1. Native High-Performance Flagura Client (<85ns local evaluation)
	// =========================================================================
	fmt.Println("--- 1. Native Flagura Client (In-Process Fast Path) ---")
	client := flagura.NewClient(endpoint, apiKey,
		flagura.WithLocalEvaluation(false),
	)
	defer client.Close()

	// Wait briefly for initial SSE flag sync
	time.Sleep(50 * time.Millisecond)

	userCtx := flagura.Context{
		UserID:  "usr_alex_42",
		Email:   "alex@company.com",
		Country: "US",
		Role:    "developer",
		Tier:    "pro",
	}

	result, err := client.Evaluate(context.Background(), "ai-smart-search", userCtx)
	if err != nil {
		log.Printf("Evaluation error: %v\n", err)
	} else {
		fmt.Printf("Flag: %s | Enabled: %v | Variant: %s | Latency: %d ns | Reason: %s\n",
			result.FlagKey, result.Enabled, result.Variant, result.EvaluationLatencyNs, result.Reason)
	}

	// =========================================================================
	// 2. Vendor-Agnostic CNCF OpenFeature Provider
	// =========================================================================
	fmt.Println("\n--- 2. CNCF OpenFeature Go Provider ---")

	// Wrap Flagura client in OpenFeature Provider
	provider := flaguraOF.NewProvider(client)

	// Set OpenFeature global provider and wait for readiness
	err = of.SetProviderAndWait(provider)
	if err != nil {
		log.Printf("OpenFeature provider wait: %v", err)
	}

	// Create standard OpenFeature client
	ofClient := of.NewClient("example-service")

	// Evaluate using OpenFeature EvaluationContext
	ofCtx := of.NewEvaluationContext(
		"usr_alex_42",
		map[string]interface{}{
			"email":   "alex@company.com",
			"country": "US",
			"tier":    "pro",
		},
	)

	// Boolean evaluation
	aiSearchEnabled, err := ofClient.BooleanValue(context.Background(), "ai-smart-search", false, ofCtx)
	if err != nil {
		log.Printf("OpenFeature eval error: %v\n", err)
	} else {
		fmt.Printf("OpenFeature BooleanValue('ai-smart-search'): %v\n", aiSearchEnabled)
	}

	// String / Variant evaluation with details
	variantDetails, err := ofClient.StringValueDetails(context.Background(), "ai-smart-search", "default-model", ofCtx)
	if err != nil {
		log.Printf("OpenFeature details error: %v\n", err)
	} else {
		fmt.Printf("OpenFeature StringValueDetails: Value=%s | Variant=%s | Reason=%s\n",
			variantDetails.Value, variantDetails.Variant, variantDetails.Reason)
	}

	fmt.Println("\n✅ Go Native & OpenFeature evaluations completed successfully.")
}
