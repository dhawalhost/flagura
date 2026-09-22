package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

// setupMockIdP spins up a local HTTP server that mimics a standard OIDC Identity Provider.
func setupMockIdP(t *testing.T, expectedClientID, expectedClientSecret string) *httptest.Server {
	var mockServer *httptest.Server
	mux := http.NewServeMux()

	// 1. Authorize endpoint: immediately redirects back with code
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		redirectURI := r.URL.Query().Get("redirect_uri")
		state := r.URL.Query().Get("state")
		if redirectURI == "" {
			http.Error(w, "missing redirect_uri", http.StatusBadRequest)
			return
		}
		target := fmt.Sprintf("%s?code=mock_auth_code_xyz&state=%s", redirectURI, url.QueryEscape(state))
		http.Redirect(w, r, target, http.StatusFound)
	})

	// 2. Token endpoint: exchanges authorization code for tokens
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		clientID := r.FormValue("client_id")
		clientSecret := r.FormValue("client_secret")
		code := r.FormValue("code")

		if clientID != expectedClientID || clientSecret != expectedClientSecret {
			http.Error(w, "invalid client credentials", http.StatusUnauthorized)
			return
		}
		if code != "mock_auth_code_xyz" {
			http.Error(w, "invalid code", http.StatusBadRequest)
			return
		}

		// Construct a mock JWT id_token (header.payload.signature)
		payload := map[string]interface{}{
			"sub":   "sub_123456789",
			"email": "employee@acme.com",
			"name":  "Acme Employee",
			"nonce": "test_nonce",
		}
		payloadJSON, _ := json.Marshal(payload)
		payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
		mockJWT := fmt.Sprintf("eyJhbGciOiJub25lIn0.%s.signature", payloadB64)

		resp := map[string]interface{}{
			"access_token": "mock_access_token_abc",
			"token_type":   "Bearer",
			"id_token":     mockJWT,
			"expires_in":   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// 3. Userinfo endpoint: returns user claims
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer mock_access_token_abc" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		resp := map[string]interface{}{
			"sub":   "sub_123456789",
			"email": "employee@acme.com",
			"name":  "Acme Employee",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mockServer = httptest.NewServer(mux)
	return mockServer
}

func TestOIDC_FullSSOLoginFlow(t *testing.T) {
	ctx := context.Background()
	memStore := store.NewMemoryStore()
	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// 1. Setup mock IdP
	clientID := "flagura-acme-client"
	clientSecret := "flagura-acme-secret"
	mockIdP := setupMockIdP(t, clientID, clientSecret)
	defer mockIdP.Close()

	// 2. Setup organization & default project in store
	orgID := "org_acme_corp"
	_, _ = memStore.CreateOrganization(ctx, domain.Organization{
		ID:   orgID,
		Name: "Acme Corporation",
		Slug: "acme-corp",
	})
	projID := "proj_acme_prod"
	_, _ = memStore.CreateProject(ctx, domain.Project{
		ID:             projID,
		OrganizationID: orgID,
		Name:           "Acme Production Flags",
		Slug:           "acme-prod",
	})

	// 3. Configure OIDC for organization
	oidcCfg := domain.OIDCConfig{
		OrganizationID: orgID,
		Enabled:        true,
		IssuerURL:      mockIdP.URL,
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		AllowedDomains: "acme.com",
		DefaultRole:    "developer",
	}
	if err := memStore.SaveOIDCConfig(ctx, oidcCfg); err != nil {
		t.Fatalf("SaveOIDCConfig failed: %v", err)
	}

	// 4. Initiate OIDC Login: GET /api/v1/auth/oidc/login?org_id=...
	loginReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/login?org_id="+orgID, nil)
	loginRec := httptest.NewRecorder()
	server.mux.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusFound {
		t.Fatalf("expected 302 Found redirect from login endpoint, got %d: %s", loginRec.Code, loginRec.Body.String())
	}

	redirectLocation := loginRec.Header().Get("Location")
	if !strings.HasPrefix(redirectLocation, mockIdP.URL+"/authorize") {
		t.Fatalf("expected redirect to IdP authorize endpoint, got: %s", redirectLocation)
	}

	// Extract state cookie and state query param
	var stateCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == domain.CookieOIDCStateName {
			stateCookie = c
			break
		}
	}
	if stateCookie == nil {
		t.Fatalf("expected %s cookie to be set", domain.CookieOIDCStateName)
	}

	parsedRedirectURL, _ := url.Parse(redirectLocation)
	stateParam := parsedRedirectURL.Query().Get("state")
	if stateParam == "" {
		t.Fatalf("missing state query param in redirect URL")
	}

	// 5. Simulate callback from IdP: GET /api/v1/auth/oidc/callback?code=mock_auth_code_xyz&state=...
	callbackReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/auth/oidc/callback?code=mock_auth_code_xyz&state=%s", stateParam), nil)
	callbackReq.AddCookie(stateCookie)
	callbackRec := httptest.NewRecorder()
	server.mux.ServeHTTP(callbackRec, callbackReq)

	if callbackRec.Code != http.StatusFound {
		t.Fatalf("expected 302 Found redirect to dashboard after callback, got %d: %s", callbackRec.Code, callbackRec.Body.String())
	}

	if callbackRec.Header().Get("Location") != "/dashboard" {
		t.Fatalf("expected redirect to /dashboard, got %s", callbackRec.Header().Get("Location"))
	}

	// 6. Verify session and project cookies were set
	var sessionCookie *http.Cookie
	var projectCookie *http.Cookie
	var stateCookieCleared bool

	for _, c := range callbackRec.Result().Cookies() {
		if c.Name == domain.CookieSessionName && c.Value != "" {
			sessionCookie = c
		}
		if c.Name == domain.CookieProjectName && c.Value != "" {
			projectCookie = c
		}
		if c.Name == domain.CookieOIDCStateName && c.MaxAge == -1 {
			stateCookieCleared = true
		}
	}

	if sessionCookie == nil {
		t.Fatalf("expected %s cookie to be set on successful OIDC login", domain.CookieSessionName)
	}
	if projectCookie == nil || projectCookie.Value != projID {
		t.Fatalf("expected %s cookie to be set to %s, got %+v", domain.CookieProjectName, projID, projectCookie)
	}
	if !stateCookieCleared {
		t.Fatalf("expected state cookie to be cleared after callback")
	}

	// 7. Verify JIT User & Org Member provisioned in store
	createdUser, err := memStore.GetUserByEmail(ctx, "employee@acme.com")
	if err != nil || createdUser == nil {
		t.Fatalf("expected JIT user to be provisioned in store, got %v", err)
	}
	if createdUser.Name != "Acme Employee" {
		t.Fatalf("expected user name 'Acme Employee', got %s", createdUser.Name)
	}

	member, err := memStore.GetOrgMember(ctx, orgID, createdUser.ID)
	if err != nil || member == nil {
		t.Fatalf("expected org membership to be provisioned, got %v", err)
	}
	if member.Role != "developer" {
		t.Fatalf("expected default role 'developer', got %s", member.Role)
	}
}

func TestOIDC_CSRFStateMismatchRejected(t *testing.T) {
	ctx := context.Background()
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)

	orgID := "org_csrf_test"
	_ = memStore.SaveOIDCConfig(ctx, domain.OIDCConfig{
		OrganizationID: orgID,
		Enabled:        true,
		IssuerURL:      "https://idp.example.com",
		ClientID:       "test-client",
		ClientSecret:   "test-secret",
	})

	// Setup valid state cookie
	stateClaims := domain.OIDCStateClaims{
		State:          "legitimate_state_123",
		OrganizationID: orgID,
		CreatedAt:      time.Now().UTC(),
	}
	data, _ := json.Marshal(stateClaims)
	encoded := base64.RawURLEncoding.EncodeToString(data)
	cookie := &http.Cookie{
		Name:  domain.CookieOIDCStateName,
		Value: encoded,
	}

	// Case 1: Attacker provides forged state param
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=anycode&state=forged_state_hacker", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("SECURITY VIOLATION: expected 400 Bad Request on state mismatch, got %d", rec.Code)
	}

	// Case 2: Missing state cookie
	reqNoCookie := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=anycode&state=legitimate_state_123", nil)
	recNoCookie := httptest.NewRecorder()
	server.mux.ServeHTTP(recNoCookie, reqNoCookie)

	if recNoCookie.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on missing state cookie, got %d", recNoCookie.Code)
	}
}

func TestOIDC_DomainRestrictionEnforced(t *testing.T) {
	ctx := context.Background()
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)

	// IdP returns evil.com user
	mockIdP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			payload := map[string]interface{}{
				"sub":   "sub_hacker",
				"email": "attacker@evil.com",
				"name":  "Evil Hacker",
			}
			payloadJSON, _ := json.Marshal(payload)
			payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
			mockJWT := fmt.Sprintf("eyJhbGciOiJub25lIn0.%s.signature", payloadB64)

			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "token_hacker",
				"id_token":     mockJWT,
			})
		}
	}))
	defer mockIdP.Close()

	orgID := "org_domain_test"
	_ = memStore.SaveOIDCConfig(ctx, domain.OIDCConfig{
		OrganizationID: orgID,
		Enabled:        true,
		IssuerURL:      mockIdP.URL,
		ClientID:       "client-id",
		ClientSecret:   "client-secret",
		AllowedDomains: "acme.com, acme.co", // Only acme domains permitted!
	})

	stateClaims := domain.OIDCStateClaims{
		State:          "valid_state_token",
		OrganizationID: orgID,
		CreatedAt:      time.Now().UTC(),
	}
	data, _ := json.Marshal(stateClaims)
	cookie := &http.Cookie{
		Name:  domain.CookieOIDCStateName,
		Value: base64.RawURLEncoding.EncodeToString(data),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=somecode&state=valid_state_token", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("SECURITY VIOLATION: expected 403 Forbidden for unauthorized domain attacker@evil.com, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestOIDC_AdminConfigManagementAndRBAC(t *testing.T) {
	ctx := context.Background()
	memStore := store.NewMemoryStore()
	server, _ := NewServer(memStore)

	orgID := "org_admin_sso_rbac"
	_, _ = memStore.CreateOrganization(ctx, domain.Organization{
		ID:   orgID,
		Name: "RBAC Test Org",
		Slug: "rbac-test-org",
	})
	projID := "proj_admin_sso"
	_, _ = memStore.CreateProject(ctx, domain.Project{
		ID:             projID,
		OrganizationID: orgID,
		Name:           "SSO Proj",
		Slug:           "sso-proj",
	})

	// Owner User
	ownerUser, _ := memStore.CreateUser(ctx, domain.User{
		ID:    "usr_owner_sso",
		Email: "owner@company.com",
		Role:  domain.RoleDeveloper, // Global developer, org owner
	})
	_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{
		OrganizationID: orgID,
		UserID:         ownerUser.ID,
		Role:           "owner",
	})
	ownerToken := "token_owner_sso"
	_ = memStore.CreateSession(ctx, domain.Session{
		Token:     ownerToken,
		UserID:    ownerUser.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		User:      ownerUser,
	})
	ownerCookie := &http.Cookie{Name: domain.CookieSessionName, Value: ownerToken}

	// Regular Developer User
	devUser, _ := memStore.CreateUser(ctx, domain.User{
		ID:    "usr_dev_sso",
		Email: "dev@company.com",
		Role:  domain.RoleDeveloper,
	})
	_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{
		OrganizationID: orgID,
		UserID:         devUser.ID,
		Role:           "developer",
	})
	devToken := "token_dev_sso"
	_ = memStore.CreateSession(ctx, domain.Session{
		Token:     devToken,
		UserID:    devUser.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		User:      devUser,
	})
	devCookie := &http.Cookie{Name: domain.CookieSessionName, Value: devToken}

	// 1. Owner saves OIDC configuration
	configPayload := []byte(`{
		"enabled": true,
		"issuer_url": "https://accounts.google.com",
		"client_id": "google-client-id-123",
		"client_secret": "google-secret-xyz",
		"allowed_domains": "company.com",
		"default_role": "developer"
	}`)
	saveReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/organizations/%s/oidc", orgID), bytes.NewReader(configPayload))
	saveReq.AddCookie(ownerCookie)
	saveRec := httptest.NewRecorder()
	server.mux.ServeHTTP(saveRec, saveReq)

	if saveRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on owner save OIDC config, got %d: %s", saveRec.Code, saveRec.Body.String())
	}

	// 2. Owner gets OIDC config -> client_secret MUST be masked
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/organizations/%s/oidc", orgID), nil)
	getReq.AddCookie(ownerCookie)
	getRec := httptest.NewRecorder()
	server.mux.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on get OIDC config, got %d", getRec.Code)
	}
	var getResp domain.OIDCConfig
	_ = json.Unmarshal(getRec.Body.Bytes(), &getResp)
	if getResp.ClientSecret != "••••••••" {
		t.Fatalf("SECURITY VIOLATION: expected masked client_secret, got: %s", getResp.ClientSecret)
	}

	// 3. Regular developer attempts to get or modify OIDC config -> MUST BE FORBIDDEN (403)
	devGetReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/organizations/%s/oidc", orgID), nil)
	devGetReq.AddCookie(devCookie)
	devGetRec := httptest.NewRecorder()
	server.mux.ServeHTTP(devGetRec, devGetReq)

	if devGetRec.Code != http.StatusForbidden {
		t.Fatalf("SECURITY VIOLATION: expected 403 Forbidden when unprivileged developer reads OIDC config, got %d", devGetRec.Code)
	}

	devPutReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/organizations/%s/oidc", orgID), bytes.NewReader(configPayload))
	devPutReq.AddCookie(devCookie)
	devPutRec := httptest.NewRecorder()
	server.mux.ServeHTTP(devPutRec, devPutReq)

	if devPutRec.Code != http.StatusForbidden {
		t.Fatalf("SECURITY VIOLATION: expected 403 Forbidden when unprivileged developer modifies OIDC config, got %d", devPutRec.Code)
	}

	// 4. Owner deletes OIDC config
	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/organizations/%s/oidc", orgID), nil)
	delReq.AddCookie(ownerCookie)
	delRec := httptest.NewRecorder()
	server.mux.ServeHTTP(delRec, delReq)

	if delRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on delete OIDC config, got %d", delRec.Code)
	}

	// Verify deleted
	cfgAfter, _ := memStore.GetOIDCConfig(ctx, orgID)
	if cfgAfter != nil {
		t.Fatalf("expected OIDC config to be deleted, but still exists: %+v", cfgAfter)
	}
}
