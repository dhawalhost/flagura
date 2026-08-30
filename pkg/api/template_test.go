package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
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
		if err := views.Dashboard(user, flags, logs, nil, mem.DriverName()).Render(ctx, &buf); err != nil {
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
}
