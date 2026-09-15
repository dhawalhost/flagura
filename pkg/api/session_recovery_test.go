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

func TestSessionRecoveryAndFlagDeletionByDeveloper(t *testing.T) {
	memStore := store.NewMemoryStore()
	srv, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("Failed to initialize server: %v", err)
	}

	ctx := context.Background()

	// 1. Create a non-admin developer user
	devUser := domain.User{
		ID:        "usr_dev_123",
		Email:     "developer@example.com",
		Name:      "Dev User",
		Role:      domain.RoleDeveloper,
		CreatedAt: time.Now(),
	}
	_, err = memStore.CreateUser(ctx, devUser)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// 2. Create organization and project for this developer
	org := domain.NewOrganization("Dev Org", "dev-org", "Dev Org")
	org.ID = "org_dev_123"
	_, _ = memStore.CreateOrganization(ctx, org)
	_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{
		OrganizationID: org.ID,
		UserID:         devUser.ID,
		Role:           "owner",
	})

	proj := domain.NewProject(org.ID, "Dev Project", "dev-proj", "Dev Project")
	proj.ID = "proj_dev_123"
	_, _ = memStore.CreateProject(ctx, proj)

	// 3. Create active session for developer
	token, _ := generateSessionToken()
	sess := domain.Session{
		Token:     token,
		UserID:    devUser.ID,
		User:      &devUser,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	_ = memStore.CreateSession(ctx, sess)

	sessionCookie := &http.Cookie{
		Name:     domain.CookieSessionName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
	}

	// 4. Create flag in dev's project
	flag := domain.FeatureFlag{
		ID:        "flag_test_key",
		ProjectID: proj.ID,
		Key:       "test-developer-flag",
		Name:      "Test Developer Flag",
		Type:      "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: true},
		},
	}
	_, err = memStore.SaveFlag(ctx, flag, devUser.Email)
	if err != nil {
		t.Fatalf("Failed to save flag: %v", err)
	}

	// 5. Test Developer deleting flag with their valid project cookie
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/flags/test-developer-flag", nil)
	delReq.AddCookie(sessionCookie)
	delReq.AddCookie(&http.Cookie{Name: domain.CookieProjectName, Value: proj.ID})
	delRec := httptest.NewRecorder()
	srv.ServeHTTP(delRec, delReq)

	if delRec.Code != http.StatusOK {
		t.Fatalf("Expected developer to be able to delete their flag (200 OK), got %d: %s", delRec.Code, delRec.Body.String())
	}

	// 6. Test Stale Cookie Fallback:
	// If the browser sends a stale/alien project cookie that dev user doesn't own,
	// API calls like /api/v1/evaluate, /api/v1/benchmark, and /api/v1/api-keys
	// must NOT return 401 project_access_denied! They should fall back to proj_dev_123.
	staleCookie := &http.Cookie{Name: domain.CookieProjectName, Value: "proj_stale_alien"}

	// Test Evaluate
	evalBody, _ := json.Marshal(map[string]interface{}{
		"flags": []string{"test-developer-flag"},
		"context": map[string]interface{}{
			"user_id":     "u1",
			"environment": "production",
		},
	})
	evalReq := httptest.NewRequest(http.MethodPost, "/api/v1/evaluate", bytes.NewReader(evalBody))
	evalReq.Header.Set("Content-Type", "application/json")
	evalReq.AddCookie(sessionCookie)
	evalReq.AddCookie(staleCookie)
	evalRec := httptest.NewRecorder()
	srv.ServeHTTP(evalRec, evalReq)

	if evalRec.Code != http.StatusOK {
		t.Fatalf("Expected evaluate to fall back to user's authorized project, got %d: %s", evalRec.Code, evalRec.Body.String())
	}

	// Test Benchmark
	benchBody, _ := json.Marshal(map[string]interface{}{
		"iterations":  100,
		"environment": "production",
	})
	benchReq := httptest.NewRequest(http.MethodPost, "/api/v1/benchmark", bytes.NewReader(benchBody))
	benchReq.Header.Set("Content-Type", "application/json")
	benchReq.AddCookie(sessionCookie)
	benchReq.AddCookie(staleCookie)
	benchRec := httptest.NewRecorder()
	srv.ServeHTTP(benchRec, benchReq)

	if benchRec.Code != http.StatusOK {
		t.Fatalf("Expected benchmark to fall back to user's authorized project, got %d: %s", benchRec.Code, benchRec.Body.String())
	}

	// Test API Keys listing
	keysReq := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil)
	keysReq.AddCookie(sessionCookie)
	keysReq.AddCookie(staleCookie)
	keysRec := httptest.NewRecorder()
	srv.ServeHTTP(keysRec, keysReq)

	if keysRec.Code != http.StatusOK {
		t.Fatalf("Expected api-keys list to fall back to user's authorized project, got %d: %s", keysRec.Code, keysRec.Body.String())
	}

	// 7. Test Telemetry Stats routing with and without trailing slash
	statsReq1 := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/stats", nil)
	statsReq1.AddCookie(sessionCookie)
	statsRec1 := httptest.NewRecorder()
	srv.ServeHTTP(statsRec1, statsReq1)
	if statsRec1.Code != http.StatusOK {
		t.Fatalf("Expected /api/v1/telemetry/stats to return 200, got %d", statsRec1.Code)
	}

	statsReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/stats/", nil)
	statsReq2.AddCookie(sessionCookie)
	statsRec2 := httptest.NewRecorder()
	srv.ServeHTTP(statsRec2, statsReq2)
	if statsRec2.Code != http.StatusOK {
		t.Fatalf("Expected /api/v1/telemetry/stats/ to return 200, got %d", statsRec2.Code)
	}
}
