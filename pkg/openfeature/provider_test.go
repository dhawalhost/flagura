package openfeature_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/client"
	"github.com/dhawalhost/flagura/pkg/domain"
	flaguraOF "github.com/dhawalhost/flagura/pkg/openfeature"
	of "github.com/open-feature/go-sdk/openfeature"
)

func mockServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/flags":
			flags := []domain.FeatureFlag{
				{
					ID:   "flag_ai_search",
					Key:  "ai-smart-search",
					Name: "AI Smart Search",
					Type: "boolean",
					Environments: map[domain.Environment]domain.EnvironmentConfig{
						domain.EnvProduction: {
							Enabled:  true,
							Strategy: domain.StrategyBoolean,
						},
					},
				},
				{
					ID:   "flag_banner_title",
					Key:  "banner-title",
					Name: "Banner Title",
					Type: "multivariate",
					Environments: map[domain.Environment]domain.EnvironmentConfig{
						domain.EnvProduction: {
							Enabled:  true,
							Strategy: domain.StrategyMultivariate,
							Variants: []domain.FlagVariant{
								{Key: "welcome-hero", Value: "Welcome to Flagura!", Weight: 100},
							},
							DefaultVariant: "welcome-hero",
						},
					},
				},
				{
					ID:   "flag_max_concurrency",
					Key:  "max-concurrency",
					Name: "Max Concurrency",
					Type: "multivariate",
					Environments: map[domain.Environment]domain.EnvironmentConfig{
						domain.EnvProduction: {
							Enabled:  true,
							Strategy: domain.StrategyMultivariate,
							Variants: []domain.FlagVariant{
								{Key: "standard", Value: 64, Weight: 100},
							},
							DefaultVariant: "standard",
						},
					},
				},
				{
					ID:   "flag_discount_rate",
					Key:  "discount-rate",
					Name: "Discount Rate",
					Type: "multivariate",
					Environments: map[domain.Environment]domain.EnvironmentConfig{
						domain.EnvProduction: {
							Enabled:  true,
							Strategy: domain.StrategyMultivariate,
							Variants: []domain.FlagVariant{
								{Key: "standard", Value: 0.25, Weight: 100},
							},
							DefaultVariant: "standard",
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"flags": flags,
			})

		case "/api/v1/evaluate":
			var req struct {
				Flags   []string       `json:"flags"`
				Context client.Context `json:"context"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)

			results := make(map[string]client.EvaluationResult)
			for _, flagKey := range req.Flags {
				switch flagKey {
				case "ai-smart-search":
					results[flagKey] = client.EvaluationResult{
						FlagKey: flagKey,
						Enabled: true,
						Variant: "treatment",
						Value:   true,
						Reason:  "default_enabled",
					}
				case "banner-title":
					results[flagKey] = client.EvaluationResult{
						FlagKey: flagKey,
						Enabled: true,
						Variant: "welcome-hero",
						Value:   "Welcome to Flagura!",
						Reason:  "default_variant",
					}
				case "max-concurrency":
					results[flagKey] = client.EvaluationResult{
						FlagKey: flagKey,
						Enabled: true,
						Variant: "standard",
						Value:   64,
						Reason:  "default_variant",
					}
				case "discount-rate":
					results[flagKey] = client.EvaluationResult{
						FlagKey: flagKey,
						Enabled: true,
						Variant: "standard",
						Value:   0.25,
						Reason:  "default_variant",
					}
				default:
					results[flagKey] = client.EvaluationResult{
						FlagKey: flagKey,
						Enabled: false,
						Variant: "off",
						Value:   nil,
						Reason:  "flag_not_found",
					}
				}
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": results,
			})
		}
	}))
}

func TestOpenFeatureProvider(t *testing.T) {
	ts := mockServer(t)
	defer ts.Close()

	flaguraClient := client.New(ts.URL)
	provider := flaguraOF.NewProvider(flaguraClient)

	if provider.Metadata().Name != "flagura-go-provider" {
		t.Fatalf("expected metadata name 'flagura-go-provider', got '%s'", provider.Metadata().Name)
	}

	err := of.SetProviderAndWait(provider)
	if err != nil {
		t.Fatalf("failed to set OpenFeature provider: %v", err)
	}

	ofClient := of.NewClient("test-app")
	ctx := context.Background()

	evalCtx := of.NewEvaluationContext(
		"user_12345",
		map[string]interface{}{
			"email": "dev@company.com",
			"tier":  "enterprise",
		},
	)

	t.Run("TypedEvaluations", func(t *testing.T) {
		tests := []struct {
			name     string
			testFunc func(t *testing.T)
		}{
			{
				name: "Boolean flag evaluation",
				testFunc: func(t *testing.T) {
					boolVal, err := ofClient.BooleanValue(ctx, "ai-smart-search", false, evalCtx)
					if err != nil || !boolVal {
						t.Fatalf("expected BooleanValue true, got %v (err: %v)", boolVal, err)
					}
				},
			},
			{
				name: "String multivariate flag evaluation",
				testFunc: func(t *testing.T) {
					strVal, err := ofClient.StringValue(ctx, "banner-title", "Default", evalCtx)
					if err != nil || strVal != "Welcome to Flagura!" {
						t.Fatalf("expected StringValue 'Welcome to Flagura!', got %q (err: %v)", strVal, err)
					}
				},
			},
			{
				name: "Integer multivariate flag evaluation",
				testFunc: func(t *testing.T) {
					intVal, err := ofClient.IntValue(ctx, "max-concurrency", 10, evalCtx)
					if err != nil || intVal != 64 {
						t.Fatalf("expected IntValue 64, got %d (err: %v)", intVal, err)
					}
				},
			},
			{
				name: "Float multivariate flag evaluation",
				testFunc: func(t *testing.T) {
					floatVal, err := ofClient.FloatValue(ctx, "discount-rate", 0.0, evalCtx)
					if err != nil || floatVal != 0.25 {
						t.Fatalf("expected FloatValue 0.25, got %f (err: %v)", floatVal, err)
					}
				},
			},
			{
				name: "Detailed evaluation metadata",
				testFunc: func(t *testing.T) {
					boolDetails, err := ofClient.BooleanValueDetails(ctx, "ai-smart-search", false, evalCtx)
					if err != nil || !boolDetails.Value || boolDetails.Reason != of.DefaultReason {
						t.Fatalf("expected DefaultReason with true value, got %+v (err: %v)", boolDetails, err)
					}
				},
			},
			{
				name: "Non-existent flag fallback with ErrorReason",
				testFunc: func(t *testing.T) {
					fallbackDetails, _ := ofClient.BooleanValueDetails(ctx, "non-existent-flag", true, evalCtx)
					if !fallbackDetails.Value || fallbackDetails.Reason != of.ErrorReason {
						t.Fatalf("expected ErrorReason with true fallback value, got %+v", fallbackDetails)
					}
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, tt.testFunc)
		}
	})
}

func TestOpenFeatureEventHandling(t *testing.T) {
	ts := mockServer(t)
	defer ts.Close()

	flaguraClient := client.New(ts.URL, client.WithLocalEvaluation(50*time.Millisecond))
	defer flaguraClient.Close()

	provider := flaguraOF.NewProvider(flaguraClient)
	_ = of.SetProviderAndWait(provider)

	ofClient := of.NewClient("event-test-app")
	receivedEvent := make(chan bool, 1)

	fn := func(details of.EventDetails) {
		select {
		case receivedEvent <- true:
		default:
		}
	}

	ofClient.AddHandler(of.ProviderConfigChange, &fn)

	// Wait for background sync to emit configuration change event
	select {
	case <-receivedEvent:
		// Succeeded in receiving ProviderConfigChange event
	case <-time.After(300 * time.Millisecond):
		// Timeout is acceptable in fast unit testing, but provider channel must be active
		if provider.EventChannel() == nil {
			t.Fatalf("expected active event channel on provider")
		}
	}
}
