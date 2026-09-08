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

func TestCanaryApiLifecycle(t *testing.T) {
	memStore := store.NewMemoryStore()
	flagKey := "canary-api-test"
	_, _ = memStore.SaveFlag(context.Background(), domain.FeatureFlag{
		ID:        "flag_canary_api",
		ProjectID: store.DefaultProjectID,
		Key:       flagKey,
		Name:      "Canary API Test",
		Type:      "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 0,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, "test-actor")

	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	adminUser, _ := memStore.CreateUser(context.Background(), domain.User{
		ID:           "usr_admin_test",
		Name:         "Admin Test",
		Email:        "admin@flagura.dev",
		PasswordHash: "fakehash",
		Role:         domain.RoleAdmin,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	})
	token := "canary_test_session_token"
	_ = memStore.CreateSession(context.Background(), domain.Session{
		Token:     token,
		UserID:    adminUser.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	})
	cookie := &http.Cookie{
		Name:  SessionCookieName,
		Value: token,
	}

	// 1. Submit Canary Schedule via POST /api/v1/flags/:key/canary
	canaryPayload := domain.CanarySchedule{
		Environment: domain.EnvProduction,
		Stages: []domain.CanaryStage{
			{Index: 0, TargetPercentage: 10.0, DurationSec: 60},
			{Index: 1, TargetPercentage: 50.0, DurationSec: 120},
			{Index: 2, TargetPercentage: 100.0, DurationSec: 180},
		},
		Guardrails: domain.CanaryGuardrails{
			MaxErrorRatePct: 1.5,
			AutoRollback:    true,
		},
	}

	data, _ := json.Marshal(canaryPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/flags/canary-api-test/canary", bytes.NewReader(data))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected HTTP 201 Created from POST canary, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// Verify initial stage rollout (10%) applied to store
	flag, _ := memStore.GetFlag(context.Background(), flagKey)
	if flag.Environments[domain.EnvProduction].Percentage != 10.0 {
		t.Fatalf("expected 10%% rollout, got %f", flag.Environments[domain.EnvProduction].Percentage)
	}

	// 2. Query active canary via GET /api/v1/flags/:key/canary
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/flags/canary-api-test/canary", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()

	server.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK from GET canary, got %d (body: %s)", getRec.Code, getRec.Body.String())
	}

	var sched domain.CanarySchedule
	_ = json.Unmarshal(getRec.Body.Bytes(), &sched)
	if sched.FlagKey != flagKey {
		t.Fatalf("expected flagKey %s, got %s", flagKey, sched.FlagKey)
	}

	// 3. Trigger emergency APM health rollback via POST /api/v1/flags/:key/canary/rollback
	rbPayload := map[string]string{"reason": "APM P99 latency breached 500ms threshold"}
	rbData, _ := json.Marshal(rbPayload)
	rbReq := httptest.NewRequest(http.MethodPost, "/api/v1/flags/canary-api-test/canary/rollback", bytes.NewReader(rbData))
	rbReq.AddCookie(cookie)
	rbRec := httptest.NewRecorder()

	server.ServeHTTP(rbRec, rbReq)

	if rbRec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK from canary rollback, got %d (body: %s)", rbRec.Code, rbRec.Body.String())
	}

	// Verify rollback to 0% in store
	flag, _ = memStore.GetFlag(context.Background(), flagKey)
	if flag.Environments[domain.EnvProduction].Percentage != 0.0 {
		t.Fatalf("expected 0%% rollout after rollback, got %f", flag.Environments[domain.EnvProduction].Percentage)
	}
}

func TestCanaryAuthAndWebhookSecret(t *testing.T) {
	t.Setenv("FLAGURA_WEBHOOK_SECRET", "super-secret-apm-webhook-key")

	memStore := store.NewMemoryStore()
	flagKey := "canary-auth-flag"
	_, _ = memStore.SaveFlag(context.Background(), domain.FeatureFlag{
		ID:        "flag_canary_auth",
		ProjectID: store.DefaultProjectID,
		Key:       flagKey,
		Type:      "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 25,
			},
		},
	}, "test")

	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// 1. Unauthenticated GET /canary -> 401 Unauthorized
	unauthGet := httptest.NewRequest(http.MethodGet, "/api/v1/flags/"+flagKey+"/canary", nil)
	rec1 := httptest.NewRecorder()
	server.ServeHTTP(rec1, unauthGet)
	if rec1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unauthenticated GET canary, got %d", rec1.Code)
	}

	// 2. Unauthenticated POST rollback -> 401 Unauthorized
	unauthRb := httptest.NewRequest(http.MethodPost, "/api/v1/flags/"+flagKey+"/canary/rollback", bytes.NewReader([]byte(`{"reason":"unauthed alert"}`)))
	rec2 := httptest.NewRecorder()
	server.ServeHTTP(rec2, unauthRb)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unauthenticated rollback, got %d", rec2.Code)
	}

	// Submit schedule so there is an active canary to rollback
	_, _ = server.canary.SubmitSchedule(context.Background(), domain.CanarySchedule{
		FlagKey:     flagKey,
		ProjectID:   store.DefaultProjectID,
		Environment: domain.EnvProduction,
		Stages: []domain.CanaryStage{
			{Index: 0, TargetPercentage: 25, DurationSec: 100},
		},
	})

	// 3. Webhook secret authorized POST rollback (via X-Webhook-Secret) -> 200 OK
	webhookRb := httptest.NewRequest(http.MethodPost, "/api/v1/flags/"+flagKey+"/canary/rollback", bytes.NewReader([]byte(`{"reason":"Datadog APM alert"}`)))
	webhookRb.Header.Set("X-Webhook-Secret", "super-secret-apm-webhook-key")
	rec3 := httptest.NewRecorder()
	server.ServeHTTP(rec3, webhookRb)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for webhook secret rollback, got %d (body: %s)", rec3.Code, rec3.Body.String())
	}

	// Verify rollback took effect
	flag, _ := memStore.GetFlag(context.Background(), flagKey)
	if flag.Environments[domain.EnvProduction].Percentage != 0.0 {
		t.Fatalf("expected 0%% rollout after webhook rollback, got %f", flag.Environments[domain.EnvProduction].Percentage)
	}
}
