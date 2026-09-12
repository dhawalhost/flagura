package api

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
	"github.com/dhawalhost/flagura/web/views"
)

func TestTemplComponents(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	flags, _ := mem.ListFlags(ctx)
	logs, _ := mem.ListAuditLogs(ctx, 10)
	user := &domain.User{
		ID:    "usr_test",
		Email: "dhawal@flagura.dev",
		Name:  "Dhawal",
		Role:  domain.RoleAdmin,
	}

	t.Run("LandingPage", func(t *testing.T) {
		var buf bytes.Buffer
		if err := views.LandingPage().Render(ctx, &buf); err != nil {
			t.Fatalf("Failed to render LandingPage: %v", err)
		}
		if buf.Len() == 0 {
			t.Fatalf("LandingPage output is empty")
		}
	})

	t.Run("AuthPage", func(t *testing.T) {
		var buf bytes.Buffer
		if err := views.AuthPage().Render(ctx, &buf); err != nil {
			t.Fatalf("Failed to render AuthPage: %v", err)
		}
		if buf.Len() == 0 {
			t.Fatalf("AuthPage output is empty")
		}
	})

	t.Run("Dashboard", func(t *testing.T) {
		var buf bytes.Buffer
		orgs, _ := mem.ListOrganizations(ctx)
		projects, _ := mem.ListProjects(ctx, "")
		if err := views.Dashboard(user, flags, logs, nil, mem.DriverName(), orgs, projects, store.DefaultProjectID).Render(ctx, &buf); err != nil {
			t.Fatalf("Failed to render Dashboard: %v", err)
		}
		if buf.Len() == 0 {
			t.Fatalf("Dashboard output is empty")
		}
	})

	t.Run("GovernanceModal", func(t *testing.T) {
		var buf bytes.Buffer
		if err := views.GovernanceModal(user, nil).Render(ctx, &buf); err != nil {
			t.Fatalf("Failed to render GovernanceModal: %v", err)
		}
		if buf.Len() == 0 {
			t.Fatalf("GovernanceModal output is empty")
		}
	})

	t.Run("SelfHostedDefaultRedirectToAuth", func(t *testing.T) {
		_ = os.Unsetenv("ENABLE_LANDING_PAGE")
		_ = os.Unsetenv("SHOW_LANDING_PAGE")

		server, _ := NewServer(mem)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect to /auth, got %d", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/auth" {
			t.Fatalf("expected Location /auth, got %s", loc)
		}
	})

	t.Run("OfficialDeploymentLandingPageEnabled", func(t *testing.T) {
		_ = os.Setenv("ENABLE_LANDING_PAGE", "true")
		defer os.Unsetenv("ENABLE_LANDING_PAGE")

		server, _ := NewServer(mem)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for official landing page, got %d", rec.Code)
		}
	})

	t.Run("DocsRedirect", func(t *testing.T) {
		server, _ := NewServer(mem)
		req := httptest.NewRequest(http.MethodGet, "/docs", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusTemporaryRedirect {
			t.Fatalf("expected 307 Temporary Redirect for /docs, got %d", rec.Code)
		}
		expectedLoc := "https://github.com/dhawalhost/flagura/blob/main/docs/product/sdks-and-api.md"
		if loc := rec.Header().Get("Location"); loc != expectedLoc {
			t.Fatalf("expected Location %s, got %s", expectedLoc, loc)
		}
	})

	t.Run("StaticAppJSServing", func(t *testing.T) {
		server, _ := NewServer(mem)
		req := httptest.NewRequest(http.MethodGet, "/static/js/app.js", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for /static/js/app.js, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "getStickyBucketJs") {
			t.Errorf("expected /static/js/app.js to contain getStickyBucketJs")
		}
		if !strings.Contains(body, "globalApp") {
			t.Errorf("expected /static/js/app.js to contain globalApp")
		}
	})

	t.Run("LayoutNonceRenderingOnServer", func(t *testing.T) {
		server, _ := NewServer(mem)
		req := httptest.NewRequest(http.MethodGet, "/auth", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for /auth, got %d", rec.Code)
		}
		csp := rec.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "nonce-") {
			t.Fatalf("expected CSP header to contain nonce-, got: %s", csp)
		}

		body := rec.Body.String()
		if !strings.Contains(body, `/static/js/app.js`) {
			t.Errorf("expected rendered page to reference /static/js/app.js")
		}
		if !strings.Contains(body, `nonce=`) {
			t.Errorf("expected rendered page to contain nonce attribute on script tags")
		}
	})

	t.Run("DashboardSubRoutesAndShortcuts", func(t *testing.T) {
		server, _ := NewServer(mem)

		// 1. Test top-level shortcut redirects
		shortcuts := map[string]string{
			"/overview":  "/dashboard",
			"/flags":     "/dashboard/flags",
			"/analytics": "/dashboard/analytics",
			"/evaluator": "/dashboard/evaluator",
			"/benchmark": "/dashboard/benchmark",
			"/audit":     "/dashboard/audit",
			"/sdk":       "/dashboard/sdk",
			"/profile":   "/dashboard/profile",
		}
		for path, expectedTarget := range shortcuts {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)
			if rec.Code != http.StatusTemporaryRedirect {
				t.Fatalf("expected 307 for shortcut %s, got %d", path, rec.Code)
			}
			if loc := rec.Header().Get("Location"); loc != expectedTarget {
				t.Fatalf("for shortcut %s, expected Location %s, got %s", path, expectedTarget, loc)
			}
		}

		// 2. Test shortcuts preserve query parameters
		reqQ := httptest.NewRequest(http.MethodGet, "/analytics?env=staging", nil)
		recQ := httptest.NewRecorder()
		server.ServeHTTP(recQ, reqQ)
		if loc := recQ.Header().Get("Location"); loc != "/dashboard/analytics?env=staging" {
			t.Fatalf("expected query parameter preservation, got %s", loc)
		}

		// 3. Create authenticated user session
		signUpPayload := domain.SignUpRequest{
			Email:    "subroutes.tester@flagura.dev",
			Name:     "Subroutes Tester",
			Password: "SubroutePass123!",
			Role:     domain.RoleDeveloper,
		}
		bodyBytes, _ := json.Marshal(signUpPayload)
		reqAuth := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewReader(bodyBytes))
		reqAuth.Header.Set("Content-Type", "application/json")
		wAuth := httptest.NewRecorder()
		server.ServeHTTP(wAuth, reqAuth)
		if wAuth.Code != http.StatusCreated {
			t.Fatalf("failed to create test user: %d", wAuth.Code)
		}
		var sessionCookie *http.Cookie
		for _, c := range wAuth.Result().Cookies() {
			if c.Name == SessionCookieName {
				sessionCookie = c
				break
			}
		}
		if sessionCookie == nil {
			t.Fatalf("missing session cookie")
		}

		// 4. Test authenticated sub-routes render correct titles
		routeTitles := map[string]string{
			"/dashboard":                 "Developer Console — Flagura",
			"/dashboard/":                "Developer Console — Flagura",
			"/dashboard/flags":           "Flags & Rollouts — Flagura",
			"/dashboard/analytics":       "Analytics & Telemetry — Flagura",
			"/dashboard/evaluator":       "Live Evaluator — Flagura",
			"/dashboard/benchmark":       "Latency Benchmark — Flagura",
			"/dashboard/audit":           "Audit Trail — Flagura",
			"/dashboard/sdk":             "SDK Integration — Flagura",
			"/dashboard/profile":         "Profile Settings — Flagura",
			"/dashboard?tab=analytics":   "Analytics & Telemetry — Flagura",
		}

		for path, expectedTitle := range routeTitles {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.AddCookie(sessionCookie)
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 OK for %s, got %d", path, rec.Code)
			}
			out := rec.Body.String()
			escapedTitle := html.EscapeString(expectedTitle)
			if !strings.Contains(out, "<title>"+escapedTitle+"</title>") {
				t.Errorf("for path %s, expected title %q in response, but got:\n%s", path, escapedTitle, out[:min(300, len(out))])
			}
		}
	})
}
