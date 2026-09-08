package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestExperimentEvents_SecurityAndTenantIsolation(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)
	ctx := context.Background()

	// 1. Create Tenant A and Tenant B orgs and projects
	orgA, _ := memStore.CreateOrganization(ctx, domain.Organization{ID: "org-tenant-a", Name: "Tenant A"})
	projA, _ := memStore.CreateProject(ctx, domain.Project{ID: "proj-tenant-a", OrganizationID: orgA.ID, Name: "Project A"})

	orgB, _ := memStore.CreateOrganization(ctx, domain.Organization{ID: "org-tenant-b", Name: "Tenant B"})
	projB, _ := memStore.CreateProject(ctx, domain.Project{ID: "proj-tenant-b", OrganizationID: orgB.ID, Name: "Project B"})

	// Create user in Tenant A
	userA, _ := memStore.CreateUser(ctx, domain.User{
		ID:           "user-a",
		Email:        "alice@tenant-a.com",
		Role:         domain.RoleDeveloper,
		PasswordHash: "hashed",
	})
	_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{
		OrganizationID: orgA.ID,
		UserID:         userA.ID,
		Role:           string(domain.RoleDeveloper),
	})

	sessA := domain.Session{
		Token:     "sess_token_alice_tenant_a",
		UserID:    userA.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}
	_ = memStore.CreateSession(ctx, sessA)
	cookieA := &http.Cookie{Name: SessionCookieName, Value: sessA.Token, Path: "/"}

	// Create flag in Project A and Project B with the same key
	flagKey := "search-algo-v2"
	_, _ = memStore.SaveFlag(ctx, domain.FeatureFlag{
		ID:        "flag-a",
		ProjectID: projA.ID,
		Key:       flagKey,
		Name:      "Search V2 in Project A",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: true, Strategy: domain.StrategyPercentage, Percentage: 50, OffVariant: "control"},
		},
	}, "test-setup")
	_, _ = memStore.SaveFlag(ctx, domain.FeatureFlag{
		ID:        "flag-b",
		ProjectID: projB.ID,
		Key:       flagKey,
		Name:      "Search V2 in Project B",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: true, Strategy: domain.StrategyPercentage, Percentage: 50, OffVariant: "control"},
		},
	}, "test-setup")

	// 2. Unauthenticated event ingestion MUST be rejected (401/403)
	payload := map[string]interface{}{
		"events": []map[string]interface{}{
			{
				"flag_key":    flagKey,
				"variant":     "treatment",
				"metric_name": "checkout_success",
				"event_type":  "conversion",
				"value":       1.0,
			},
		},
	}
	b, _ := json.Marshal(payload)
	reqUnauth := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(b))
	reqUnauth.Header.Set("Content-Type", "application/json")
	wUnauth := httptest.NewRecorder()
	server.ServeHTTP(wUnauth, reqUnauth)

	if wUnauth.Code != http.StatusUnauthorized && wUnauth.Code != http.StatusForbidden && wUnauth.Code != http.StatusBadRequest {
		t.Fatalf("Expected 401/403/400 for unauthenticated /api/v1/events, got %d", wUnauth.Code)
	}

	// 3. Spoofed project_id attack: Alice (Tenant A) sends events claiming project_id="proj-tenant-b"
	spoofPayload := map[string]interface{}{
		"events": []map[string]interface{}{
			{
				"project_id":  projB.ID, // Attacker tries to poison Tenant B!
				"flag_key":    flagKey,
				"variant":     "treatment",
				"metric_name": "checkout_success",
				"event_type":  "conversion",
				"value":       9999.0,
			},
		},
	}
	bSpoof, _ := json.Marshal(spoofPayload)
	reqSpoof := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(bSpoof))
	reqSpoof.Header.Set("Content-Type", "application/json")
	reqSpoof.Header.Set(domain.HeaderProjectID, projA.ID) // Scoped to Tenant A
	reqSpoof.AddCookie(cookieA)
	wSpoof := httptest.NewRecorder()
	server.ServeHTTP(wSpoof, reqSpoof)

	if wSpoof.Code != http.StatusOK && wSpoof.Code != http.StatusAccepted {
		t.Fatalf("Expected 200/202 for authorized event ingestion, got %d (%s)", wSpoof.Code, wSpoof.Body.String())
	}

	// Verify in store: the event MUST have been forced to projA.ID, NOT projB.ID
	eventsB, err := memStore.GetExperimentEventsByProject(ctx, projB.ID, flagKey, 10)
	if err != nil {
		t.Fatalf("GetExperimentEventsByProject failed: %v", err)
	}
	if len(eventsB) != 0 {
		t.Fatalf("SECURITY VIOLATION: Tenant B experiment events were poisoned! Found: %+v", eventsB)
	}

	eventsA, err := memStore.GetExperimentEventsByProject(ctx, projA.ID, flagKey, 10)
	if err != nil {
		t.Fatalf("GetExperimentEventsByProject failed: %v", err)
	}
	if len(eventsA) != 1 {
		t.Fatalf("Expected 1 event stored in Tenant A, got %d", len(eventsA))
	}
	if eventsA[0].ProjectID != projA.ID {
		t.Fatalf("Expected event ProjectID to be forced to Tenant A (%s), got: %s", projA.ID, eventsA[0].ProjectID)
	}

	// 4. Test handleGetExperimentReport multi-tenant isolation
	// Alice requests report for Project A -> should succeed
	reqRepA := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/experiments/%s?metric=checkout_success", flagKey), nil)
	reqRepA.Header.Set(domain.HeaderProjectID, projA.ID)
	reqRepA.AddCookie(cookieA)
	wRepA := httptest.NewRecorder()
	server.ServeHTTP(wRepA, reqRepA)

	if wRepA.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on GET /api/v1/experiments for authorized project, got %d (%s)", wRepA.Code, wRepA.Body.String())
	}

	// Alice tries to request report for Project B -> should be rejected with 403 Forbidden
	reqRepB := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/experiments/%s?metric=checkout_success", flagKey), nil)
	reqRepB.Header.Set(domain.HeaderProjectID, projB.ID)
	reqRepB.AddCookie(cookieA)
	wRepB := httptest.NewRecorder()
	server.ServeHTTP(wRepB, reqRepB)

	if wRepB.Code != http.StatusForbidden {
		t.Fatalf("SECURITY VIOLATION: Alice accessed Tenant B's experiment report! Expected 403 Forbidden, got %d", wRepB.Code)
	}
}

func TestWebhookKillSwitch_ProjectScopedAndAuthorized(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)
	ctx := context.Background()

	// Create Tenant A and Tenant B
	orgA, _ := memStore.CreateOrganization(ctx, domain.Organization{ID: "org-kill-a", Name: "Org A"})
	projA, _ := memStore.CreateProject(ctx, domain.Project{ID: "proj-kill-a", OrganizationID: orgA.ID, Name: "Proj A"})

	orgB, _ := memStore.CreateOrganization(ctx, domain.Organization{ID: "org-kill-b", Name: "Org B"})
	projB, _ := memStore.CreateProject(ctx, domain.Project{ID: "proj-kill-b", OrganizationID: orgB.ID, Name: "Proj B"})

	// User Alice in Tenant A only
	userA, _ := memStore.CreateUser(ctx, domain.User{
		ID:           "user-kill-alice",
		Email:        "alice@kill-a.com",
		Role:         domain.RoleDeveloper,
		PasswordHash: "hashed",
	})
	_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{
		OrganizationID: orgA.ID,
		UserID:         userA.ID,
		Role:           string(domain.RoleDeveloper),
	})
	sessA := domain.Session{
		Token:     "sess_kill_alice_token",
		UserID:    userA.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}
	_ = memStore.CreateSession(ctx, sessA)
	cookieA := &http.Cookie{Name: SessionCookieName, Value: sessA.Token, Path: "/"}

	flagKey := "kill-target-flag"
	// Save flag in Tenant A (enabled)
	_, _ = memStore.SaveFlag(ctx, domain.FeatureFlag{
		ID:        "flag-kill-a",
		ProjectID: projA.ID,
		Key:       flagKey,
		Name:      "Target Flag in Tenant A",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: true, Strategy: domain.StrategyBoolean},
		},
	}, "test-setup")
	// Save same flag key in Tenant B (enabled)
	_, _ = memStore.SaveFlag(ctx, domain.FeatureFlag{
		ID:        "flag-kill-b",
		ProjectID: projB.ID,
		Key:       flagKey,
		Name:      "Target Flag in Tenant B",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: true, Strategy: domain.StrategyBoolean},
		},
	}, "test-setup")

	// 1. Alice tries to trigger kill-switch on Tenant B's flag -> MUST return 403 Forbidden
	reqAttack := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/webhooks/kill-switch/%s?env=production", flagKey), nil)
	reqAttack.Header.Set(domain.HeaderProjectID, projB.ID)
	reqAttack.AddCookie(cookieA)
	wAttack := httptest.NewRecorder()
	server.ServeHTTP(wAttack, reqAttack)

	if wAttack.Code != http.StatusForbidden {
		t.Fatalf("SECURITY VIOLATION: Alice killed Tenant B's flag! Expected 403 Forbidden, got %d", wAttack.Code)
	}

	// Verify Tenant B flag is still enabled
	flagB, _ := memStore.GetFlagByProject(ctx, projB.ID, flagKey)
	if !flagB.Environments[domain.EnvProduction].Enabled {
		t.Fatalf("Tenant B flag should NOT have been disabled by Tenant A user")
	}

	// 2. Alice triggers kill-switch on Tenant A's flag -> MUST succeed
	reqValid := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/webhooks/kill-switch/%s?env=production", flagKey), nil)
	reqValid.Header.Set(domain.HeaderProjectID, projA.ID)
	reqValid.AddCookie(cookieA)
	wValid := httptest.NewRecorder()
	server.ServeHTTP(wValid, reqValid)

	if wValid.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for authorized kill-switch, got %d (%s)", wValid.Code, wValid.Body.String())
	}

	// Verify Tenant A flag is disabled, but Tenant B flag remains enabled
	flagA, _ := memStore.GetFlagByProject(ctx, projA.ID, flagKey)
	if flagA.Environments[domain.EnvProduction].Enabled {
		t.Fatalf("Expected Tenant A flag to be disabled")
	}
	flagBAfter, _ := memStore.GetFlagByProject(ctx, projB.ID, flagKey)
	if !flagBAfter.Environments[domain.EnvProduction].Enabled {
		t.Fatalf("Tenant B flag was unintentionally modified!")
	}
}

func TestAuth_LoginSessionTokenNotInResponseBody(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)
	ctx := context.Background()

	// Register user
	signUpPayload := map[string]interface{}{
		"name":     "Privacy Tester",
		"email":    "privacy@flagura.dev",
		"password": "StrongSecretPass123!",
	}
	bSignUp, _ := json.Marshal(signUpPayload)
	reqSignUp := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewReader(bSignUp))
	reqSignUp.Header.Set("Content-Type", "application/json")
	wSignUp := httptest.NewRecorder()
	server.ServeHTTP(wSignUp, reqSignUp)

	if wSignUp.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created on signup, got %d (%s)", wSignUp.Code, wSignUp.Body.String())
	}

	// Perform login
	loginPayload := domain.LoginRequest{
		Email:    "privacy@flagura.dev",
		Password: "StrongSecretPass123!",
	}
	bLogin, _ := json.Marshal(loginPayload)
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(bLogin))
	reqLogin.Header.Set("Content-Type", "application/json")
	wLogin := httptest.NewRecorder()
	server.ServeHTTP(wLogin, reqLogin)

	if wLogin.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on login, got %d", wLogin.Code)
	}

	// Check response body: "token" must be omitted / empty
	var rawResp map[string]interface{}
	if err := json.NewDecoder(wLogin.Body).Decode(&rawResp); err != nil {
		t.Fatalf("Failed to parse login response JSON: %v", err)
	}
	if tok, exists := rawResp["token"]; exists && tok != "" && tok != nil {
		t.Fatalf("SECURITY FLAW: session token leaked in login JSON response body: %v", tok)
	}

	// Check that HttpOnly cookie was set
	cookies := wLogin.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatalf("Expected session cookie in login response")
	}
	if !sessionCookie.HttpOnly {
		t.Fatalf("Expected session cookie to have HttpOnly=true")
	}

	// Verify that the session cookie allows calling /me
	reqMe := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqMe.AddCookie(sessionCookie)
	wMe := httptest.NewRecorder()
	server.ServeHTTP(wMe, reqMe)

	if wMe.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on /me using HttpOnly session cookie, got %d", wMe.Code)
	}

	_ = ctx
}
