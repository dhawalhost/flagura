package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestSecurityHeaders(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got: %d", w.Code)
	}

	headers := w.Header()

	expectedHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "SAMEORIGIN",
		"X-XSS-Protection":       "1; mode=block",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}

	for k, expectedVal := range expectedHeaders {
		val := headers.Get(k)
		if val != expectedVal {
			t.Errorf("Expected header %q = %q, got %q", k, expectedVal, val)
		}
	}

	csp := headers.Get("Content-Security-Policy")
	if csp == "" {
		t.Fatalf("Expected Content-Security-Policy header to be set")
	}

	// Verify nonce is present in script-src
	if !strings.Contains(csp, "script-src 'self' 'nonce-") {
		t.Errorf("Expected script-src to contain 'nonce-<base64>', got: %s", csp)
	}

	// Verify unsafe-inline is removed from script-src
	scriptSrcIdx := strings.Index(csp, "script-src")
	semiIdx := strings.Index(csp[scriptSrcIdx:], ";")
	scriptDirective := csp[scriptSrcIdx : scriptSrcIdx+semiIdx]
	if strings.Contains(scriptDirective, "'unsafe-inline'") {
		t.Errorf("script-src must NOT contain 'unsafe-inline', got directive: %s", scriptDirective)
	}
	if strings.Contains(scriptDirective, "'unsafe-eval'") {
		t.Errorf("script-src must NOT contain 'unsafe-eval', got directive: %s", scriptDirective)
	}

	// Verify each request gets a distinct random nonce
	req2 := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w2 := httptest.NewRecorder()
	server.ServeHTTP(w2, req2)
	csp2 := w2.Header().Get("Content-Security-Policy")
	if csp == csp2 {
		t.Errorf("Expected per-request unique nonces in CSP headers across different requests")
	}
}

func TestUnauthenticatedMutationEndpoints(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "Read Flags Unauthenticated",
			method: http.MethodGet,
			path:   "/api/v1/flags",
			body:   "",
		},
		{
			name:   "Read Audit Logs Unauthenticated",
			method: http.MethodGet,
			path:   "/api/v1/audit-logs",
			body:   "",
		},
		{
			name:   "Create Flag Unauthenticated",
			method: http.MethodPost,
			path:   "/api/v1/flags",
			body:   `{"key":"secret-flag","name":"Secret"}`,
		},
		{
			name:   "Toggle Flag Unauthenticated",
			method: http.MethodPatch,
			path:   "/api/v1/flags/checkout_v2/toggle",
			body:   `{"environment":"production"}`,
		},
		{
			name:   "Update Rollout Unauthenticated",
			method: http.MethodPatch,
			path:   "/api/v1/flags/checkout_v2/rollout",
			body:   `{"environment":"production","percentage":50}`,
		},
		{
			name:   "Delete Flag Unauthenticated",
			method: http.MethodDelete,
			path:   "/api/v1/flags/checkout_v2",
			body:   "",
		},
		{
			name:   "Reset Database Unauthenticated",
			method: http.MethodPost,
			path:   "/api/v1/reset",
			body:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader *bytes.Reader
			if tt.body != "" {
				bodyReader = bytes.NewReader([]byte(tt.body))
			} else {
				bodyReader = bytes.NewReader(nil)
			}

			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("[%s] Expected status 401 Unauthorized, got: %d (%s)", tt.name, w.Code, w.Body.String())
			}
		})
	}
}

func TestRBACAndActorVerification(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)
	ctx := context.Background()

	_, _ = memStore.CreateOrganization(ctx, domain.Organization{
		ID:   domain.DefaultOrgID,
		Name: domain.DefaultOrgName,
		Slug: domain.DefaultOrgSlug,
	})
	_, _ = memStore.CreateProject(ctx, domain.Project{
		ID:             domain.DefaultProjectID,
		OrganizationID: domain.DefaultOrgID,
		Name:           domain.DefaultProjectName,
		Slug:           domain.DefaultProjectSlug,
	})

	// 1. Create Developer user & session
	devUser, _ := memStore.CreateUser(ctx, domain.User{
		Email: "dev@company.com",
		Name:  "Developer",
		Role:  domain.RoleDeveloper,
	})
	_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{
		OrganizationID: domain.DefaultOrgID,
		UserID:         devUser.ID,
		Role:           string(domain.RoleDeveloper),
	})
	devToken := "dev_session_token_123"
	_ = memStore.CreateSession(ctx, domain.Session{
		Token:     devToken,
		UserID:    devUser.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	// 2. Create Admin user & session
	adminUser, _ := memStore.CreateUser(ctx, domain.User{
		Email: "admin@flagura.dev",
		Name:  "Admin",
		Role:  domain.RoleAdmin,
	})
	_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{
		OrganizationID: domain.DefaultOrgID,
		UserID:         adminUser.ID,
		Role:           string(domain.RoleAdmin),
	})
	adminToken := "admin_session_token_123"
	_ = memStore.CreateSession(ctx, domain.Session{
		Token:     adminToken,
		UserID:    adminUser.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	// Developer can create flags and actor is attributed to authenticated email
	flagPayload := domain.FeatureFlag{
		ProjectID:   "proj_default",
		Key:         "guardrail_test_flag",
		Name:        "Guardrail Test Flag",
		Description: "Testing security guardrails",
		Type:        "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: true, Strategy: domain.StrategyBoolean},
		},
	}
	body, _ := json.Marshal(flagPayload)
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/flags", bytes.NewReader(body))
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate.Header.Set("Authorization", "Bearer "+devToken)
	reqCreate.Header.Set("X-Project-ID", "proj_default")
	reqCreate.Header.Set("X-Actor", "spoofed_hacker@evil.com") // Spoofed header should be ignored
	wCreate := httptest.NewRecorder()
	server.ServeHTTP(wCreate, reqCreate)

	if wCreate.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created for authenticated developer, got: %d (%s)", wCreate.Code, wCreate.Body.String())
	}

	// Verify audit log has the real authenticated user, not spoofed header
	logs, _ := memStore.ListAuditLogs(ctx, 5)
	if len(logs) == 0 {
		t.Fatalf("Expected audit log entry to be created")
	}
	if logs[0].Actor != "dev@company.com" {
		t.Fatalf("Expected audit log actor to be 'dev@company.com', got %q", logs[0].Actor)
	}

	// Developer CANNOT reset the database (RBAC: 403 Forbidden)
	reqResetDev := httptest.NewRequest(http.MethodPost, "/api/v1/reset", nil)
	reqResetDev.Header.Set("Authorization", "Bearer "+devToken)
	wResetDev := httptest.NewRecorder()
	server.ServeHTTP(wResetDev, reqResetDev)
	if wResetDev.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden when developer calls reset, got: %d", wResetDev.Code)
	}

	// Admin CAN reset the database (RBAC: 200 OK)
	reqResetAdmin := httptest.NewRequest(http.MethodPost, "/api/v1/reset", nil)
	reqResetAdmin.Header.Set("Authorization", "Bearer "+adminToken)
	wResetAdmin := httptest.NewRecorder()
	server.ServeHTTP(wResetAdmin, reqResetAdmin)
	if wResetAdmin.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK when admin calls reset, got: %d (%s)", wResetAdmin.Code, wResetAdmin.Body.String())
	}
}

func TestMaxBytesLimit(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)
	ctx := context.Background()

	adminUser, _ := memStore.CreateUser(ctx, domain.User{
		Email: "admin@flagura.dev",
		Name:  "Admin",
		Role:  domain.RoleAdmin,
	})
	token := "admin_token_large_test"
	_ = memStore.CreateSession(ctx, domain.Session{
		Token:     token,
		UserID:    adminUser.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	// Create oversized payload (> 1MB)
	largeString := strings.Repeat("A", 1024*1024+500)
	payload := map[string]string{
		"key":         "large_flag",
		"description": largeString,
	}
	data, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/flags", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	// MaxBytesReader causes JSON decoder to error out with Bad Request or 413
	if w.Code != http.StatusBadRequest && w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("Expected 400 Bad Request or 413 for oversized body, got: %d", w.Code)
	}
}

func TestTenantProjectAuthorizationEnforcement(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)
	ctx := context.Background()

	// 1. Create Tenant A (Org A + Proj A + User A)
	orgA, _ := memStore.CreateOrganization(ctx, domain.Organization{ID: "org_a", Name: "Tenant A"})
	projA, _ := memStore.CreateProject(ctx, domain.Project{ID: "proj_a", OrganizationID: orgA.ID, Name: "Project A"})
	userA, _ := memStore.CreateUser(ctx, domain.User{Email: "alice@tenanta.com", Name: "Alice", Role: domain.RoleDeveloper})
	_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{OrganizationID: orgA.ID, UserID: userA.ID, Role: string(domain.RoleDeveloper)})
	tokenA := "session_token_alice"
	_ = memStore.CreateSession(ctx, domain.Session{Token: tokenA, UserID: userA.ID, ExpiresAt: time.Now().Add(24 * time.Hour)})

	// 2. Create Tenant B (Org B + Proj B + Flag B)
	orgB, _ := memStore.CreateOrganization(ctx, domain.Organization{ID: "org_b", Name: "Tenant B"})
	projB, _ := memStore.CreateProject(ctx, domain.Project{ID: "proj_b", OrganizationID: orgB.ID, Name: "Project B"})
	_, _ = memStore.SaveFlag(ctx, domain.FeatureFlag{
		ID:        "flag_secret_b",
		ProjectID: projB.ID,
		Key:       "secret-feature-b",
		Name:      "Secret Feature B",
		Type:      "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: true, Strategy: domain.StrategyBoolean},
		},
	}, "system")

	// 3. Alice attempts to READ Tenant B's flags via X-Project-ID header -> MUST return 403 Forbidden
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/flags", nil)
	reqGet.Header.Set("Authorization", "Bearer "+tokenA)
	reqGet.Header.Set("X-Project-ID", projB.ID)
	wGet := httptest.NewRecorder()
	server.ServeHTTP(wGet, reqGet)

	if wGet.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden when Alice accesses Tenant B's flags, got: %d (%s)", wGet.Code, wGet.Body.String())
	}

	// 4. Alice attempts to TOGGLE Tenant B's flag -> MUST return 403 Forbidden
	toggleBody, _ := json.Marshal(map[string]interface{}{
		"environment": "production",
		"enabled":     false,
	})
	reqToggle := httptest.NewRequest(http.MethodPatch, "/api/v1/flags/secret-feature-b/toggle", bytes.NewReader(toggleBody))
	reqToggle.Header.Set("Content-Type", "application/json")
	reqToggle.Header.Set("Authorization", "Bearer "+tokenA)
	reqToggle.Header.Set("X-Project-ID", projB.ID)
	wToggle := httptest.NewRecorder()
	server.ServeHTTP(wToggle, reqToggle)

	if wToggle.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden when Alice toggles Tenant B's flag, got: %d (%s)", wToggle.Code, wToggle.Body.String())
	}

	// 5. Alice CAN read her own project flags
	reqGetOwn := httptest.NewRequest(http.MethodGet, "/api/v1/flags", nil)
	reqGetOwn.Header.Set("Authorization", "Bearer "+tokenA)
	reqGetOwn.Header.Set("X-Project-ID", projA.ID)
	wGetOwn := httptest.NewRecorder()
	server.ServeHTTP(wGetOwn, reqGetOwn)

	if wGetOwn.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK when Alice accesses her own project, got: %d (%s)", wGetOwn.Code, wGetOwn.Body.String())
	}
}

func TestCORSSubstringProtection(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)

	// Attacker origin that contains host as substring (e.g. host.attacker.com)
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/flags", nil)
	req.Host = "api.flagura.io"
	req.Header.Set("Origin", "https://api.flagura.io.attacker.com")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if acao := w.Header().Get("Access-Control-Allow-Origin"); acao != "" {
		t.Fatalf("Expected empty Access-Control-Allow-Origin for spoofed substring origin, got: %s", acao)
	}

	// Exact host origin should be allowed
	reqExact := httptest.NewRequest(http.MethodOptions, "/api/v1/flags", nil)
	reqExact.Host = "api.flagura.io"
	reqExact.Header.Set("Origin", "https://api.flagura.io")
	wExact := httptest.NewRecorder()
	server.ServeHTTP(wExact, reqExact)

	if acao := wExact.Header().Get("Access-Control-Allow-Origin"); acao != "https://api.flagura.io" {
		t.Fatalf("Expected Access-Control-Allow-Origin for exact host, got: %s", acao)
	}
}

// TestSSEStreamAuthenticationAndAuthorization verifies Issue 8: SSE streaming requires authentication and project authorization.
func TestSSEStreamAuthenticationAndAuthorization(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)
	ctx := context.Background()

	orgA, _ := memStore.CreateOrganization(ctx, domain.Organization{ID: "org_stream_a", Name: "Tenant Stream A"})
	projA, _ := memStore.CreateProject(ctx, domain.Project{ID: "proj_stream_a", OrganizationID: orgA.ID, Name: "Project A"})
	userA, _ := memStore.CreateUser(ctx, domain.User{Email: "alice.stream@flagura.dev", Name: "Alice"})
	_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{OrganizationID: orgA.ID, UserID: userA.ID, Role: "owner"})

	orgB, _ := memStore.CreateOrganization(ctx, domain.Organization{ID: "org_stream_b", Name: "Tenant Stream B"})
	projB, _ := memStore.CreateProject(ctx, domain.Project{ID: "proj_stream_b", OrganizationID: orgB.ID, Name: "Project B"})
	userB, _ := memStore.CreateUser(ctx, domain.User{Email: "bob.stream@flagura.dev", Name: "Bob"})
	_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{OrganizationID: orgB.ID, UserID: userB.ID, Role: "owner"})

	tokenA := "sess_stream_alice"
	_ = memStore.CreateSession(ctx, domain.Session{Token: tokenA, UserID: userA.ID, ExpiresAt: time.Now().Add(time.Hour)})

	// 1. Unauthenticated request to /api/v1/flags/stream -> MUST return 401 Unauthorized
	reqUnauth := httptest.NewRequest(http.MethodGet, "/api/v1/flags/stream?project_id="+projA.ID, nil)
	wUnauth := httptest.NewRecorder()
	server.ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized for unauthenticated stream request, got %d", wUnauth.Code)
	}

	// 2. Authenticated Alice attempts to access Tenant B's stream -> MUST return 403 Forbidden
	reqCross := httptest.NewRequest(http.MethodGet, "/api/v1/flags/stream?project_id="+projB.ID, nil)
	reqCross.Header.Set("Authorization", "Bearer "+tokenA)
	wCross := httptest.NewRecorder()
	server.ServeHTTP(wCross, reqCross)
	if wCross.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden when Alice accesses Tenant B's stream, got %d", wCross.Code)
	}

	// 3. Authenticated Alice accessing her own project stream -> 200 OK
	ctxStream, cancelStream := context.WithCancel(context.Background())
	reqOwn := httptest.NewRequest(http.MethodGet, "/api/v1/flags/stream?project_id="+projA.ID, nil).WithContext(ctxStream)
	reqOwn.Header.Set("Authorization", "Bearer "+tokenA)
	wOwn := httptest.NewRecorder()
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancelStream()
	}()
	server.ServeHTTP(wOwn, reqOwn)
	if wOwn.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK when Alice accesses her own stream, got %d", wOwn.Code)
	}
}

// TestOrgInvitationForgedAccountTakeoverPrevention verifies Issue 9:
// Non-members cannot invite to an org, non-owners cannot invite admins, and tokens cannot be accepted by unintended emails.
func TestOrgInvitationForgedAccountTakeoverPrevention(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)
	ctx := context.Background()

	orgA, _ := memStore.CreateOrganization(ctx, domain.Organization{ID: "org_victim", Name: "Victim Corp"})
	ownerA, _ := memStore.CreateUser(ctx, domain.User{Email: "victim.owner@victim.com", Name: "Victim Owner"})
	_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{OrganizationID: orgA.ID, UserID: ownerA.ID, Role: "owner"})

	// Attacker user has their own separate workspace
	orgEvil, _ := memStore.CreateOrganization(ctx, domain.Organization{ID: "org_evil", Name: "Evil Org"})
	attacker, _ := memStore.CreateUser(ctx, domain.User{Email: "attacker@evil.com", Name: "Attacker"})
	_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{OrganizationID: orgEvil.ID, UserID: attacker.ID, Role: "owner"})
	attackerToken := "sess_attacker_token"
	_ = memStore.CreateSession(ctx, domain.Session{Token: attackerToken, UserID: attacker.ID, ExpiresAt: time.Now().Add(time.Hour)})

	ownerToken := "sess_victim_owner"
	_ = memStore.CreateSession(ctx, domain.Session{Token: ownerToken, UserID: ownerA.ID, ExpiresAt: time.Now().Add(time.Hour)})

	// 1. Attacker attempts to forge an invitation into Victim Corp -> MUST return 403 Forbidden
	forgePayload, _ := json.Marshal(map[string]string{
		"organization_id": orgA.ID,
		"email":           "attacker@evil.com",
		"role":            "admin",
	})
	reqForge := httptest.NewRequest(http.MethodPost, "/api/v1/invitations", bytes.NewReader(forgePayload))
	reqForge.Header.Set("Authorization", "Bearer "+attackerToken)
	reqForge.Header.Set("Content-Type", "application/json")
	wForge := httptest.NewRecorder()
	server.ServeHTTP(wForge, reqForge)
	if wForge.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden when non-member tries to invite to org, got: %d (%s)", wForge.Code, wForge.Body.String())
	}

	// 2. Legitimate invitation created for bob@victim.com
	legitPayload, _ := json.Marshal(map[string]string{
		"organization_id": orgA.ID,
		"email":           "bob@victim.com",
		"role":            "developer",
	})
	reqLegit := httptest.NewRequest(http.MethodPost, "/api/v1/invitations", bytes.NewReader(legitPayload))
	reqLegit.Header.Set("Authorization", "Bearer "+ownerToken)
	reqLegit.Header.Set("Content-Type", "application/json")
	wLegit := httptest.NewRecorder()
	server.ServeHTTP(wLegit, reqLegit)
	if wLegit.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created for owner invitation, got %d", wLegit.Code)
	}

	var invResp struct {
		Invitation domain.OrgInvitation `json:"invitation"`
	}
	_ = json.Unmarshal(wLegit.Body.Bytes(), &invResp)
	token := invResp.Invitation.Token

	// 3. Attacker intercepts or gets the token and tries to accept it with their own account -> MUST return 403 Forbidden
	acceptPayload, _ := json.Marshal(map[string]string{
		"token": token,
	})
	reqAcceptAttacker := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/accept", bytes.NewReader(acceptPayload))
	reqAcceptAttacker.Header.Set("Authorization", "Bearer "+attackerToken)
	reqAcceptAttacker.Header.Set("Content-Type", "application/json")
	wAcceptAttacker := httptest.NewRecorder()
	server.ServeHTTP(wAcceptAttacker, reqAcceptAttacker)
	if wAcceptAttacker.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden when wrong email accepts invitation, got: %d (%s)", wAcceptAttacker.Code, wAcceptAttacker.Body.String())
	}
}

// TestAPIKeyRevokeCrossTenantPrevention verifies Issue 10:
// Users from Tenant A cannot delete or revoke API keys belonging to Tenant B.
func TestAPIKeyRevokeCrossTenantPrevention(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)
	ctx := context.Background()

	orgA, _ := memStore.CreateOrganization(ctx, domain.Organization{ID: "org_key_a", Name: "Org A"})
	projA, _ := memStore.CreateProject(ctx, domain.Project{ID: "proj_key_a", OrganizationID: orgA.ID, Name: "Proj A"})
	_ = projA
	userA, _ := memStore.CreateUser(ctx, domain.User{Email: "alice.keys@a.com", Name: "Alice"})
	_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{OrganizationID: orgA.ID, UserID: userA.ID, Role: "owner"})
	tokenA := "sess_key_alice"
	_ = memStore.CreateSession(ctx, domain.Session{Token: tokenA, UserID: userA.ID, ExpiresAt: time.Now().Add(time.Hour)})

	orgB, _ := memStore.CreateOrganization(ctx, domain.Organization{ID: "org_key_b", Name: "Org B"})
	projB, _ := memStore.CreateProject(ctx, domain.Project{ID: "proj_key_b", OrganizationID: orgB.ID, Name: "Proj B"})

	// Tenant B creates an API key
	keyB, _ := memStore.CreateAPIKey(ctx, domain.APIKey{
		ID:          "key_tenant_b_prod",
		ProjectID:   projB.ID,
		Name:        "Tenant B Production Key",
		Role:        domain.RoleDeveloper,
		Environment: "production",
	})

	// Alice (Tenant A) attempts to revoke Tenant B's API key -> MUST return 403 Forbidden
	reqRevoke := httptest.NewRequest(http.MethodDelete, "/api/v1/api-keys/"+keyB.ID, nil)
	reqRevoke.Header.Set("Authorization", "Bearer "+tokenA)
	wRevoke := httptest.NewRecorder()
	server.ServeHTTP(wRevoke, reqRevoke)

	if wRevoke.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden when Alice attempts to revoke Tenant B's key, got: %d (%s)", wRevoke.Code, wRevoke.Body.String())
	}

	// Verify key in Tenant B is still active
	keyAfter, err := memStore.GetAPIKeyByID(ctx, keyB.ID)
	if err != nil || keyAfter.Revoked {
		t.Fatalf("Tenant B key should not have been revoked: %v", keyAfter)
	}
}

// TestStrictMultiTenancyEnforcement verifies that omitting project credentials results in 400 Bad Request.
func TestStrictMultiTenancyEnforcement(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)

	// POST /api/v1/evaluate with NO API key and NO X-Project-ID -> 400 Bad Request (ErrCodeProjectRequired)
	evalPayload, _ := json.Marshal(map[string]interface{}{
		"flags": []string{"some-flag"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evaluate", bytes.NewReader(evalPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request for unauthenticated evaluate without project ID, got: %d (%s)", w.Code, w.Body.String())
	}
}

// TestFlagKeyValidation verifies OWASP A03 / Stored XSS prevention:
// Flag keys must conform strictly to ^[a-zA-Z0-9_-]{1,64}$.
func TestFlagKeyValidation(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)
	ctx := context.Background()

	org, _ := memStore.CreateOrganization(ctx, domain.Organization{ID: "org_val", Name: "Validation Org"})
	proj, _ := memStore.CreateProject(ctx, domain.Project{ID: "proj_val", OrganizationID: org.ID, Name: "Proj Val"})
	user, _ := memStore.CreateUser(ctx, domain.User{Email: "admin.val@example.com", Name: "Admin Val", Role: domain.RoleAdmin})
	_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{OrganizationID: org.ID, UserID: user.ID, Role: "admin"})
	sessToken := "sess_admin_val"
	_ = memStore.CreateSession(ctx, domain.Session{Token: sessToken, UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour)})

	invalidKeys := []string{
		"<script>alert(1)</script>",
		"flag with spaces",
		"flag;rm -rf",
		"flag$injection",
		"\"quoted\"",
		"'single_quoted'",
		"",
		strings.Repeat("a", 65), // > 64 chars
	}

	for _, badKey := range invalidKeys {
		payload, _ := json.Marshal(map[string]interface{}{
			"key":        badKey,
			"name":       "Malicious Flag",
			"project_id": proj.ID,
			"type":       "boolean",
			"environments": map[string]interface{}{
				"production": map[string]interface{}{
					"enabled":  true,
					"strategy": "boolean",
				},
			},
		})

		req := httptest.NewRequest(http.MethodPost, "/api/v1/flags", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+sessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Project-ID", proj.ID)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 Bad Request for invalid flag key %q, got: %d (%s)", badKey, w.Code, w.Body.String())
		}
	}

	validKeys := []string{
		"checkout_v2",
		"ai-smart-search",
		"DARK_MODE_2026",
		"f",
		strings.Repeat("b", 64),
	}

	for _, goodKey := range validKeys {
		payload, _ := json.Marshal(map[string]interface{}{
			"key":        goodKey,
			"name":       "Legitimate Flag",
			"project_id": proj.ID,
			"type":       "boolean",
			"environments": map[string]interface{}{
				"production": map[string]interface{}{
					"enabled":  true,
					"strategy": "boolean",
				},
			},
		})

		req := httptest.NewRequest(http.MethodPost, "/api/v1/flags", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+sessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Project-ID", proj.ID)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected 201 Created for valid flag key %q, got: %d (%s)", goodKey, w.Code, w.Body.String())
		}
	}
}

// TestCookieSecureFlagDefault verifies OWASP A02/A05:
// Cookies default to Secure: true, and can be opted out with ALLOW_INSECURE_COOKIES=true.
func TestCookieSecureFlagDefault(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)

	// 1. Default configuration -> Cookies MUST have Secure: true
	t.Setenv("ALLOW_INSECURE_COOKIES", "")
	t.Setenv("SECURE_COOKIE", "")

	w := httptest.NewRecorder()
	server.setSessionCookie(w, nil, "token_secure_test", time.Now().Add(time.Hour))
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("Expected cookie to be set")
	}
	if !cookies[0].Secure {
		t.Errorf("Expected default session cookie to have Secure: true, got false")
	}

	// 2. Opt-out via ALLOW_INSECURE_COOKIES=true -> Cookies have Secure: false
	t.Setenv("ALLOW_INSECURE_COOKIES", "true")
	wInsecure := httptest.NewRecorder()
	server.setSessionCookie(wInsecure, nil, "token_insecure_test", time.Now().Add(time.Hour))
	insecureCookies := wInsecure.Result().Cookies()
	if len(insecureCookies) == 0 {
		t.Fatal("Expected cookie to be set")
	}
	if insecureCookies[0].Secure {
		t.Errorf("Expected session cookie with ALLOW_INSECURE_COOKIES=true to have Secure: false, got true")
	}
}

// TestCanaryWebhookConstantTimeAuth verifies constant-time webhook authentication.
func TestCanaryWebhookConstantTimeAuth(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)
	ctx := context.Background()

	org, _ := memStore.CreateOrganization(ctx, domain.Organization{ID: "org_canary", Name: "Canary Org"})
	proj, _ := memStore.CreateProject(ctx, domain.Project{ID: "proj_canary", OrganizationID: org.ID, Name: "Proj Canary"})
	_ = proj

	flag := domain.FeatureFlag{
		Key:       "canary_timing_flag",
		ProjectID: proj.ID,
		Name:      "Canary Timing Flag",
		Type:      "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: true, Strategy: domain.StrategyPercentage, Percentage: 25},
		},
	}
	_, _ = memStore.SaveFlag(ctx, flag, "system@flagura.dev")

	_, _ = server.canary.SubmitSchedule(ctx, domain.CanarySchedule{
		FlagKey:     "canary_timing_flag",
		ProjectID:   proj.ID,
		Environment: domain.EnvProduction,
		Stages: []domain.CanaryStage{
			{Index: 0, TargetPercentage: 25, DurationSec: 100},
		},
	})

	t.Setenv("FLAGURA_WEBHOOK_SECRET", "super-secret-webhook-key-999")

	// 1. Valid Secret in X-Webhook-Secret header -> 200 OK
	reqValid := httptest.NewRequest(http.MethodPost, "/api/v1/flags/canary_timing_flag/canary/rollback", bytes.NewReader([]byte(`{"reason":"APM Alert"}`)))
	reqValid.Header.Set("X-Webhook-Secret", "super-secret-webhook-key-999")
	reqValid.Header.Set("X-Project-ID", proj.ID)
	wValid := httptest.NewRecorder()
	server.ServeHTTP(wValid, reqValid)
	if wValid.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK with valid webhook secret, got: %d (%s)", wValid.Code, wValid.Body.String())
	}

	// 2. Invalid Secret -> 401 Unauthorized
	reqInvalid := httptest.NewRequest(http.MethodPost, "/api/v1/flags/canary_timing_flag/canary/rollback", nil)
	reqInvalid.Header.Set("X-Webhook-Secret", "wrong-secret-token")
	reqInvalid.Header.Set("X-Project-ID", proj.ID)
	wInvalid := httptest.NewRecorder()
	server.ServeHTTP(wInvalid, reqInvalid)
	if wInvalid.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized with wrong webhook secret, got: %d (%s)", wInvalid.Code, wInvalid.Body.String())
	}
}


