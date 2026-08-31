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

func TestProjectsAPI_EndpointsAndIsolation(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("Failed to initialize server: %v", err)
	}

	// 1. Authenticate session
	session := domain.Session{
		Token:     "test-session-admin-token",
		UserID:    "usr_admin_default",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		User: &domain.User{
			ID:    "usr_admin_default",
			Email: "dhawal@flagura.dev",
			Role:  domain.RoleAdmin,
		},
	}
	_ = memStore.CreateSession(t.Context(), session)

	// Helper to perform authenticated requests
	doAuthReq := func(method, url string, body interface{}, extraHeaders map[string]string) *httptest.ResponseRecorder {
		var reqBody []byte
		if body != nil {
			reqBody, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, url, bytes.NewReader(reqBody))
		req.Header.Set("Authorization", "Bearer "+session.Token)
		req.Header.Set("Content-Type", "application/json")
		for k, v := range extraHeaders {
			req.Header.Set(k, v)
		}
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)
		return rr
	}

	// 2. GET /api/v1/organizations
	rr := doAuthReq("GET", "/api/v1/organizations", nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/organizations failed with code %d: %s", rr.Code, rr.Body.String())
	}
	var orgsResp struct {
		Organizations []domain.Organization `json:"organizations"`
		Count         int                   `json:"count"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &orgsResp)
	if orgsResp.Count == 0 {
		t.Fatalf("Expected default organization to be listed, got 0")
	}

	// 3. POST /api/v1/organizations (Create new org)
	createOrgReq := map[string]string{
		"name": "Beta Labs Inc",
		"slug": "beta-labs",
	}
	rr = doAuthReq("POST", "/api/v1/organizations", createOrgReq, nil)
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/organizations failed with code %d: %s", rr.Code, rr.Body.String())
	}
	var newOrg domain.Organization
	_ = json.Unmarshal(rr.Body.Bytes(), &newOrg)

	// 4. POST /api/v1/projects (Create new project in new org)
	createProjReq := map[string]string{
		"organization_id": newOrg.ID,
		"name":            "Beta Microservice A",
		"slug":            "beta-microservice-a",
	}
	rr = doAuthReq("POST", "/api/v1/projects", createProjReq, nil)
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/projects failed with code %d: %s", rr.Code, rr.Body.String())
	}
	var newProj domain.Project
	_ = json.Unmarshal(rr.Body.Bytes(), &newProj)

	// 5. POST /api/v1/flags with X-Project-ID header to create flag in new project
	createFlagReq := domain.FeatureFlag{
		Key:         "beta-test-flag",
		Name:        "Beta Feature Test Flag",
		Type:        "boolean",
		Description: "A flag inside beta project",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:  true,
				Strategy: domain.StrategyBoolean,
			},
		},
	}
	rr = doAuthReq("POST", "/api/v1/flags", createFlagReq, map[string]string{
		"X-Project-ID": newProj.ID,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/flags failed with code %d: %s", rr.Code, rr.Body.String())
	}

	// 6. GET /api/v1/flags with X-Project-ID should return 1 flag
	rr = doAuthReq("GET", "/api/v1/flags", nil, map[string]string{
		"X-Project-ID": newProj.ID,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/flags with X-Project-ID failed: %d", rr.Code)
	}
	var flagsResp struct {
		Flags []domain.FeatureFlag `json:"flags"`
		Count int                  `json:"count"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &flagsResp)
	if flagsResp.Count != 1 || flagsResp.Flags[0].Key != "beta-test-flag" {
		t.Fatalf("Expected 1 flag 'beta-test-flag' for new project, got %d", flagsResp.Count)
	}

	// 7. GET /api/v1/flags without X-Project-ID should return default project flags (not beta-test-flag)
	rr = doAuthReq("GET", "/api/v1/flags", nil, nil)
	_ = json.Unmarshal(rr.Body.Bytes(), &flagsResp)
	for _, f := range flagsResp.Flags {
		if f.Key == "beta-test-flag" {
			t.Fatalf("ISOLATION BREACH: beta-test-flag leaked into default project list!")
		}
	}

	// 8. POST /api/v1/evaluate with X-Project-ID evaluates beta-test-flag as enabled
	evalReq := map[string]interface{}{
		"flags": []string{"beta-test-flag"},
		"context": domain.EvaluationContext{
			UserID:      "test-user-123",
			Environment: domain.EnvProduction,
		},
	}
	rr = doAuthReq("POST", "/api/v1/evaluate", evalReq, map[string]string{
		"X-Project-ID": newProj.ID,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/evaluate failed: %d", rr.Code)
	}
	var evalResp domain.BatchEvaluationResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &evalResp)
	res, ok := evalResp.Results["beta-test-flag"]
	if !ok || !res.Enabled {
		t.Fatalf("Expected beta-test-flag to evaluate to enabled in new project, got: %+v", res)
	}

	// 9. POST /api/v1/projects/active switches active project
	switchReq := map[string]string{
		"project_id": newProj.ID,
	}
	rr = doAuthReq("POST", "/api/v1/projects/active", switchReq, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/projects/active failed: %d", rr.Code)
	}
}

func TestMultiTenantUserSignUpIsolation(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 1. Sign up User A
	userAPayload := []byte(`{"email":"alice@acme.com","password":"Password123!","name":"Alice Acme"}`)
	reqA := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewReader(userAPayload))
	reqA.Header.Set("Content-Type", "application/json")
	recA := httptest.NewRecorder()
	server.ServeHTTP(recA, reqA)

	if recA.Code != http.StatusCreated {
		t.Fatalf("User A signup failed: %d: %s", recA.Code, recA.Body.String())
	}

	// Extract User A's project cookie
	var userAProjectID string
	for _, c := range recA.Result().Cookies() {
		if c.Name == domain.CookieProjectName {
			userAProjectID = c.Value
		}
	}
	if userAProjectID == "" || userAProjectID == domain.DefaultProjectID {
		t.Fatalf("Expected User A to receive unique default project ID, got: %q", userAProjectID)
	}

	// 2. Sign up User B
	userBPayload := []byte(`{"email":"bob@stark.com","password":"Password123!","name":"Bob Stark"}`)
	reqB := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewReader(userBPayload))
	reqB.Header.Set("Content-Type", "application/json")
	recB := httptest.NewRecorder()
	server.ServeHTTP(recB, reqB)

	if recB.Code != http.StatusCreated {
		t.Fatalf("User B signup failed: %d: %s", recB.Code, recB.Body.String())
	}

	// Extract User B's project cookie
	var userBProjectID string
	for _, c := range recB.Result().Cookies() {
		if c.Name == domain.CookieProjectName {
			userBProjectID = c.Value
		}
	}
	if userBProjectID == "" || userBProjectID == domain.DefaultProjectID {
		t.Fatalf("Expected User B to receive unique default project ID, got: %q", userBProjectID)
	}

	// 3. User A and User B MUST have distinct project IDs (zero tenant collision)
	if userAProjectID == userBProjectID {
		t.Fatalf("MULTI-TENANT COLLISION: User A and User B share the same project ID: %s", userAProjectID)
	}
}

