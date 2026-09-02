package store

import (
	"context"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func TestSQLiteStore_Lifecycle(t *testing.T) {
	ctx := context.Background()
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}
	defer s.Close()

	if s.DriverName() != "SQLite" {
		t.Errorf("expected DriverName to be 'SQLite', got %s", s.DriverName())
	}

	if err := s.Ping(ctx); err != nil {
		t.Errorf("ping failed: %v", err)
	}

	// 1. Verify Seeded Defaults
	flags, err := s.ListFlags(ctx)
	if err != nil {
		t.Fatalf("failed to list seeded flags: %v", err)
	}
	if len(flags) == 0 {
		t.Errorf("expected seeded flags, got 0")
	}

	// 2. Organization & Projects
	org, err := s.CreateOrganization(ctx, domain.Organization{
		Name: "Acme Corp",
		Slug: "acme-corp",
	})
	if err != nil {
		t.Fatalf("failed to create org: %v", err)
	}

	fetchedOrg, err := s.GetOrganization(ctx, org.Slug)
	if err != nil || fetchedOrg.Name != "Acme Corp" {
		t.Errorf("get organization mismatch: %v", err)
	}

	proj, err := s.CreateProject(ctx, domain.Project{
		OrganizationID: org.ID,
		Name:           "Mobile App",
		Slug:           "mobile-app",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	projects, err := s.ListProjects(ctx, org.ID)
	if err != nil || len(projects) != 1 {
		t.Errorf("expected 1 project, got %d (err: %v)", len(projects), err)
	}

	// 3. Flags CRUD & Rollouts
	newFlag := domain.FeatureFlag{
		ProjectID:   proj.ID,
		Key:         "dark-mode-v2",
		Name:        "Dark Mode v2",
		Type:        "boolean",
		Tags:        []string{"ui", "theme"},
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: false, Strategy: domain.StrategyPercentage, Percentage: 0},
		},
	}
	_, err = s.SaveFlag(ctx, newFlag, "tester@acme.com")
	if err != nil {
		t.Fatalf("failed to save flag: %v", err)
	}

	savedFlag, err := s.GetFlagByProject(ctx, proj.ID, "dark-mode-v2")
	if err != nil || savedFlag.Key != "dark-mode-v2" {
		t.Fatalf("failed to get flag by project: %v", err)
	}

	// Toggle flag
	enabled := true
	updatedFlag, audit, err := s.ToggleFlag(ctx, savedFlag.ID, domain.EnvProduction, &enabled, "tester@acme.com")
	if err != nil {
		t.Fatalf("failed to toggle flag: %v", err)
	}
	if !updatedFlag.Environments[domain.EnvProduction].Enabled {
		t.Errorf("expected flag to be enabled")
	}
	if audit.Action != "TOGGLE_FLAG" {
		t.Errorf("expected audit action TOGGLE_FLAG, got %s", audit.Action)
	}

	// Update rollout
	updatedFlag, _, err = s.UpdateRollout(ctx, savedFlag.ID, domain.EnvProduction, 45.5, "tester@acme.com")
	if err != nil {
		t.Fatalf("failed to update rollout: %v", err)
	}
	if updatedFlag.Environments[domain.EnvProduction].Percentage != 45.5 {
		t.Errorf("expected rollout 45.5, got %v", updatedFlag.Environments[domain.EnvProduction].Percentage)
	}

	// 4. API Keys
	apiKey := domain.APIKey{
		ProjectID:   proj.ID,
		Environment: "production",
		KeyPrefix:   "flg_live_1234",
		KeyHash:     "hash_of_secret_key_12345",
		Name:        "Production Secret",
		Role:        domain.RoleDeveloper,
		CreatedBy:   "tester@acme.com",
	}
	createdKey, err := s.CreateAPIKey(ctx, apiKey)
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	fetchedKey, err := s.GetAPIKeyByHash(ctx, apiKey.KeyHash)
	if err != nil || fetchedKey.ID != createdKey.ID {
		t.Errorf("failed to get api key by hash: %v", err)
	}

	keysByProj, err := s.ListAPIKeysByProject(ctx, proj.ID)
	if err != nil || len(keysByProj) != 1 {
		t.Errorf("expected 1 api key in project, got %d (err: %v)", len(keysByProj), err)
	}

	err = s.RevokeAPIKey(ctx, createdKey.ID, "tester@acme.com")
	if err != nil {
		t.Fatalf("failed to revoke api key: %v", err)
	}

	_, err = s.GetAPIKeyByHash(ctx, apiKey.KeyHash)
	if err == nil {
		t.Errorf("expected revoked api key to fail validation")
	}

	// 5. Change Requests (4-Eyes Governance)
	cr, err := s.CreateChangeRequest(ctx, domain.ChangeRequest{
		ProjectID:    proj.ID,
		FlagKey:      savedFlag.Key,
		Environment:  domain.EnvProduction,
		Title:        "Rollout 100% to Prod",
		AuthorUserID: "usr_alice",
		AuthorEmail:  "alice@acme.com",
		AuthorName:   "Alice",
		ProposedConfig: domain.EnvironmentConfig{
			Enabled:    true,
			Strategy:   domain.StrategyPercentage,
			Percentage: 100,
		},
	})
	if err != nil {
		t.Fatalf("failed to create change request: %v", err)
	}

	// Author cannot review own request
	_, err = s.ReviewChangeRequest(ctx, cr.ID, "usr_alice", "alice@acme.com", "Alice", true, "Looks good")
	if err == nil {
		t.Errorf("expected author review to fail under 4-Eyes rule")
	}

	// Peer review approves
	reviewedCR, err := s.ReviewChangeRequest(ctx, cr.ID, "usr_bob", "bob@acme.com", "Bob", true, "Approved for rollout")
	if err != nil {
		t.Fatalf("peer review failed: %v", err)
	}
	if reviewedCR.Status != domain.ChangeRequestStatusApproved {
		t.Errorf("expected ChangeRequestStatusApproved, got %s", reviewedCR.Status)
	}

	// Apply change request
	appliedFlag, appliedCR, _, err := s.ApplyChangeRequest(ctx, cr.ID, "bob@acme.com")
	if err != nil {
		t.Fatalf("apply CR failed: %v", err)
	}
	if appliedCR.Status != domain.ChangeRequestStatusApplied {
		t.Errorf("expected ChangeRequestStatusApplied, got %s", appliedCR.Status)
	}
	if appliedFlag.Environments[domain.EnvProduction].Percentage != 100 {
		t.Errorf("expected 100%% rollout after CR apply, got %v", appliedFlag.Environments[domain.EnvProduction].Percentage)
	}

	// 6. Users & Sessions
	user, err := s.CreateUser(ctx, domain.User{
		Email:        "charlie@acme.com",
		PasswordHash: "hashed_pwd",
		Name:         "Charlie",
		Role:         domain.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	fetchedUser, err := s.GetUserByEmail(ctx, "charlie@acme.com")
	if err != nil || fetchedUser.ID != user.ID {
		t.Errorf("get user by email mismatch: %v", err)
	}

	sess := domain.Session{
		Token:     "sess_token_12345",
		UserID:    user.ID,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		CreatedAt: time.Now().UTC(),
	}
	err = s.CreateSession(ctx, sess)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	fetchedSess, err := s.GetSession(ctx, "sess_token_12345")
	if err != nil || fetchedSess.UserID != user.ID {
		t.Errorf("get session failed: %v", err)
	}

	_ = s.DeleteSession(ctx, "sess_token_12345")
	_, err = s.GetSession(ctx, "sess_token_12345")
	if err == nil {
		t.Errorf("expected deleted session to return error")
	}

	// 7. Audit Logs
	logs, err := s.ListAuditLogsByProject(ctx, proj.ID, 10)
	if err != nil {
		t.Fatalf("failed to list audit logs: %v", err)
	}
	if len(logs) == 0 {
		t.Errorf("expected audit logs to be recorded")
	}

	// 8. Experiment Events
	err = s.RecordExperimentEvents(ctx, []domain.ExperimentEvent{
		{
			FlagKey:     savedFlag.Key,
			Variant:     "treatment",
			MetricName:  "signup_conversion",
			Value:       1.0,
			UserID:      "usr_1001",
			Environment: domain.EnvProduction,
		},
	})
	if err != nil {
		t.Fatalf("failed to record experiment events: %v", err)
	}

	events, err := s.GetExperimentEvents(ctx, savedFlag.Key, 10)
	if err != nil || len(events) != 1 {
		t.Errorf("expected 1 experiment event, got %d (err: %v)", len(events), err)
	}
}
