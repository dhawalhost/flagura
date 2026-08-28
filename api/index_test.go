package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVercelHandlerRouting(t *testing.T) {
	tests := []struct {
		name           string
		target         string
		expectedStatus int
		expectedHeader string
	}{
		{
			name:           "Root Landing Page",
			target:         "/",
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
