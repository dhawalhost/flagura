package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func getAdminAuthCookie(t *testing.T, memStore store.Store) *http.Cookie {
	ctx := context.Background()
	user := domain.NewUser("admin.suite@flagura.dev", "Admin Tester", "hashed_pwd", domain.RoleAdmin)
	createdUser, err := memStore.CreateUser(ctx, user)
	if err != nil {
		createdUser, _ = memStore.GetUserByEmail(ctx, "admin.suite@flagura.dev")
	}
	org, err := memStore.GetOrganization(ctx, "org_suite")
	if err != nil || org == nil {
		org, _ = memStore.CreateOrganization(ctx, domain.Organization{ID: "org_suite", Name: "Suite Org"})
	}
	if org != nil && createdUser != nil {
		_, _ = memStore.CreateOrgMember(ctx, domain.OrgMember{OrganizationID: org.ID, UserID: createdUser.ID, Role: string(domain.RoleAdmin)})
		_, _ = memStore.CreateProject(ctx, domain.Project{ID: domain.DefaultProjectID, OrganizationID: org.ID, Name: "Default Project"})
	}

	token, _ := generateSessionToken()
	session := domain.Session{
		Token:     token,
		UserID:    createdUser.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}
	if err := memStore.CreateSession(ctx, session); err != nil {
		t.Fatalf("failed to create admin session: %v", err)
	}

	return &http.Cookie{
		Name:     "flagura_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
	}
}

func TestHandlers_ComprehensiveSuite(t *testing.T) {
	memStore := store.NewMemoryStore()
	srv, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	authCookie := getAdminAuthCookie(t, memStore)

	// 1. handleUpdateFlag & handleDeleteFlag
	t.Run("UpdateAndDeleteFlag", func(t *testing.T) {
		initialFlag := domain.FeatureFlag{
			ID:        "flag_update_test",
			ProjectID: domain.DefaultProjectID,
			Key:       "ai-smart-search",
			Name:      "AI Smart Search",
			Type:      "boolean",
			Environments: map[domain.Environment]domain.EnvironmentConfig{
				domain.EnvProduction: {Enabled: true, Percentage: 50},
			},
		}
		_, _ = memStore.SaveFlag(context.Background(), initialFlag, "admin.suite@flagura.dev")

		updatePayload := domain.FeatureFlag{
			ID:          "flag_update_test",
			ProjectID:   domain.DefaultProjectID,
			Key:         "ai-smart-search",
			Name:        "AI Smart Search Updated",
			Description: "Updated description",
			Type:        "boolean",
			Environments: map[domain.Environment]domain.EnvironmentConfig{
				domain.EnvProduction: {
					Enabled:    true,
					Percentage: 100,
				},
			},
		}
		b, _ := json.Marshal(updatePayload)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/flags/ai-smart-search", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(authCookie)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("handleUpdateFlag returned status %d: %s", w.Code, w.Body.String())
		}

		// Delete flag
		delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/flags/ai-smart-search", nil)
		delReq.AddCookie(authCookie)
		delW := httptest.NewRecorder()
		srv.ServeHTTP(delW, delReq)

		if delW.Code != http.StatusOK {
			t.Fatalf("handleDeleteFlag returned status %d: %s", delW.Code, delW.Body.String())
		}
	})

	// 2. handleBenchmark & handleGetAuditLogs
	t.Run("BenchmarkAndAuditLogs", func(t *testing.T) {
		benchFlag := domain.FeatureFlag{
			ID:        "flag_bench_test",
			ProjectID: domain.DefaultProjectID,
			Key:       "rate-limiter-v2",
			Name:      "Rate Limiter",
			Type:      "boolean",
			Environments: map[domain.Environment]domain.EnvironmentConfig{
				domain.EnvProduction: {Enabled: true, Strategy: domain.StrategyBoolean},
			},
		}
		_, _ = memStore.SaveFlag(context.Background(), benchFlag, "admin.suite@flagura.dev")

		benchReq := httptest.NewRequest(http.MethodPost, "/api/v1/benchmark?flag=rate-limiter-v2&iterations=500", nil)
		benchReq.AddCookie(authCookie)
		benchW := httptest.NewRecorder()
		srv.ServeHTTP(benchW, benchReq)

		if benchW.Code != http.StatusOK {
			t.Fatalf("handleBenchmark returned status %d: %s", benchW.Code, benchW.Body.String())
		}

		auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?limit=5", nil)
		auditReq.AddCookie(authCookie)
		auditW := httptest.NewRecorder()
		srv.ServeHTTP(auditW, auditReq)

		if auditW.Code != http.StatusOK {
			t.Fatalf("handleGetAuditLogs returned status %d: %s", auditW.Code, auditW.Body.String())
		}
	})

	// 3. Canary Routes: handleDeleteCanary
	t.Run("CanaryDelete", func(t *testing.T) {
		delCanaryReq := httptest.NewRequest(http.MethodDelete, "/api/v1/flags/non-existent-flag/canary", nil)
		delCanaryReq.AddCookie(authCookie)
		delCanaryW := httptest.NewRecorder()
		srv.ServeHTTP(delCanaryW, delCanaryReq)

		if delCanaryW.Code != http.StatusNotFound {
			t.Errorf("expected 404 on deleting non-existent canary, got %d", delCanaryW.Code)
		}
	})

	// 4. Multi-Tenancy: handleListProjects & handleGetProject
	t.Run("ProjectsAndOrganizations", func(t *testing.T) {
		org, _ := memStore.CreateOrganization(context.Background(), domain.Organization{Name: "Default Org", Slug: "default-org"})
		proj, _ := memStore.CreateProject(context.Background(), domain.Project{OrganizationID: org.ID, Name: "Default Proj", Slug: "default-proj"})

		listProjReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
		listProjReq.AddCookie(authCookie)
		listProjW := httptest.NewRecorder()
		srv.ServeHTTP(listProjW, listProjReq)

		if listProjW.Code != http.StatusOK {
			t.Fatalf("handleListProjects returned status %d: %s", listProjW.Code, listProjW.Body.String())
		}

		getProjReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+proj.ID, nil)
		getProjReq.AddCookie(authCookie)
		getProjW := httptest.NewRecorder()
		srv.ServeHTTP(getProjW, getProjReq)

		if getProjW.Code != http.StatusOK {
			t.Fatalf("handleGetProject returned status %d: %s", getProjW.Code, getProjW.Body.String())
		}

		listOrgsReq := httptest.NewRequest(http.MethodGet, "/api/v1/organizations", nil)
		listOrgsReq.AddCookie(authCookie)
		listOrgsW := httptest.NewRecorder()
		srv.ServeHTTP(listOrgsW, listOrgsReq)

		if listOrgsW.Code != http.StatusOK {
			t.Fatalf("handleListOrganizations returned status %d: %s", listOrgsW.Code, listOrgsW.Body.String())
		}
	})

	// 5. UI Page routes (Landing, Auth, Dashboard)
	t.Run("UIPages", func(t *testing.T) {
		landingReq := httptest.NewRequest(http.MethodGet, "/", nil)
		landingW := httptest.NewRecorder()
		srv.ServeHTTP(landingW, landingReq)

		if landingW.Code != http.StatusSeeOther && landingW.Code != http.StatusOK {
			t.Fatalf("handleLanding returned status %d", landingW.Code)
		}

		authReq := httptest.NewRequest(http.MethodGet, "/auth", nil)
		authW := httptest.NewRecorder()
		srv.ServeHTTP(authW, authReq)

		if authW.Code != http.StatusOK {
			t.Fatalf("handleAuth returned status %d", authW.Code)
		}
	})

	// 6. Context and Middleware helpers
	t.Run("ContextAndMiddlewareHelpers", func(t *testing.T) {
		ctx := context.Background()

		// APIKey context
		apiKey := &domain.APIKey{ID: "key_123"}
		ctxWithKey := WithAPIKeyContext(ctx, apiKey)
		if gotKey := APIKeyFromContext(ctxWithKey); gotKey != apiKey {
			t.Errorf("APIKeyFromContext mismatch")
		}

		// Project context
		projID := "proj_123"
		ctxWithProj := WithProjectContext(ctx, projID)
		if gotProj := ProjectFromContext(ctxWithProj); gotProj != projID {
			t.Errorf("ProjectFromContext mismatch")
		}

		// RequireRole middleware
		roleHandler := srv.RequireRole(domain.RoleAdmin, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Viewer role attempting admin route -> 403 Forbidden
		viewerUser := &domain.User{ID: "u_viewer", Role: domain.RoleViewer}
		reqViewer := httptest.NewRequest(http.MethodGet, "/admin", nil)
		reqViewer = reqViewer.WithContext(WithUserContext(reqViewer.Context(), viewerUser))
		wViewer := httptest.NewRecorder()
		roleHandler.ServeHTTP(wViewer, reqViewer)

		if wViewer.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for viewer role on admin route, got %d", wViewer.Code)
		}
	})

	// 7. StreamHub methods
	t.Run("StreamHubMethods", func(t *testing.T) {
		hub := NewStreamHub()
		go hub.Run()
		time.Sleep(10 * time.Millisecond)
		defer hub.Close()

		if hub.ActiveConnections() != 0 {
			t.Errorf("expected 0 active connections")
		}
		hub.BroadcastFlags([]domain.FeatureFlag{{Key: "ai-smart-search"}})
	})

	// 8. Change Requests Flow
	t.Run("ChangeRequestsWorkflow", func(t *testing.T) {
		crPayload := domain.ChangeRequest{
			FlagKey:     "rate-limiter-v2",
			Environment: domain.EnvProduction,
			Title:       "Enable Rate Limiter V2 in Production",
			Description: "Approved per RFC-889",
			ProposedConfig: domain.EnvironmentConfig{
				Enabled:    true,
				Percentage: 100,
			},
		}
		b, _ := json.Marshal(crPayload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/change-requests", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(authCookie)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("handleCreateChangeRequest failed: %d (%s)", w.Code, w.Body.String())
		}

		var createdCR domain.ChangeRequest
		_ = json.Unmarshal(w.Body.Bytes(), &createdCR)

		// List CRs
		listReq := httptest.NewRequest(http.MethodGet, "/api/v1/change-requests?status=PENDING", nil)
		listReq.AddCookie(authCookie)
		listW := httptest.NewRecorder()
		srv.ServeHTTP(listW, listReq)

		if listW.Code != http.StatusOK {
			t.Fatalf("handleListChangeRequests failed: %d", listW.Code)
		}

		// Get CR
		getReq := httptest.NewRequest(http.MethodGet, "/api/v1/change-requests/"+createdCR.ID, nil)
		getReq.AddCookie(authCookie)
		getW := httptest.NewRecorder()
		srv.ServeHTTP(getW, getReq)

		if getW.Code != http.StatusOK {
			t.Fatalf("handleGetChangeRequest failed: %d", getW.Code)
		}
	})

	// 9. API Keys Management
	t.Run("APIKeysManagement", func(t *testing.T) {
		keyPayload := map[string]interface{}{
			"name":        "Test Automation Key",
			"environment": "production",
			"role":        "developer",
		}
		b, _ := json.Marshal(keyPayload)
		createReq := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", bytes.NewReader(b))
		createReq.Header.Set("Content-Type", "application/json")
		createReq.AddCookie(authCookie)
		createW := httptest.NewRecorder()
		srv.ServeHTTP(createW, createReq)

		if createW.Code != http.StatusCreated {
			t.Fatalf("handleCreateAPIKey failed: %d (%s)", createW.Code, createW.Body.String())
		}

		var resp struct {
			Key domain.APIKey `json:"key"`
		}
		_ = json.Unmarshal(createW.Body.Bytes(), &resp)

		// List API Keys
		listReq := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil)
		listReq.AddCookie(authCookie)
		listW := httptest.NewRecorder()
		srv.ServeHTTP(listW, listReq)

		if listW.Code != http.StatusOK {
			t.Fatalf("handleListAPIKeys failed: %d", listW.Code)
		}

		// Revoke API Key
		if resp.Key.ID != "" {
			delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/api-keys/"+resp.Key.ID, nil)
			delReq.AddCookie(authCookie)
			delW := httptest.NewRecorder()
			srv.ServeHTTP(delW, delReq)

			if delW.Code != http.StatusOK {
				t.Fatalf("handleRevokeAPIKey failed: %d", delW.Code)
			}
		}
	})

	// 10. Canary Scheduling & Rollback
	t.Run("CanarySchedulingAndRollback", func(t *testing.T) {
		_, _ = memStore.SaveFlag(context.Background(), domain.FeatureFlag{
			ID:        "flag_canary_api_test",
			ProjectID: domain.DefaultProjectID,
			Key:       "canary-api-flag",
			Name:      "Canary API Flag",
			Type:      "boolean",
			Environments: map[domain.Environment]domain.EnvironmentConfig{
				domain.EnvProduction: {
					Enabled:    true,
					Strategy:   domain.StrategyPercentage,
					Percentage: 0,
				},
			},
		}, "test")

		schedPayload := domain.CanarySchedule{
			FlagKey:     "canary-api-flag",
			Environment: domain.EnvProduction,
			Stages: []domain.CanaryStage{
				{Index: 0, TargetPercentage: 10, DurationSec: 60},
				{Index: 1, TargetPercentage: 100, DurationSec: 120},
			},
		}
		b, _ := json.Marshal(schedPayload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/flags/canary-api-flag/canary", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(authCookie)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK && w.Code != http.StatusCreated {
			t.Fatalf("handleCreateCanary failed: %d (%s)", w.Code, w.Body.String())
		}

		// Get Canary
		getReq := httptest.NewRequest(http.MethodGet, "/api/v1/flags/canary-api-flag/canary", nil)
		getReq.AddCookie(authCookie)
		getW := httptest.NewRecorder()
		srv.ServeHTTP(getW, getReq)

		if getW.Code != http.StatusOK {
			t.Fatalf("handleGetCanary failed: %d", getW.Code)
		}

		// Rollback Canary
		rbPayload := map[string]string{"reason": "APM spike"}
		rbBytes, _ := json.Marshal(rbPayload)
		rbReq := httptest.NewRequest(http.MethodPost, "/api/v1/flags/canary-api-flag/canary/rollback", bytes.NewReader(rbBytes))
		rbReq.Header.Set("Content-Type", "application/json")
		rbReq.AddCookie(authCookie)
		rbW := httptest.NewRecorder()
		srv.ServeHTTP(rbW, rbReq)

		if rbW.Code != http.StatusOK {
			t.Fatalf("handleCanaryRollback failed: %d (%s)", rbW.Code, rbW.Body.String())
		}
	})

	// 11. Projects & Organizations Creation & Switch
	t.Run("CreateOrgAndProject", func(t *testing.T) {
		orgPayload := map[string]string{
			"name": "Global Corp",
			"slug": "global-corp",
		}
		b, _ := json.Marshal(orgPayload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(authCookie)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("handleCreateOrganization failed: %d (%s)", w.Code, w.Body.String())
		}

		var createdOrg domain.Organization
		_ = json.Unmarshal(w.Body.Bytes(), &createdOrg)

		// Create Project
		projPayload := map[string]string{
			"name":           "Search Microservice",
			"slug":           "search-microservice",
			"organizationId": createdOrg.ID,
		}
		bProj, _ := json.Marshal(projPayload)
		projReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(bProj))
		projReq.Header.Set("Content-Type", "application/json")
		projReq.AddCookie(authCookie)
		projW := httptest.NewRecorder()
		srv.ServeHTTP(projW, projReq)

		if projW.Code != http.StatusCreated {
			t.Fatalf("handleCreateProject failed: %d (%s)", projW.Code, projW.Body.String())
		}

		var createdProj domain.Project
		_ = json.Unmarshal(projW.Body.Bytes(), &createdProj)

		// Switch Active Project
		switchPayload := map[string]string{"project_id": createdProj.ID}
		bSwitch, _ := json.Marshal(switchPayload)
		swReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/active", bytes.NewReader(bSwitch))
		swReq.Header.Set("Content-Type", "application/json")
		swReq.AddCookie(authCookie)
		swW := httptest.NewRecorder()
		srv.ServeHTTP(swW, swReq)

		if swW.Code != http.StatusOK {
			t.Fatalf("handleSwitchActiveProject failed: %d", swW.Code)
		}
	})

	// 12. Experiment Events & Report
	t.Run("ExperimentEventsAndReport", func(t *testing.T) {
		eventsPayload := map[string]interface{}{
			"events": []domain.ExperimentEvent{
				{
					FlagKey:     "canary-api-flag",
					Variant:     "treatment",
					MetricName:  "checkout_success",
					EventType:   domain.EventTypeConversion,
					Value:       1.0,
					UserID:      "usr_e2e_99",
					Environment: domain.EnvProduction,
				},
			},
		}
		b, _ := json.Marshal(eventsPayload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK && w.Code != http.StatusAccepted {
			t.Fatalf("handleIngestEvents failed: %d (%s)", w.Code, w.Body.String())
		}

		// Get Experiment Report
		repReq := httptest.NewRequest(http.MethodGet, "/api/v1/experiments/canary-api-flag?metric=checkout_success", nil)
		repReq.AddCookie(authCookie)
		repW := httptest.NewRecorder()
		srv.ServeHTTP(repW, repReq)

		if repW.Code != http.StatusOK {
			t.Fatalf("handleGetExperimentReport failed: %d", repW.Code)
		}
	})

	// 13. Auth Endpoints: Logout, Me, Forgot Password
	t.Run("AuthEndpoints", func(t *testing.T) {
		// Me
		meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		meReq.AddCookie(authCookie)
		meW := httptest.NewRecorder()
		srv.ServeHTTP(meW, meReq)

		if meW.Code != http.StatusOK {
			t.Fatalf("handleMe failed: %d", meW.Code)
		}

		// Forgot password (returns 400 when mailer is disabled)
		forgotPayload := map[string]string{"email": "admin.suite@flagura.dev"}
		bForgot, _ := json.Marshal(forgotPayload)
		forgotReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", bytes.NewReader(bForgot))
		forgotReq.Header.Set("Content-Type", "application/json")
		forgotW := httptest.NewRecorder()
		srv.ServeHTTP(forgotW, forgotReq)

		if forgotW.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 on forgot-password without mailer, got: %d", forgotW.Code)
		}

		// Logout
		logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		logoutReq.AddCookie(authCookie)
		logoutW := httptest.NewRecorder()
		srv.ServeHTTP(logoutW, logoutReq)

		if logoutW.Code != http.StatusOK {
			t.Fatalf("handleLogout failed: %d", logoutW.Code)
		}
	})

	// 14. Invitations & Memberships
	t.Run("InvitationsAndMembership", func(t *testing.T) {
		invAuthCookie := getAdminAuthCookie(t, memStore)
		org, _ := memStore.CreateOrganization(context.Background(), domain.Organization{Name: "Collab Org", Slug: "collab-org"})

		// Create invitation
		invPayload := map[string]string{
			"organization_id": org.ID,
			"email":           "colleague@flagura.dev",
			"role":            "developer",
		}
		bInv, _ := json.Marshal(invPayload)
		invReq := httptest.NewRequest(http.MethodPost, "/api/v1/invitations", bytes.NewReader(bInv))
		invReq.Header.Set("Content-Type", "application/json")
		invReq.AddCookie(invAuthCookie)
		invW := httptest.NewRecorder()
		srv.ServeHTTP(invW, invReq)

		if invW.Code != http.StatusCreated {
			t.Fatalf("handleInvitations POST failed: %d (%s)", invW.Code, invW.Body.String())
		}

		var invRes struct {
			Invitation domain.OrgInvitation `json:"invitation"`
			InviteURL  string               `json:"invite_url"`
		}
		_ = json.Unmarshal(invW.Body.Bytes(), &invRes)

		// Get invitation by token
		getInvReq := httptest.NewRequest(http.MethodGet, "/api/v1/invitations/"+invRes.Invitation.Token, nil)
		getInvW := httptest.NewRecorder()
		srv.ServeHTTP(getInvW, getInvReq)

		if getInvW.Code != http.StatusOK {
			t.Fatalf("handleGetInvitationByToken failed: %d", getInvW.Code)
		}

		// Accept invitation
		acceptPayload := map[string]string{"token": invRes.Invitation.Token}
		bAccept, _ := json.Marshal(acceptPayload)
		acceptReq := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/accept", bytes.NewReader(bAccept))
		acceptReq.Header.Set("Content-Type", "application/json")
		acceptReq.AddCookie(invAuthCookie)
		acceptW := httptest.NewRecorder()
		srv.ServeHTTP(acceptW, acceptReq)

		if acceptW.Code != http.StatusOK {
			t.Fatalf("handleAcceptInvitation failed: %d (%s)", acceptW.Code, acceptW.Body.String())
		}
	})
}
