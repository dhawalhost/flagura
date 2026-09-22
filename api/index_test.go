package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestVercelHandlerRouting(t *testing.T) {
	tests := []struct {
		name           string
		target         string
		enableLanding  bool
		expectedStatus int
		expectedHeader string
	}{
		{
			name:           "Root URL (Default Self-Hosted Redirect to /auth)",
			target:         "/",
			enableLanding:  false,
			expectedStatus: http.StatusSeeOther,
		},
		{
			name:           "Root Landing Page (Official Cloud Demo ENABLE_LANDING_PAGE=true)",
			target:         "/",
			enableLanding:  true,
			expectedStatus: http.StatusOK,
			expectedHeader: "text/html",
		},
		{
			name:           "Auth Page",
			target:         "/auth",
			expectedStatus: http.StatusOK,
			expectedHeader: "text/html",
		},
		{
			name:           "Dashboard (Redirects to Auth)",
			target:         "/dashboard",
			expectedStatus: http.StatusSeeOther,
		},
		{
			name:           "Static Logo Asset",
			target:         "/static/img/flagura-logo.png",
			expectedStatus: http.StatusOK,
			expectedHeader: "image/png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.enableLanding {
				_ = os.Setenv("ENABLE_LANDING_PAGE", "true")
				defer os.Unsetenv("ENABLE_LANDING_PAGE")
			} else {
				_ = os.Unsetenv("ENABLE_LANDING_PAGE")
			}

			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			rec := httptest.NewRecorder()

			Handler(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d for %s", tt.expectedStatus, rec.Code, tt.target)
			}
			if tt.expectedHeader != "" {
				contentType := rec.Header().Get("Content-Type")
				if len(contentType) < len(tt.expectedHeader) || contentType[:len(tt.expectedHeader)] != tt.expectedHeader {
					t.Fatalf("expected Content-Type containing %q, got %q", tt.expectedHeader, contentType)
				}
			}
		})
	}
}

func TestServerlessDatabaseConnectionUnavailable(t *testing.T) {
	serverMu.Lock()
	oldServer := server
	server = nil
	serverMu.Unlock()
	defer func() {
		serverMu.Lock()
		server = oldServer
		serverMu.Unlock()
	}()

	t.Setenv("DATABASE_URL", "postgres://invalid:invalid@localhost:54321/db?sslmode=disable&connect_timeout=1")
	t.Setenv("ALLOW_MEMORY_FALLBACK", "false")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	Handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503 Service Unavailable when DB is down, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "2" {
		t.Errorf("expected Retry-After: 2 header, got %q", rec.Header().Get("Retry-After"))
	}
}

func TestServerlessDatabaseConnectionFallbackAllowed(t *testing.T) {
	serverMu.Lock()
	oldServer := server
	server = nil
	serverMu.Unlock()
	defer func() {
		serverMu.Lock()
		server = oldServer
		serverMu.Unlock()
	}()

	t.Setenv("DATABASE_URL", "postgres://invalid:invalid@localhost:54321/db?sslmode=disable&connect_timeout=1")
	t.Setenv("ALLOW_MEMORY_FALLBACK", "true")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	Handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK when memory fallback allowed, got %d", rec.Code)
	}
}
