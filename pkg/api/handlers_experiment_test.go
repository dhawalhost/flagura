package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestExperimentIngestAndReportFlow(t *testing.T) {
	memStore := store.NewMemoryStore()
	_, _ = memStore.SaveFlag(context.Background(), domain.FeatureFlag{
		ID:   "flag_exp_test",
		Key:  "pricing-experiment",
		Name: "Pricing Tier Experiment",
		Type: "string",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:        true,
				Strategy:       domain.StrategyPercentage,
				Percentage:     50,
				DefaultVariant: "tier_b",
				OffVariant:     "tier_a",
				Variants: []domain.FlagVariant{
					{Key: "tier_a", Name: "Control ($29)", Value: "tier_a", Weight: 50},
					{Key: "tier_b", Name: "Treatment ($39)", Value: "tier_b", Weight: 50},
				},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, "test-actor")

	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Create user and auth cookie
	user, _ := memStore.CreateUser(context.Background(), domain.User{
		Email: "analyst@company.com",
		Role:  domain.RoleDeveloper,
	})
	token := "analyst_session_token_123"
	_ = memStore.CreateSession(context.Background(), domain.Session{
		Token:     token,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// 1. Ingest batch of conversion events
	eventsPayload := map[string]interface{}{
		"events": []map[string]interface{}{
			{
				"flag_key":    "pricing-experiment",
				"variant":     "tier_a",
				"metric_name": "checkout_completed",
				"value":       1.0,
				"user_id":     "usr_101",
				"environment": "production",
			},
			{
				"flag_key":    "pricing-experiment",
				"variant":     "tier_b",
				"metric_name": "checkout_completed",
				"value":       1.0,
				"user_id":     "usr_102",
				"environment": "production",
			},
		},
	}

	data, _ := json.Marshal(eventsPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(data))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected HTTP 202 Accepted from /api/v1/events, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// 2. Query statistical report via GET /api/v1/experiments/pricing-experiment
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/experiments/pricing-experiment?metric=checkout_completed&control=tier_a", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()

	server.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK from /api/v1/experiments, got %d (body: %s)", getRec.Code, getRec.Body.String())
	}

	var report domain.ExperimentReport
	if err := json.Unmarshal(getRec.Body.Bytes(), &report); err != nil {
		t.Fatalf("failed to parse experiment report: %v", err)
	}

	if report.FlagKey != "pricing-experiment" {
		t.Errorf("expected flag_key 'pricing-experiment', got %q", report.FlagKey)
	}
	if report.MetricName != "checkout_completed" {
		t.Errorf("expected metric 'checkout_completed', got %q", report.MetricName)
	}
	if report.TotalEvents != 2 {
		t.Errorf("expected total events 2, got %d", report.TotalEvents)
	}
}
