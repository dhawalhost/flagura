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
		ID:        "flag_cli_test",
		ProjectID: store.DefaultProjectID,
		Key:       "cli-feature",
		Name:      "CLI Feature Flag",
		Type:      "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvStaging: {
				Enabled:    true,
				Percentage: 100,
			},
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
	projectID = store.DefaultProjectID

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

	// Test audit/scan execution
	runAudit(".", false)

	// Seed session token for API key CLI test
	sessionToken := "cli_test_session_token"
	_ = memStore.CreateSession(context.Background(), domain.Session{
		Token:     sessionToken,
		UserID:    "usr_admin_default",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		User: &domain.User{
			ID:    "usr_admin_default",
			Email: "admin@flagura.dev",
			Role:  domain.RoleAdmin,
		},
	})
	apiKey = sessionToken
	// Test API Key CLI creation and listing
	jsonOut = true
	runAPIKey("create", []string{"create"}, "Test CLI Key", "developer")
	runAPIKey("list", []string{"list"}, "", "")
	jsonOut = false

	// Test CLI runner functions
	runList()
	jsonOut = true
	runList()
	jsonOut = false

	runGet("cli-feature")
	jsonOut = true
	runGet("cli-feature")
	jsonOut = false

	runToggle("cli-feature")
	jsonOut = true
	runToggle("cli-feature")
	jsonOut = false

	runRollout("cli-feature", 75.0)
	jsonOut = true
	runRollout("cli-feature", 75.0)
	jsonOut = false

	runEvaluate("cli-feature", "usr_10", "usr_10@test.com", true)
	jsonOut = true
	runEvaluate("cli-feature", "usr_10", "usr_10@test.com", false)
	jsonOut = false

	runPromote("cli-feature", "staging", "production")
	jsonOut = true
	runPromote("cli-feature", "staging", "production")
	jsonOut = false

	runCanary("cli-feature", "10%:1m,50%:5m,100%:0s", false)
	jsonOut = true
	runCanary("cli-feature", "10%:1m,50%:5m,100%:0s", false)
	jsonOut = false

	runCanary("cli-feature", "", true)

	runChangeRequest("list", []string{"list"}, "", "")
	jsonOut = true
	runChangeRequest("list", []string{"list"}, "", "")
	jsonOut = false

	runExperiment("cli-feature", "conversion", "control")
	jsonOut = true
	runExperiment("cli-feature", "conversion", "control")
	jsonOut = false

	runHealth()
	printHelp()
}

