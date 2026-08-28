package api

import (
	"bytes"
	"context"
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
		if err := views.Dashboard(user, flags, logs, mem.DriverName()).Render(ctx, &buf); err != nil {
			t.Fatalf("Failed to render Dashboard: %v", err)
		}
		if buf.Len() == 0 {
			t.Fatalf("Dashboard output is empty")
		}
	})
}
