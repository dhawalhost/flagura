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
		ID:   "flag_canary_api",
		Key:  flagKey,
		Name: "Canary API Test",
		Type: "boolean",
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
