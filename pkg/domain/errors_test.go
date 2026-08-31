package domain

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestLayeredErrorCodesAndSentinelMapping(t *testing.T) {
	tests := []struct {
		name          string
		sentinelErr   error
		expectedCode  ErrorCode
		expectedLayer string
		expectedHTTP  int
	}{
		{
			name:          "Unauthorized Sentinel",
			sentinelErr:   ErrUnauthorized,
			expectedCode:  ErrCodeUnauthorized,
			expectedLayer: "SecurityLayer",
			expectedHTTP:  http.StatusUnauthorized,
		},
		{
			name:          "Environment Restricted Sentinel",
			sentinelErr:   ErrEnvironmentRestricted,
			expectedCode:  ErrCodeEnvironmentRestricted,
			expectedLayer: "SecurityLayer",
			expectedHTTP:  http.StatusForbidden,
		},
		{
			name:          "Project Not Found Sentinel",
			sentinelErr:   ErrProjectNotFound,
			expectedCode:  ErrCodeProjectNotFound,
			expectedLayer: "MultiTenancyLayer",
			expectedHTTP:  http.StatusNotFound,
		},
		{
			name:          "Flag Not Found Sentinel",
			sentinelErr:   ErrFlagNotFound,
			expectedCode:  ErrCodeFlagNotFound,
			expectedLayer: "FlagEngineLayer",
			expectedHTTP:  http.StatusNotFound,
		},
		{
			name:          "Four Eyes Self Approval Sentinel",
			sentinelErr:   ErrFourEyesSelfApproval,
			expectedCode:  ErrCodeFourEyesSelfApproval,
			expectedLayer: "GovernanceLayer",
			expectedHTTP:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test wrapping with %w preserves errors.Is
			wrapped := fmt.Errorf("wrapped context: %w", tt.sentinelErr)
			if !errors.Is(wrapped, tt.sentinelErr) {
				t.Fatalf("expected errors.Is to match sentinel error %v", tt.sentinelErr)
			}

			appErr := MapSentinelToAppError(wrapped)
			if appErr == nil {
				t.Fatalf("expected non-nil AppError")
			}

			if appErr.Code != tt.expectedCode {
				t.Errorf("expected code %d, got %d", tt.expectedCode, appErr.Code)
			}

			if appErr.Layer != tt.expectedLayer {
				t.Errorf("expected layer %q, got %q", tt.expectedLayer, appErr.Layer)
			}

			if appErr.HTTPStatus != tt.expectedHTTP {
				t.Errorf("expected HTTP status %d, got %d", tt.expectedHTTP, appErr.HTTPStatus)
			}

			if !errors.Is(appErr, tt.sentinelErr) {
				t.Errorf("expected AppError.Unwrap() to preserve sentinel error")
			}
		})
	}
}

func TestIDGenerationAndConstructors(t *testing.T) {
	u := NewUser("alice@example.com", "Alice Smith", "hash123", RoleDeveloper)
	if u.ID == "" || u.Email != "alice@example.com" || u.Role != RoleDeveloper {
		t.Fatalf("unexpected user entity: %+v", u)
	}

	org := NewOrganization("Acme Corp", "acme-corp", "Acme organization")
	if org.ID == "" || org.Slug != "acme-corp" {
		t.Fatalf("unexpected organization entity: %+v", org)
	}

	proj := NewProject(org.ID, "Mobile App", "mobile-app", "iOS and Android flags")
	if proj.ID == "" || proj.OrganizationID != org.ID || proj.Slug != "mobile-app" {
		t.Fatalf("unexpected project entity: %+v", proj)
	}
}
