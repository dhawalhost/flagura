package domain

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
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

func TestFeatureFlag_HelperMethods(t *testing.T) {
	tests := []struct {
		name               string
		flag               FeatureFlag
		expectedProdOn     bool
		expectedProdPct    float64
		expectedProdStrat  string
		expectedRulesCount int
	}{
		{
			name: "Fully configured production flag",
			flag: FeatureFlag{
				Environments: map[Environment]EnvironmentConfig{
					EnvProduction: {
						Enabled:    true,
						Percentage: 75.5,
						Strategy:   StrategyPercentage,
						Rules: []TargetingRule{
							{ID: "r1", Name: "Rule 1"},
							{ID: "r2", Name: "Rule 2"},
						},
					},
				},
			},
			expectedProdOn:     true,
			expectedProdPct:    75.5,
			expectedProdStrat:  "percentage",
			expectedRulesCount: 2,
		},
		{
			name: "Flag with no production environment",
			flag: FeatureFlag{
				Environments: map[Environment]EnvironmentConfig{
					EnvStaging: {Enabled: true},
				},
			},
			expectedProdOn:     false,
			expectedProdPct:    0,
			expectedProdStrat:  "boolean",
			expectedRulesCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.flag.IsProdEnabled() != tt.expectedProdOn {
				t.Errorf("IsProdEnabled() = %v, expected %v", tt.flag.IsProdEnabled(), tt.expectedProdOn)
			}
			if tt.flag.ProdPercentage() != tt.expectedProdPct {
				t.Errorf("ProdPercentage() = %v, expected %v", tt.flag.ProdPercentage(), tt.expectedProdPct)
			}
			if tt.flag.ProdStrategy() != tt.expectedProdStrat {
				t.Errorf("ProdStrategy() = %v, expected %v", tt.flag.ProdStrategy(), tt.expectedProdStrat)
			}
			if tt.flag.ProdRulesCount() != tt.expectedRulesCount {
				t.Errorf("ProdRulesCount() = %v, expected %v", tt.flag.ProdRulesCount(), tt.expectedRulesCount)
			}
			_ = tt.flag.EnvConfig("unknown_env")
		})
	}
}

func TestErrorCode_Methods(t *testing.T) {
	codes := []ErrorCode{
		ErrCodeUnauthorized,
		ErrCodeForbidden,
		ErrCodeInvalidCredentials,
		ErrCodeAPIKeyNotFound,
		ErrCodeAPIKeyRevoked,
		ErrCodeEnvironmentRestricted,
		ErrCodePasswordTooWeak,
		ErrCodeEmailAlreadyExists,
		ErrCodeUserNotFound,
		ErrCodeOrgNotFound,
		ErrCodeOrgConflict,
		ErrCodeProjectNotFound,
		ErrCodeProjectConflict,
		ErrCodeProjectAccessDenied,
		ErrCodeFlagNotFound,
		ErrCodeFlagAlreadyExists,
		ErrCodeInvalidEnvironment,
		ErrCodeInvalidRollout,
		ErrCodeEvaluationMissingCtx,
		ErrCodeNotFound,
		ErrCodeChangeRequestNotFound,
		ErrCodeFourEyesSelfApproval,
		ErrCodeChangeRequestReviewed,
		ErrCodeExperimentNotFound,
		ErrCodeDatabaseConnection,
		ErrCodeDatabaseQuery,
		ErrCodeDatabaseConstraint,
		ErrCodeSSEStreamDisconnect,
		ErrCodeCircuitBreakerOpen,
		ErrCodeRateLimitExceeded,
		ErrCodePayloadTooLarge,
		ErrCodeMalformedPayload,
		ErrCodeInternal,
		ErrorCode(99999), // Unknown code fallback
		ErrorCode(500),   // Unknown low code fallback
	}

	for _, code := range codes {
		t.Run(fmt.Sprintf("Code_%d", code), func(t *testing.T) {
			layer := code.Layer()
			if layer == "" {
				t.Errorf("expected non-empty layer for code %d", code)
			}
			str := code.String()
			if str == "" {
				t.Errorf("expected non-empty string for code %d", code)
			}
		})
	}
}

func TestMapSentinelToAppError_Exhaustive(t *testing.T) {
	tests := []struct {
		name        error
		expectedNil bool
	}{
		{nil, true},
		{ErrNotFound, false},
		{ErrUnauthorized, false},
		{ErrForbidden, false},
		{ErrConflict, false},
		{ErrInvalidInput, false},
		{ErrUserNotFound, false},
		{ErrEmailAlreadyExists, false},
		{ErrOrgNotFound, false},
		{ErrProjectNotFound, false},
		{ErrFlagNotFound, false},
		{ErrInvalidEnvironment, false},
		{ErrEnvironmentRestricted, false},
		{ErrKeyNotFound, false},
		{ErrKeyRevoked, false},
		{ErrFourEyesSelfApproval, false},
		{ErrInternal, false},
		{errors.New("unmapped unknown error"), false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Sentinel_%v", tt.name), func(t *testing.T) {
			res := MapSentinelToAppError(tt.name)
			if (res == nil) != tt.expectedNil {
				t.Fatalf("MapSentinelToAppError(%v) returned nil=%v, expected nil=%v", tt.name, res == nil, tt.expectedNil)
			}
			if res != nil {
				if res.Error() == "" {
					t.Errorf("expected non-empty Error() string")
				}
			}
		})
	}
}

func TestAppError_Formatting(t *testing.T) {
	cause := errors.New("raw db error")
	appErr := NewAppError(ErrCodeDatabaseQuery, "Failed to query database", http.StatusInternalServerError, cause)

	if appErr.Error() != "[DATABASE_QUERY_ERROR/5002] Failed to query database: raw db error" {
		t.Errorf("unexpected Error() string: %s", appErr.Error())
	}
	if !errors.Is(appErr, cause) {
		t.Errorf("expected errors.Is to match wrapped cause")
	}

	noCauseErr := NewAppError(ErrCodeNotFound, "Resource missing", http.StatusNotFound, nil)
	if noCauseErr.Error() != "[NOT_FOUND/3006] Resource missing" {
		t.Errorf("unexpected Error() string: %s", noCauseErr.Error())
	}
	if noCauseErr.Unwrap() != nil {
		t.Errorf("expected nil unwrap for no cause error")
	}
}

func TestSessionAndToken_Expiry(t *testing.T) {
	expiredSession := &Session{ExpiresAt: time.Now().Add(-1 * time.Hour)}
	if !expiredSession.IsExpired() {
		t.Errorf("expected expired session to report true")
	}

	validSession := &Session{ExpiresAt: time.Now().Add(1 * time.Hour)}
	if validSession.IsExpired() {
		t.Errorf("expected valid session to report false")
	}

	expiredToken := &PasswordResetToken{ExpiresAt: time.Now().Add(-10 * time.Minute)}
	if !expiredToken.IsExpired() {
		t.Errorf("expected expired reset token to report true")
	}

	validToken := &PasswordResetToken{ExpiresAt: time.Now().Add(10 * time.Minute)}
	if validToken.IsExpired() {
		t.Errorf("expected valid reset token to report false")
	}
}

func TestAPIKey_AllowsEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		key      *APIKey
		env      Environment
		expected bool
	}{
		{"Nil key allows all", nil, EnvProduction, true},
		{"Empty env string allows all", &APIKey{Environment: ""}, EnvProduction, true},
		{"Wildcard env allows all", &APIKey{Environment: "*"}, EnvStaging, true},
		{"All keyword allows all", &APIKey{Environment: "all"}, EnvDevelopment, true},
		{"Matching production", &APIKey{Environment: "production"}, EnvProduction, true},
		{"Mismatch production on staging key", &APIKey{Environment: "staging"}, EnvProduction, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed := tt.key.AllowsEnvironment(tt.env)
			if allowed != tt.expected {
				t.Errorf("AllowsEnvironment(%q) = %v, expected %v", tt.env, allowed, tt.expected)
			}
		})
	}
}

func TestFeatureFlag_DeepCopy(t *testing.T) {
	orig := FeatureFlag{
		ID:   "flg_orig",
		Key:  "test-flag",
		Tags: []string{"frontend", "v2"},
		Environments: map[Environment]EnvironmentConfig{
			EnvProduction: {
				Enabled:    true,
				Percentage: 50,
				Rules: []TargetingRule{
					{
						ID:        "r1",
						Values:    []string{"admin", "staff"},
						CustomKey: "role",
					},
				},
				Variants: []FlagVariant{
					{Key: "v1", Value: "A", Weight: 50},
					{Key: "v2", Value: "B", Weight: 50},
				},
			},
		},
	}

	copied := orig.DeepCopy()
	if copied.ID != orig.ID || len(copied.Tags) != len(orig.Tags) {
		t.Fatalf("DeepCopy mismatch: %+v", copied)
	}

	// Mutate copy
	copied.Tags[0] = "mutated"
	copied.Environments[EnvProduction].Rules[0].Values[0] = "mutated"
	if orig.Tags[0] == "mutated" || orig.Environments[EnvProduction].Rules[0].Values[0] == "mutated" {
		t.Fatalf("DeepCopy leaked mutable slice/map references")
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Acme Corp", "acme-corp"},
		{"Payments & Billing API!!!", "payments-billing-api"},
		{"   ", "default"},
		{"---hello_world---", "hello-world"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			res := Slugify(tt.input)
			if res != tt.expected {
				t.Errorf("Slugify(%q) = %q, expected %q", tt.input, res, tt.expected)
			}
		})
	}
}

func TestCanarySchedule_NextStage(t *testing.T) {
	now := time.Now()
	sched := &CanarySchedule{
		Status:          CanaryStatusActive,
		CurrentStageIdx: 0,
		Stages: []CanaryStage{
			{Index: 0, TargetPercentage: 10, DurationSec: 60, StartedAt: now.Add(-70 * time.Second)},
			{Index: 1, TargetPercentage: 50, DurationSec: 60, StartedAt: now},
		},
	}

	advanced, next := sched.NextStage(now)
	if !advanced || next == nil || next.TargetPercentage != 50 {
		t.Fatalf("expected advance to stage 2, got advanced=%v, next=%+v", advanced, next)
	}

	// Not ready to advance yet
	advanced2, next2 := sched.NextStage(now)
	if advanced2 || next2 != nil {
		t.Fatalf("expected not ready to advance, got advanced=%v", advanced2)
	}

	// Inactive status
	sched.Status = CanaryStatusPaused
	if adv, _ := sched.NextStage(now); adv {
		t.Fatalf("expected inactive schedule not to advance")
	}
}

func TestChangeRequest_Review(t *testing.T) {
	cr := ChangeRequest{
		ID:           "cr_1",
		AuthorUserID: "usr_alice",
		Status:       ChangeRequestStatusPending,
	}

	// Self-approval failure
	if err := cr.Review("usr_alice", "alice@example.com", "Alice", true, "LGTM"); !errors.Is(err, ErrFourEyesSelfApproval) {
		t.Fatalf("expected ErrFourEyesSelfApproval, got %v", err)
	}

	// Peer review approval
	if err := cr.Review("usr_bob", "bob@example.com", "Bob", true, "Approved"); err != nil {
		t.Fatalf("expected peer review to succeed, got %v", err)
	}
	if cr.Status != ChangeRequestStatusApproved || cr.ReviewerUserID != "usr_bob" {
		t.Fatalf("unexpected change request status: %+v", cr)
	}

	// Already reviewed
	if err := cr.Review("usr_charlie", "charlie@example.com", "Charlie", false, "Reject"); err == nil {
		t.Fatalf("expected error reviewing non-pending change request")
	}

	// Rejection test
	crPending := ChangeRequest{
		ID:           "cr_2",
		AuthorUserID: "usr_alice",
		Status:       ChangeRequestStatusPending,
	}
	if err := crPending.Review("usr_bob", "bob@example.com", "Bob", false, "Needs changes"); err != nil {
		t.Fatalf("expected rejection review to succeed, got %v", err)
	}
	if crPending.Status != ChangeRequestStatusRejected {
		t.Fatalf("expected status REJECTED, got %s", crPending.Status)
	}
}

func TestConstructors_DefaultFallbacks(t *testing.T) {
	org := NewOrganization("Acme", "", "desc")
	if org.Slug != "acme" {
		t.Errorf("expected slug acme, got %s", org.Slug)
	}

	proj := NewProject("", "My Project", "", "desc")
	if proj.OrganizationID != DefaultOrgID || proj.Slug != "my-project" {
		t.Errorf("expected default org and slug, got orgID=%s, slug=%s", proj.OrganizationID, proj.Slug)
	}

	user := NewUser("user@test.com", "Test User", "hash", "")
	if user.Role != RoleDeveloper {
		t.Errorf("expected default role developer, got %s", user.Role)
	}
}
