package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
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
				Flags   []string `json:"flags"`
				Context Context  `json:"context"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)

			results := make(map[string]EvaluationResult)
			for _, flagKey := range req.Flags {
				if flagKey == "ai-smart-search" {
					results[flagKey] = EvaluationResult{
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
					results[flagKey] = EvaluationResult{
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
					results[flagKey] = EvaluationResult{
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

		case "/api/v1/events":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "recorded"})

		default:
			http.NotFound(w, r)
		}
	}))
}

func TestClientRemoteEvaluation(t *testing.T) {
	ts := mockServer(t)
	defer ts.Close()

	c := New(ts.URL)
	defer c.Close()

	ctx := context.Background()
	evalCtx := Context{
		UserID: "usr_100",
		Email:  "dhawal@flagura.dev",
	}

	t.Run("IsEnabled", func(t *testing.T) {
		tests := []struct {
			name            string
			flagKey         string
			expectedEnabled bool
		}{
			{"Existing enabled flag", "ai-smart-search", true},
			{"Non-existent flag", "non-existent-flag", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				enabled := c.IsEnabled(ctx, tt.flagKey, evalCtx)
				if enabled != tt.expectedEnabled {
					t.Errorf("IsEnabled(%q) = %v, expected %v", tt.flagKey, enabled, tt.expectedEnabled)
				}
			})
		}
	})

	t.Run("GetVariant", func(t *testing.T) {
		tests := []struct {
			name            string
			flagKey         string
			fallback        string
			expectedVariant string
		}{
			{"Multivariate matched variant", "beta-dark-theme", "default", "dark-blue"},
			{"Non-existent flag fallback", "non-existent", "fallback-v", "fallback-v"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				variant := c.GetVariant(ctx, tt.flagKey, evalCtx, tt.fallback)
				if variant != tt.expectedVariant {
					t.Errorf("GetVariant(%q) = %q, expected %q", tt.flagKey, variant, tt.expectedVariant)
				}
			})
		}
	})

	t.Run("EvaluateBatch", func(t *testing.T) {
		tests := []struct {
			name          string
			flagKeys      []string
			expectedCount int
		}{
			{"Batch evaluation of 2 flags", []string{"ai-smart-search", "beta-dark-theme"}, 2},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				batch, err := c.EvaluateBatch(ctx, tt.flagKeys, evalCtx)
				if err != nil {
					t.Fatalf("unexpected batch evaluate error: %v", err)
				}
				if len(batch) != tt.expectedCount {
					t.Errorf("expected %d flags in batch response, got %d", tt.expectedCount, len(batch))
				}
			})
		}
	})
}

func TestClientLocalEvaluation(t *testing.T) {
	ts := mockServer(t)
	defer ts.Close()

	c := New(ts.URL, WithLocalEvaluation(100*time.Millisecond))
	defer c.Close()

	ctx := context.Background()
	evalCtx := Context{
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
	c1 := New(ts.URL, WithLocalEvaluation(1*time.Second), WithSnapshotFile(tmpFile))
	c1.Close()
	ts.Close() // Simulate server shutdown / network partition

	// 2. New client boots up pointing to dead endpoint with snapshot file
	c2 := New("http://127.0.0.1:54321", WithLocalEvaluation(1*time.Second), WithSnapshotFile(tmpFile))
	defer c2.Close()

	ctx := context.Background()
	evalCtx := Context{
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

func TestClientTrackExperimentEvent(t *testing.T) {
	ts := mockServer(t)
	defer ts.Close()

	c := New(ts.URL, WithAPIKey("test-key"))
	defer c.Close()

	ctx := context.Background()
	err := c.Track(ctx, "ai-smart-search", "treatment", "checkout_completed", 1.0, "usr_101")
	if err != nil {
		t.Fatalf("expected Track to succeed, got error: %v", err)
	}
}

func TestClientWithProject(t *testing.T) {
	var receivedProjectID string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedProjectID = r.Header.Get("X-Project-ID")
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/evaluate" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": map[string]EvaluationResult{
					"custom-flag": {
						FlagKey: "custom-flag",
						Enabled: true,
						Variant: "on",
					},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"flags": []domain.FeatureFlag{},
		})
	}))
	defer ts.Close()

	c := New(ts.URL, WithAPIKey("test-key"), WithProject("proj_payments_v2"))
	defer c.Close()

	ctx := context.Background()
	res, err := c.Evaluate(ctx, "custom-flag", Context{UserID: "u1"})
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if !res.Enabled {
		t.Fatalf("expected enabled flag")
	}
	if receivedProjectID != "proj_payments_v2" {
		t.Fatalf("expected X-Project-ID 'proj_payments_v2', got '%s'", receivedProjectID)
	}
}

type testLogger struct {
	debugLogs []string
	infoLogs  []string
}

func (l *testLogger) Debugf(format string, args ...interface{}) {
	l.debugLogs = append(l.debugLogs, fmt.Sprintf(format, args...))
}
func (l *testLogger) Infof(format string, args ...interface{}) {
	l.infoLogs = append(l.infoLogs, fmt.Sprintf(format, args...))
}
func (l *testLogger) Warnf(format string, args ...interface{}) {}
func (l *testLogger) Errorf(format string, args ...interface{}) {}

func TestSDKFunctionalOptionsAndLogger(t *testing.T) {
	ts := mockServer(t)
	defer ts.Close()

	logger := &testLogger{}
	c := New(
		ts.URL,
		WithAPIKey("test-api-key"),
		WithProject("proj_custom_test"),
		WithEnvironment(domain.EnvStaging),
		WithLogger(logger),
	)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := c.Evaluate(ctx, "ai-smart-search", Context{UserID: "usr_logger_test"})
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if !res.Enabled {
		t.Errorf("expected flag to be enabled")
	}
}

func TestSDKAllOptionsAndBatchLocalEvaluation(t *testing.T) {
	ts := mockServer(t)
	defer ts.Close()

	customHTTP := &http.Client{Timeout: 5 * time.Second}
	var updateNotified bool

	c := New(
		ts.URL,
		WithHTTPClient(customHTTP),
		WithDisabledCircuitBreaker(),
		WithDisabledTelemetry(),
		WithTelemetryFlushInterval(10*time.Millisecond),
		WithLocalEvaluation(50*time.Millisecond),
	)
	defer c.Close()

	c.RegisterUpdateListener(func(flags map[string]domain.FeatureFlag, changedKeys []string) {
		updateNotified = true
	})

	if c.CircuitBreakerState() != StateClosed {
		t.Errorf("expected circuit breaker state CLOSED, got %s", c.CircuitBreakerState())
	}

	// Wait for local cache sync
	time.Sleep(100 * time.Millisecond)

	// Test EvaluateBatch using local evaluator
	ctx := context.Background()
	results, err := c.EvaluateBatch(ctx, []string{"ai-smart-search", "beta-dark-theme", "missing-flag"}, Context{UserID: "usr_batch_01"})
	if err != nil {
		t.Fatalf("unexpected EvaluateBatch error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	if !results["ai-smart-search"].Enabled {
		t.Errorf("expected ai-smart-search enabled")
	}
	if results["missing-flag"].Enabled {
		t.Errorf("expected missing-flag disabled")
	}

	// Test Track
	if err := c.Track(ctx, "ai-smart-search", "treatment", "signup_event", 1.0, "usr_01"); err != nil {
		t.Errorf("expected nil error on Track, got %v", err)
	}

	// Test nopLogger methods
	nopLog := &nopLogger{}
	nopLog.Debugf("debug %s", "msg")
	nopLog.Infof("info %s", "msg")
	nopLog.Warnf("warn %s", "msg")
	nopLog.Errorf("error %s", "msg")

	_ = updateNotified
}

