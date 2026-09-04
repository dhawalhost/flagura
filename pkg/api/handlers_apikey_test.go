package api

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func TestAPIKeyEnvironmentScopingAndRestrictions(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// 1. Create an admin session
	sessionToken := "admin_session_token_scope_test"
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

	// 2. Create an API Key explicitly scoped to "staging"
	createPayload := []byte(`{"name":"Staging Microservice Worker","role":"developer","environment":"staging"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewReader(createPayload))
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for staging key, got %d: %s", rec.Code, rec.Body.String())
	}

	var stagingKeyResp struct {
		APIKey domain.APIKey `json:"api_key"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &stagingKeyResp)
	stagingKey := stagingKeyResp.APIKey.Key

	// 3. Evaluate flag targeting "staging" -> Must SUCCEED (200 OK)
	evalStagingPayload := []byte(`{
		"flags": ["ai-smart-search"],
		"context": {
			"user_id": "usr_test_1",
			"environment": "staging"
		}
	}`)
	evalReq := httptest.NewRequest(http.MethodPost, "/api/v1/evaluate", bytes.NewReader(evalStagingPayload))
	evalReq.Header.Set("Authorization", "Bearer "+stagingKey)
	evalReq.Header.Set("Content-Type", "application/json")
	evalRec := httptest.NewRecorder()
	server.mux.ServeHTTP(evalRec, evalReq)

	if evalRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for staging key on staging env, got %d: %s", evalRec.Code, evalRec.Body.String())
	}

	// 4. Evaluate flag targeting "production" with staging key -> Must be FORBIDDEN (403)
	evalProdPayload := []byte(`{
		"flags": ["ai-smart-search"],
		"context": {
			"user_id": "usr_test_1",
			"environment": "production"
		}
	}`)
	evalProdReq := httptest.NewRequest(http.MethodPost, "/api/v1/evaluate", bytes.NewReader(evalProdPayload))
	evalProdReq.Header.Set("Authorization", "Bearer "+stagingKey)
	evalProdReq.Header.Set("Content-Type", "application/json")
	evalProdRec := httptest.NewRecorder()
	server.mux.ServeHTTP(evalProdRec, evalProdReq)

	if evalProdRec.Code != http.StatusForbidden {
		t.Fatalf("SECURITY BREACH: expected 403 Forbidden when staging key accesses production, got %d: %s", evalProdRec.Code, evalProdRec.Body.String())
	}

	// 5. Attempt flag toggle on production with staging key -> Must be FORBIDDEN (403)
	togglePayload := []byte(`{"environment":"production"}`)
	toggleReq := httptest.NewRequest(http.MethodPatch, "/api/v1/flags/ai-smart-search/toggle", bytes.NewReader(togglePayload))
	toggleReq.Header.Set("Authorization", "Bearer "+stagingKey)
	toggleReq.Header.Set("Content-Type", "application/json")
	toggleRec := httptest.NewRecorder()
	server.mux.ServeHTTP(toggleRec, toggleReq)

	if toggleRec.Code != http.StatusForbidden {
		t.Fatalf("SECURITY BREACH: expected 403 Forbidden when staging key toggles production flag, got %d: %s", toggleRec.Code, toggleRec.Body.String())
	}

	// 6. Create an Admin API Key with "all" environments
	createAllPayload := []byte(`{"name":"CI/CD Admin Key","role":"admin","environment":"all"}`)
	reqAll := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewReader(createAllPayload))
	reqAll.Header.Set("Authorization", "Bearer "+sessionToken)
	reqAll.Header.Set("Content-Type", "application/json")
	recAll := httptest.NewRecorder()
	server.mux.ServeHTTP(recAll, reqAll)

	if recAll.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for all-env key, got %d: %s", recAll.Code, recAll.Body.String())
	}

	var allKeyResp struct {
		APIKey domain.APIKey `json:"api_key"`
	}
	_ = json.Unmarshal(recAll.Body.Bytes(), &allKeyResp)
	allKey := allKeyResp.APIKey.Key

	// 7. Evaluate on both production and staging using "all" key -> Both must SUCCEED (200 OK)
	for _, env := range []string{"production", "staging", "development"} {
		testPayload := []byte(fmt.Sprintf(`{"flags":["ai-smart-search"],"context":{"user_id":"u1","environment":"%s"}}`, env))
		tReq := httptest.NewRequest(http.MethodPost, "/api/v1/evaluate", bytes.NewReader(testPayload))
		tReq.Header.Set("Authorization", "Bearer "+allKey)
		tReq.Header.Set("Content-Type", "application/json")
		tRec := httptest.NewRecorder()
		server.mux.ServeHTTP(tRec, tReq)

		if tRec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for 'all' key on env '%s', got %d: %s", env, tRec.Code, tRec.Body.String())
		}
	}
}
