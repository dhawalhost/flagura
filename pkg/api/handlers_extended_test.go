package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestHandlers_ExtendedEdgeCases(t *testing.T) {
	memStore := store.NewMemoryStore()
	srv, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	authCookie := getAdminAuthCookie(t, memStore)

	// Create test org and project
	org, err := memStore.CreateOrganization(context.Background(), domain.Organization{
		Name: "Edge Test Org",
		Slug: "edge-test-org",
	})
	if err != nil {
		t.Fatalf("failed to create test org: %v", err)
	}

	proj, err := memStore.CreateProject(context.Background(), domain.Project{
		OrganizationID: org.ID,
		Name:           "Edge Test Project",
		Slug:           "edge-test-proj",
	})
	if err != nil {
		t.Fatalf("failed to create test proj: %v", err)
	}

	// Create a flag for edge testing
	flagKey := "edge-flag-01"
	_, err = memStore.SaveFlag(context.Background(), domain.FeatureFlag{
		ProjectID:   proj.ID,
		Key:         flagKey,
		Name:        "Edge Flag",
		Type:        "boolean",
		Description: "Flag for edge case tests",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:  false,
				Strategy: domain.StrategyBoolean,
			},
		},
	}, "test-suite")
	if err != nil {
		t.Fatalf("failed to create flag: %v", err)
	}

	tests := []struct {
		name           string
		method         string
		url            string
		body           interface{}
		cookie         *http.Cookie
		headers        map[string]string
		expectedStatus int
	}{
		// Health & Probes
		{
			name:           "Healthz endpoint",
			method:         http.MethodGet,
			url:            "/healthz",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Readyz endpoint",
			method:         http.MethodGet,
			url:            "/readyz",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "API Health endpoint",
			method:         http.MethodGet,
			url:            "/api/health",
			expectedStatus: http.StatusOK,
		},

		// Organization & Project Edges
		{
			name:           "Create Org - Malformed JSON",
			method:         http.MethodPost,
			url:            "/api/v1/organizations",
			body:           "invalid-json",
			cookie:         authCookie,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Create Project - Malformed JSON",
			method:         http.MethodPost,
			url:            "/api/v1/projects",
			body:           "invalid-json",
			cookie:         authCookie,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Get Project - NonExistent",
			method:         http.MethodGet,
			url:            "/api/v1/projects/non_existent_id",
			cookie:         authCookie,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Switch Active Project - Empty body",
			method:         http.MethodPost,
			url:            "/api/v1/projects/active",
			body:           map[string]string{},
			cookie:         authCookie,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Switch Active Project - NonExistent",
			method:         http.MethodPost,
			url:            "/api/v1/projects/active",
			body:           map[string]string{"project_id": "proj_non_existent"},
			cookie:         authCookie,
			expectedStatus: http.StatusNotFound,
		},

		// Flag Mutation Edges
		{
			name:           "Toggle Flag - NonExistent",
			method:         http.MethodPatch,
			url:            "/api/v1/flags/non-existent-flag/toggle",
			body:           map[string]interface{}{"environment": "production", "enabled": true},
			cookie:         authCookie,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Rollout Flag - NonExistent",
			method:         http.MethodPatch,
			url:            "/api/v1/flags/non-existent-flag/rollout",
			body:           map[string]interface{}{"environment": "production", "percentage": 50},
			cookie:         authCookie,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Delete Flag - NonExistent",
			method:         http.MethodDelete,
			url:            "/api/v1/flags/non-existent-flag",
			cookie:         authCookie,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Create Flag - Malformed JSON",
			method:         http.MethodPost,
			url:            "/api/v1/flags",
			body:           "invalid-json-body",
			cookie:         authCookie,
			expectedStatus: http.StatusBadRequest,
		},

		// Canary Edges
		{
			name:           "Get Canary - NonExistent Flag",
			method:         http.MethodGet,
			url:            "/api/v1/flags/non-existent-flag/canary",
			cookie:         authCookie,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Delete Canary - NonExistent Flag",
			method:         http.MethodDelete,
			url:            "/api/v1/flags/non-existent-flag/canary",
			cookie:         authCookie,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Rollback Canary - NonExistent Flag",
			method:         http.MethodPost,
			url:            "/api/v1/flags/non-existent-flag/canary/rollback",
			cookie:         authCookie,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "Create Canary - Malformed JSON",
			method:         http.MethodPost,
			url:            "/api/v1/flags/" + flagKey + "/canary",
			body:           "invalid-json",
			cookie:         authCookie,
			expectedStatus: http.StatusBadRequest,
		},

		// Change Requests Edges
		{
			name:           "Create Change Request - Malformed JSON",
			method:         http.MethodPost,
			url:            "/api/v1/change-requests",
			body:           "invalid-json",
			cookie:         authCookie,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Create Change Request - Missing Fields",
			method:         http.MethodPost,
			url:            "/api/v1/change-requests",
			body:           map[string]interface{}{"title": ""},
			cookie:         authCookie,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Get Change Request - NonExistent",
			method:         http.MethodGet,
			url:            "/api/v1/change-requests/cr_non_existent",
			cookie:         authCookie,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Review Change Request - NonExistent",
			method:         http.MethodPost,
			url:            "/api/v1/change-requests/cr_non_existent/review",
			body:           map[string]interface{}{"approved": true},
			cookie:         authCookie,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Apply Change Request - NonExistent",
			method:         http.MethodPost,
			url:            "/api/v1/change-requests/cr_non_existent/apply",
			body:           map[string]interface{}{},
			cookie:         authCookie,
			expectedStatus: http.StatusBadRequest,
		},

		// API Keys Edges
		{
			name:           "Create API Key - Malformed JSON",
			method:         http.MethodPost,
			url:            "/api/v1/api-keys",
			body:           "invalid-json",
			cookie:         authCookie,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Revoke API Key - NonExistent",
			method:         http.MethodDelete,
			url:            "/api/v1/api-keys/key_non_existent",
			cookie:         authCookie,
			expectedStatus: http.StatusNotFound,
		},

		// Invitations Edges
		{
			name:           "Create Invitation - Malformed JSON",
			method:         http.MethodPost,
			url:            "/api/v1/invitations",
			body:           "invalid-json",
			cookie:         authCookie,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Create Invitation - Missing Email",
			method: http.MethodPost,
			url:    "/api/v1/invitations",
			body: map[string]string{
				"organization_id": org.ID,
				"email":           "",
			},
			cookie:         authCookie,
			expectedStatus: http.StatusCreated,
		},
		{
			name:   "Create Invitation - NonExistent Org",
			method: http.MethodPost,
			url:    "/api/v1/invitations",
			body: map[string]string{
				"organization_id": "org_non_existent",
				"email":           "invitee@flagura.dev",
			},
			cookie:         authCookie,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Get Invitation - NonExistent Token",
			method:         http.MethodGet,
			url:            "/api/v1/invitations/inv_tok_non_existent",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Accept Invitation - Malformed JSON",
			method:         http.MethodPost,
			url:            "/api/v1/invitations/accept",
			body:           "invalid-json",
			cookie:         authCookie,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Accept Invitation - Empty Token",
			method:         http.MethodPost,
			url:            "/api/v1/invitations/accept",
			body:           map[string]string{"token": ""},
			cookie:         authCookie,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Accept Invitation - NonExistent Token",
			method:         http.MethodPost,
			url:            "/api/v1/invitations/accept",
			body:           map[string]string{"token": "inv_tok_non_existent"},
			cookie:         authCookie,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "List Invitations by Org",
			method:         http.MethodGet,
			url:            "/api/v1/invitations?organization_id=" + org.ID,
			cookie:         authCookie,
			expectedStatus: http.StatusOK,
		},

		// Auth Edges
		{
			name:           "Signup - Malformed JSON",
			method:         http.MethodPost,
			url:            "/api/v1/auth/signup",
			body:           "invalid-json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Login - Malformed JSON",
			method:         http.MethodPost,
			url:            "/api/v1/auth/login",
			body:           "invalid-json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Login - Invalid Credentials",
			method:         http.MethodPost,
			url:            "/api/v1/auth/login",
			body:           map[string]string{"email": "nobody@flagura.dev", "password": fmt.Sprintf("invalid_pass_%d", time.Now().UnixNano())},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Reset Password - Malformed JSON",
			method:         http.MethodPost,
			url:            "/api/v1/auth/reset-password",
			body:           "invalid-json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Reset Password - Invalid Token",
			method:         http.MethodPost,
			url:            "/api/v1/auth/reset-password",
			body:           map[string]string{"token": "invalid_tok_123", "new_password": fmt.Sprintf("new_mock_pwd_%d", time.Now().UnixNano())},
			expectedStatus: http.StatusBadRequest,
		},

		// Experiments & Telemetry Edges
		{
			name:           "Ingest Experiment Events - Malformed JSON",
			method:         http.MethodPost,
			url:            "/api/v1/events",
			body:           "invalid-json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Get Experiment Report - NonExistent Flag",
			method:         http.MethodGet,
			url:            "/api/v1/experiments/non-existent-experiment-flag",
			cookie:         authCookie,
			expectedStatus: http.StatusNotFound,
		},

		// Actor & Project Resolution Variations
		{
			name:           "Get Flags - With X-Actor Header",
			method:         http.MethodGet,
			url:            "/api/v1/flags",
			headers:        map[string]string{"X-Actor": "ci-automation-bot", "X-Project-ID": proj.ID},
			cookie:         authCookie,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Get Flags - With Project Query Param",
			method:         http.MethodGet,
			url:            "/api/v1/flags?project_id=" + proj.ID,
			cookie:         authCookie,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Get Flags - With Active Project Cookie",
			method:         http.MethodGet,
			url:            "/api/v1/flags",
			cookie:         &http.Cookie{Name: "flagura_active_project", Value: proj.ID},
			headers:        map[string]string{"Authorization": "Bearer " + authCookie.Value},
			expectedStatus: http.StatusOK,
		},

		// Dashboard Views & Tab Navigation
		{
			name:           "Dashboard - Overview Tab",
			method:         http.MethodGet,
			url:            "/dashboard?tab=overview",
			cookie:         authCookie,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Dashboard - Audit Logs Tab",
			method:         http.MethodGet,
			url:            "/dashboard?tab=audit",
			cookie:         authCookie,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Dashboard - Filter by Environment & Search",
			method:         http.MethodGet,
			url:            "/dashboard?tab=matrix&env=production&search=edge&tag=core",
			cookie:         authCookie,
			expectedStatus: http.StatusOK,
		},

		// Change Request - Valid Creation
		{
			name:   "Create Change Request - Valid Proposal",
			method: http.MethodPost,
			url:    "/api/v1/change-requests",
			body: domain.ChangeRequest{
				FlagKey:     flagKey,
				Environment: domain.EnvProduction,
				Title:       "Enable edge flag for 20% rollout",
				Description: "Gradual ramp test",
				ProposedConfig: domain.EnvironmentConfig{
					Enabled:    true,
					Strategy:   domain.StrategyPercentage,
					Percentage: 20,
				},
			},
			cookie:         authCookie,
			expectedStatus: http.StatusCreated,
		},

		// Canary Lifecycle on Existing Flag
		{
			name:   "Create Canary - Valid",
			method: http.MethodPost,
			url:    "/api/v1/flags/" + flagKey + "/canary",
			body: domain.CanarySchedule{
				FlagKey:     flagKey,
				Environment: domain.EnvProduction,
				Stages: []domain.CanaryStage{
					{TargetPercentage: 10, DurationSec: 60},
					{TargetPercentage: 50, DurationSec: 60},
					{TargetPercentage: 100, DurationSec: 60},
				},
				Guardrails: domain.CanaryGuardrails{
					MaxErrorRatePct: 1.0,
					AutoRollback:    true,
				},
			},
			cookie:         authCookie,
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "Get Canary - Valid",
			method:         http.MethodGet,
			url:            "/api/v1/flags/" + flagKey + "/canary",
			cookie:         authCookie,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Delete Canary - Valid",
			method:         http.MethodDelete,
			url:            "/api/v1/flags/" + flagKey + "/canary",
			cookie:         authCookie,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var bodyReader *bytes.Reader
			if strBody, ok := tc.body.(string); ok {
				bodyReader = bytes.NewReader([]byte(strBody))
			} else if tc.body != nil {
				b, _ := json.Marshal(tc.body)
				bodyReader = bytes.NewReader(b)
			} else {
				bodyReader = bytes.NewReader(nil)
			}

			req := httptest.NewRequest(tc.method, tc.url, bodyReader)
			if tc.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			if tc.cookie != nil {
				req.AddCookie(tc.cookie)
			}
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Fatalf("expected status %d, got %d. Response: %s", tc.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}
