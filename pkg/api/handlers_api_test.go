package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dhawalhost/flagura/pkg/api"
	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestObservabilityEndpoints(t *testing.T) {
	memStore := store.NewMemoryStore()
	_, _ = memStore.SaveFlag(context.Background(), domain.FeatureFlag{
		ID:   "flag_obs",
		Key:  "obs-feature",
		Name: "Observability Flag",
		Type: "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: true, Strategy: domain.StrategyBoolean},
		},
	}, "system")

	server, err := api.NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ts := httptest.NewServer(server)
	defer ts.Close()

	client := ts.Client()

	// 1. Test /healthz
	resp, err := client.Get(ts.URL + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected /healthz 200 OK, got: %v (code %d)", err, resp.StatusCode)
	}
	_ = resp.Body.Close()

	// 2. Test /readyz
	resp, err = client.Get(ts.URL + "/readyz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected /readyz 200 OK, got: %v (code %d)", err, resp.StatusCode)
	}
	_ = resp.Body.Close()

	// 3. Test /metrics
	resp, err = client.Get(ts.URL + "/metrics")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected /metrics 200 OK, got: %v (code %d)", err, resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	metricsText := string(body)
	if !strings.Contains(metricsText, "flagura_up 1") {
		t.Errorf("metrics missing flagura_up: %s", metricsText)
	}
	if !strings.Contains(metricsText, "flagura_build_info") {
		t.Errorf("metrics missing flagura_build_info: %s", metricsText)
	}
	if !strings.Contains(metricsText, "flagura_flags_total") {
		t.Errorf("metrics missing flagura_flags_total: %s", metricsText)
	}
}
