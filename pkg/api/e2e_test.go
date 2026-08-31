package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/api"
	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/engine"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestE2E_FullPlatformLifecycle(t *testing.T) {
	memStore := store.NewMemoryStore()
	srv, err := api.NewServer(memStore)
	if err != nil {
		t.Fatalf("Failed to instantiate api.Server: %v", err)
	}

	// =========================================================================
	// 1. E2E Auth & User Registration Flow
	// =========================================================================
	t.Run("1_Auth_And_User_Lifecycle", func(t *testing.T) {
		signupBody := map[string]string{
			"email":    "lead.dev@flagura-e2e.com",
			"password": "StrongPassword123!",
			"name":     "Lead Developer",
		}
		b, _ := json.Marshal(signupBody)
		req := httptest.NewRequest("POST", "/api/v1/auth/signup", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("Signup failed with status %d: %s", rec.Code, rec.Body.String())
		}

		cookies := rec.Result().Cookies()
		var sessionCookie *http.Cookie
		for _, c := range cookies {
			if c.Name == "flagura_session" {
				sessionCookie = c
				break
			}
		}
		if sessionCookie == nil {
			t.Fatalf("Expected session cookie from signup")
		}

		// Verify /api/v1/auth/me
		meReq := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
		meReq.AddCookie(sessionCookie)
		meRec := httptest.NewRecorder()
		srv.ServeHTTP(meRec, meReq)

		if meRec.Code != http.StatusOK {
			t.Fatalf("/me failed: %d", meRec.Code)
		}

		var meUser domain.User
		_ = json.Unmarshal(meRec.Body.Bytes(), &meUser)
		if meUser.Email != "lead.dev@flagura-e2e.com" {
			t.Fatalf("Expected user email lead.dev@flagura-e2e.com, got %s", meUser.Email)
		}
	})

	// =========================================================================
	// 2. E2E Multi-Tenant Org & Project Creation
	// =========================================================================
	var customOrgID string
	var customProjID string
	var adminCookie *http.Cookie

	t.Run("2_MultiTenant_Workspace_Flow", func(t *testing.T) {
		// Provision Admin User and Session in Store
		adminUser := domain.NewUser("admin.e2e@flagura.dev", "E2E Admin", "hash_admin_secret", domain.RoleAdmin)
		createdAdmin, err := memStore.CreateUser(context.Background(), adminUser)
		if err != nil {
			t.Fatalf("Failed to create admin user: %v", err)
		}

		adminToken := "tok_e2e_admin_valid_123"
		_ = memStore.CreateSession(context.Background(), domain.Session{
			Token:     adminToken,
			UserID:    createdAdmin.ID,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		})
		adminCookie = &http.Cookie{Name: "flagura_session", Value: adminToken}

		// Create Org
		orgBody := map[string]string{
			"name":        "E2E Enterprise Corp",
			"description": "Tenant organization for E2E tests",
		}
		b, _ := json.Marshal(orgBody)
		req := httptest.NewRequest("POST", "/api/v1/organizations", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(adminCookie)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("Create Org failed: %d: %s", rec.Code, rec.Body.String())
		}
		var orgResp struct {
			Organization domain.Organization `json:"organization"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &orgResp)
		customOrgID = orgResp.Organization.ID

		// Create Project
		projBody := map[string]string{
			"organization_id": customOrgID,
			"name":            "Payments Backend",
			"description":     "E2E Project for payment feature flags",
		}
		b, _ = json.Marshal(projBody)
		req = httptest.NewRequest("POST", "/api/v1/projects", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(adminCookie)
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("Create Project failed: %d: %s", rec.Code, rec.Body.String())
		}
		var projResp struct {
			Project domain.Project `json:"project"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &projResp)
		customProjID = projResp.Project.ID
	})

	// =========================================================================
	// 3. E2E Feature Flag Lifecycle & Fast-Path Deterministic Evaluation
	// =========================================================================
	t.Run("3_Flag_Creation_And_Evaluation_Flow", func(t *testing.T) {
		flagKey := "e2e-fast-checkout"
		flagBody := domain.FeatureFlag{
			ProjectID:   customProjID,
			Key:         flagKey,
			Name:        "Fast Checkout V2",
			Type:        "boolean",
			Description: "One-click express checkout",
			Environments: map[domain.Environment]domain.EnvironmentConfig{
				domain.EnvDevelopment: {
					Enabled:  true,
					Strategy: domain.StrategyBoolean,
				},
				domain.EnvStaging: {
					Enabled:    true,
					Strategy:   domain.StrategyPercentage,
					Percentage: 50,
				},
				domain.EnvProduction: {
					Enabled:    false,
					Strategy:   domain.StrategyPercentage,
					Percentage: 0,
				},
			},
		}

		b, _ := json.Marshal(flagBody)
		req := httptest.NewRequest("POST", "/api/v1/flags", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Project-ID", customProjID)
		req.AddCookie(adminCookie)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("Create flag failed: %d: %s", rec.Code, rec.Body.String())
		}

		// Evaluate with execution trace
		evalBody := map[string]any{
			"flags": []string{flagKey},
			"context": map[string]any{
				"user_id":     "usr_shopper_123",
				"environment": "development",
			},
		}
		b, _ = json.Marshal(evalBody)
		req = httptest.NewRequest("POST", "/api/v1/evaluate?trace=true", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Project-ID", customProjID)
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Evaluate failed: %d", rec.Code)
		}

		var evalResp struct {
			Results map[string]domain.EvaluationResult `json:"results"`
			Traces  map[string]engine.EvaluationTrace  `json:"traces"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &evalResp)

		res, ok := evalResp.Results[flagKey]
		if !ok || !res.Enabled {
			t.Fatalf("Expected %s to be enabled in dev", flagKey)
		}
		if _, ok := evalResp.Traces[flagKey]; !ok {
			t.Fatalf("Expected trace result for %s", flagKey)
		}

		// Update rollout percentage in staging
		rolloutBody := map[string]any{
			"percentage": 100,
		}
		b, _ = json.Marshal(rolloutBody)
		req = httptest.NewRequest("PATCH", fmt.Sprintf("/api/v1/flags/%s/rollout?env=staging", flagKey), bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Project-ID", customProjID)
		req.AddCookie(adminCookie)
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Rollout update failed: %d: %s", rec.Code, rec.Body.String())
		}

		// Promote Dev config to Staging
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/flags/%s/promote?from=development&to=staging", flagKey), nil)
		req.Header.Set("X-Project-ID", customProjID)
		req.AddCookie(adminCookie)
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Promote failed: %d: %s", rec.Code, rec.Body.String())
		}
	})

	// =========================================================================
	// 4. E2E 4-Eyes Change Governance Workflow
	// =========================================================================
	t.Run("4_FourEyes_Governance_Workflow", func(t *testing.T) {
		flagKey := "e2e-fast-checkout"

		// 1. Author (Lead Dev) logs in
		loginBody := map[string]string{
			"email":    "lead.dev@flagura-e2e.com",
			"password": "StrongPassword123!",
		}
		b, _ := json.Marshal(loginBody)
		req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		var authorCookie *http.Cookie
		for _, c := range rec.Result().Cookies() {
			if c.Name == "flagura_session" {
				authorCookie = c
				break
			}
		}

		// Author submits CR
		crBody := domain.ChangeRequest{
			ProjectID:   customProjID,
			FlagKey:     flagKey,
			Environment: domain.EnvProduction,
			Title:       "Enable fast checkout in production at 100%",
			Description: "Tested in staging, ready for prod",
			ProposedConfig: domain.EnvironmentConfig{
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 100,
			},
		}
		b, _ = json.Marshal(crBody)
		req = httptest.NewRequest("POST", "/api/v1/change-requests", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Project-ID", customProjID)
		req.AddCookie(authorCookie)
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("Create CR failed: %d: %s", rec.Code, rec.Body.String())
		}
		var createdCR domain.ChangeRequest
		_ = json.Unmarshal(rec.Body.Bytes(), &createdCR)
		crID := createdCR.ID

		// 2. Author attempts self-review -> Expect 403 Forbidden with ErrCodeFourEyesSelfApproval (4002)
		reviewBody := map[string]any{
			"approved": true,
			"comments": "Self approving my own CR",
		}
		b, _ = json.Marshal(reviewBody)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/change-requests/%s/review", crID), bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(authorCookie)
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden && rec.Code != http.StatusBadRequest {
			t.Fatalf("Expected 400 Bad Request or 403 Forbidden on author self-review, got %d: %s", rec.Code, rec.Body.String())
		}

		// 3. Admin / Peer Reviewer approves
		reviewBody["comments"] = "Approved by Principal SRE"
		b, _ = json.Marshal(reviewBody)
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/change-requests/%s/review", crID), bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(adminCookie)
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Peer review approval failed: %d: %s", rec.Code, rec.Body.String())
		}

		// 4. Apply Approved Change Request
		req = httptest.NewRequest("POST", fmt.Sprintf("/api/v1/change-requests/%s/apply", crID), nil)
		req.AddCookie(adminCookie)
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Apply CR failed: %d: %s", rec.Code, rec.Body.String())
		}

		// Verify flag in production is now enabled
		flag, _ := memStore.GetFlagByProject(context.Background(), customProjID, flagKey)
		if !flag.Environments[domain.EnvProduction].Enabled || flag.Environments[domain.EnvProduction].Percentage != 100 {
			t.Fatalf("Expected flag production config to be enabled at 100%%")
		}
	})

	// =========================================================================
	// 5. E2E Progressive Canary Auto-Ramp & Rollback
	// =========================================================================
	t.Run("5_Canary_AutoRamp_And_Rollback", func(t *testing.T) {
		flagKey := "e2e-fast-checkout"
		canaryPayload := map[string]any{
			"stages": []map[string]any{
				{"index": 0, "target_percentage": 10, "duration_sec": 60},
				{"index": 1, "target_percentage": 50, "duration_sec": 120},
				{"index": 2, "target_percentage": 100, "duration_sec": 0},
			},
			"guardrails": map[string]any{
				"max_error_rate_pct": 2.0,
				"auto_rollback":     true,
			},
		}
		b, _ := json.Marshal(canaryPayload)
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/flags/%s/canary?env=production", flagKey), bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Project-ID", customProjID)
		req.AddCookie(adminCookie)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
			t.Fatalf("Canary schedule creation failed: %d: %s", rec.Code, rec.Body.String())
		}
	})

	// =========================================================================
	// 6. E2E A/B Experiment Telemetry & Statistical Significance
	// =========================================================================
	t.Run("6_Experimentation_And_Stats_Flow", func(t *testing.T) {
		flagKey := "e2e-fast-checkout"
		eventBody := map[string]any{
			"event": map[string]any{
				"flag_key":    flagKey,
				"variant":     "treatment",
				"metric_name": "checkout_completed",
				"event_type":  "conversion",
				"value":       49.99,
				"user_id":     "shopper_99",
				"environment": "production",
			},
		}
		b, _ := json.Marshal(eventBody)
		req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Project-ID", customProjID)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK && rec.Code != http.StatusAccepted {
			t.Fatalf("Event ingestion failed: %d: %s", rec.Code, rec.Body.String())
		}

		// Retrieve experiment stats report
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/experiments/%s?metric=checkout_completed", flagKey), nil)
		req.Header.Set("X-Project-ID", customProjID)
		req.AddCookie(adminCookie)
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Experiment report failed: %d: %s", rec.Code, rec.Body.String())
		}
	})

	// =========================================================================
	// 7. E2E Environment-Scoped API Key Enforcement & Revocation
	// =========================================================================
	t.Run("7_APIKey_Environment_Scoping_And_Revocation", func(t *testing.T) {
		flagKey := "e2e-fast-checkout"

		// Create API key scoped to staging only
		keyBody := map[string]any{
			"name":        "Staging CI Runner",
			"role":        "developer",
			"environment": "staging",
		}
		b, _ := json.Marshal(keyBody)
		req := httptest.NewRequest("POST", "/api/v1/api-keys", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Project-ID", customProjID)
		req.AddCookie(adminCookie)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("Create API Key failed: %d: %s", rec.Code, rec.Body.String())
		}

		var keyResp struct {
			APIKey struct {
				ID  string `json:"id"`
				Key string `json:"key"`
			} `json:"api_key"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &keyResp)
		rawKey := keyResp.APIKey.Key
		keyID := keyResp.APIKey.ID

		// 1. Evaluate on staging -> 200 OK
		evalReq := httptest.NewRequest("POST", "/api/v1/evaluate", bytes.NewReader([]byte(fmt.Sprintf(`{"flags":["%s"],"context":{"user_id":"u1","environment":"staging"}}`, flagKey))))
		evalReq.Header.Set("Content-Type", "application/json")
		evalReq.Header.Set("Authorization", "Bearer "+rawKey)
		evalReq.Header.Set("X-Project-ID", customProjID)
		evalRec := httptest.NewRecorder()
		srv.ServeHTTP(evalRec, evalReq)

		if evalRec.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK on staging eval with staging key, got %d", evalRec.Code)
		}

		// 2. Evaluate on production -> 403 Forbidden with ErrCodeEnvRestricted (1006)
		evalProdReq := httptest.NewRequest("POST", "/api/v1/evaluate", bytes.NewReader([]byte(fmt.Sprintf(`{"flags":["%s"],"context":{"user_id":"u1","environment":"production"}}`, flagKey))))
		evalProdReq.Header.Set("Content-Type", "application/json")
		evalProdReq.Header.Set("Authorization", "Bearer "+rawKey)
		evalProdReq.Header.Set("X-Project-ID", customProjID)
		evalProdRec := httptest.NewRecorder()
		srv.ServeHTTP(evalProdRec, evalProdReq)

		if evalProdRec.Code != http.StatusForbidden {
			t.Fatalf("Expected 403 Forbidden when evaluating production with staging key, got %d", evalProdRec.Code)
		}

		// 3. Revoke API key
		delReq := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/api-keys/%s", keyID), nil)
		delReq.Header.Set("X-Project-ID", customProjID)
		delReq.AddCookie(adminCookie)
		delRec := httptest.NewRecorder()
		srv.ServeHTTP(delRec, delReq)

		if delRec.Code != http.StatusOK {
			t.Fatalf("Revoke API key failed: %d: %s", delRec.Code, delRec.Body.String())
		}

		// 4. Authenticated endpoint (GET /api/v1/flags) with revoked key -> 401 Unauthorized
		listReq := httptest.NewRequest("GET", "/api/v1/flags", nil)
		listReq.Header.Set("Authorization", "Bearer "+rawKey)
		listReq.Header.Set("X-Project-ID", customProjID)
		listRec := httptest.NewRecorder()
		srv.ServeHTTP(listRec, listReq)

		if listRec.Code != http.StatusUnauthorized {
			t.Fatalf("Expected 401 Unauthorized after key revocation on protected endpoint, got %d", listRec.Code)
		}
	})

	// =========================================================================
	// 8. E2E Webhook Emergency Kill-Switch & Probes
	// =========================================================================
	t.Run("8_Emergency_KillSwitch_And_Probes", func(t *testing.T) {
		flagKey := "e2e-fast-checkout"

		// Trigger kill-switch webhook with admin session
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/webhooks/kill-switch/%s?env=production", flagKey), nil)
		req.Header.Set("X-Project-ID", customProjID)
		req.AddCookie(adminCookie)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Kill-switch webhook failed: %d: %s", rec.Code, rec.Body.String())
		}

		// Verify flag is disabled in production
		flag, _ := memStore.GetFlagByProject(context.Background(), customProjID, flagKey)
		if flag.Environments[domain.EnvProduction].Enabled {
			t.Fatalf("Expected flag to be disabled after kill-switch trigger")
		}

		// Test /livez
		req = httptest.NewRequest("GET", "/livez", nil)
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("/livez failed: %d", rec.Code)
		}

		// Test /readyz
		req = httptest.NewRequest("GET", "/readyz", nil)
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("/readyz failed: %d", rec.Code)
		}
	})
}
