package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/client"
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
							Enabled:    true,
							Strategy:   domain.StrategyPercentage,
							Percentage: 50,
						},
					},
				},
				{
					ID:   "flag_beta_theme",
					Key:  "beta-dark-theme",
					Name: "Beta Dark Theme",
					Type: "multivariate",
					Environments: map[domain.Environment]domain.EnvironmentConfig{
						domain.EnvProduction: {
							Enabled:  true,
							Strategy: domain.StrategyMultivariate,
							Variants: []domain.FlagVariant{
								{Key: "dark-blue", Value: "dark-blue", Weight: 50},
								{Key: "dark-slate", Value: "dark-slate", Weight: 50},
							},
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
				if flagKey == "ai-smart-search" {
					results[flagKey] = client.EvaluationResult{
						FlagKey:             flagKey,
						Enabled:             true,
						Variant:             "treatment",
						Value:               true,
						Reason:              "percentage_rollout_match",
						Bucket:              42.5,
						EvaluationLatencyNs: 85,
						EvaluationLatencyUs: 0.085,
					}
				} else if flagKey == "beta-dark-theme" {
					results[flagKey] = client.EvaluationResult{
						FlagKey:             flagKey,
						Enabled:             true,
						Variant:             "dark-blue",
						Value:               "dark-blue",
						Reason:              "multivariate_variant_match",
						Bucket:              20.0,
						EvaluationLatencyNs: 90,
						EvaluationLatencyUs: 0.09,
					}
				} else {
					results[flagKey] = client.EvaluationResult{
						FlagKey: flagKey,
						Enabled: false,
						Variant: "off",
						Value:   false,
						Reason:  "flag_not_found",
					}
				}
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": results,
			})

		default:
			http.NotFound(w, r)
		}
	}))
}

func TestClientRemoteEvaluation(t *testing.T) {
	ts := mockServer(t)
	defer ts.Close()

	c := client.New(ts.URL)
	defer c.Close()

	ctx := context.Background()
	evalCtx := client.Context{
		UserID: "usr_100",
		Email:  "dhawal@flagura.dev",
	}

	// 1. IsEnabled
	if !c.IsEnabled(ctx, "ai-smart-search", evalCtx) {
		t.Errorf("expected ai-smart-search to be enabled")
	}

	if c.IsEnabled(ctx, "non-existent-flag", evalCtx) {
		t.Errorf("expected non-existent flag to be disabled")
	}

	// 2. Evaluate
	res, err := c.Evaluate(ctx, "ai-smart-search", evalCtx)
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if !res.Enabled || res.Variant != "treatment" {
		t.Errorf("unexpected evaluation result: %+v", res)
	}

	// 3. GetVariant
	variant := c.GetVariant(ctx, "beta-dark-theme", evalCtx, "default")
	if variant != "dark-blue" {
		t.Errorf("expected variant dark-blue, got: %s", variant)
	}

	fallbackVariant := c.GetVariant(ctx, "non-existent", evalCtx, "fallback-v")
	if fallbackVariant != "fallback-v" {
		t.Errorf("expected fallback variant, got: %s", fallbackVariant)
	}

	// 4. EvaluateBatch
	batch, err := c.EvaluateBatch(ctx, []string{"ai-smart-search", "beta-dark-theme"}, evalCtx)
	if err != nil {
		t.Fatalf("unexpected batch evaluate error: %v", err)
	}
	if len(batch) != 2 {
		t.Errorf("expected 2 flags in batch response, got: %d", len(batch))
	}
}

func TestClientLocalEvaluation(t *testing.T) {
	ts := mockServer(t)
	defer ts.Close()

	c := client.New(ts.URL, client.WithLocalEvaluation(100*time.Millisecond))
	defer c.Close()

	ctx := context.Background()
	evalCtx := client.Context{
		UserID: "usr_500",
		Email:  "dhawal@flagura.dev",
	}

	// Local evaluation should resolve directly from cached memory
	res, err := c.Evaluate(ctx, "ai-smart-search", evalCtx)
	if err != nil {
		t.Fatalf("unexpected local evaluate error: %v", err)
	}
	if res.FlagKey != "ai-smart-search" {
		t.Errorf("unexpected flag key in local evaluation: %s", res.FlagKey)
	}
}

func TestClientSnapshotPersistenceAndOfflineBootstrap(t *testing.T) {
	ts := mockServer(t)

	tmpFile := t.TempDir() + "/flagura_snapshot.json"

	// 1. Initial client syncs from server and saves snapshot to disk
	c1 := client.New(ts.URL, client.WithLocalEvaluation(1*time.Second), client.WithSnapshotFile(tmpFile))
	c1.Close()
	ts.Close() // Simulate server shutdown / network partition

	// 2. New client boots up pointing to dead endpoint with snapshot file
	c2 := client.New("http://127.0.0.1:54321", client.WithLocalEvaluation(1*time.Second), client.WithSnapshotFile(tmpFile))
	defer c2.Close()

	ctx := context.Background()
	evalCtx := client.Context{
		UserID: "offline_user_1",
		Email:  "user@example.com",
	}

	// 3. Evaluation must succeed locally from offline snapshot
	res, err := c2.Evaluate(ctx, "ai-smart-search", evalCtx)
	if err != nil {
		t.Fatalf("expected offline evaluation to succeed from snapshot, got error: %v", err)
	}
	if res.FlagKey != "ai-smart-search" {
		t.Fatalf("expected flag_key 'ai-smart-search', got '%s'", res.FlagKey)
	}
}
