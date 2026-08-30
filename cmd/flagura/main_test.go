package main

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/api"
	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestCLIApiInteraction(t *testing.T) {
	memStore := store.NewMemoryStore()
	_, _ = memStore.SaveFlag(context.Background(), domain.FeatureFlag{
		ID:   "flag_cli_test",
		Key:  "cli-feature",
		Name: "CLI Feature Flag",
		Type: "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 50,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, "test")

	server, err := api.NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ts := httptest.NewServer(server)
	defer ts.Close()

	endpoint = ts.URL
	env = "production"

	// Test health request
	resp, body, err := makeRequest("GET", "/healthz", nil)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("expected 200 OK from healthz, got %d (err %v)", resp.StatusCode, err)
	}
	if len(body) == 0 {
		t.Fatalf("expected non-empty response body")
	}

	// Test evaluate request with trace
	evalPayload := map[string]interface{}{
		"flags": []string{"cli-feature"},
		"context": map[string]interface{}{
			"user_id":     "usr_cli_tester",
			"environment": "production",
		},
		"trace": true,
	}
	resp, evalBody, err := makeRequest("POST", "/api/v1/evaluate?trace=true", evalPayload)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("expected 200 OK from evaluate, got %d (err %v)", resp.StatusCode, err)
	}
	if len(evalBody) == 0 {
		t.Fatalf("expected non-empty evaluation body")
	}
}
