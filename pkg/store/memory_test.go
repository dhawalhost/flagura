package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func createTestFlag(key, projectID string) domain.FeatureFlag {
	return domain.FeatureFlag{
		ID:          "flag_" + key,
		ProjectID:   projectID,
		Key:         key,
		Name:        "Test " + key,
		Description: "Testing feature flag",
		Type:        "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 50,
			},
			domain.EnvStaging: {
				Enabled:    true,
				Strategy:   domain.StrategyBoolean,
				Percentage: 100,
			},
			domain.EnvDevelopment: {
				Enabled:    true,
				Strategy:   domain.StrategyBoolean,
				Percentage: 100,
			},
		},
	}
}

func TestMemoryStore_DeepCopyIsolation(t *testing.T) {
	memStore := NewMemoryStore()
	ctx := context.Background()

	testFlag := createTestFlag("ai-smart-search", DefaultProjectID)
	_, err := memStore.SaveFlag(ctx, testFlag, "tester")
	if err != nil {
		t.Fatalf("Failed to save initial flag: %v", err)
	}

	// 1. Get initial flag
	flag1, err := memStore.GetFlag(ctx, "ai-smart-search")
	if err != nil {
		t.Fatalf("Failed to get flag: %v", err)
	}

	// 2. Mutate map directly on the returned struct copy WITHOUT calling SaveFlag
	prodConfig := flag1.Environments[domain.EnvProduction]
	prodConfig.Percentage = 99.99
	flag1.Environments[domain.EnvProduction] = prodConfig

	// 3. Fetch fresh copy from store
	flag2, err := memStore.GetFlag(ctx, "ai-smart-search")
	if err != nil {
		t.Fatalf("Failed to get flag second time: %v", err)
	}

	// 4. Assert that store state was NOT mutated by caller's map modification
	if flag2.Environments[domain.EnvProduction].Percentage == 99.99 {
		t.Fatalf("DATA INTEGRITY BUG: MemoryStore leaked mutable map reference! Percentage was modified without SaveFlag.")
	}

	// 5. Test ListFlags isolation
	flagsList, err := memStore.ListFlags(ctx)
	if err != nil {
		t.Fatalf("Failed to list flags: %v", err)
	}
	for _, f := range flagsList {
		if f.Key == "ai-smart-search" {
			f.Environments[domain.EnvProduction] = domain.EnvironmentConfig{Percentage: 1.23}
		}
	}

	flag3, _ := memStore.GetFlag(ctx, "ai-smart-search")
	if flag3.Environments[domain.EnvProduction].Percentage == 1.23 {
		t.Fatalf("DATA INTEGRITY BUG: ListFlags leaked mutable map reference!")
	}
}

func TestMemoryStore_ConcurrentReadWriteRace(t *testing.T) {
	memStore := NewMemoryStore()
	ctx := context.Background()

	testFlag := createTestFlag("ai-smart-search", DefaultProjectID)
	_, _ = memStore.SaveFlag(ctx, testFlag, "tester")

	const numGoroutines = 50
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3)

	// Readers: GetFlag & ListFlags
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_, _ = memStore.ListFlags(ctx)
				flag, err := memStore.GetFlag(ctx, "ai-smart-search")
				if err == nil && flag != nil {
					// Read from map to ensure no race
					_ = flag.Environments[domain.EnvProduction].Percentage
				}
			}
		}(g)
	}

	// Writers: ToggleFlag & UpdateRollout
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				pct := float64((id*10 + i) % 100)
				_, _, _ = memStore.UpdateRollout(ctx, "ai-smart-search", domain.EnvProduction, pct, "concurrency-tester")
				enabled := (i % 2) == 0
				_, _, _ = memStore.ToggleFlag(ctx, "ai-smart-search", domain.EnvProduction, &enabled, "concurrency-tester")
			}
		}(g)
	}

	// Writers: SaveFlag
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				flag, err := memStore.GetFlag(ctx, "ai-smart-search")
				if err == nil && flag != nil {
					flag.Name = fmt.Sprintf("AI Smart Search (v%d.%d)", id, i)
					_, _ = memStore.SaveFlag(ctx, *flag, "concurrency-writer")
				}
			}
		}(g)
	}

	wg.Wait()
}

func TestMemoryStore_AuditLogRetention(t *testing.T) {
	memStore := NewMemoryStore()
	ctx := context.Background()

	testFlag := createTestFlag("ai-smart-search", DefaultProjectID)
	_, _ = memStore.SaveFlag(ctx, testFlag, "tester")

	initialLogs, err := memStore.ListAuditLogs(ctx, 100)
	if err != nil {
		t.Fatalf("Failed to list audit logs: %v", err)
	}

	initialCount := len(initialLogs)

	// Perform operations that generate audit logs
	_, _, _ = memStore.UpdateRollout(ctx, "ai-smart-search", domain.EnvProduction, 80, "audit-tester")
	_, _, _ = memStore.ToggleFlag(ctx, "ai-smart-search", domain.EnvProduction, nil, "audit-tester")

	updatedLogs, err := memStore.ListAuditLogs(ctx, 100)
	if err != nil {
		t.Fatalf("Failed to list audit logs: %v", err)
	}

	if len(updatedLogs) != initialCount+2 {
		t.Fatalf("Expected %d audit logs after mutations, got %d", initialCount+2, len(updatedLogs))
	}
}

func BenchmarkMemoryStore_GetFlag_Atomic(b *testing.B) {
	memStore := NewMemoryStore()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = memStore.GetFlag(ctx, "ai-smart-search")
	}
}

func BenchmarkMemoryStore_ListFlags_Atomic(b *testing.B) {
	memStore := NewMemoryStore()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = memStore.ListFlags(ctx)
	}
}

func TestMemoryStore_OrgAndProjectHierarchy(t *testing.T) {
	memStore := NewMemoryStore()
	ctx := context.Background()

	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Clean initial state with zero seeded orgs or projects",
			testFunc: func(t *testing.T) {
				orgs, err := memStore.ListOrganizations(ctx)
				if err != nil {
					t.Fatalf("ListOrganizations failed: %v", err)
				}
				if len(orgs) != 0 {
					t.Fatalf("Expected 0 organizations initially, got %d", len(orgs))
				}
			},
		},
		{
			name: "Create and lookup custom organization and project",
			testFunc: func(t *testing.T) {
				customOrg, err := memStore.CreateOrganization(ctx, domain.Organization{
					Name: "Acme Enterprise",
					Slug: "acme-enterprise",
				})
				if err != nil {
					t.Fatalf("Failed to create organization: %v", err)
				}

				customProj, err := memStore.CreateProject(ctx, domain.Project{
					OrganizationID: customOrg.ID,
					Name:           "Mobile Checkout v2",
					Slug:           "mobile-checkout-v2",
				})
				if err != nil {
					t.Fatalf("Failed to create project: %v", err)
				}

				retrievedOrg, err := memStore.GetOrganization(ctx, customOrg.ID)
				if err != nil || retrievedOrg.Name != "Acme Enterprise" {
					t.Fatalf("Failed GetOrganization: %v", err)
				}
				retrievedProj, err := memStore.GetProject(ctx, customProj.ID)
				if err != nil || retrievedProj.Name != "Mobile Checkout v2" {
					t.Fatalf("Failed GetProject: %v", err)
				}

				projects, err := memStore.ListProjects(ctx, customOrg.ID)
				if err != nil || len(projects) != 1 {
					t.Fatalf("ListProjects failed: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestMemoryStore_UserAndSessionManagement(t *testing.T) {
	memStore := NewMemoryStore()
	ctx := context.Background()

	var createdUser *domain.User

	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Create user successfully",
			testFunc: func(t *testing.T) {
				u := domain.NewUser("sarah@example.com", "Sarah Connor", "hashed_pwd_123", domain.RoleDeveloper)
				var err error
				createdUser, err = memStore.CreateUser(ctx, u)
				if err != nil {
					t.Fatalf("Failed to create user: %v", err)
				}
			},
		},
		{
			name: "Duplicate email creation fails",
			testFunc: func(t *testing.T) {
				u := domain.NewUser("sarah@example.com", "Sarah Connor", "hashed_pwd_123", domain.RoleDeveloper)
				if _, err := memStore.CreateUser(ctx, u); err == nil {
					t.Fatalf("Expected duplicate email to fail")
				}
			},
		},
		{
			name: "Lookup user by email and ID",
			testFunc: func(t *testing.T) {
				byEmail, err := memStore.GetUserByEmail(ctx, "sarah@example.com")
				if err != nil || byEmail.ID != createdUser.ID {
					t.Fatalf("GetUserByEmail failed: %v", err)
				}
				byID, err := memStore.GetUserByID(ctx, createdUser.ID)
				if err != nil || byID.Email != "sarah@example.com" {
					t.Fatalf("GetUserByID failed: %v", err)
				}
			},
		},
		{
			name: "Update user profile and password",
			testFunc: func(t *testing.T) {
				updated, err := memStore.UpdateUser(ctx, domain.User{
					ID:        createdUser.ID,
					Name:      "Sarah Connor Updated",
					AvatarURL: "https://flagura.dev/avatar.png",
				})
				if err != nil {
					t.Fatalf("UpdateUser failed: %v", err)
				}
				if updated.Name != "Sarah Connor Updated" || updated.AvatarURL != "https://flagura.dev/avatar.png" {
					t.Fatalf("Unexpected updated user data: %+v", updated)
				}

				if err := memStore.UpdateUserPassword(ctx, createdUser.ID, "new_hashed_pwd_456"); err != nil {
					t.Fatalf("UpdateUserPassword failed: %v", err)
				}

				byID, err := memStore.GetUserByID(ctx, createdUser.ID)
				if err != nil || byID.PasswordHash != "new_hashed_pwd_456" {
					t.Fatalf("Expected password hash to be updated, got: %v", byID)
				}
			},
		},
		{
			name: "Session lifecycle (create, get, delete)",
			testFunc: func(t *testing.T) {
				sess := domain.Session{
					Token:     "tok_session_test_987",
					UserID:    createdUser.ID,
					ExpiresAt: time.Now().Add(24 * time.Hour),
				}
				if err := memStore.CreateSession(ctx, sess); err != nil {
					t.Fatalf("CreateSession failed: %v", err)
				}
				gotSess, err := memStore.GetSession(ctx, "tok_session_test_987")
				if err != nil || gotSess.UserID != createdUser.ID {
					t.Fatalf("GetSession failed: %v", err)
				}
				if err := memStore.DeleteSession(ctx, "tok_session_test_987"); err != nil {
					t.Fatalf("DeleteSession failed: %v", err)
				}
				if _, err := memStore.GetSession(ctx, "tok_session_test_987"); err == nil {
					t.Fatalf("Expected session to be deleted")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestMemoryStore_ChangeRequestsAndGovernance(t *testing.T) {
	memStore := NewMemoryStore()
	ctx := context.Background()

	testFlag := createTestFlag("ai-smart-search", DefaultProjectID)
	_, _ = memStore.SaveFlag(ctx, testFlag, "tester")

	cr := domain.ChangeRequest{
		ProjectID:    DefaultProjectID,
		FlagKey:      "ai-smart-search",
		Environment:  domain.EnvProduction,
		Title:        "Turn off search in production",
		AuthorUserID: "usr_alice",
		AuthorEmail:  "alice@flagura.dev",
		AuthorName:   "Alice",
		ProposedConfig: domain.EnvironmentConfig{
			Enabled:  false,
			Strategy: domain.StrategyBoolean,
		},
		Status: domain.ChangeRequestStatusPending,
	}

	createdCR, err := memStore.CreateChangeRequest(ctx, cr)
	if err != nil {
		t.Fatalf("CreateChangeRequest failed: %v", err)
	}

	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "List change requests by project",
			testFunc: func(t *testing.T) {
				crs, err := memStore.ListChangeRequestsByProject(ctx, DefaultProjectID, "")
				if err != nil || len(crs) == 0 {
					t.Fatalf("ListChangeRequestsByProject failed: %v", err)
				}
			},
		},
		{
			name: "Author self-review fails 4-Eyes policy",
			testFunc: func(t *testing.T) {
				_, err = memStore.ReviewChangeRequest(ctx, createdCR.ID, "usr_alice", "alice@flagura.dev", "Alice", true, "LGTM")
				if err == nil {
					t.Fatalf("Expected author self-review to fail")
				}
			},
		},
		{
			name: "Peer approval succeeds",
			testFunc: func(t *testing.T) {
				updatedCR, err := memStore.ReviewChangeRequest(ctx, createdCR.ID, "usr_bob", "bob@flagura.dev", "Bob", true, "Approved for rollout")
				if err != nil || updatedCR.Status != domain.ChangeRequestStatusApproved {
					t.Fatalf("ReviewChangeRequest failed: %v", err)
				}
			},
		},
		{
			name: "Apply change request updates flag and creates audit entry",
			testFunc: func(t *testing.T) {
				flag, appliedCR, audit, err := memStore.ApplyChangeRequest(ctx, createdCR.ID, "bob@flagura.dev")
				if err != nil {
					t.Fatalf("ApplyChangeRequest failed: %v", err)
				}
				if flag.Environments[domain.EnvProduction].Enabled {
					t.Fatalf("Expected flag to be disabled after applying CR")
				}
				if appliedCR.Status != domain.ChangeRequestStatusApplied {
					t.Fatalf("Expected CR status APPLIED, got %s", appliedCR.Status)
				}
				if audit == nil {
					t.Fatalf("Expected non-nil audit log")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestMemoryStore_APIKeysAndServiceTokens(t *testing.T) {
	memStore := NewMemoryStore()
	ctx := context.Background()

	var createdKey *domain.APIKey

	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Create API key successfully",
			testFunc: func(t *testing.T) {
				k := domain.APIKey{
					ProjectID:   DefaultProjectID,
					Name:        "CI/CD Token",
					KeyPrefix:   "flg_live_1234",
					KeyHash:     "hash_secret_5678",
					Role:        domain.RoleAdmin,
					Environment: "production",
				}
				var err error
				createdKey, err = memStore.CreateAPIKey(ctx, k)
				if err != nil {
					t.Fatalf("Failed to create API key: %v", err)
				}
			},
		},
		{
			name: "Validate valid API key by hash",
			testFunc: func(t *testing.T) {
				validKey, err := memStore.GetAPIKeyByHash(ctx, "hash_secret_5678")
				if err != nil || validKey.ID != createdKey.ID {
					t.Fatalf("GetAPIKeyByHash failed: %v", err)
				}
			},
		},
		{
			name: "Lookup non-existent API key hash fails",
			testFunc: func(t *testing.T) {
				_, err := memStore.GetAPIKeyByHash(ctx, "invalid_hash")
				if err == nil {
					t.Fatalf("Expected invalid API key hash to fail lookup")
				}
			},
		},
		{
			name: "Revoke API key",
			testFunc: func(t *testing.T) {
				if err := memStore.RevokeAPIKey(ctx, createdKey.ID, "admin@flagura.dev"); err != nil {
					t.Fatalf("RevokeAPIKey failed: %v", err)
				}
				_, err := memStore.GetAPIKeyByHash(ctx, "hash_secret_5678")
				if err == nil {
					t.Fatalf("Expected revoked API key to fail lookup")
				}
			},
		},
		{
			name: "List API keys by project",
			testFunc: func(t *testing.T) {
				keys, err := memStore.ListAPIKeysByProject(ctx, DefaultProjectID)
				if err != nil {
					t.Fatalf("ListAPIKeysByProject failed: %v", err)
				}
				if len(keys) != 1 || !keys[0].Revoked {
					t.Fatalf("Expected 1 revoked key, got %+v", keys)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestMemoryStore_ExperimentEvents(t *testing.T) {
	memStore := NewMemoryStore()
	ctx := context.Background()

	tests := []struct {
		name     string
		events   []domain.ExperimentEvent
		expected int
	}{
		{
			name: "Record and retrieve experiment conversion event",
			events: []domain.ExperimentEvent{
				{
					ID:          domain.NewID("exp"),
					ProjectID:   DefaultProjectID,
					FlagKey:     "checkout-v2",
					Variant:     "treatment",
					MetricName:  "purchase_completed",
					EventType:   "conversion",
					Value:       99.95,
					UserID:      "usr_buyer_1",
					Environment: domain.EnvProduction,
					Timestamp:   time.Now().UTC(),
				},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := memStore.RecordExperimentEvents(ctx, tt.events); err != nil {
				t.Fatalf("RecordExperimentEvents failed: %v", err)
			}
			storedEvents, err := memStore.GetExperimentEvents(ctx, "checkout-v2", 10)
			if err != nil || len(storedEvents) != tt.expected {
				t.Fatalf("GetExperimentEvents failed: %v (count: %d)", err, len(storedEvents))
			}
		})
	}
}

func TestMemoryStore_PasswordResetFlow(t *testing.T) {
	memStore := NewMemoryStore()
	ctx := context.Background()

	u := domain.NewUser("charlie@flagura.dev", "Charlie", "old_hash", domain.RoleDeveloper)
	_, _ = memStore.CreateUser(ctx, u)

	token, err := memStore.CreatePasswordResetToken(ctx, "charlie@flagura.dev", 15*time.Minute)
	if err != nil || token == "" {
		t.Fatalf("CreatePasswordResetToken failed: %v", err)
	}

	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Lookup valid token",
			testFunc: func(t *testing.T) {
				storedToken, err := memStore.GetPasswordResetToken(ctx, token)
				if err != nil || storedToken.Email != "charlie@flagura.dev" {
					t.Fatalf("GetPasswordResetToken failed: %v", err)
				}
			},
		},
		{
			name: "Reset password with token succeeds",
			testFunc: func(t *testing.T) {
				if err := memStore.ResetPasswordWithToken(ctx, token, "new_password_hash"); err != nil {
					t.Fatalf("ResetPasswordWithToken failed: %v", err)
				}
			},
		},
		{
			name: "Re-using consumed token fails",
			testFunc: func(t *testing.T) {
				if err := memStore.ResetPasswordWithToken(ctx, token, "another_hash"); err == nil {
					t.Fatalf("Expected second password reset with same token to fail")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestMemoryStore_FlagDeletionAndReset(t *testing.T) {
	memStore := NewMemoryStore()
	ctx := context.Background()

	testFlag := createTestFlag("ai-smart-search", DefaultProjectID)
	_, _ = memStore.SaveFlag(ctx, testFlag, "tester")

	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Delete flag generates audit log and removes flag",
			testFunc: func(t *testing.T) {
				log, err := memStore.DeleteFlag(ctx, "ai-smart-search", "admin@flagura.dev")
				if err != nil || log == nil {
					t.Fatalf("DeleteFlag failed: %v", err)
				}
				if _, err := memStore.GetFlag(ctx, "ai-smart-search"); err == nil {
					t.Fatalf("Expected flag to be deleted")
				}
			},
		},
		{
			name: "Store reset clears all data cleanly",
			testFunc: func(t *testing.T) {
				_ = memStore.Reset(ctx)
				flags, _ := memStore.ListFlags(ctx)
				if len(flags) != 0 {
					t.Fatalf("Expected 0 flags after reset, got %d", len(flags))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestMemoryStore_OrgMembersAndInvitations(t *testing.T) {
	memStore := NewMemoryStore()
	ctx := context.Background()

	// 1. Create Org
	org, err := memStore.CreateOrganization(ctx, domain.Organization{
		Name: "Flagura Labs",
		Slug: "flagura-labs",
	})
	if err != nil {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	// 2. Add Member
	member := domain.OrgMember{
		OrganizationID: org.ID,
		UserID:         "usr_owner",
		Role:           "owner",
	}
	createdMember, err := memStore.CreateOrgMember(ctx, member)
	if err != nil || createdMember.OrganizationID != org.ID {
		t.Fatalf("CreateOrgMember failed: %v", err)
	}

	members, err := memStore.ListOrgMembers(ctx, org.ID)
	if err != nil || len(members) != 1 {
		t.Fatalf("ListOrgMembers failed: %v", err)
	}

	userOrgs, err := memStore.ListUserOrganizations(ctx, "usr_owner")
	if err != nil || len(userOrgs) != 1 || userOrgs[0].ID != org.ID {
		t.Fatalf("ListUserOrganizations failed: %v", err)
	}

	// 3. Create Invitation
	invite := domain.OrgInvitation{
		OrganizationID: org.ID,
		Email:          "invitee@example.com",
		Role:           "developer",
		InvitedBy:      "usr_owner",
	}
	createdInvite, err := memStore.CreateOrgInvitation(ctx, invite)
	if err != nil || createdInvite.Token == "" {
		t.Fatalf("CreateOrgInvitation failed: %v", err)
	}

	// 4. Get and Accept Invitation
	retrievedInvite, err := memStore.GetOrgInvitation(ctx, createdInvite.Token)
	if err != nil || retrievedInvite.Email != "invitee@example.com" {
		t.Fatalf("GetOrgInvitation failed: %v", err)
	}

	acceptedMember, err := memStore.AcceptOrgInvitation(ctx, createdInvite.Token, "usr_invitee")
	if err != nil || acceptedMember.Role != "developer" {
		t.Fatalf("AcceptOrgInvitation failed: %v", err)
	}

	// 5. Verify list of invitations
	invites, err := memStore.ListOrgInvitations(ctx, org.ID)
	if err != nil || len(invites) != 1 || !invites[0].IsAccepted() {
		t.Fatalf("ListOrgInvitations failed: %v", err)
	}
}

func TestMemoryStore_ProjectScopedAndGovernanceQueries(t *testing.T) {
	memStore := NewMemoryStore()
	ctx := context.Background()

	testFlag := createTestFlag("ai-smart-search", DefaultProjectID)
	_, _ = memStore.SaveFlag(ctx, testFlag, "tester")

	// 1. ListFlagsByProject & GetFlagByProject
	flags, err := memStore.ListFlagsByProject(ctx, DefaultProjectID)
	if err != nil || len(flags) == 0 {
		t.Fatalf("expected flags for default project, got %d (err: %v)", len(flags), err)
	}

	flg, err := memStore.GetFlagByProject(ctx, DefaultProjectID, "ai-smart-search")
	if err != nil || flg == nil {
		t.Fatalf("expected to get flag by project, got %v", err)
	}

	_, errNotFound := memStore.GetFlagByProject(ctx, "non-existent-proj", "ai-smart-search")
	if errNotFound == nil {
		t.Fatalf("expected error getting flag for non-existent project")
	}

	// 2. ListAuditLogsByProject
	logs, err := memStore.ListAuditLogsByProject(ctx, DefaultProjectID, 10)
	if err != nil {
		t.Fatalf("unexpected error listing audit logs by project: %v", err)
	}
	_ = logs

	// 3. Create, Get, and List ChangeRequests
	cr := domain.ChangeRequest{
		ID:           "cr_query_test",
		ProjectID:    DefaultProjectID,
		FlagKey:      "ai-smart-search",
		Environment:  domain.EnvProduction,
		AuthorUserID: "usr_alice",
		Status:       domain.ChangeRequestStatusPending,
	}
	if _, err := memStore.CreateChangeRequest(ctx, cr); err != nil {
		t.Fatalf("failed to create change request: %v", err)
	}

	fetchedCR, err := memStore.GetChangeRequest(ctx, "cr_query_test")
	if err != nil || fetchedCR == nil {
		t.Fatalf("GetChangeRequest failed: %v", err)
	}

	allCRs, err := memStore.ListChangeRequests(ctx, domain.ChangeRequestStatusPending)
	if err != nil || len(allCRs) == 0 {
		t.Fatalf("ListChangeRequests failed: %v", err)
	}

	// 4. Create and List APIKeys
	key := domain.APIKey{
		ID:        "key_query_test",
		ProjectID: DefaultProjectID,
		KeyHash:   "hash_test_123",
		Name:      "Test Key",
	}
	if _, err := memStore.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	allKeys, err := memStore.ListAPIKeys(ctx)
	if err != nil || len(allKeys) == 0 {
		t.Fatalf("ListAPIKeys failed: %v", err)
	}

	// 5. Create user and verify ListUsers
	u := domain.NewUser("testuser@flagura.dev", "Test User", "hash123", domain.RoleDeveloper)
	_, _ = memStore.CreateUser(ctx, u)

	users, err := memStore.ListUsers(ctx)
	if err != nil || len(users) == 0 {
		t.Fatalf("ListUsers failed: %v", err)
	}

	if memStore.DriverName() != "In-Memory Edge Store" {
		t.Errorf("expected driver name 'In-Memory Edge Store', got %s", memStore.DriverName())
	}
	if err := memStore.Ping(ctx); err != nil {
		t.Errorf("expected nil error on Ping, got %v", err)
	}
}
