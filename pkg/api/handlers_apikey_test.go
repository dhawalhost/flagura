package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestAPIKeyLifecycleAndAuth(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// 1. Create an admin session for request authentication
	sessionToken := "test_admin_session_token_123"
	_ = memStore.CreateSession(nil, domain.Session{
		Token:     sessionToken,
		UserID:    "usr_admin_default",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		User: &domain.User{
			ID:    "usr_admin_default",
			Email: "admin@flagura.dev",
			Role:  domain.RoleAdmin,
		},
	})

	// 2. POST /api/v1/api-keys to create a new API key
	createPayload := []byte(`{"name":"Prod Microservice Worker","role":"developer"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewReader(createPayload))
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	var createResp struct {
		APIKey domain.APIKey `json:"api_key"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	rawKey := createResp.APIKey.Key
	keyID := createResp.APIKey.ID
	if rawKey == "" || !bytes.HasPrefix([]byte(rawKey), []byte("flg_live_")) {
		t.Fatalf("expected raw key starting with flg_live_, got %s", rawKey)
	}
	if keyID == "" {
		t.Fatalf("expected non-empty key ID")
	}

	// 3. GET /api/v1/api-keys to list active keys
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil)
	listReq.Header.Set("Authorization", "Bearer "+sessionToken)
	listRec := httptest.NewRecorder()

	server.mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", listRec.Code)
	}

	var listResp struct {
		APIKeys []domain.APIKey `json:"api_keys"`
	}
	_ = json.Unmarshal(listRec.Body.Bytes(), &listResp)
	if len(listResp.APIKeys) != 1 {
		t.Fatalf("expected 1 API key, got %d", len(listResp.APIKeys))
	}
	if listResp.APIKeys[0].Key != "" {
		t.Fatalf("raw key should be redacted from list response, got %s", listResp.APIKeys[0].Key)
	}
	if listResp.APIKeys[0].KeyPrefix == "" {
		t.Fatalf("expected non-empty key prefix")
	}

	// 4. Test Authenticating with the generated API key (e.g. GET /api/v1/flags)
	authReq := httptest.NewRequest(http.MethodGet, "/api/v1/flags", nil)
	authReq.Header.Set("X-API-Key", rawKey)
	authRec := httptest.NewRecorder()

	server.mux.ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK using generated API key, got %d: %s", authRec.Code, authRec.Body.String())
	}

	// 5. Test Revoking the API Key (DELETE /api/v1/api-keys/:id)
	revokeReq := httptest.NewRequest(http.MethodDelete, "/api/v1/api-keys/"+keyID, nil)
	revokeReq.Header.Set("Authorization", "Bearer "+sessionToken)
	revokeRec := httptest.NewRecorder()

	server.mux.ServeHTTP(revokeRec, revokeReq)
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on revoke, got %d: %s", revokeRec.Code, revokeRec.Body.String())
	}

	// 6. Test Authenticating after revocation must fail (401 Unauthorized)
	postRevokeReq := httptest.NewRequest(http.MethodGet, "/api/v1/flags", nil)
	postRevokeReq.Header.Set("X-API-Key", rawKey)
	postRevokeRec := httptest.NewRecorder()

	server.mux.ServeHTTP(postRevokeRec, postRevokeReq)
	if postRevokeRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized after key revocation, got %d", postRevokeRec.Code)
	}
}
