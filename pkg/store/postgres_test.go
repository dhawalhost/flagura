package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/dhawalhost/flagura/pkg/domain"
)

func newMockPostgresStore(t *testing.T) (*PostgresStore, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	return &PostgresStore{
		db:         db,
		driverName: "Local PostgreSQL",
	}, mock
}

func TestPostgresStore_DetectDriverName(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"postgres://user:pass@aws-0-us-east-1.supabase.co:5432/postgres", "Supabase PostgreSQL"},
		{"postgres://user:pass@ep-cool-frog.neon.tech/neondb", "Neon PostgreSQL"},
		{"postgres://user:pass@mydb.rds.amazonaws.com:5432/db", "AWS RDS PostgreSQL"},
		{"postgres://user:pass@localhost:5432/flagura", "Local PostgreSQL"},
		{"postgres://user:pass@remote-db.com:5432/flagura", "PostgreSQL"},
	}

	for _, tt := range tests {
		got := detectDriverName(tt.url)
		if got != tt.expected {
			t.Errorf("detectDriverName(%q) = %q, expected %q", tt.url, got, tt.expected)
		}
	}
}

func TestPostgresStore_DriverNameAndPing(t *testing.T) {
	st, _ := newMockPostgresStore(t)
	defer st.db.Close()

	if st.DriverName() != "Local PostgreSQL" {
		t.Errorf("expected Local PostgreSQL driver name, got %s", st.DriverName())
	}
}

func TestPostgresStore_UserAndSessionOperations(t *testing.T) {
	st, mock := newMockPostgresStore(t)
	defer st.db.Close()
	ctx := context.Background()

	// 1. CreateUser
	user := domain.NewUser("alice@flagura.dev", "Alice Dev", "hashed_pwd", domain.RoleDeveloper)
	mock.ExpectExec(`INSERT INTO users`).
		WithArgs(sqlmock.AnyArg(), user.Email, user.PasswordHash, user.Name, string(user.Role), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	created, err := st.CreateUser(ctx, user)
	if err != nil || created.Email != "alice@flagura.dev" {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// 2. GetUserByEmail
	mock.ExpectQuery(`SELECT id, email, password_hash, name, role, avatar_url, created_at, updated_at FROM users WHERE email = \$1`).
		WithArgs("alice@flagura.dev").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "name", "role", "avatar_url", "created_at", "updated_at"}).
			AddRow("u_alice_01", "alice@flagura.dev", "hashed_pwd", "Alice Dev", "developer", "", time.Now(), time.Now()))

	fetchedUser, err := st.GetUserByEmail(ctx, "alice@flagura.dev")
	if err != nil || fetchedUser.Email != "alice@flagura.dev" {
		t.Fatalf("GetUserByEmail failed: %v", err)
	}

	// 3. GetUserByID
	mock.ExpectQuery(`SELECT id, email, password_hash, name, role, avatar_url, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs("u_alice_01").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "name", "role", "avatar_url", "created_at", "updated_at"}).
			AddRow("u_alice_01", "alice@flagura.dev", "hashed_pwd", "Alice Dev", "developer", "", time.Now(), time.Now()))

	byID, err := st.GetUserByID(ctx, "u_alice_01")
	if err != nil || byID.ID != "u_alice_01" {
		t.Fatalf("GetUserByID failed: %v", err)
	}

	// 4. ListUsers
	mock.ExpectQuery(`SELECT id, email, name, role, avatar_url, created_at, updated_at FROM users ORDER BY created_at ASC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name", "role", "avatar_url", "created_at", "updated_at"}).
			AddRow("u_alice_01", "alice@flagura.dev", "Alice Dev", "developer", "", time.Now(), time.Now()))

	users, err := st.ListUsers(ctx)
	if err != nil || len(users) != 1 {
		t.Fatalf("ListUsers failed: %v", err)
	}

	// UpdateUser
	mock.ExpectExec(`UPDATE users SET name = \$1, avatar_url = \$2, updated_at = \$3 WHERE id = \$4`).
		WithArgs("Alice Smith", "https://flagura.dev/alice.png", sqlmock.AnyArg(), "u_alice_01").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, email, password_hash, name, role, avatar_url, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs("u_alice_01").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "name", "role", "avatar_url", "created_at", "updated_at"}).
			AddRow("u_alice_01", "alice@flagura.dev", "hashed_pwd", "Alice Smith", "developer", "https://flagura.dev/alice.png", time.Now(), time.Now()))

	updatedUser, err := st.UpdateUser(ctx, domain.User{
		ID:        "u_alice_01",
		Name:      "Alice Smith",
		AvatarURL: "https://flagura.dev/alice.png",
	})
	if err != nil || updatedUser.Name != "Alice Smith" {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	// UpdateUserPassword
	mock.ExpectExec(`UPDATE users SET password_hash = \$1, updated_at = \$2 WHERE id = \$3`).
		WithArgs("new_hashed_pwd_789", sqlmock.AnyArg(), "u_alice_01").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := st.UpdateUserPassword(ctx, "u_alice_01", "new_hashed_pwd_789"); err != nil {
		t.Fatalf("UpdateUserPassword failed: %v", err)
	}

	// 5. CreateSession
	session := domain.Session{
		Token:     "tok_session_123",
		UserID:    "u_alice_01",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}
	mock.ExpectExec(`INSERT INTO sessions`).
		WithArgs(session.Token, session.UserID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := st.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// 6. GetSession
	mock.ExpectQuery(`SELECT token, user_id, expires_at, created_at FROM sessions WHERE token = \$1`).
		WithArgs("tok_session_123").
		WillReturnRows(sqlmock.NewRows([]string{"token", "user_id", "expires_at", "created_at"}).
			AddRow("tok_session_123", "u_alice_01", time.Now().Add(24*time.Hour), time.Now()))

	mock.ExpectQuery(`SELECT id, email, password_hash, name, role, avatar_url, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs("u_alice_01").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "name", "role", "avatar_url", "created_at", "updated_at"}).
			AddRow("u_alice_01", "alice@flagura.dev", "hashed_pwd", "Alice Dev", "developer", "", time.Now(), time.Now()))

	sess, err := st.GetSession(ctx, "tok_session_123")
	if err != nil || sess == nil || sess.UserID != "u_alice_01" {
		t.Fatalf("GetSession failed: %v", err)
	}

	// 7. DeleteSession
	mock.ExpectExec(`DELETE FROM sessions WHERE token = \$1`).
		WithArgs("tok_session_123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := st.DeleteSession(ctx, "tok_session_123"); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}
}

func TestPostgresStore_FlagOperations(t *testing.T) {
	st, mock := newMockPostgresStore(t)
	defer st.db.Close()
	ctx := context.Background()

	sampleFlag := domain.FeatureFlag{
		ID:          "flag_01",
		ProjectID:   "proj_default",
		Key:         "ai-smart-search",
		Name:        "AI Smart Search",
		Description: "AI search desc",
		Type:        "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 50,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	envJSON, _ := json.Marshal(sampleFlag.Environments)

	// 1. GetFlag
	mock.ExpectQuery(`SELECT id, project_id, config_version, key, name, description, type, tags, environments, created_at, updated_at FROM feature_flags`).
		WithArgs("proj_default", "ai-smart-search").
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "config_version", "key", "name", "description", "type", "tags", "environments", "created_at", "updated_at"}).
			AddRow(sampleFlag.ID, sampleFlag.ProjectID, 1, sampleFlag.Key, sampleFlag.Name, sampleFlag.Description, sampleFlag.Type, "{ai,search}", envJSON, sampleFlag.CreatedAt, sampleFlag.UpdatedAt))

	flg, err := st.GetFlag(ctx, "ai-smart-search")
	if err != nil || flg.Key != "ai-smart-search" {
		t.Fatalf("GetFlag failed: %v", err)
	}

	// 2. ListFlags
	mock.ExpectQuery(`SELECT id, project_id, config_version, key, name, description, type, tags, environments, created_at, updated_at FROM feature_flags`).
		WithArgs("proj_default").
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "config_version", "key", "name", "description", "type", "tags", "environments", "created_at", "updated_at"}).
			AddRow(sampleFlag.ID, sampleFlag.ProjectID, 1, sampleFlag.Key, sampleFlag.Name, sampleFlag.Description, sampleFlag.Type, "{ai,search}", envJSON, sampleFlag.CreatedAt, sampleFlag.UpdatedAt))

	flags, err := st.ListFlags(ctx)
	if err != nil || len(flags) != 1 {
		t.Fatalf("ListFlags failed: %v", err)
	}

	// 3. SaveFlag
	mock.ExpectExec(`INSERT INTO feature_flags`).
		WithArgs(sqlmock.AnyArg(), sampleFlag.ProjectID, sampleFlag.Key, sampleFlag.Name, sampleFlag.Description, sampleFlag.Type, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO audit_logs`).
		WithArgs(sqlmock.AnyArg(), sampleFlag.ProjectID, sampleFlag.Key, "FLAG_UPDATED", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	log, err := st.SaveFlag(ctx, sampleFlag, "admin@flagura.dev")
	if err != nil || log == nil {
		t.Fatalf("SaveFlag failed: %v", err)
	}

	// 4. ToggleFlag
	mock.ExpectQuery(`SELECT id, project_id, config_version, key, name, description, type, tags, environments, created_at, updated_at FROM feature_flags`).
		WithArgs("proj_default", "ai-smart-search").
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "config_version", "key", "name", "description", "type", "tags", "environments", "created_at", "updated_at"}).
			AddRow(sampleFlag.ID, sampleFlag.ProjectID, 1, sampleFlag.Key, sampleFlag.Name, sampleFlag.Description, sampleFlag.Type, "{ai,search}", envJSON, sampleFlag.CreatedAt, sampleFlag.UpdatedAt))
	mock.ExpectExec(`UPDATE feature_flags SET environments = \$1, config_version = config_version \+ 1, updated_at = \$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sampleFlag.ID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO audit_logs`).
		WithArgs(sqlmock.AnyArg(), "ai-smart-search", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	newVal := false
	_, _, err = st.ToggleFlag(ctx, "ai-smart-search", domain.EnvProduction, &newVal, "admin@flagura.dev")
	if err != nil {
		t.Fatalf("ToggleFlag failed: %v", err)
	}

	// 5. UpdateRollout
	mock.ExpectQuery(`SELECT id, project_id, config_version, key, name, description, type, tags, environments, created_at, updated_at FROM feature_flags`).
		WithArgs("proj_default", "ai-smart-search").
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "config_version", "key", "name", "description", "type", "tags", "environments", "created_at", "updated_at"}).
			AddRow(sampleFlag.ID, sampleFlag.ProjectID, 1, sampleFlag.Key, sampleFlag.Name, sampleFlag.Description, sampleFlag.Type, "{ai,search}", envJSON, sampleFlag.CreatedAt, sampleFlag.UpdatedAt))
	mock.ExpectExec(`UPDATE feature_flags SET environments = \$1, config_version = config_version \+ 1, updated_at = \$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sampleFlag.ID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO audit_logs`).
		WithArgs(sqlmock.AnyArg(), "ai-smart-search", "ROLLOUT_CHANGED", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	_, _, err = st.UpdateRollout(ctx, "ai-smart-search", domain.EnvProduction, 85.0, "admin@flagura.dev")
	if err != nil {
		t.Fatalf("UpdateRollout failed: %v", err)
	}

	// 6. DeleteFlag
	mock.ExpectQuery(`SELECT id, project_id, config_version, key, name, description, type, tags, environments, created_at, updated_at FROM feature_flags`).
		WithArgs("proj_default", "ai-smart-search").
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "config_version", "key", "name", "description", "type", "tags", "environments", "created_at", "updated_at"}).
			AddRow(sampleFlag.ID, sampleFlag.ProjectID, 1, sampleFlag.Key, sampleFlag.Name, sampleFlag.Description, sampleFlag.Type, "{ai,search}", envJSON, sampleFlag.CreatedAt, sampleFlag.UpdatedAt))
	mock.ExpectExec(`DELETE FROM feature_flags WHERE id = \$1 OR key = \$1`).
		WithArgs("ai-smart-search").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO audit_logs`).
		WithArgs(sqlmock.AnyArg(), "ai-smart-search", "FLAG_DELETED", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	delLog, err := st.DeleteFlag(ctx, "ai-smart-search", "admin@flagura.dev")
	if err != nil || delLog == nil {
		t.Fatalf("DeleteFlag failed: %v", err)
	}

	// 7. ListAuditLogs
	mock.ExpectQuery(`SELECT id, flag_key, action, environment, actor, details, timestamp FROM audit_logs ORDER BY timestamp DESC LIMIT \$1`).
		WithArgs(10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "flag_key", "action", "environment", "actor", "details", "timestamp"}).
			AddRow("log_01", "ai-smart-search", "FLAG_CREATED", "production", "admin@flagura.dev", "Created flag", time.Now()))

	logs, err := st.ListAuditLogs(ctx, 10)
	if err != nil || len(logs) != 1 {
		t.Fatalf("ListAuditLogs failed: %v", err)
	}
}

func TestPostgresStore_GovernanceAndMultiTenancy(t *testing.T) {
	st, mock := newMockPostgresStore(t)
	defer st.db.Close()
	ctx := context.Background()

	// 1. Organization
	org := domain.Organization{
		ID:          "org_test_01",
		Name:        "Test Org",
		Slug:        "test-org",
		Description: "Test Org Description",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	mock.ExpectExec(`INSERT INTO organizations`).
		WithArgs(org.ID, org.Name, org.Slug, org.Description, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	createdOrg, err := st.CreateOrganization(ctx, org)
	if err != nil || createdOrg.ID != org.ID {
		t.Fatalf("CreateOrganization failed: %v", err)
	}

	mock.ExpectQuery(`SELECT id, name, slug, description, created_at, updated_at FROM organizations WHERE id = \$1`).
		WithArgs(org.ID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "description", "created_at", "updated_at"}).
			AddRow(org.ID, org.Name, org.Slug, org.Description, org.CreatedAt, org.UpdatedAt))

	fetchedOrg, err := st.GetOrganization(ctx, org.ID)
	if err != nil || fetchedOrg.ID != org.ID {
		t.Fatalf("GetOrganization failed: %v", err)
	}

	mock.ExpectQuery(`SELECT id, name, slug, description, created_at, updated_at FROM organizations ORDER BY created_at ASC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "description", "created_at", "updated_at"}).
			AddRow(org.ID, org.Name, org.Slug, org.Description, org.CreatedAt, org.UpdatedAt))

	orgs, err := st.ListOrganizations(ctx)
	if err != nil || len(orgs) != 1 {
		t.Fatalf("ListOrganizations failed: %v", err)
	}

	// 2. Project
	proj := domain.Project{
		ID:             "proj_test_01",
		OrganizationID: org.ID,
		Name:           "Test Project",
		Slug:           "test-project",
		Description:    "Test project desc",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	mock.ExpectExec(`INSERT INTO projects`).
		WithArgs(proj.ID, proj.OrganizationID, proj.Name, proj.Slug, proj.Description, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	createdProj, err := st.CreateProject(ctx, proj)
	if err != nil || createdProj.ID != proj.ID {
		t.Fatalf("CreateProject failed: %v", err)
	}

	mock.ExpectQuery(`SELECT id, organization_id, name, slug, description, created_at, updated_at FROM projects WHERE id = \$1`).
		WithArgs(proj.ID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "name", "slug", "description", "created_at", "updated_at"}).
			AddRow(proj.ID, proj.OrganizationID, proj.Name, proj.Slug, proj.Description, proj.CreatedAt, proj.UpdatedAt))

	fetchedProj, err := st.GetProject(ctx, proj.ID)
	if err != nil || fetchedProj.ID != proj.ID {
		t.Fatalf("GetProject failed: %v", err)
	}

	mock.ExpectQuery(`SELECT id, organization_id, name, slug, description, created_at, updated_at FROM projects`).
		WithArgs(org.ID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "name", "slug", "description", "created_at", "updated_at"}).
			AddRow(proj.ID, proj.OrganizationID, proj.Name, proj.Slug, proj.Description, proj.CreatedAt, proj.UpdatedAt))

	projects, err := st.ListProjects(ctx, org.ID)
	if err != nil || len(projects) != 1 {
		t.Fatalf("ListProjects failed: %v", err)
	}

	// 3. APIKey
	apiKey := domain.APIKey{
		ID:          "key_01",
		ProjectID:   proj.ID,
		Environment: "production",
		KeyPrefix:   "flg_live_***",
		KeyHash:     "hash_123",
		Name:        "Test Key",
		Role:        domain.RoleDeveloper,
		CreatedBy:   "admin@flagura.dev",
		CreatedAt:   time.Now(),
	}
	mock.ExpectExec(`INSERT INTO api_keys`).
		WithArgs(apiKey.ID, apiKey.ProjectID, apiKey.Environment, apiKey.KeyPrefix, apiKey.KeyHash, apiKey.Name, string(apiKey.Role), apiKey.CreatedBy, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	createdKey, err := st.CreateAPIKey(ctx, apiKey)
	if err != nil || createdKey.ID != apiKey.ID {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	mock.ExpectQuery(`SELECT id, project_id, environment, key_prefix, key_hash, name, role, created_by, created_at, last_used_at, revoked FROM api_keys WHERE key_hash = \$1`).
		WithArgs("hash_123").
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "environment", "key_prefix", "key_hash", "name", "role", "created_by", "created_at", "last_used_at", "revoked"}).
			AddRow(apiKey.ID, apiKey.ProjectID, apiKey.Environment, apiKey.KeyPrefix, apiKey.KeyHash, apiKey.Name, string(apiKey.Role), apiKey.CreatedBy, apiKey.CreatedAt, nil, false))

	byHash, err := st.GetAPIKeyByHash(ctx, "hash_123")
	if err != nil || byHash.ID != apiKey.ID {
		t.Fatalf("GetAPIKeyByHash failed: %v", err)
	}

	mock.ExpectQuery(`SELECT id, project_id, environment, key_prefix, name, role, created_by, created_at, last_used_at, revoked FROM api_keys ORDER BY created_at DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "environment", "key_prefix", "name", "role", "created_by", "created_at", "last_used_at", "revoked"}).
			AddRow(apiKey.ID, apiKey.ProjectID, apiKey.Environment, apiKey.KeyPrefix, apiKey.Name, string(apiKey.Role), apiKey.CreatedBy, apiKey.CreatedAt, nil, false))

	keys, err := st.ListAPIKeys(ctx)
	if err != nil || len(keys) != 1 {
		t.Fatalf("ListAPIKeys failed: %v", err)
	}

	mock.ExpectExec(`UPDATE api_keys`).
		WithArgs(apiKey.ID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO audit_logs`).
		WithArgs(sqlmock.AnyArg(), "api-keys", "all", "API_KEY_REVOKED", "admin@flagura.dev", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := st.RevokeAPIKey(ctx, apiKey.ID, "admin@flagura.dev"); err != nil {
		t.Fatalf("RevokeAPIKey failed: %v", err)
	}

	// 4. ChangeRequest
	cr := domain.ChangeRequest{
		ID:           "cr_01",
		ProjectID:    proj.ID,
		FlagKey:      "ai-smart-search",
		Environment:  domain.EnvProduction,
		AuthorUserID: "u_alice_01",
		Title:        "Turn on AI search",
		Description:  "CR description",
		Status:       domain.ChangeRequestStatusPending,
		ProposedConfig: domain.EnvironmentConfig{
			Enabled:    true,
			Percentage: 100,
		},
		CreatedAt: time.Now(),
	}
	crJSON, _ := json.Marshal(cr.ProposedConfig)

	mock.ExpectExec(`INSERT INTO change_requests`).
		WithArgs(cr.ID, cr.FlagKey, string(cr.Environment), cr.Title, cr.Description, cr.AuthorUserID, cr.AuthorEmail, cr.AuthorName, crJSON, string(cr.Status), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	createdCR, err := st.CreateChangeRequest(ctx, cr)
	if err != nil || createdCR.ID != cr.ID {
		t.Fatalf("CreateChangeRequest failed: %v", err)
	}

	mock.ExpectQuery(`SELECT id, flag_key, environment, title, description, author_user_id, author_email, author_name, proposed_config, status, reviewer_user_id, reviewer_email, reviewer_name, review_comments, created_at, reviewed_at, applied_at FROM change_requests WHERE id = \$1`).
		WithArgs(cr.ID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "flag_key", "environment", "title", "description", "author_user_id", "author_email", "author_name", "proposed_config", "status", "reviewer_user_id", "reviewer_email", "reviewer_name", "review_comments", "created_at", "reviewed_at", "applied_at"}).
			AddRow(cr.ID, cr.FlagKey, string(cr.Environment), cr.Title, cr.Description, cr.AuthorUserID, "alice@flagura.dev", "Alice", crJSON, string(cr.Status), "", "", "", "", cr.CreatedAt, nil, nil))

	fetchedCR, err := st.GetChangeRequest(ctx, cr.ID)
	if err != nil || fetchedCR.ID != cr.ID {
		t.Fatalf("GetChangeRequest failed: %v", err)
	}

	mock.ExpectQuery(`SELECT id, flag_key, environment, title, description, author_user_id, author_email, author_name, proposed_config, status, reviewer_user_id, reviewer_email, reviewer_name, review_comments, created_at, reviewed_at, applied_at FROM change_requests ORDER BY created_at DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "flag_key", "environment", "title", "description", "author_user_id", "author_email", "author_name", "proposed_config", "status", "reviewer_user_id", "reviewer_email", "reviewer_name", "review_comments", "created_at", "reviewed_at", "applied_at"}).
			AddRow(cr.ID, cr.FlagKey, string(cr.Environment), cr.Title, cr.Description, cr.AuthorUserID, "alice@flagura.dev", "Alice", crJSON, string(cr.Status), "", "", "", "", cr.CreatedAt, nil, nil))

	crs, err := st.ListChangeRequests(ctx, "")
	if err != nil || len(crs) != 1 {
		t.Fatalf("ListChangeRequests failed: %v", err)
	}

	// 5. PasswordResetToken
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE email = \$1`).
		WithArgs("alice@flagura.dev").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO password_reset_tokens`).
		WithArgs(sqlmock.AnyArg(), "alice@flagura.dev", sqlmock.AnyArg(), false, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	tok, err := st.CreatePasswordResetToken(ctx, "alice@flagura.dev", 1*time.Hour)
	if err != nil || tok == "" {
		t.Fatalf("CreatePasswordResetToken failed: %v", err)
	}

	mock.ExpectQuery(`SELECT token, email, expires_at, used, created_at FROM password_reset_tokens WHERE token = \$1`).
		WithArgs(tok).
		WillReturnRows(sqlmock.NewRows([]string{"token", "email", "expires_at", "used", "created_at"}).
			AddRow(tok, "alice@flagura.dev", time.Now().Add(1*time.Hour), false, time.Now()))

	fetchedToken, err := st.GetPasswordResetToken(ctx, tok)
	if err != nil || fetchedToken.Email != "alice@flagura.dev" {
		t.Fatalf("GetPasswordResetToken failed: %v", err)
	}
}

func TestPostgresStore_ExperimentEvents(t *testing.T) {
	st, mock := newMockPostgresStore(t)
	defer st.db.Close()
	ctx := context.Background()

	events := []domain.ExperimentEvent{
		{
			ID:          "ev_01",
			ProjectID:   "proj_default",
			FlagKey:     "checkout-v2",
			Variant:     "treatment",
			MetricName:  "signup",
			EventType:   domain.EventTypeConversion,
			Value:       1.0,
			UserID:      "usr_100",
			Environment: domain.EnvProduction,
			Timestamp:   time.Now(),
		},
	}

	mock.ExpectBegin()
	mock.ExpectPrepare(`INSERT INTO experiment_events`)
	mock.ExpectExec(`INSERT INTO experiment_events`).
		WithArgs(sqlmock.AnyArg(), events[0].FlagKey, events[0].Variant, events[0].MetricName, string(events[0].EventType), events[0].Value, events[0].UserID, string(events[0].Environment), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := st.RecordExperimentEvents(ctx, events); err != nil {
		t.Fatalf("RecordExperimentEvents failed: %v", err)
	}

	mock.ExpectQuery(`SELECT id, flag_key, variant, metric_name, event_type, value, user_id, environment, timestamp FROM experiment_events WHERE flag_key = \$1 ORDER BY timestamp DESC LIMIT \$2`).
		WithArgs("checkout-v2", 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "flag_key", "variant", "metric_name", "event_type", "value", "user_id", "environment", "timestamp"}).
			AddRow(events[0].ID, events[0].FlagKey, events[0].Variant, events[0].MetricName, string(events[0].EventType), events[0].Value, events[0].UserID, string(events[0].Environment), events[0].Timestamp))

	fetchedEvents, err := st.GetExperimentEvents(ctx, "checkout-v2", 100)
	if err != nil || len(fetchedEvents) != 1 {
		t.Fatalf("GetExperimentEvents failed: %v", err)
	}
}

func TestPostgresStore_ProjectScopedLists(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	st := &PostgresStore{db: db}
	ctx := context.Background()

	// 1. ListAuditLogsByProject
	mock.ExpectQuery(`SELECT id, project_id, flag_key, action, environment, actor, details, timestamp FROM audit_logs WHERE project_id = \$1 ORDER BY timestamp DESC LIMIT \$2`).
		WithArgs("proj_100", 50).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "flag_key", "action", "environment", "actor", "details", "timestamp"}).
			AddRow("log_01", "proj_100", "feat-1", "TOGGLE", "production", "actor@test.com", "toggled", time.Now()))

	logs, err := st.ListAuditLogsByProject(ctx, "proj_100", 50)
	if err != nil || len(logs) != 1 {
		t.Fatalf("ListAuditLogsByProject failed: %v", err)
	}

	// 2. ListChangeRequestsByProject (with status)
	mock.ExpectQuery(`SELECT id, project_id, flag_key, environment, title, description, author_user_id, author_email, author_name, proposed_config, status, reviewer_user_id, reviewer_email, reviewer_name, review_comments, created_at, reviewed_at, applied_at FROM change_requests WHERE project_id = \$1 AND status = \$2 ORDER BY created_at DESC`).
		WithArgs("proj_100", domain.ChangeRequestStatusPending).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "flag_key", "environment", "title", "description", "author_user_id", "author_email", "author_name", "proposed_config", "status", "reviewer_user_id", "reviewer_email", "reviewer_name", "review_comments", "created_at", "reviewed_at", "applied_at"}).
			AddRow("cr_01", "proj_100", "feat-1", "production", "title", "desc", "u1", "u1@test.com", "U1", []byte(`{}`), string(domain.ChangeRequestStatusPending), "", "", "", "", time.Now(), nil, nil))

	crsWithStatus, err := st.ListChangeRequestsByProject(ctx, "proj_100", domain.ChangeRequestStatusPending)
	if err != nil || len(crsWithStatus) != 1 {
		t.Fatalf("ListChangeRequestsByProject with status failed: %v", err)
	}

	// 3. ListChangeRequestsByProject (without status)
	mock.ExpectQuery(`SELECT id, project_id, flag_key, environment, title, description, author_user_id, author_email, author_name, proposed_config, status, reviewer_user_id, reviewer_email, reviewer_name, review_comments, created_at, reviewed_at, applied_at FROM change_requests WHERE project_id = \$1 ORDER BY created_at DESC`).
		WithArgs("proj_100").
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "flag_key", "environment", "title", "description", "author_user_id", "author_email", "author_name", "proposed_config", "status", "reviewer_user_id", "reviewer_email", "reviewer_name", "review_comments", "created_at", "reviewed_at", "applied_at"}).
			AddRow("cr_01", "proj_100", "feat-1", "production", "title", "desc", "u1", "u1@test.com", "U1", []byte(`{}`), string(domain.ChangeRequestStatusPending), "", "", "", "", time.Now(), nil, nil))

	crsAll, err := st.ListChangeRequestsByProject(ctx, "proj_100", "")
	if err != nil || len(crsAll) != 1 {
		t.Fatalf("ListChangeRequestsByProject all failed: %v", err)
	}

	// 4. ListAPIKeysByProject
	mock.ExpectQuery(`SELECT id, project_id, environment, key_prefix, name, role, created_by, created_at, last_used_at, revoked FROM api_keys WHERE project_id = \$1 AND revoked = FALSE ORDER BY created_at DESC`).
		WithArgs("proj_100").
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "environment", "key_prefix", "name", "role", "created_by", "created_at", "last_used_at", "revoked"}).
			AddRow("key_01", "proj_100", "production", "flg_live_", "Key 1", "developer", "u1", time.Now(), nil, false))

	apiKeys, err := st.ListAPIKeysByProject(ctx, "proj_100")
	if err != nil || len(apiKeys) != 1 {
		t.Fatalf("ListAPIKeysByProject failed: %v", err)
	}
}

func TestPostgresStore_FlagMutationsAndUserOperations(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	st := &PostgresStore{db: db}
	ctx := context.Background()

	now := time.Now()
	testFlag := domain.FeatureFlag{
		ID:        "flg_mutate_01",
		ProjectID: "proj_100",
		Key:       "mutate-feat",
		Name:      "Mutate Feature",
		Type:      "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    true,
				Percentage: 50,
				Strategy:   domain.StrategyPercentage,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	envsJSON, _ := json.Marshal(testFlag.Environments)

	// 1. ToggleFlag
	mock.ExpectQuery(`SELECT id, project_id, config_version, key, name, description, type, tags, environments, created_at, updated_at FROM feature_flags WHERE project_id = \$1 AND \(key = \$2 OR id = \$2\) LIMIT 1`).
		WithArgs(DefaultProjectID, "mutate-feat").
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "config_version", "key", "name", "description", "type", "tags", "environments", "created_at", "updated_at"}).
			AddRow(testFlag.ID, testFlag.ProjectID, 1, testFlag.Key, testFlag.Name, testFlag.Description, testFlag.Type, "{}", envsJSON, now, now))

	mock.ExpectExec(`UPDATE feature_flags SET environments = \$1, config_version = config_version \+ 1, updated_at = \$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), testFlag.ID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec(`INSERT INTO audit_logs`).
		WithArgs(sqlmock.AnyArg(), testFlag.Key, "KILL_SWITCH_TOGGLED", sqlmock.AnyArg(), "admin@flagura.dev", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	toggled, _, err := st.ToggleFlag(ctx, "mutate-feat", domain.EnvProduction, nil, "admin@flagura.dev")
	if err != nil || toggled == nil {
		t.Fatalf("ToggleFlag failed: %v", err)
	}

	// 2. UpdateRollout
	mock.ExpectQuery(`SELECT id, project_id, config_version, key, name, description, type, tags, environments, created_at, updated_at FROM feature_flags WHERE project_id = \$1 AND \(key = \$2 OR id = \$2\) LIMIT 1`).
		WithArgs(DefaultProjectID, "mutate-feat").
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "config_version", "key", "name", "description", "type", "tags", "environments", "created_at", "updated_at"}).
			AddRow(testFlag.ID, testFlag.ProjectID, 1, testFlag.Key, testFlag.Name, testFlag.Description, testFlag.Type, "{}", envsJSON, now, now))

	mock.ExpectExec(`UPDATE feature_flags SET environments = \$1, config_version = config_version \+ 1, updated_at = \$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), testFlag.ID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec(`INSERT INTO audit_logs`).
		WithArgs(sqlmock.AnyArg(), testFlag.Key, "ROLLOUT_CHANGED", sqlmock.AnyArg(), "admin@flagura.dev", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	rolled, _, err := st.UpdateRollout(ctx, "mutate-feat", domain.EnvProduction, 80, "admin@flagura.dev")
	if err != nil || rolled == nil {
		t.Fatalf("UpdateRollout failed: %v", err)
	}

	// 3. DeleteFlag
	mock.ExpectQuery(`SELECT id, project_id, config_version, key, name, description, type, tags, environments, created_at, updated_at FROM feature_flags WHERE project_id = \$1 AND \(key = \$2 OR id = \$2\) LIMIT 1`).
		WithArgs(DefaultProjectID, "mutate-feat").
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "config_version", "key", "name", "description", "type", "tags", "environments", "created_at", "updated_at"}).
			AddRow(testFlag.ID, testFlag.ProjectID, 1, testFlag.Key, testFlag.Name, testFlag.Description, testFlag.Type, "{}", envsJSON, now, now))

	mock.ExpectExec(`DELETE FROM feature_flags WHERE id = \$1 OR key = \$1`).
		WithArgs("mutate-feat").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec(`INSERT INTO audit_logs`).
		WithArgs(sqlmock.AnyArg(), testFlag.Key, "FLAG_DELETED", "all", "admin@flagura.dev", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	delLog, err := st.DeleteFlag(ctx, "mutate-feat", "admin@flagura.dev")
	if err != nil || delLog == nil {
		t.Fatalf("DeleteFlag failed: %v", err)
	}

	// 4. GetUserByEmail & ListUsers & DeleteSession
	mock.ExpectQuery(`SELECT id, email, password_hash, name, role, avatar_url, created_at, updated_at FROM users WHERE email = \$1`).
		WithArgs("alice@flagura.dev").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "name", "role", "avatar_url", "created_at", "updated_at"}).
			AddRow("u_alice_01", "alice@flagura.dev", "pwd_hash", "Alice", "admin", "", now, now))

	uByEmail, err := st.GetUserByEmail(ctx, "alice@flagura.dev")
	if err != nil || uByEmail == nil {
		t.Fatalf("GetUserByEmail failed: %v", err)
	}

	mock.ExpectQuery(`SELECT id, email, name, role, avatar_url, created_at, updated_at FROM users ORDER BY created_at ASC`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name", "role", "avatar_url", "created_at", "updated_at"}).
			AddRow("u_alice_01", "alice@flagura.dev", "Alice", "admin", "", now, now))

	users, err := st.ListUsers(ctx)
	if err != nil || len(users) != 1 {
		t.Fatalf("ListUsers failed: %v", err)
	}

	mock.ExpectExec(`DELETE FROM sessions WHERE token = \$1`).
		WithArgs("tok_session_123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := st.DeleteSession(ctx, "tok_session_123"); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}
}

func TestPostgresStore_GovernanceAndAuthExtended(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	st := &PostgresStore{db: db, driverName: "PostgreSQL Mock"}
	ctx := context.Background()

	// 1. DriverName and Ping
	if st.DriverName() != "PostgreSQL Mock" {
		t.Errorf("DriverName mismatch")
	}
	mock.ExpectPing()
	if err := st.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	// 2. ReviewChangeRequest
	cr := domain.ChangeRequest{
		ID:           "cr_review_01",
		ProjectID:    "proj_default",
		FlagKey:      "feat-cr",
		Environment:  domain.EnvProduction,
		AuthorUserID: "u_author_01",
		Title:        "Turn on feat-cr",
		Status:       domain.ChangeRequestStatusPending,
		CreatedAt:    time.Now(),
	}
	mock.ExpectQuery(`SELECT id, flag_key, environment, title, description, author_user_id, author_email, author_name, proposed_config, status, reviewer_user_id, reviewer_email, reviewer_name, review_comments, created_at, reviewed_at, applied_at FROM change_requests WHERE id = \$1`).
		WithArgs(cr.ID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "flag_key", "environment", "title", "description", "author_user_id", "author_email", "author_name", "proposed_config", "status", "reviewer_user_id", "reviewer_email", "reviewer_name", "review_comments", "created_at", "reviewed_at", "applied_at"}).
			AddRow(cr.ID, cr.FlagKey, string(cr.Environment), cr.Title, "", cr.AuthorUserID, "author@test.com", "Author", []byte(`{}`), string(cr.Status), "", "", "", "", cr.CreatedAt, nil, nil))

	mock.ExpectExec(`UPDATE change_requests SET status = \$1, reviewer_user_id = \$2, reviewer_email = \$3, reviewer_name = \$4, review_comments = \$5, reviewed_at = \$6 WHERE id = \$7`).
		WithArgs(string(domain.ChangeRequestStatusApproved), "u_reviewer_01", "reviewer@test.com", "Reviewer", "LGTM", sqlmock.AnyArg(), cr.ID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	reviewedCR, err := st.ReviewChangeRequest(ctx, cr.ID, "u_reviewer_01", "reviewer@test.com", "Reviewer", true, "LGTM")
	if err != nil || reviewedCR.Status != domain.ChangeRequestStatusApproved {
		t.Fatalf("ReviewChangeRequest failed: %v", err)
	}

	// 3. ApplyChangeRequest
	mock.ExpectQuery(`SELECT id, flag_key, environment, title, description, author_user_id, author_email, author_name, proposed_config, status, reviewer_user_id, reviewer_email, reviewer_name, review_comments, created_at, reviewed_at, applied_at FROM change_requests WHERE id = \$1`).
		WithArgs(cr.ID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "flag_key", "environment", "title", "description", "author_user_id", "author_email", "author_name", "proposed_config", "status", "reviewer_user_id", "reviewer_email", "reviewer_name", "review_comments", "created_at", "reviewed_at", "applied_at"}).
			AddRow(cr.ID, cr.FlagKey, string(cr.Environment), cr.Title, "", cr.AuthorUserID, "author@test.com", "Author", []byte(`{"enabled":true,"percentage":100}`), string(domain.ChangeRequestStatusApproved), "u_reviewer_01", "reviewer@test.com", "Reviewer", "LGTM", cr.CreatedAt, time.Now(), nil))

	mock.ExpectQuery(`SELECT id, project_id, config_version, key, name, description, type, tags, environments, created_at, updated_at FROM feature_flags WHERE project_id = \$1 AND \(key = \$2 OR id = \$2\) LIMIT 1`).
		WithArgs(DefaultProjectID, cr.FlagKey).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "config_version", "key", "name", "description", "type", "tags", "environments", "created_at", "updated_at"}).
			AddRow("flg_feat_cr", "proj_default", 1, cr.FlagKey, "Feat CR", "description", "boolean", "{}", []byte(`{}`), time.Now(), time.Now()))

	mock.ExpectExec(`INSERT INTO feature_flags`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), cr.FlagKey, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec(`INSERT INTO audit_logs`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), cr.FlagKey, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec(`UPDATE change_requests SET status = \$1, applied_at = \$2 WHERE id = \$3`).
		WithArgs(string(domain.ChangeRequestStatusApplied), sqlmock.AnyArg(), cr.ID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	appliedFlag, appliedCR, _, err := st.ApplyChangeRequest(ctx, cr.ID, "admin@flagura.dev")
	if err != nil || appliedFlag == nil || appliedCR.Status != domain.ChangeRequestStatusApplied {
		t.Fatalf("ApplyChangeRequest failed: %v", err)
	}

	// 4. ResetPasswordWithToken
	mock.ExpectQuery(`SELECT token, email, expires_at, used, created_at FROM password_reset_tokens WHERE token = \$1`).
		WithArgs("tok_reset_123").
		WillReturnRows(sqlmock.NewRows([]string{"token", "email", "expires_at", "used", "created_at"}).
			AddRow("tok_reset_123", "alice@flagura.dev", time.Now().Add(15*time.Minute), false, time.Now()))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users SET password_hash = \$1, updated_at = \$2 WHERE email = \$3`).
		WithArgs("new_hashed_pwd", sqlmock.AnyArg(), "alice@flagura.dev").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec(`UPDATE password_reset_tokens SET used = TRUE WHERE token = \$1`).
		WithArgs("tok_reset_123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec(`DELETE FROM sessions WHERE user_id IN \(SELECT id FROM users WHERE email = \$1\)`).
		WithArgs("alice@flagura.dev").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	if err := st.ResetPasswordWithToken(ctx, "tok_reset_123", "new_hashed_pwd"); err != nil {
		t.Fatalf("ResetPasswordWithToken failed: %v", err)
	}
}

func TestPostgresStore_OrgMembersAndInvitations(t *testing.T) {
	st, mock := newMockPostgresStore(t)
	defer st.db.Close()
	ctx := context.Background()

	// 1. CreateOrgMember
	member := domain.OrgMember{
		ID:             "mem_test_01",
		OrganizationID: "org_test_01",
		UserID:         "usr_owner_01",
		Role:           "owner",
		CreatedAt:      time.Now(),
	}
	mock.ExpectExec(`INSERT INTO org_members`).
		WithArgs(sqlmock.AnyArg(), member.OrganizationID, member.UserID, member.Role, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	createdMember, err := st.CreateOrgMember(ctx, member)
	if err != nil || createdMember.OrganizationID != member.OrganizationID {
		t.Fatalf("CreateOrgMember failed: %v", err)
	}

	// 2. ListOrgMembers
	mock.ExpectQuery(`SELECT id, organization_id, user_id, role, created_at FROM org_members WHERE organization_id = \$1`).
		WithArgs("org_test_01").
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "user_id", "role", "created_at"}).
			AddRow(member.ID, member.OrganizationID, member.UserID, member.Role, member.CreatedAt))

	members, err := st.ListOrgMembers(ctx, "org_test_01")
	if err != nil || len(members) != 1 {
		t.Fatalf("ListOrgMembers failed: %v", err)
	}

	// 3. ListUserOrganizations
	mock.ExpectQuery(`SELECT role FROM users WHERE id = \$1`).
		WithArgs("usr_owner_01").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("developer"))

	mock.ExpectQuery(`SELECT o.id, o.name, o.slug, o.description, o.created_at, o.updated_at FROM organizations o INNER JOIN org_members m ON o.id = m.organization_id WHERE m.user_id = \$1 ORDER BY o.created_at ASC`).
		WithArgs("usr_owner_01").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "description", "created_at", "updated_at"}).
			AddRow("org_test_01", "Test Org", "test-org", "Description", time.Now(), time.Now()))

	userOrgs, err := st.ListUserOrganizations(ctx, "usr_owner_01")
	if err != nil || len(userOrgs) != 1 {
		t.Fatalf("ListUserOrganizations failed: %v", err)
	}

	// 4. CreateOrgInvitation
	invite := domain.OrgInvitation{
		OrganizationID: "org_test_01",
		Email:          "invitee@example.com",
		Role:           "developer",
		InvitedBy:      "usr_owner_01",
	}
	mock.ExpectExec(`INSERT INTO org_invitations`).
		WithArgs(sqlmock.AnyArg(), invite.OrganizationID, sqlmock.AnyArg(), invite.Email, sqlmock.AnyArg(), invite.Role, invite.InvitedBy, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	createdInvite, err := st.CreateOrgInvitation(ctx, invite)
	if err != nil || createdInvite.Token == "" {
		t.Fatalf("CreateOrgInvitation failed: %v", err)
	}

	// 5. GetOrgInvitation
	mock.ExpectQuery(`SELECT id, organization_id, org_name, email, token, role, invited_by, expires_at, accepted_at, created_at FROM org_invitations WHERE token = \$1`).
		WithArgs(createdInvite.Token).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "org_name", "email", "token", "role", "invited_by", "expires_at", "accepted_at", "created_at"}).
			AddRow("inv_01", "org_test_01", "Test Org", "invitee@example.com", createdInvite.Token, "developer", "usr_owner_01", time.Now().Add(7*24*time.Hour), nil, time.Now()))

	retrievedInvite, err := st.GetOrgInvitation(ctx, createdInvite.Token)
	if err != nil || retrievedInvite.Email != "invitee@example.com" {
		t.Fatalf("GetOrgInvitation failed: %v", err)
	}

	// 6. AcceptOrgInvitation
	mock.ExpectQuery(`SELECT id, organization_id, org_name, email, token, role, invited_by, expires_at, accepted_at, created_at FROM org_invitations WHERE token = \$1`).
		WithArgs(createdInvite.Token).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "org_name", "email", "token", "role", "invited_by", "expires_at", "accepted_at", "created_at"}).
			AddRow("inv_01", "org_test_01", "Test Org", "invitee@example.com", createdInvite.Token, "developer", "usr_owner_01", time.Now().Add(7*24*time.Hour), nil, time.Now()))

	mock.ExpectExec(`UPDATE org_invitations SET accepted_at = \$1 WHERE token = \$2`).
		WithArgs(sqlmock.AnyArg(), createdInvite.Token).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec(`INSERT INTO org_members`).
		WithArgs(sqlmock.AnyArg(), "org_test_01", "usr_invitee", "developer", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	acceptedMember, err := st.AcceptOrgInvitation(ctx, createdInvite.Token, "usr_invitee")
	if err != nil || acceptedMember.Role != "developer" {
		t.Fatalf("AcceptOrgInvitation failed: %v", err)
	}

	// 7. ListOrgInvitations
	mock.ExpectQuery(`SELECT id, organization_id, org_name, email, token, role, invited_by, expires_at, accepted_at, created_at FROM org_invitations WHERE organization_id = \$1`).
		WithArgs("org_test_01").
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "org_name", "email", "role", "token", "invited_by", "expires_at", "accepted_at", "created_at"}).
			AddRow("inv_01", "org_test_01", "Test Org", "invitee@example.com", "developer", createdInvite.Token, "usr_owner_01", time.Now().Add(7*24*time.Hour), time.Now(), time.Now()))

	invites, err := st.ListOrgInvitations(ctx, "org_test_01")
	if err != nil || len(invites) != 1 {
		t.Fatalf("ListOrgInvitations failed: %v", err)
	}

	// 8. Reset
	mock.ExpectExec(`TRUNCATE feature_flags, audit_logs`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS organizations`).WillReturnResult(sqlmock.NewResult(1, 1))

	if err := st.Reset(ctx); err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	// 9. ListAPIKeysByProject
	mock.ExpectQuery(`SELECT id, project_id, environment, key_prefix, name, role, created_by, created_at, last_used_at, revoked FROM api_keys WHERE project_id = \$1 AND revoked = FALSE ORDER BY created_at DESC`).
		WithArgs("proj_test_01").
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "environment", "key_prefix", "name", "role", "created_by", "created_at", "last_used_at", "revoked"}).
			AddRow("key_01", "proj_test_01", "production", "flg_live_***", "Test Key", string(domain.RoleDeveloper), "admin@flagura.dev", time.Now(), nil, false))

	keys, err := st.ListAPIKeysByProject(ctx, "proj_test_01")
	if err != nil || len(keys) != 1 {
		t.Fatalf("ListAPIKeysByProject failed: %v", err)
	}
}
