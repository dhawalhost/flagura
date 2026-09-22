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

// TestMultiCookieSessionResolution verifies AC-1.1:
// When multiple flagura_session cookies are presented (e.g. stale first, valid second),
// getUserFromRequest must evaluate all candidates and authenticate the valid one.
func TestMultiCookieSessionResolution(t *testing.T) {
	memStore := store.NewMemoryStore()
	srv, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("Failed to initialize server: %v", err)
	}

	ctx := context.Background()

	// 1. Create test user
	user := domain.User{
		ID:        "usr_multi_cookie_test",
		Email:     "multicookie@flagura.dev",
		Name:      "Multi Cookie Tester",
		Role:      domain.RoleDeveloper,
		CreatedAt: time.Now(),
	}
	_, err = memStore.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	org := domain.NewOrganization("Multi Org", "multi-org", "Multi Org")
	org.ID = "org_multi"
	_, _ = memStore.CreateOrganization(ctx, org)
	_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{
		OrganizationID: org.ID,
		UserID:         user.ID,
		Role:           "owner",
	})
	proj := domain.NewProject(org.ID, "Multi Proj", "multi-proj", "Multi Proj")
	proj.ID = "proj_multi"
	_, _ = memStore.CreateProject(ctx, proj)

	// Create flag
	flag := domain.FeatureFlag{
		ID:        "flag_multi_toggle",
		ProjectID: proj.ID,
		Key:       "multi-cookie-feature",
		Name:      "Multi Cookie Feature",
		Type:      "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: false},
		},
	}
	_, _ = memStore.SaveFlag(ctx, flag, user.Email)

	// 2. Create one valid session
	validToken, _ := generateSessionToken()
	validSess := domain.Session{
		Token:     validToken,
		UserID:    user.ID,
		User:      &user,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	_ = memStore.CreateSession(ctx, validSess)

	staleToken := "stale_invalid_token_999999"

	// 3. Make toggle request with STALE token FIRST, VALID token SECOND
	bodyBytes, _ := json.Marshal(map[string]interface{}{
		"environment": "production",
		"enabled":     true,
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/flags/multi-cookie-feature/toggle", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	// Add project cookie
	req.AddCookie(&http.Cookie{Name: domain.CookieProjectName, Value: proj.ID})
	// Add stale cookie first, valid cookie second
	req.Header.Add("Cookie", "flagura_session="+staleToken+"; flagura_session="+validToken)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK when secondary cookie is valid, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify flag actually toggled to true
	updated, _ := memStore.GetFlagByProject(ctx, proj.ID, "multi-cookie-feature")
	if !updated.Environments[domain.EnvProduction].Enabled {
		t.Fatalf("Expected flag to be enabled after toggle with multi-cookie resolution")
	}
}

// TestCookieDomainAndEviction verifies AC-3.1 and AC-3.2:
// setSessionCookie / setProjectCookie use resolved domain, and clearSessionCookie emits dual-scope eviction.
func TestCookieDomainAndEviction(t *testing.T) {
	memStore := store.NewMemoryStore()
	srv, _ := NewServer(memStore)

	// Test clearing cookies
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Host = "flagura.dev"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()

	srv.clearSessionCookie(rec, req)

	cookies := rec.Result().Cookies()
	var hostOnlyCleared, domainCleared bool
	for _, c := range cookies {
		if c.Name == domain.CookieSessionName && c.MaxAge < 0 {
			if c.Domain == "" {
				hostOnlyCleared = true
			} else if c.Domain == "flagura.dev" || c.Domain == ".flagura.dev" {
				domainCleared = true
			}
		}
	}

	if !hostOnlyCleared {
		t.Errorf("Expected host-only clearing cookie to be emitted")
	}
	if !domainCleared {
		t.Errorf("Expected domain-scoped clearing cookie to be emitted for flagura.dev")
	}
}

// TestCORSMultiOriginSupport verifies AC-3.4:
// Comma-separated FLAGURA_ALLOWED_ORIGIN matches both apex and www subdomains.
func TestCORSMultiOriginSupport(t *testing.T) {
	memStore := store.NewMemoryStore()
	srv, _ := NewServer(memStore)

	t.Setenv("FLAGURA_ALLOWED_ORIGIN", "https://flagura.dev, https://www.flagura.dev")

	// 1. Test request from https://www.flagura.dev
	reqWWW := httptest.NewRequest(http.MethodOptions, "/api/v1/flags", nil)
	reqWWW.Header.Set("Origin", "https://www.flagura.dev")
	recWWW := httptest.NewRecorder()
	srv.ServeHTTP(recWWW, reqWWW)

	if acao := recWWW.Header().Get("Access-Control-Allow-Origin"); acao != "https://www.flagura.dev" {
		t.Fatalf("Expected https://www.flagura.dev to be allowed, got: %s", acao)
	}

	// 2. Test request from https://flagura.dev
	reqApex := httptest.NewRequest(http.MethodOptions, "/api/v1/flags", nil)
	reqApex.Header.Set("Origin", "https://flagura.dev")
	recApex := httptest.NewRecorder()
	srv.ServeHTTP(recApex, reqApex)

	if acao := recApex.Header().Get("Access-Control-Allow-Origin"); acao != "https://flagura.dev" {
		t.Fatalf("Expected https://flagura.dev to be allowed, got: %s", acao)
	}
}
