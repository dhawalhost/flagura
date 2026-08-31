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

func TestNewID(t *testing.T) {
	prefixes := []string{"usr", "org", "proj", "flg", "key", "req"}
	for _, prefix := range prefixes {
		t.Run("Prefix_"+prefix, func(t *testing.T) {
			id := NewID(prefix)
			expectedPrefix := prefix + "_"
			if len(id) <= len(expectedPrefix) || id[:len(expectedPrefix)] != expectedPrefix {
				t.Errorf("NewID(%q) = %q, expected prefix %q", prefix, id, expectedPrefix)
			}
		})
	}
}

func TestConstructors(t *testing.T) {
	t.Run("NewUser", func(t *testing.T) {
		tests := []struct {
			name         string
			email        string
			fullName     string
			passwordHash string
			role         UserRole
		}{
			{"Developer User", "alice@example.com", "Alice Smith", "hash123", RoleDeveloper},
			{"Admin User", "admin@example.com", "Admin Boss", "adminhash", RoleAdmin},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				u := NewUser(tt.email, tt.fullName, tt.passwordHash, tt.role)
				if u.ID == "" || u.Email != tt.email || u.Name != tt.fullName || u.Role != tt.role {
					t.Fatalf("unexpected user entity: %+v", u)
				}
			})
		}
	})

	t.Run("NewOrganization", func(t *testing.T) {
		tests := []struct {
			name        string
			orgName     string
			slug        string
			description string
		}{
			{"Standard Org", "Acme Corp", "acme-corp", "Acme organization"},
			{"Tech Org", "Flagura Inc", "flagura-inc", "Feature flag platform"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				org := NewOrganization(tt.orgName, tt.slug, tt.description)
				if org.ID == "" || org.Name != tt.orgName || org.Slug != tt.slug || org.Description != tt.description {
					t.Fatalf("unexpected organization entity: %+v", org)
				}
			})
		}
	})

	t.Run("NewProject", func(t *testing.T) {
		tests := []struct {
			name        string
			orgID       string
			projName    string
			slug        string
			description string
		}{
			{"Mobile Project", "org_123", "Mobile App", "mobile-app", "iOS and Android flags"},
			{"Payments Project", "org_456", "Payments API", "payments-api", "Stripe integration flags"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				proj := NewProject(tt.orgID, tt.projName, tt.slug, tt.description)
				if proj.ID == "" || proj.OrganizationID != tt.orgID || proj.Name != tt.projName || proj.Slug != tt.slug {
					t.Fatalf("unexpected project entity: %+v", proj)
				}
			})
		}
	})
}
