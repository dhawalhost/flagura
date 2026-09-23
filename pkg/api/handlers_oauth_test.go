package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/config"
	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func setupOAuthTestServer(t *testing.T) (*Server, *store.MemoryStore) {
	t.Helper()
	st := store.NewMemoryStore()
	cfg := &config.Config{
		ServerPort:         "3000",
		Host:               "localhost",
		BaseURL:            "http://localhost:3000",
		Environment:        domain.EnvProduction,
		RateLimitRPS:       100,
		RateLimitBurst:     200,
		CORSAllowedOrigins: []string{"*"},
		GoogleClientID:     "test-google-client-id",
		GoogleClientSecret: "test-google-client-secret",
		GitHubClientID:     "test-github-client-id",
		GitHubClientSecret: "test-github-client-secret",
	}
	srv, err := NewServer(st)
	if err != nil {
		t.Fatalf("Failed to create test server: %v", err)
	}
	srv.SetConfig(cfg)
	return srv, st
}

func TestOAuthProvidersEndpoint(t *testing.T) {
	srv, _ := setupOAuthTestServer(t)

	// When both providers are configured
	req := httptest.NewRequest(http.MethodGet, RouteAuthOAuthProviders, nil)
	rr := httptest.NewRecorder()
	srv.handleOAuthProviders(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp OAuthProvidersResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Google || !resp.GitHub {
		t.Errorf("expected both google and github to be true, got google=%v, github=%v", resp.Google, resp.GitHub)
	}

	// When Google is disabled
	srv.cfg.GoogleClientID = ""
	rr2 := httptest.NewRecorder()
	srv.handleOAuthProviders(rr2, req)
	var resp2 OAuthProvidersResponse
	_ = json.NewDecoder(rr2.Body).Decode(&resp2)
	if resp2.Google {
		t.Errorf("expected google to be false when unconfigured, got %v", resp2.Google)
	}
	if !resp2.GitHub {
		t.Errorf("expected github to remain true, got %v", resp2.GitHub)
	}
}

func TestOAuthLogin_RedirectsAndSetsStateCookie(t *testing.T) {
	srv, _ := setupOAuthTestServer(t)

	// Test Google Login Initiation
	reqGoogle := httptest.NewRequest(http.MethodGet, RouteAuthOAuthGoogleLogin, nil)
	rrGoogle := httptest.NewRecorder()
	srv.handleOAuthGoogleLogin(rrGoogle, reqGoogle)

	if rrGoogle.Code != http.StatusFound {
		t.Fatalf("expected status 302 Found, got %d", rrGoogle.Code)
	}
	locGoogle := rrGoogle.Header().Get("Location")
	if !strings.Contains(locGoogle, "accounts.google.com") || !strings.Contains(locGoogle, "client_id=test-google-client-id") {
		t.Errorf("unexpected Google redirect location: %s", locGoogle)
	}

	// Verify state cookie
	cookies := rrGoogle.Result().Cookies()
	var stateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == domain.CookieOAuthStateName {
			stateCookie = c
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("expected flagura_oauth_state cookie to be set")
	}
	if !stateCookie.HttpOnly {
		t.Error("expected state cookie to be HttpOnly")
	}

	// Test GitHub Login Initiation
	reqGitHub := httptest.NewRequest(http.MethodGet, RouteAuthOAuthGitHubLogin, nil)
	rrGitHub := httptest.NewRecorder()
	srv.handleOAuthGitHubLogin(rrGitHub, reqGitHub)

	if rrGitHub.Code != http.StatusFound {
		t.Fatalf("expected status 302 Found, got %d", rrGitHub.Code)
	}
	locGitHub := rrGitHub.Header().Get("Location")
	if !strings.Contains(locGitHub, "github.com/login/oauth/authorize") || !strings.Contains(locGitHub, "client_id=test-github-client-id") {
		t.Errorf("unexpected GitHub redirect location: %s", locGitHub)
	}
}

func TestOAuthCallback_StateMismatch_Fails(t *testing.T) {
	srv, _ := setupOAuthTestServer(t)

	// Provide bad state in request
	req := httptest.NewRequest(http.MethodGet, RouteAuthOAuthGoogleCallback+"?code=valid-code&state=bad-state", nil)
	// Set valid state claims in cookie
	claims := OAuthStateClaims{
		Provider:  "google",
		State:     "good-state",
		Nonce:     "nonce-123",
		CreatedAt: time.Now().UTC(),
	}
	claimsBytes, _ := json.Marshal(claims)
	req.AddCookie(&http.Cookie{
		Name:  domain.CookieOAuthStateName,
		Value: base64.RawURLEncoding.EncodeToString(claimsBytes),
	})

	rr := httptest.NewRecorder()
	srv.handleOAuthGoogleCallback(rr, req)

	// Should redirect to /auth with error query param
	if rr.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "/auth?error=") {
		t.Errorf("expected redirect to /auth with error, got %s", loc)
	}
}

func TestOAuthCallback_Google_NewUserProvisioning(t *testing.T) {
	srv, st := setupOAuthTestServer(t)

	// Spin up mock Google OAuth & UserInfo server
	mockGoogle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "mock-google-access-token",
				"id_token":     "mock-id-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":     "google-sub-12345",
				"email":   "developer@gmail.com",
				"name":    "Alice Developer",
				"picture": "https://lh3.googleusercontent.com/a/alice-avatar",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockGoogle.Close()

	// Configure mock endpoints
	googleEndpoints = oauthEndpoints{
		authURL:     mockGoogle.URL + "/auth",
		tokenURL:    mockGoogle.URL + "/token",
		userInfoURL: mockGoogle.URL + "/userinfo",
	}

	state := "valid-google-state"
	req := httptest.NewRequest(http.MethodGet, RouteAuthOAuthGoogleCallback+"?code=auth-code-123&state="+state, nil)
	claims := OAuthStateClaims{
		Provider:  "google",
		State:     state,
		Nonce:     "nonce-google",
		CreatedAt: time.Now().UTC(),
	}
	claimsBytes, _ := json.Marshal(claims)
	req.AddCookie(&http.Cookie{
		Name:  domain.CookieOAuthStateName,
		Value: base64.RawURLEncoding.EncodeToString(claimsBytes),
	})

	rr := httptest.NewRecorder()
	srv.handleOAuthGoogleCallback(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect 302 Found, got %d: body=%s", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if loc != "/dashboard" {
		t.Errorf("expected redirect to /dashboard, got %s", loc)
	}

	// Verify user was created in store
	user, err := st.GetUserByEmail(context.Background(), "developer@gmail.com")
	if err != nil {
		t.Fatalf("expected user to be created in store: %v", err)
	}
	if user.Name != "Alice Developer" {
		t.Errorf("expected user name 'Alice Developer', got %q", user.Name)
	}
	if user.AvatarURL != "https://lh3.googleusercontent.com/a/alice-avatar" {
		t.Errorf("expected avatar URL to be saved, got %q", user.AvatarURL)
	}

	// Verify organization was auto-provisioned
	orgs, err := st.ListUserOrganizations(context.Background(), user.ID)
	if err != nil || len(orgs) == 0 {
		t.Fatalf("expected auto-provisioned organization, got orgs=%v, err=%v", orgs, err)
	}
	if !strings.Contains(orgs[0].Name, "Alice Developer's Workspace") {
		t.Errorf("unexpected org name: %s", orgs[0].Name)
	}

	// Verify session cookies were set
	cookies := rr.Result().Cookies()
	var sessionCookie, projectCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == domain.CookieSessionName && c.Value != "" {
			sessionCookie = c
		}
		if c.Name == domain.CookieProjectName && c.Value != "" {
			projectCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected flagura_session cookie to be issued")
	}
	if projectCookie == nil {
		t.Fatal("expected flagura_project_id cookie to be issued")
	}
}

func TestOAuthCallback_GitHub_ExistingUserAccountLinking(t *testing.T) {
	srv, st := setupOAuthTestServer(t)

	// Pre-create existing user
	existingUser := domain.NewUser("octocat@github.com", "The Octocat", "existing-hash", domain.RoleDeveloper)
	createdUser, err := st.CreateUser(context.Background(), existingUser)
	if err != nil {
		t.Fatalf("failed to create existing user: %v", err)
	}

	// Spin up mock GitHub server
	mockGitHub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "mock-github-access-token",
				"token_type":   "Bearer",
				"scope":        "read:user,user:email",
			})
		case "/user":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"login":      "octocat",
				"name":       "Mona Lisa Octocat",
				"avatar_url": "https://github.com/images/error/octocat_happy.gif",
			})
		case "/user/emails":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"email":    "unverified@github.com",
					"primary":  false,
					"verified": false,
				},
				{
					"email":    "octocat@github.com",
					"primary":  true,
					"verified": true,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockGitHub.Close()

	githubEndpoints = oauthEndpoints{
		authURL:     mockGitHub.URL + "/login/oauth/authorize",
		tokenURL:    mockGitHub.URL + "/login/oauth/access_token",
		userInfoURL: mockGitHub.URL + "/user",
		emailsURL:   mockGitHub.URL + "/user/emails",
	}

	state := "valid-github-state"
	req := httptest.NewRequest(http.MethodGet, RouteAuthOAuthGitHubCallback+"?code=github-code-123&state="+state, nil)
	claims := OAuthStateClaims{
		Provider:  "github",
		State:     state,
		Nonce:     "nonce-github",
		CreatedAt: time.Now().UTC(),
	}
	claimsBytes, _ := json.Marshal(claims)
	req.AddCookie(&http.Cookie{
		Name:  domain.CookieOAuthStateName,
		Value: base64.RawURLEncoding.EncodeToString(claimsBytes),
	})

	rr := httptest.NewRecorder()
	srv.handleOAuthGitHubCallback(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect 302, got %d: body=%s", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if loc != "/dashboard" {
		t.Errorf("expected redirect to /dashboard, got %s", loc)
	}

	// Verify existing user's ID was preserved and avatar updated
	user, err := st.GetUserByEmail(context.Background(), "octocat@github.com")
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}
	if user.ID != createdUser.ID {
		t.Errorf("expected user ID %q, got %q (account duplication bug)", createdUser.ID, user.ID)
	}
	if user.AvatarURL != "https://github.com/images/error/octocat_happy.gif" {
		t.Errorf("expected avatar to be linked, got %q", user.AvatarURL)
	}
}

func TestOAuthCallback_GitHub_UnverifiedEmail_Rejected(t *testing.T) {
	srv, _ := setupOAuthTestServer(t)

	// Mock GitHub server where primary email is not verified
	mockGitHub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "mock-github-access-token",
			})
		case "/user":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "spammer"})
		case "/user/emails":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"email":    "unverified@example.com",
					"primary":  true,
					"verified": false, // Not verified!
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockGitHub.Close()

	githubEndpoints = oauthEndpoints{
		authURL:     mockGitHub.URL + "/login/oauth/authorize",
		tokenURL:    mockGitHub.URL + "/login/oauth/access_token",
		userInfoURL: mockGitHub.URL + "/user",
		emailsURL:   mockGitHub.URL + "/user/emails",
	}

	state := "github-unverified-state"
	req := httptest.NewRequest(http.MethodGet, RouteAuthOAuthGitHubCallback+"?code=code-123&state="+state, nil)
	claims := OAuthStateClaims{
		Provider:  "github",
		State:     state,
		CreatedAt: time.Now().UTC(),
	}
	claimsBytes, _ := json.Marshal(claims)
	req.AddCookie(&http.Cookie{
		Name:  domain.CookieOAuthStateName,
		Value: base64.RawURLEncoding.EncodeToString(claimsBytes),
	})

	rr := httptest.NewRecorder()
	srv.handleOAuthGitHubCallback(rr, req)

	// Expect redirect to /auth with error message
	if rr.Code != http.StatusFound {
		t.Fatalf("expected redirect 302, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "/auth?error=") {
		t.Errorf("expected error redirect to /auth, got %s", loc)
	}
	if !strings.Contains(strings.ToLower(loc), "verified") {
		t.Errorf("expected error description to mention verification, got %s", loc)
	}
}
