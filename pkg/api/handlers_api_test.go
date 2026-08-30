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
		t.Errorf("expected flagura_up metric in Prometheus output")
	}
	if !strings.Contains(metricsText, "flagura_evaluations_total") {
		t.Errorf("expected flagura_evaluations_total metric in Prometheus output")
	}
	if !strings.Contains(metricsText, "flagura_build_info") {
		t.Errorf("metrics missing flagura_build_info: %s", metricsText)
	}
	if !strings.Contains(metricsText, "flagura_flags_total") {
		t.Errorf("metrics missing flagura_flags_total: %s", metricsText)
	}
}

func TestWebhookKillSwitch(t *testing.T) {
	memStore := store.NewMemoryStore()
	_, _ = memStore.SaveFlag(context.Background(), domain.FeatureFlag{
		ID:   "flag_webhook_test",
		Key:  "webhook-target",
		Name: "Webhook Target",
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

	// 1. Unauthenticated webhook request MUST be rejected with 401 Unauthorized
	unauthResp, err := client.Post(ts.URL+"/api/v1/webhooks/kill-switch/webhook-target?env=production", "application/json", nil)
	if err != nil || unauthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unauthenticated webhook kill-switch, got: %v (code %d)", err, unauthResp.StatusCode)
	}
	unauthResp.Body.Close()

	// 2. Set webhook secret in environment and authenticate
	t.Setenv("FLAGURA_WEBHOOK_SECRET", "secret_webhook_token_123")

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/webhooks/kill-switch/webhook-target?env=production", nil)
	req.Header.Set("X-Webhook-Secret", "secret_webhook_token_123")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from authenticated webhook kill-switch, got: %v (code %d)", err, resp.StatusCode)
	}
	resp.Body.Close()

	// 3. Verify flag is disabled
	flag, err := memStore.GetFlag(context.Background(), "webhook-target")
	if err != nil {
		t.Fatalf("failed to retrieve flag: %v", err)
	}
	if flag.Environments[domain.EnvProduction].Enabled {
		t.Fatalf("expected flag to be disabled after webhook kill-switch")
	}

	// 4. Test non-existent flag with valid auth returns 404
	req404, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/webhooks/kill-switch/non-existent", nil)
	req404.Header.Set("X-Webhook-Secret", "secret_webhook_token_123")
	resp404, _ := client.Do(req404)
	if resp404.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for non-existent flag, got %d", resp404.StatusCode)
	}
	resp404.Body.Close()
}

func TestPromoteEnvironment(t *testing.T) {
	memStore := store.NewMemoryStore()
	_, _ = memStore.SaveFlag(context.Background(), domain.FeatureFlag{
		ID:   "flag_promote_test",
		Key:  "promote-target",
		Name: "Promote Target",
		Type: "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvStaging: {
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 75,
				Rules: []domain.TargetingRule{
					{
						ID:        "rule_beta",
						Name:      "Beta Testers",
						Attribute: domain.AttrTier,
						Operator:  domain.OpEquals,
						Values:    []string{"beta"},
						Action:    domain.ActionForceEnabled,
					},
				},
			},
			domain.EnvProduction: {
				Enabled:    false,
				Strategy:   domain.StrategyBoolean,
				Percentage: 0,
			},
		},
	}, "system")

	server, err := api.NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ts := httptest.NewServer(server)
	defer ts.Close()

	// 1. Sign up to get session cookie
	signUpBody := `{"name":"Admin User","email":"admin@flagura.dev","password":"Password123!","role":"admin"}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/signup", strings.NewReader(signUpBody))
	req.Header.Set("Content-Type", "application/json")
	signUpResp, err := ts.Client().Do(req)
	if err != nil || signUpResp.StatusCode != http.StatusCreated {
		t.Fatalf("signup failed: %v (code %d)", err, signUpResp.StatusCode)
	}
	cookie := signUpResp.Header.Get("Set-Cookie")
	signUpResp.Body.Close()

	// 2. Promote staging to production
	promoteReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/flags/promote-target/promote?from=staging&to=production", nil)
	promoteReq.Header.Set("Cookie", cookie)
	promoteResp, err := ts.Client().Do(promoteReq)
	if err != nil || promoteResp.StatusCode != http.StatusOK {
		t.Fatalf("promote request failed: %v (status %d)", err, promoteResp.StatusCode)
	}
	promoteResp.Body.Close()

	// 3. Verify production environment has been promoted
	flag, err := memStore.GetFlag(context.Background(), "promote-target")
	if err != nil {
		t.Fatalf("failed to get flag: %v", err)
	}

	prodConfig := flag.Environments[domain.EnvProduction]
	if !prodConfig.Enabled || prodConfig.Percentage != 75 || len(prodConfig.Rules) != 1 {
		t.Fatalf("expected production to match staging config, got enabled=%v, pct=%v, rules=%d",
			prodConfig.Enabled, prodConfig.Percentage, len(prodConfig.Rules))
	}
}
