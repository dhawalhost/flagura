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

	"github.com/dhawalhost/flagura/internal/domain"
	"github.com/dhawalhost/flagura/internal/store"
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

	if csp := headers.Get("Content-Security-Policy"); csp == "" {
		t.Errorf("Expected Content-Security-Policy header to be set")
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

	// 1. Create Developer user & session
	devUser, _ := memStore.CreateUser(ctx, domain.User{
		Email: "dev@company.com",
		Name:  "Developer",
		Role:  domain.RoleDeveloper,
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
	adminToken := "admin_session_token_123"
	_ = memStore.CreateSession(ctx, domain.Session{
		Token:     adminToken,
		UserID:    adminUser.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	// Developer can create flags and actor is attributed to authenticated email
	flagPayload := domain.FeatureFlag{
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
