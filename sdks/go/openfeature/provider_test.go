package openfeature

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	flagura "github.com/dhawalhost/flagura/sdks/go"
	of "github.com/open-feature/go-sdk/openfeature"
)

func TestOpenFeatureProvider(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/evaluate" {
			var req struct {
				Flags   []string        `json:"flags"`
				Context flagura.Context `json:"context"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)

			results := make(map[string]flagura.EvaluationResult)
			for _, f := range req.Flags {
				switch f {
				case "bool-flag":
					results[f] = flagura.EvaluationResult{
						FlagKey: f,
						Enabled: true,
						Reason:  "STRATEGY_BOOLEAN",
					}
				case "string-flag":
					results[f] = flagura.EvaluationResult{
						FlagKey: f,
						Enabled: true,
						Variant: "dark-mode",
						Value:   "dark-mode",
						Reason:  "MULTIVARIATE_MATCH",
					}
				case "int-flag":
					results[f] = flagura.EvaluationResult{
						FlagKey: f,
						Enabled: true,
						Value:   float64(42),
						Reason:  "STRATEGY_BOOLEAN",
					}
				case "float-flag":
					results[f] = flagura.EvaluationResult{
						FlagKey: f,
						Enabled: true,
						Value:   99.95,
						Reason:  "STRATEGY_BOOLEAN",
					}
				case "obj-flag":
					results[f] = flagura.EvaluationResult{
						FlagKey: f,
						Enabled: true,
						Value:   map[string]interface{}{"title": "Welcome", "max_items": 10},
						Reason:  "STRATEGY_BOOLEAN",
					}
				}
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(flagura.BatchEvaluationResponse{Results: results})
			return
		}
		http.NotFound(w, r)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := flagura.NewClient(server.URL, "flg_live_test_key", flagura.WithProjectID("proj_core"))
	defer client.Close()

	provider := NewProvider(client)
	if err := of.SetProviderAndWait(provider); err != nil {
		t.Fatalf("SetProviderAndWait failed: %v", err)
	}

	ofClient := of.NewClient("test-service")
	ctx := context.Background()
	evalCtx := of.NewEvaluationContext("user_abc", map[string]interface{}{
		"email": "user@example.com",
		"role":  "admin",
	})

	t.Run("Boolean evaluation", func(t *testing.T) {
		val, err := ofClient.BooleanValue(ctx, "bool-flag", false, evalCtx)
		if err != nil || !val {
			t.Fatalf("expected true, got %v (err: %v)", val, err)
		}
	})

	t.Run("String evaluation", func(t *testing.T) {
		val, err := ofClient.StringValue(ctx, "string-flag", "light-mode", evalCtx)
		if err != nil || val != "dark-mode" {
			t.Fatalf("expected 'dark-mode', got %s (err: %v)", val, err)
		}
	})

	t.Run("Int evaluation", func(t *testing.T) {
		val, err := ofClient.IntValue(ctx, "int-flag", 0, evalCtx)
		if err != nil || val != 42 {
			t.Fatalf("expected 42, got %d (err: %v)", val, err)
		}
	})

	t.Run("Float evaluation", func(t *testing.T) {
		val, err := ofClient.FloatValue(ctx, "float-flag", 0.0, evalCtx)
		if err != nil || val != 99.95 {
			t.Fatalf("expected 99.95, got %f (err: %v)", val, err)
		}
	})

	t.Run("Object evaluation", func(t *testing.T) {
		val, err := ofClient.ObjectValue(ctx, "obj-flag", nil, evalCtx)
		if err != nil || val == nil {
			t.Fatalf("expected non-nil object, got %v (err: %v)", val, err)
		}
	})

	t.Run("Non-existent flag fallback with ErrorReason", func(t *testing.T) {
		details, _ := ofClient.BooleanValueDetails(ctx, "non-existent-flag", false, evalCtx)
		if details.Value != false || details.Reason != of.ErrorReason {
			t.Fatalf("expected false with ErrorReason, got: %+v", details)
		}
	})

	t.Run("Provider lifecycle and metadata", func(t *testing.T) {
		if provider.Metadata().Name != "flagura-go-provider" {
			t.Fatalf("expected metadata name 'flagura-go-provider', got %s", provider.Metadata().Name)
		}
		if err := provider.Init(evalCtx); err != nil {
			t.Fatalf("Init failed: %v", err)
		}
		if len(provider.Hooks()) != 0 {
			t.Fatalf("expected empty hooks list")
		}
		provider.Shutdown()
	})
}

func TestOpenFeatureEventHandling(t *testing.T) {
	client := flagura.NewClient("http://localhost:8080", "key")
	defer client.Close()

	provider := NewProvider(client)
	ch := provider.EventChannel()

	go func() {
		_ = provider.Init(of.NewEvaluationContext("u1", nil))
	}()

	select {
	case ev := <-ch:
		if ev.EventType != of.ProviderReady {
			t.Fatalf("expected ProviderReady event, got: %v", ev.EventType)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for ProviderReady event")
	}
}
