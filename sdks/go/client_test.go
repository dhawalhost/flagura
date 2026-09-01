package flagura

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientRemoteEvaluation(t *testing.T) {
	var receivedProjectID string
	var receivedAuth string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedProjectID = r.Header.Get(HeaderProjectID)
		receivedAuth = r.Header.Get("Authorization")

		if r.URL.Path == "/api/v1/evaluate" {
			var req struct {
				Flags   []string `json:"flags"`
				Context Context  `json:"context"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)

			results := make(map[string]EvaluationResult)
			for _, f := range req.Flags {
				if f == "enabled-flag" {
					results[f] = EvaluationResult{
						FlagKey: f,
						Enabled: true,
						Reason:  "STRATEGY_BOOLEAN",
					}
				} else if f == "multivariate-flag" {
					results[f] = EvaluationResult{
						FlagKey: f,
						Enabled: true,
						Variant: "variant-b",
						Value:   map[string]interface{}{"cta": "Buy Now"},
						Reason:  "MULTIVARIATE_MATCH",
					}
				}
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(BatchEvaluationResponse{Results: results})
			return
		}

		http.NotFound(w, r)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	ctx := context.Background()
	c := NewClient(
		server.URL,
		"flg_live_secret_key_123",
		WithProjectID("proj_checkout_v2"),
		WithEnvironment(EnvProduction),
		WithCircuitBreaker(3, 1*time.Second),
	)
	defer c.Close()

	tests := []struct {
		name        string
		flagKey     string
		evalCtx     Context
		wantEnabled bool
		wantVariant string
		wantErr     bool
	}{
		{
			name:        "IsEnabled on existing flag",
			flagKey:     "enabled-flag",
			evalCtx:     Context{UserID: "u123"},
			wantEnabled: true,
			wantErr:     false,
		},
		{
			name:        "IsEnabled on non-existent flag",
			flagKey:     "non-existent-flag",
			evalCtx:     Context{UserID: "u123"},
			wantEnabled: false,
			wantErr:     true,
		},
		{
			name:        "GetVariant on multivariate flag",
			flagKey:     "multivariate-flag",
			evalCtx:     Context{UserID: "u456"},
			wantEnabled: true,
			wantVariant: "variant-b",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled, err := c.IsEnabled(ctx, tt.flagKey, tt.evalCtx)
			if (err != nil) != tt.wantErr {
				t.Fatalf("IsEnabled() error = %v, wantErr %v", err, tt.wantErr)
			}
			if enabled != tt.wantEnabled {
				t.Errorf("IsEnabled() = %v, want %v", enabled, tt.wantEnabled)
			}

			if tt.wantVariant != "" {
				variant, err := c.GetVariant(ctx, tt.flagKey, tt.evalCtx)
				if err != nil || variant != tt.wantVariant {
					t.Errorf("GetVariant() = %v, want %v", variant, tt.wantVariant)
				}
			}
		})
	}

	// Verify headers passed
	if receivedProjectID != "proj_checkout_v2" {
		t.Errorf("expected X-Project-ID 'proj_checkout_v2', got '%s'", receivedProjectID)
	}
	if receivedAuth != "Bearer flg_live_secret_key_123" {
		t.Errorf("expected Authorization 'Bearer flg_live_secret_key_123', got '%s'", receivedAuth)
	}
}

func TestClientLocalEvaluationAndSnapshot(t *testing.T) {
	flags := []FeatureFlag{
		{
			Key:  "local-promo-banner",
			Name: "Local Promo Banner",
			Type: FlagTypeBoolean,
			Environments: map[Environment]EnvironmentConfig{
				EnvProduction: {
					Enabled:  true,
					Strategy: StrategyBoolean,
				},
			},
		},
		{
			Key:  "local-percentage-flag",
			Name: "Local Percentage Flag",
			Type: FlagTypeBoolean,
			Environments: map[Environment]EnvironmentConfig{
				EnvProduction: {
					Enabled:    true,
					Strategy:   StrategyPercentage,
					Percentage: 50,
				},
			},
		},
		{
			Key:  "local-rules-flag",
			Name: "Local Rules Flag",
			Type: FlagTypeBoolean,
			Environments: map[Environment]EnvironmentConfig{
				EnvProduction: {
					Enabled:  true,
					Strategy: StrategyRules,
					Rules: []Rule{
						{
							ID:      "beta-users",
							Enabled: true,
							Variant: "beta-v1",
							Conditions: []RuleCondition{
								{Attribute: "tier", Operator: "EQUALS", Value: "enterprise"},
							},
						},
					},
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/flags" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"flags": flags,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	tempSnapshot := fmt.Sprintf("%s/flagura_test_snap_%d.json", os.TempDir(), time.Now().UnixNano())
	defer os.Remove(tempSnapshot)

	ctx := context.Background()
	c := NewClient(
		server.URL,
		"flg_live_test",
		WithLocalEvaluation(true),
		WithSnapshotFile(tempSnapshot),
		WithSyncInterval(100*time.Millisecond),
	)
	defer c.Close()

	// Wait for local cache sync
	time.Sleep(50 * time.Millisecond)

	// 1. Local Boolean
	enabled, err := c.IsEnabled(ctx, "local-promo-banner", Context{})
	if err != nil || !enabled {
		t.Fatalf("local boolean evaluation failed: %v", err)
	}

	// 2. Local Rules Match
	res, err := c.Evaluate(ctx, "local-rules-flag", Context{Tier: "enterprise"})
	if err != nil || !res.Enabled || res.Variant != "beta-v1" {
		t.Fatalf("local rule evaluation match failed: %+v", res)
	}

	// 3. Local Rules Non-Match
	res, err = c.Evaluate(ctx, "local-rules-flag", Context{Tier: "free"})
	if err != nil || res.Enabled {
		t.Fatalf("local rule evaluation non-match should be disabled, got: %+v", res)
	}

	// 4. Batch evaluation
	batchRes, err := c.EvaluateBatch(ctx, []string{"local-promo-banner", "local-rules-flag"}, Context{Tier: "enterprise"})
	if err != nil || len(batchRes) != 2 {
		t.Fatalf("EvaluateBatch failed: %v", err)
	}

	// 5. Track experiment conversion
	c.Track("local-promo-banner", "cta_click", 1.0, "user_123")

	// 6. Test snapshot persistence
	c.saveSnapshot(tempSnapshot)
	if _, err := os.Stat(tempSnapshot); err != nil {
		t.Fatalf("snapshot file was not created: %v", err)
	}
}

func TestClientCircuitBreaker(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx := context.Background()
	c := NewClient(
		server.URL,
		"flg_live_test",
		WithCircuitBreaker(2, 50*time.Millisecond),
		WithHTTPClient(&http.Client{Timeout: 100 * time.Millisecond}),
	)
	defer c.Close()

	// 1st failure
	_, err1 := c.Evaluate(ctx, "any-flag", Context{})
	if err1 == nil {
		t.Fatalf("expected error on 1st request")
	}

	// 2nd failure -> Trips circuit breaker to OPEN
	_, err2 := c.Evaluate(ctx, "any-flag", Context{})
	if err2 == nil {
		t.Fatalf("expected error on 2nd request")
	}

	// 3rd attempt -> Fast-fails immediately with ErrCircuitBreakerOpen
	_, err3 := c.Evaluate(ctx, "any-flag", Context{})
	if err3 != ErrCircuitBreakerOpen {
		t.Fatalf("expected ErrCircuitBreakerOpen, got: %v", err3)
	}

	// Wait for cooldown -> Transitions to HALF_OPEN
	time.Sleep(60 * time.Millisecond)
	if c.circuitBreaker.State() != StateHalfOpen {
		t.Fatalf("expected StateHalfOpen after cooldown, got: %v", c.circuitBreaker.State())
	}
}
