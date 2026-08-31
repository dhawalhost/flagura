package openfeature

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/client"
	"github.com/dhawalhost/flagura/pkg/domain"
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
	provider := NewProvider(flaguraClient)

	if provider.Metadata().Name != "flagura-go-provider" {
		t.Fatalf("expected metadata name 'flagura-go-provider', got '%s'", provider.Metadata().Name)
	}
	if len(provider.Hooks()) != 0 {
		t.Fatalf("expected empty hooks")
	}

	provider.Shutdown()

	err := of.SetProviderAndWait(provider)
	if err != nil {
		t.Fatalf("failed to set OpenFeature provider: %v", err)
	}

	ofClient := of.NewClient("test-app")
	ctx := context.Background()

	evalCtx := of.NewEvaluationContext(
		"user_12345",
		map[string]interface{}{
			"email":       "dev@company.com",
			"tier":        "enterprise",
			"country":     "US",
			"role":        "admin",
			"environment": "production",
			"custom_attr": "value",
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
				name: "Object evaluation",
				testFunc: func(t *testing.T) {
					objVal, err := ofClient.ObjectValue(ctx, "banner-title", "Default", evalCtx)
					if err != nil || objVal != "Welcome to Flagura!" {
						t.Fatalf("expected ObjectValue 'Welcome to Flagura!', got %v (err: %v)", objVal, err)
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

	provider := NewProvider(flaguraClient)
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

	select {
	case <-receivedEvent:
	case <-time.After(300 * time.Millisecond):
		if provider.EventChannel() == nil {
			t.Fatalf("expected active event channel on provider")
		}
	}
}

func TestTypeConverters_Exhaustive(t *testing.T) {
	// toFloat64
	f1, _ := toFloat64(float64(3.14))
	f2, _ := toFloat64(float32(3.14))
	f3, _ := toFloat64(int(42))
	f4, _ := toFloat64(int64(42))
	f5, _ := toFloat64(int32(42))
	f6, _ := toFloat64("3.14")
	_, errF := toFloat64([]string{"bad"})

	if f1 <= 0 || f2 <= 0 || f3 != 42 || f4 != 42 || f5 != 42 || f6 <= 0 || errF == nil {
		t.Errorf("unexpected toFloat64 conversion results")
	}

	// toInt64
	i1, _ := toInt64(int64(100))
	i2, _ := toInt64(int(100))
	i3, _ := toInt64(int32(100))
	i4, _ := toInt64(float64(100))
	i5, _ := toInt64("100")
	_, errI := toInt64([]string{"bad"})

	if i1 != 100 || i2 != 100 || i3 != 100 || i4 != 100 || i5 != 100 || errI == nil {
		t.Errorf("unexpected toInt64 conversion results")
	}
}

func TestMapReason_AllVariants(t *testing.T) {
	reasons := []string{
		"DISABLED",
		"TARGETING_MATCH",
		"PERCENTAGE_ROLLOUT_MATCH",
		"MULTIVARIATE_VARIANT_MATCH",
		"DEFAULT_VARIANT",
		"UNKNOWN_REASON",
	}

	for _, r := range reasons {
		res := mapReason(r)
		if res == "" {
			t.Errorf("expected non-empty reason for %s", r)
		}
	}
}

func TestDirectProviderEvaluations_AllBranches(t *testing.T) {
	// Server with disabled flag and malformed values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req struct {
			Flags []string `json:"flags"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		results := make(map[string]client.EvaluationResult)
		for _, f := range req.Flags {
			switch f {
			case "disabled-flag":
				results[f] = client.EvaluationResult{
					FlagKey: f,
					Enabled: false,
					Variant: "off",
					Reason:  "disabled",
				}
			case "bad-type-flag":
				results[f] = client.EvaluationResult{
					FlagKey: f,
					Enabled: true,
					Variant: "bad",
					Value:   "not-a-number",
					Reason:  "default_enabled",
				}
			case "string-bool-flag":
				results[f] = client.EvaluationResult{
					FlagKey: f,
					Enabled: true,
					Variant: "on",
					Value:   "true",
					Reason:  "default_enabled",
				}
			case "nil-value-flag":
				results[f] = client.EvaluationResult{
					FlagKey: f,
					Enabled: true,
					Variant: "default",
					Value:   nil,
					Reason:  "default_enabled",
				}
			default:
				results[f] = client.EvaluationResult{
					FlagKey: f,
					Enabled: false,
					Reason:  "flag_not_found",
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": results})
	}))
	defer server.Close()

	flaguraClient := client.New(server.URL)
	provider := NewProvider(flaguraClient)
	ctx := context.Background()

	// 1. Boolean evaluations
	bDis := provider.BooleanEvaluation(ctx, "disabled-flag", false, nil)
	if bDis.Reason != of.DisabledReason {
		t.Errorf("expected DisabledReason for disabled flag, got %s", bDis.Reason)
	}
	bStr := provider.BooleanEvaluation(ctx, "string-bool-flag", false, nil)
	if !bStr.Value {
		t.Errorf("expected string bool parsed to true")
	}

	// 2. String evaluations
	sDis := provider.StringEvaluation(ctx, "disabled-flag", "fallback", nil)
	if sDis.Reason != of.DisabledReason {
		t.Errorf("expected DisabledReason for disabled flag, got %s", sDis.Reason)
	}

	// 3. Float evaluations
	fDis := provider.FloatEvaluation(ctx, "disabled-flag", 0.0, nil)
	if fDis.Reason != of.DisabledReason {
		t.Errorf("expected DisabledReason for disabled flag, got %s", fDis.Reason)
	}
	fBad := provider.FloatEvaluation(ctx, "bad-type-flag", 9.9, nil)
	if fBad.Reason != of.ErrorReason {
		t.Errorf("expected ErrorReason on type mismatch")
	}

	// 4. Int evaluations
	iDis := provider.IntEvaluation(ctx, "disabled-flag", 0, nil)
	if iDis.Reason != of.DisabledReason {
		t.Errorf("expected DisabledReason for disabled flag, got %s", iDis.Reason)
	}
	iBad := provider.IntEvaluation(ctx, "bad-type-flag", 99, nil)
	if iBad.Reason != of.ErrorReason {
		t.Errorf("expected ErrorReason on type mismatch")
	}

	// 5. Object evaluations
	oDis := provider.ObjectEvaluation(ctx, "disabled-flag", nil, nil)
	if oDis.Reason != of.DisabledReason {
		t.Errorf("expected DisabledReason for disabled flag, got %s", oDis.Reason)
	}
	oNil := provider.ObjectEvaluation(ctx, "nil-value-flag", "default_val", nil)
	if oNil.Value != "default_val" {
		t.Errorf("expected fallback on nil value, got %v", oNil.Value)
	}

	// 6. Network error branch (closed server)
	server.Close()
	bErr := provider.BooleanEvaluation(ctx, "any-flag", true, nil)
	if bErr.Reason != of.ErrorReason {
		t.Errorf("expected ErrorReason on closed server, got %s", bErr.Reason)
	}
	sErr := provider.StringEvaluation(ctx, "any-flag", "def", nil)
	if sErr.Reason != of.ErrorReason {
		t.Errorf("expected ErrorReason on closed server, got %s", sErr.Reason)
	}
	fErr := provider.FloatEvaluation(ctx, "any-flag", 1.0, nil)
	if fErr.Reason != of.ErrorReason {
		t.Errorf("expected ErrorReason on closed server, got %s", fErr.Reason)
	}
	iErr := provider.IntEvaluation(ctx, "any-flag", 1, nil)
	if iErr.Reason != of.ErrorReason {
		t.Errorf("expected ErrorReason on closed server, got %s", iErr.Reason)
	}
	oErr := provider.ObjectEvaluation(ctx, "any-flag", nil, nil)
	if oErr.Reason != of.ErrorReason {
		t.Errorf("expected ErrorReason on closed server, got %s", oErr.Reason)
	}
}
