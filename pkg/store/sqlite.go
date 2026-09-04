package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	_ "modernc.org/sqlite"
)

// SQLiteStore provides a persistent, embedded SQLite-backed implementation of Store.
type SQLiteStore struct {
	db *sql.DB
	mu sync.RWMutex
}

// CleanSQLitePath cleans a database URL or path to a filesystem path or :memory:.
func CleanSQLitePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == ":memory:" {
		return ":memory:"
	}
	if strings.HasPrefix(raw, "sqlite://") {
		raw = strings.TrimPrefix(raw, "sqlite://")
	} else if strings.HasPrefix(raw, "sqlite3://") {
		raw = strings.TrimPrefix(raw, "sqlite3://")
	} else if strings.HasPrefix(raw, "file:") {
		raw = strings.TrimPrefix(raw, "file:")
	}
	return raw
}

// NewSQLiteStore initializes a new SQLite store with WAL mode and schema auto-migrations.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	cleanPath := CleanSQLitePath(dbPath)

	if cleanPath != ":memory:" {
		dir := filepath.Dir(cleanPath)
		if dir != "." && dir != "/" {
			if err := os.MkdirAll(dir, 0750); err != nil {
				return nil, fmt.Errorf("failed to create sqlite directory: %w", err)
			}
		}
	}

	dsn := cleanPath
	if cleanPath != ":memory:" && !strings.Contains(cleanPath, "?") {
		dsn = cleanPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1) // Serialized connection for safe embedded writes
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	s := &SQLiteStore{db: db}

	// Apply PRAGMAs explicitly
	_, _ = db.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA foreign_keys = ON;
		PRAGMA busy_timeout = 5000;
		PRAGMA synchronous = NORMAL;
	`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.autoMigrate(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to auto-migrate sqlite schema: %w", err)
	}

	if err := s.seedDefaults(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to seed sqlite defaults: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) autoMigrate(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS organizations (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		slug TEXT UNIQUE NOT NULL,
		description TEXT DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS projects (
		id TEXT PRIMARY KEY,
		organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		slug TEXT NOT NULL,
		description TEXT DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(organization_id, slug)
	);

	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		name TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'developer',
		avatar_url TEXT DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS sessions (
		token TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at TEXT NOT NULL,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS feature_flags (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL DEFAULT 'proj_default',
		key TEXT NOT NULL,
		config_version INTEGER NOT NULL DEFAULT 1,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		type TEXT NOT NULL DEFAULT 'boolean',
		tags TEXT DEFAULT '[]',
		environments TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(project_id, key)
	);

	CREATE TABLE IF NOT EXISTS audit_logs (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL DEFAULT 'proj_default',
		flag_key TEXT NOT NULL,
		action TEXT NOT NULL,
		environment TEXT NOT NULL,
		actor TEXT NOT NULL,
		details TEXT NOT NULL,
		timestamp TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS experiment_events (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL DEFAULT 'proj_default',
		flag_key TEXT NOT NULL,
		variant TEXT NOT NULL,
		metric_name TEXT NOT NULL,
		event_type TEXT NOT NULL DEFAULT 'conversion',
		value REAL NOT NULL DEFAULT 1.0,
		user_id TEXT DEFAULT '',
		environment TEXT NOT NULL DEFAULT 'production',
		timestamp TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS change_requests (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL DEFAULT 'proj_default',
		flag_key TEXT NOT NULL,
		environment TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		author_user_id TEXT NOT NULL,
		author_email TEXT NOT NULL,
		author_name TEXT NOT NULL,
		proposed_config TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'PENDING',
		reviewer_user_id TEXT DEFAULT '',
		reviewer_email TEXT DEFAULT '',
		reviewer_name TEXT DEFAULT '',
		review_comments TEXT DEFAULT '',
		created_at TEXT NOT NULL,
		reviewed_at TEXT,
		applied_at TEXT
	);

	CREATE TABLE IF NOT EXISTS api_keys (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL DEFAULT 'proj_default',
		environment TEXT NOT NULL DEFAULT 'production',
		key_prefix TEXT NOT NULL,
		key_hash TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'developer',
		created_by TEXT NOT NULL,
		created_at TEXT NOT NULL,
		last_used_at TEXT,
		revoked INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS password_reset_tokens (
		token TEXT PRIMARY KEY,
		email TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		used INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS org_members (
		id TEXT PRIMARY KEY,
		organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		role TEXT NOT NULL DEFAULT 'developer',
		created_at TEXT NOT NULL,
		UNIQUE(organization_id, user_id)
	);

	CREATE TABLE IF NOT EXISTS org_invitations (
		id TEXT PRIMARY KEY,
		organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
		org_name TEXT NOT NULL,
		email TEXT NOT NULL,
		token TEXT UNIQUE NOT NULL,
		role TEXT NOT NULL DEFAULT 'developer',
		invited_by TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		accepted_at TEXT
	);
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func (s *SQLiteStore) seedDefaults(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)

	// Seed default organization if missing
	var orgCount int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM organizations WHERE id = ?", DefaultOrgID).Scan(&orgCount); err == nil && orgCount == 0 {
		_, _ = s.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO organizations (id, name, slug, description, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, DefaultOrgID, DefaultOrgName, DefaultOrgSlug, "Default workspace organization", now, now)
	}

	// Seed default project if missing
	var projCount int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM projects WHERE id = ?", DefaultProjectID).Scan(&projCount); err == nil && projCount == 0 {
		_, _ = s.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO projects (id, organization_id, name, slug, description, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, DefaultProjectID, DefaultOrgID, DefaultProjectName, DefaultProjectSlug, "Default feature flagging project", now, now)
	}

	// Check flags count in default project
	var flagCount int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM feature_flags WHERE project_id = ?", DefaultProjectID).Scan(&flagCount); err == nil && flagCount == 0 {
		seedFlags := []domain.FeatureFlag{
			{
				ID:            "flag_smart_search",
				ProjectID:     DefaultProjectID,
				Key:           "ai-smart-search",
				ConfigVersion: 1,
				Name:          "AI Smart Search Assistant",
				Description:   "Deterministic canary rollout for LLM semantic search",
				Type:          "boolean",
				Tags:          []string{"frontend", "search", "ai"},
				Environments: map[domain.Environment]domain.EnvironmentConfig{
					domain.EnvProduction:  {Enabled: true, Strategy: domain.StrategyPercentage, Percentage: 50},
					domain.EnvStaging:     {Enabled: true, Strategy: domain.StrategyPercentage, Percentage: 100},
					domain.EnvDevelopment: {Enabled: true, Strategy: domain.StrategyBoolean, Percentage: 100},
				},
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
			{
				ID:            "flag_v3_checkout",
				ProjectID:     DefaultProjectID,
				Key:           "checkout-v3-streamline",
				ConfigVersion: 1,
				Name:          "Streamlined Checkout Flow v3",
				Description:   "One-click multi-currency checkout modal",
				Type:          "boolean",
				Tags:          []string{"checkout", "payments"},
				Environments: map[domain.Environment]domain.EnvironmentConfig{
					domain.EnvProduction:  {Enabled: true, Strategy: domain.StrategyPercentage, Percentage: 100},
					domain.EnvStaging:     {Enabled: true, Strategy: domain.StrategyPercentage, Percentage: 100},
					domain.EnvDevelopment: {Enabled: true, Strategy: domain.StrategyBoolean, Percentage: 100},
				},
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
			{
				ID:            "flag_killswitch_crypto",
				ProjectID:     DefaultProjectID,
				Key:           "killswitch-crypto-gateways",
				ConfigVersion: 1,
				Name:          "Emergency Crypto Gateway Killswitch",
				Description:   "Instant circuit breaker for decentralized settlement layer",
				Type:          "boolean",
				Tags:          []string{"payments", "compliance"},
				Environments: map[domain.Environment]domain.EnvironmentConfig{
					domain.EnvProduction:  {Enabled: false, Strategy: domain.StrategyBoolean, Percentage: 0},
					domain.EnvStaging:     {Enabled: true, Strategy: domain.StrategyBoolean, Percentage: 100},
					domain.EnvDevelopment: {Enabled: true, Strategy: domain.StrategyBoolean, Percentage: 100},
				},
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
		}

		for _, flag := range seedFlags {
			_, _ = s.SaveFlag(ctx, flag, "system_seed")
		}
	}

	return nil
}

// DriverName returns "SQLite"
func (s *SQLiteStore) DriverName() string {
	return "SQLite"
}

// Ping verifies database health
func (s *SQLiteStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close closes the database handle
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Reset clears and re-seeds the database
func (s *SQLiteStore) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	truncateQueries := []string{
		"DELETE FROM experiment_events",
		"DELETE FROM audit_logs",
		"DELETE FROM change_requests",
		"DELETE FROM api_keys",
		"DELETE FROM feature_flags",
		"DELETE FROM sessions",
		"DELETE FROM password_reset_tokens",
		"DELETE FROM org_invitations",
		"DELETE FROM org_members",
		"DELETE FROM projects",
		"DELETE FROM organizations",
		"DELETE FROM users",
	}

	for _, q := range truncateQueries {
		// #nosec G202 -- q is a static internal query with zero dynamic user input
		_, _ = s.db.ExecContext(ctx, q)
	}

	return s.seedDefaults(ctx)
}

// --- ORGANIZATIONS & PROJECTS ---

func (s *SQLiteStore) CreateOrganization(ctx context.Context, org domain.Organization) (*domain.Organization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if org.ID == "" {
		org.ID = "org_" + generateHexToken(12)
	}
	now := time.Now().UTC()
	org.CreatedAt = now
	org.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO organizations (id, name, slug, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, org.ID, org.Name, org.Slug, org.Description, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return &org, nil
}

func (s *SQLiteStore) GetOrganization(ctx context.Context, idOrSlug string) (*domain.Organization, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, slug, description, created_at, updated_at
		FROM organizations
		WHERE id = ? OR slug = ?
	`, idOrSlug, idOrSlug)

	var org domain.Organization
	var createdStr, updatedStr string
	if err := row.Scan(&org.ID, &org.Name, &org.Slug, &org.Description, &createdStr, &updatedStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("organization not found: %s", idOrSlug)
		}
		return nil, err
	}
	org.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	org.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &org, nil
}

func (s *SQLiteStore) ListOrganizations(ctx context.Context) ([]domain.Organization, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, slug, description, created_at, updated_at
		FROM organizations
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.Organization
	for rows.Next() {
		var org domain.Organization
		var createdStr, updatedStr string
		if err := rows.Scan(&org.ID, &org.Name, &org.Slug, &org.Description, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		org.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		org.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		list = append(list, org)
	}
	return list, nil
}

func (s *SQLiteStore) ListUserOrganizations(ctx context.Context, userID string) ([]domain.Organization, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT o.id, o.name, o.slug, o.description, o.created_at, o.updated_at
		FROM organizations o
		JOIN org_members m ON o.id = m.organization_id
		WHERE m.user_id = ?
		ORDER BY o.name ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.Organization
	for rows.Next() {
		var org domain.Organization
		var createdStr, updatedStr string
		if err := rows.Scan(&org.ID, &org.Name, &org.Slug, &org.Description, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		org.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		org.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		list = append(list, org)
	}
	if len(list) == 0 {
		return s.ListOrganizations(ctx)
	}
	return list, nil
}

func (s *SQLiteStore) CreateProject(ctx context.Context, project domain.Project) (*domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if project.ID == "" {
		project.ID = "proj_" + generateHexToken(12)
	}
	if project.OrganizationID == "" {
		project.OrganizationID = DefaultOrgID
	}
	now := time.Now().UTC()
	project.CreatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (id, organization_id, name, slug, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, project.ID, project.OrganizationID, project.Name, project.Slug, project.Description, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func (s *SQLiteStore) GetProject(ctx context.Context, idOrSlug string) (*domain.Project, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, organization_id, name, slug, description, created_at, updated_at
		FROM projects
		WHERE id = ? OR slug = ?
	`, idOrSlug, idOrSlug)

	var p domain.Project
	var createdStr, updatedStr string
	if err := row.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.Description, &createdStr, &updatedStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("project not found: %s", idOrSlug)
		}
		return nil, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &p, nil
}

func (s *SQLiteStore) ListProjects(ctx context.Context, organizationID string) ([]domain.Project, error) {
	var rows *sql.Rows
	var err error
	if organizationID != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, organization_id, name, slug, description, created_at, updated_at
			FROM projects
			WHERE organization_id = ?
			ORDER BY name ASC
		`, organizationID)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, organization_id, name, slug, description, created_at, updated_at
			FROM projects
			ORDER BY name ASC
		`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.Project
	for rows.Next() {
		var p domain.Project
		var createdStr, updatedStr string
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.Description, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		list = append(list, p)
	}
	return list, nil
}

// --- FEATURE FLAGS ---

func (s *SQLiteStore) ListFlagsByProject(ctx context.Context, projectID string) ([]domain.FeatureFlag, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, key, config_version, name, description, type, tags, environments, created_at, updated_at
		FROM feature_flags
		WHERE project_id = ?
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flags []domain.FeatureFlag
	for rows.Next() {
		var f domain.FeatureFlag
		var tagsJSON, envsJSON string
		var createdStr, updatedStr string
		if err := rows.Scan(
			&f.ID, &f.ProjectID, &f.Key, &f.ConfigVersion, &f.Name, &f.Description,
			&f.Type, &tagsJSON, &envsJSON, &createdStr, &updatedStr,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tagsJSON), &f.Tags)
		_ = json.Unmarshal([]byte(envsJSON), &f.Environments)
		f.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		f.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		flags = append(flags, f)
	}
	return flags, nil
}

func (s *SQLiteStore) GetFlagByProject(ctx context.Context, projectID, keyOrID string) (*domain.FeatureFlag, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, key, config_version, name, description, type, tags, environments, created_at, updated_at
		FROM feature_flags
		WHERE project_id = ? AND (key = ? OR id = ?)
	`, projectID, keyOrID, keyOrID)

	var f domain.FeatureFlag
	var tagsJSON, envsJSON string
	var createdStr, updatedStr string
	if err := row.Scan(
		&f.ID, &f.ProjectID, &f.Key, &f.ConfigVersion, &f.Name, &f.Description,
		&f.Type, &tagsJSON, &envsJSON, &createdStr, &updatedStr,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("flag %q not found in project %s", keyOrID, projectID)
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(tagsJSON), &f.Tags)
	_ = json.Unmarshal([]byte(envsJSON), &f.Environments)
	f.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	f.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &f, nil
}

func (s *SQLiteStore) ListFlags(ctx context.Context) ([]domain.FeatureFlag, error) {
	return s.ListFlagsByProject(ctx, DefaultProjectID)
}

func (s *SQLiteStore) GetFlag(ctx context.Context, keyOrID string) (*domain.FeatureFlag, error) {
	f, err := s.GetFlagByProject(ctx, DefaultProjectID, keyOrID)
	if err == nil {
		return f, nil
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, key, config_version, name, description, type, tags, environments, created_at, updated_at
		FROM feature_flags
		WHERE id = ? OR key = ?
		LIMIT 1
	`, keyOrID, keyOrID)

	var flag domain.FeatureFlag
	var tagsJSON, envsJSON string
	var createdStr, updatedStr string
	if err := row.Scan(
		&flag.ID, &flag.ProjectID, &flag.Key, &flag.ConfigVersion, &flag.Name, &flag.Description,
		&flag.Type, &tagsJSON, &envsJSON, &createdStr, &updatedStr,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("flag %q not found", keyOrID)
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(tagsJSON), &flag.Tags)
	_ = json.Unmarshal([]byte(envsJSON), &flag.Environments)
	flag.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	flag.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &flag, nil
}

func (s *SQLiteStore) SaveFlag(ctx context.Context, flag domain.FeatureFlag, actor string) (*domain.AuditLogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if flag.ID == "" {
		flag.ID = "flag_" + generateHexToken(12)
	}
	if flag.ProjectID == "" {
		flag.ProjectID = DefaultProjectID
	}
	now := time.Now().UTC()
	if flag.CreatedAt.IsZero() {
		flag.CreatedAt = now
	}
	flag.UpdatedAt = now
	flag.ConfigVersion++

	tagsBytes, _ := json.Marshal(flag.Tags)
	envsBytes, _ := json.Marshal(flag.Environments)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO feature_flags (
			id, project_id, key, config_version, name, description, type, tags, environments, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, key) DO UPDATE SET
			config_version = feature_flags.config_version + 1,
			name = excluded.name,
			description = excluded.description,
			type = excluded.type,
			tags = excluded.tags,
			environments = excluded.environments,
			updated_at = excluded.updated_at
	`,
		flag.ID, flag.ProjectID, flag.Key, flag.ConfigVersion, flag.Name, flag.Description,
		flag.Type, string(tagsBytes), string(envsBytes),
		flag.CreatedAt.Format(time.RFC3339), flag.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}

	audit := domain.AuditLogEntry{
		ID:          "audit_" + generateHexToken(12),
		ProjectID:   flag.ProjectID,
		FlagKey:     flag.Key,
		Action:      "SAVE_FLAG",
		Environment: domain.Environment("all"),
		Actor:       actor,
		Details:     fmt.Sprintf("Saved flag %q with %d environments", flag.Key, len(flag.Environments)),
		Timestamp:   now,
	}
	_ = s.insertAuditLog(ctx, audit)

	return &audit, nil
}

func (s *SQLiteStore) DeleteFlag(ctx context.Context, keyOrID string, actor string) (*domain.AuditLogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	flag, err := s.GetFlag(ctx, keyOrID)
	if err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx, "DELETE FROM feature_flags WHERE id = ? OR (project_id = ? AND key = ?)", flag.ID, flag.ProjectID, flag.Key)
	if err != nil {
		return nil, err
	}

	audit := domain.AuditLogEntry{
		ID:          "audit_" + generateHexToken(12),
		ProjectID:   flag.ProjectID,
		FlagKey:     flag.Key,
		Action:      "DELETE_FLAG",
		Environment: domain.Environment("all"),
		Actor:       actor,
		Details:     fmt.Sprintf("Deleted flag %q permanently", flag.Key),
		Timestamp:   time.Now().UTC(),
	}
	_ = s.insertAuditLog(ctx, audit)

	return &audit, nil
}

func (s *SQLiteStore) ToggleFlag(ctx context.Context, keyOrID string, env domain.Environment, enabled *bool, actor string) (*domain.FeatureFlag, *domain.AuditLogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	flag, err := s.GetFlag(ctx, keyOrID)
	if err != nil {
		return nil, nil, err
	}

	cfg := flag.Environments[env]
	var nextState bool
	if enabled != nil {
		nextState = *enabled
	} else {
		nextState = !cfg.Enabled
	}
	cfg.Enabled = nextState
	if flag.Environments == nil {
		flag.Environments = make(map[domain.Environment]domain.EnvironmentConfig)
	}
	flag.Environments[env] = cfg
	flag.UpdatedAt = time.Now().UTC()
	flag.ConfigVersion++

	envsBytes, _ := json.Marshal(flag.Environments)
	_, err = s.db.ExecContext(ctx, `
		UPDATE feature_flags
		SET environments = ?, config_version = config_version + 1, updated_at = ?
		WHERE id = ?
	`, string(envsBytes), flag.UpdatedAt.Format(time.RFC3339), flag.ID)
	if err != nil {
		return nil, nil, err
	}

	audit := domain.AuditLogEntry{
		ID:          "audit_" + generateHexToken(12),
		ProjectID:   flag.ProjectID,
		FlagKey:     flag.Key,
		Action:      "TOGGLE_FLAG",
		Environment: env,
		Actor:       actor,
		Details:     fmt.Sprintf("Toggled flag %q [%s] -> %v", flag.Key, env, nextState),
		Timestamp:   flag.UpdatedAt,
	}
	_ = s.insertAuditLog(ctx, audit)

	return flag, &audit, nil
}

func (s *SQLiteStore) UpdateRollout(ctx context.Context, keyOrID string, env domain.Environment, pct float64, actor string) (*domain.FeatureFlag, *domain.AuditLogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	flag, err := s.GetFlag(ctx, keyOrID)
	if err != nil {
		return nil, nil, err
	}

	cfg := flag.Environments[env]
	cfg.Percentage = pct
	cfg.Strategy = domain.StrategyPercentage
	if flag.Environments == nil {
		flag.Environments = make(map[domain.Environment]domain.EnvironmentConfig)
	}
	flag.Environments[env] = cfg
	flag.UpdatedAt = time.Now().UTC()
	flag.ConfigVersion++

	envsBytes, _ := json.Marshal(flag.Environments)
	_, err = s.db.ExecContext(ctx, `
		UPDATE feature_flags
		SET environments = ?, config_version = config_version + 1, updated_at = ?
		WHERE id = ?
	`, string(envsBytes), flag.UpdatedAt.Format(time.RFC3339), flag.ID)
	if err != nil {
		return nil, nil, err
	}

	audit := domain.AuditLogEntry{
		ID:          "audit_" + generateHexToken(12),
		ProjectID:   flag.ProjectID,
		FlagKey:     flag.Key,
		Action:      "UPDATE_ROLLOUT",
		Environment: env,
		Actor:       actor,
		Details:     fmt.Sprintf("Updated rollout for %q [%s] to %.1f%%", flag.Key, env, pct),
		Timestamp:   flag.UpdatedAt,
	}
	_ = s.insertAuditLog(ctx, audit)

	return flag, &audit, nil
}

// --- AUDIT LOGS ---

func (s *SQLiteStore) insertAuditLog(ctx context.Context, entry domain.AuditLogEntry) error {
	if entry.ID == "" {
		entry.ID = "audit_" + generateHexToken(12)
	}
	if entry.ProjectID == "" {
		entry.ProjectID = DefaultProjectID
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (id, project_id, flag_key, action, environment, actor, details, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.ID, entry.ProjectID, entry.FlagKey, entry.Action, string(entry.Environment), entry.Actor, entry.Details, entry.Timestamp.Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) ListAuditLogsByProject(ctx context.Context, projectID string, limit int) ([]domain.AuditLogEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, flag_key, action, environment, actor, details, timestamp
		FROM audit_logs
		WHERE project_id = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []domain.AuditLogEntry
	for rows.Next() {
		var a domain.AuditLogEntry
		var envStr, tsStr string
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.FlagKey, &a.Action, &envStr, &a.Actor, &a.Details, &tsStr); err != nil {
			return nil, err
		}
		a.Environment = domain.Environment(envStr)
		a.Timestamp, _ = time.Parse(time.RFC3339, tsStr)
		logs = append(logs, a)
	}
	return logs, nil
}

func (s *SQLiteStore) ListAuditLogs(ctx context.Context, limit int) ([]domain.AuditLogEntry, error) {
	return s.ListAuditLogsByProject(ctx, DefaultProjectID, limit)
}

// --- USERS & SESSIONS ---

func (s *SQLiteStore) CreateUser(ctx context.Context, user domain.User) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if user.ID == "" {
		user.ID = "usr_" + generateHexToken(12)
	}
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, name, role, avatar_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, user.ID, strings.ToLower(user.Email), user.PasswordHash, user.Name, user.Role, user.AvatarURL, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *SQLiteStore) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, email, password_hash, name, role, avatar_url, created_at, updated_at
		FROM users
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		var createdStr, updatedStr string
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.AvatarURL, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		u.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		users = append(users, u)
	}
	return users, nil
}

func (s *SQLiteStore) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, name, role, avatar_url, created_at, updated_at
		FROM users
		WHERE email = ?
	`, strings.ToLower(email))

	var u domain.User
	var createdStr, updatedStr string
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.AvatarURL, &createdStr, &updatedStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("user not found: %s", email)
		}
		return nil, err
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &u, nil
}

func (s *SQLiteStore) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, name, role, avatar_url, created_at, updated_at
		FROM users
		WHERE id = ?
	`, id)

	var u domain.User
	var createdStr, updatedStr string
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.AvatarURL, &createdStr, &updatedStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("user not found: %s", id)
		}
		return nil, err
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &u, nil
}

func (s *SQLiteStore) CreateSession(ctx context.Context, session domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (token, user_id, expires_at, created_at)
		VALUES (?, ?, ?, ?)
	`, session.Token, session.UserID, session.ExpiresAt.Format(time.RFC3339), session.CreatedAt.Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) GetSession(ctx context.Context, token string) (*domain.Session, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT token, user_id, expires_at, created_at
		FROM sessions
		WHERE token = ?
	`, token)

	var sess domain.Session
	var expStr, createdStr string
	if err := row.Scan(&sess.Token, &sess.UserID, &expStr, &createdStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("session not found")
		}
		return nil, err
	}
	sess.ExpiresAt, _ = time.Parse(time.RFC3339, expStr)
	sess.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)

	if time.Now().UTC().After(sess.ExpiresAt) {
		_ = s.DeleteSession(ctx, token)
		return nil, errors.New("session expired")
	}

	return &sess, nil
}

func (s *SQLiteStore) DeleteSession(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token = ?", token)
	return err
}

func (s *SQLiteStore) CreatePasswordResetToken(ctx context.Context, email string, ttl time.Duration) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	token := "flg_rst_" + generateHexToken(32)
	now := time.Now().UTC()
	exp := now.Add(ttl)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO password_reset_tokens (token, email, expires_at, used, created_at)
		VALUES (?, ?, ?, 0, ?)
	`, token, strings.ToLower(email), exp.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *SQLiteStore) GetPasswordResetToken(ctx context.Context, token string) (*domain.PasswordResetToken, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT token, email, expires_at, used, created_at
		FROM password_reset_tokens
		WHERE token = ?
	`, token)

	var pr domain.PasswordResetToken
	var expStr, createdStr string
	var usedInt int
	if err := row.Scan(&pr.Token, &pr.Email, &expStr, &usedInt, &createdStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("reset token not found")
		}
		return nil, err
	}
	pr.ExpiresAt, _ = time.Parse(time.RFC3339, expStr)
	pr.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	pr.Used = usedInt == 1
	return &pr, nil
}

func (s *SQLiteStore) ResetPasswordWithToken(ctx context.Context, token string, newPasswordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pr, err := s.GetPasswordResetToken(ctx, token)
	if err != nil {
		return err
	}
	if pr.Used {
		return errors.New("token already used")
	}
	if time.Now().UTC().After(pr.ExpiresAt) {
		return errors.New("token expired")
	}

	_, err = s.db.ExecContext(ctx, "UPDATE users SET password_hash = ?, updated_at = ? WHERE email = ?",
		newPasswordHash, time.Now().UTC().Format(time.RFC3339), pr.Email)
	if err != nil {
		return err
	}

	_, _ = s.db.ExecContext(ctx, "UPDATE password_reset_tokens SET used = 1 WHERE token = ?", token)
	return nil
}

func (s *SQLiteStore) UpdateUser(ctx context.Context, user domain.User) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET name = ?, avatar_url = ?, updated_at = ?
		WHERE id = ?
	`, user.Name, user.AvatarURL, now, user.ID)
	if err != nil {
		return nil, err
	}
	rows, err := res.RowsAffected()
	if err != nil || rows == 0 {
		return nil, fmt.Errorf("user not found with ID: %s", user.ID)
	}

	// Read updated user back
	row := s.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, name, role, avatar_url, created_at, updated_at
		FROM users
		WHERE id = ?
	`, user.ID)

	var u domain.User
	var createdStr, updatedStr string
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.AvatarURL, &createdStr, &updatedStr); err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &u, nil
}

func (s *SQLiteStore) UpdateUserPassword(ctx context.Context, userID string, newPasswordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET password_hash = ?, updated_at = ?
		WHERE id = ?
	`, newPasswordHash, now, userID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil || rows == 0 {
		return fmt.Errorf("user not found with ID: %s", userID)
	}
	return nil
}

// --- MEMBERS & INVITATIONS ---

func (s *SQLiteStore) CreateOrgMember(ctx context.Context, member domain.OrgMember) (*domain.OrgMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if member.ID == "" {
		member.ID = "mem_" + generateHexToken(12)
	}
	now := time.Now().UTC()
	member.CreatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO org_members (id, organization_id, user_id, role, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(organization_id, user_id) DO UPDATE SET role = excluded.role
	`, member.ID, member.OrganizationID, member.UserID, member.Role, now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return &member, nil
}

func (s *SQLiteStore) ListOrgMembers(ctx context.Context, organizationID string) ([]domain.OrgMember, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, organization_id, user_id, role, created_at
		FROM org_members
		WHERE organization_id = ?
		ORDER BY created_at ASC
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.OrgMember
	for rows.Next() {
		var m domain.OrgMember
		var createdStr string
		if err := rows.Scan(&m.ID, &m.OrganizationID, &m.UserID, &m.Role, &createdStr); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		list = append(list, m)
	}
	return list, nil
}

func (s *SQLiteStore) CreateOrgInvitation(ctx context.Context, inv domain.OrgInvitation) (*domain.OrgInvitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if inv.ID == "" {
		inv.ID = "inv_" + generateHexToken(12)
	}
	if inv.Token == "" {
		inv.Token = generateHexToken(32)
	}
	if inv.ExpiresAt.IsZero() {
		inv.ExpiresAt = time.Now().UTC().Add(7 * 24 * time.Hour)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO org_invitations (id, organization_id, org_name, email, token, role, invited_by, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, inv.ID, inv.OrganizationID, inv.OrgName, strings.ToLower(inv.Email), inv.Token, inv.Role, inv.InvitedBy, inv.ExpiresAt.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (s *SQLiteStore) GetOrgInvitation(ctx context.Context, token string) (*domain.OrgInvitation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, organization_id, org_name, email, token, role, invited_by, expires_at, accepted_at
		FROM org_invitations
		WHERE token = ?
	`, token)

	var inv domain.OrgInvitation
	var expStr string
	var accStr sql.NullString
	if err := row.Scan(&inv.ID, &inv.OrganizationID, &inv.OrgName, &inv.Email, &inv.Token, &inv.Role, &inv.InvitedBy, &expStr, &accStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("invitation not found")
		}
		return nil, err
	}
	inv.ExpiresAt, _ = time.Parse(time.RFC3339, expStr)
	if accStr.Valid {
		t, _ := time.Parse(time.RFC3339, accStr.String)
		inv.AcceptedAt = &t
	}
	return &inv, nil
}

func (s *SQLiteStore) AcceptOrgInvitation(ctx context.Context, token, userID string) (*domain.OrgMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	inv, err := s.GetOrgInvitation(ctx, token)
	if err != nil {
		return nil, err
	}
	if inv.AcceptedAt != nil {
		return nil, errors.New("invitation already accepted")
	}
	if time.Now().UTC().After(inv.ExpiresAt) {
		return nil, errors.New("invitation expired")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.ExecContext(ctx, "UPDATE org_invitations SET accepted_at = ? WHERE token = ?", now, token)

	member := domain.OrgMember{
		ID:             "mem_" + generateHexToken(12),
		OrganizationID: inv.OrganizationID,
		UserID:         userID,
		Role:           inv.Role,
		CreatedAt:      time.Now().UTC(),
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO org_members (id, organization_id, user_id, role, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(organization_id, user_id) DO UPDATE SET role = excluded.role
	`, member.ID, member.OrganizationID, member.UserID, member.Role, member.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return &member, nil
}

func (s *SQLiteStore) ListOrgInvitations(ctx context.Context, organizationID string) ([]domain.OrgInvitation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, organization_id, org_name, email, token, role, invited_by, expires_at, accepted_at
		FROM org_invitations
		WHERE organization_id = ?
		ORDER BY expires_at DESC
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.OrgInvitation
	for rows.Next() {
		var inv domain.OrgInvitation
		var expStr string
		var accStr sql.NullString
		if err := rows.Scan(&inv.ID, &inv.OrganizationID, &inv.OrgName, &inv.Email, &inv.Token, &inv.Role, &inv.InvitedBy, &expStr, &accStr); err != nil {
			return nil, err
		}
		inv.ExpiresAt, _ = time.Parse(time.RFC3339, expStr)
		if accStr.Valid {
			t, _ := time.Parse(time.RFC3339, accStr.String)
			inv.AcceptedAt = &t
		}
		list = append(list, inv)
	}
	return list, nil
}

// --- EXPERIMENTS ---

func (s *SQLiteStore) RecordExperimentEvents(ctx context.Context, events []domain.ExperimentEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO experiment_events (
			id, project_id, flag_key, variant, metric_name, event_type, value, user_id, environment, timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ev := range events {
		if ev.ID == "" {
			ev.ID = "exp_" + generateHexToken(12)
		}
		if ev.ProjectID == "" {
			ev.ProjectID = DefaultProjectID
		}
		if ev.Timestamp.IsZero() {
			ev.Timestamp = time.Now().UTC()
		}
		if ev.EventType == "" {
			ev.EventType = "conversion"
		}
		if ev.Value == 0 {
			ev.Value = 1.0
		}
		_, err := stmt.ExecContext(ctx,
			ev.ID, ev.ProjectID, ev.FlagKey, ev.Variant, ev.MetricName,
			ev.EventType, ev.Value, ev.UserID, string(ev.Environment), ev.Timestamp.Format(time.RFC3339),
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) GetExperimentEvents(ctx context.Context, flagKey string, limit int) ([]domain.ExperimentEvent, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, flag_key, variant, metric_name, event_type, value, user_id, environment, timestamp
		FROM experiment_events
		WHERE flag_key = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, flagKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []domain.ExperimentEvent
	for rows.Next() {
		var ev domain.ExperimentEvent
		var envStr, tsStr string
		if err := rows.Scan(
			&ev.ID, &ev.ProjectID, &ev.FlagKey, &ev.Variant, &ev.MetricName,
			&ev.EventType, &ev.Value, &ev.UserID, &envStr, &tsStr,
		); err != nil {
			return nil, err
		}
		ev.Environment = domain.Environment(envStr)
		ev.Timestamp, _ = time.Parse(time.RFC3339, tsStr)
		events = append(events, ev)
	}
	return events, nil
}

// --- CHANGE REQUESTS (4-EYES GOVERNANCE) ---

func (s *SQLiteStore) CreateChangeRequest(ctx context.Context, cr domain.ChangeRequest) (*domain.ChangeRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cr.ID == "" {
		cr.ID = fmt.Sprintf("cr_%d_%s", time.Now().UnixNano(), generateHexToken(4))
	}
	if cr.ProjectID == "" {
		cr.ProjectID = DefaultProjectID
	}
	now := time.Now().UTC()
	cr.CreatedAt = now
	cr.Status = domain.ChangeRequestStatusPending

	proposedBytes, _ := json.Marshal(cr.ProposedConfig)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO change_requests (
			id, project_id, flag_key, environment, title, description,
			author_user_id, author_email, author_name, proposed_config, status, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, cr.ID, cr.ProjectID, cr.FlagKey, string(cr.Environment), cr.Title, cr.Description,
		cr.AuthorUserID, cr.AuthorEmail, cr.AuthorName, string(proposedBytes), string(cr.Status), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return &cr, nil
}

func (s *SQLiteStore) GetChangeRequest(ctx context.Context, id string) (*domain.ChangeRequest, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, flag_key, environment, title, description,
		       author_user_id, author_email, author_name, proposed_config, status,
		       reviewer_user_id, reviewer_email, reviewer_name, review_comments,
		       created_at, reviewed_at, applied_at
		FROM change_requests
		WHERE id = ?
	`, id)

	var cr domain.ChangeRequest
	var envStr, statusStr, proposedJSON string
	var createdStr string
	var reviewedStr, appliedStr sql.NullString
	if err := row.Scan(
		&cr.ID, &cr.ProjectID, &cr.FlagKey, &envStr, &cr.Title, &cr.Description,
		&cr.AuthorUserID, &cr.AuthorEmail, &cr.AuthorName, &proposedJSON, &statusStr,
		&cr.ReviewerUserID, &cr.ReviewerEmail, &cr.ReviewerName, &cr.ReviewComments,
		&createdStr, &reviewedStr, &appliedStr,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("change request %s not found", id)
		}
		return nil, err
	}
	cr.Environment = domain.Environment(envStr)
	cr.Status = domain.ChangeRequestStatus(statusStr)
	_ = json.Unmarshal([]byte(proposedJSON), &cr.ProposedConfig)
	cr.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	if reviewedStr.Valid {
		t, _ := time.Parse(time.RFC3339, reviewedStr.String)
		cr.ReviewedAt = &t
	}
	if appliedStr.Valid {
		t, _ := time.Parse(time.RFC3339, appliedStr.String)
		cr.AppliedAt = &t
	}
	return &cr, nil
}

func (s *SQLiteStore) ListChangeRequests(ctx context.Context, status domain.ChangeRequestStatus) ([]domain.ChangeRequest, error) {
	return s.ListChangeRequestsByProject(ctx, DefaultProjectID, status)
}

func (s *SQLiteStore) ListChangeRequestsByProject(ctx context.Context, projectID string, status domain.ChangeRequestStatus) ([]domain.ChangeRequest, error) {
	if projectID == "" {
		projectID = DefaultProjectID
	}
	var rows *sql.Rows
	var err error
	if status != "" && status != "all" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, project_id, flag_key, environment, title, description,
			       author_user_id, author_email, author_name, proposed_config, status,
			       reviewer_user_id, reviewer_email, reviewer_name, review_comments,
			       created_at, reviewed_at, applied_at
			FROM change_requests
			WHERE project_id = ? AND status = ?
			ORDER BY created_at DESC
		`, projectID, string(status))
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, project_id, flag_key, environment, title, description,
			       author_user_id, author_email, author_name, proposed_config, status,
			       reviewer_user_id, reviewer_email, reviewer_name, review_comments,
			       created_at, reviewed_at, applied_at
			FROM change_requests
			WHERE project_id = ?
			ORDER BY created_at DESC
		`, projectID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.ChangeRequest
	for rows.Next() {
		var cr domain.ChangeRequest
		var envStr, statusStr, proposedJSON string
		var createdStr string
		var reviewedStr, appliedStr sql.NullString
		if err := rows.Scan(
			&cr.ID, &cr.ProjectID, &cr.FlagKey, &envStr, &cr.Title, &cr.Description,
			&cr.AuthorUserID, &cr.AuthorEmail, &cr.AuthorName, &proposedJSON, &statusStr,
			&cr.ReviewerUserID, &cr.ReviewerEmail, &cr.ReviewerName, &cr.ReviewComments,
			&createdStr, &reviewedStr, &appliedStr,
		); err != nil {
			return nil, err
		}
		cr.Environment = domain.Environment(envStr)
		cr.Status = domain.ChangeRequestStatus(statusStr)
		_ = json.Unmarshal([]byte(proposedJSON), &cr.ProposedConfig)
		cr.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		if reviewedStr.Valid {
			t, _ := time.Parse(time.RFC3339, reviewedStr.String)
			cr.ReviewedAt = &t
		}
		if appliedStr.Valid {
			t, _ := time.Parse(time.RFC3339, appliedStr.String)
			cr.AppliedAt = &t
		}
		list = append(list, cr)
	}
	return list, nil
}

func (s *SQLiteStore) ReviewChangeRequest(ctx context.Context, id, reviewerID, reviewerEmail, reviewerName string, approved bool, comments string) (*domain.ChangeRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cr, err := s.GetChangeRequest(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := cr.Review(reviewerID, reviewerEmail, reviewerName, approved, comments); err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE change_requests
		SET status = ?, reviewer_user_id = ?, reviewer_email = ?, reviewer_name = ?, review_comments = ?, reviewed_at = ?
		WHERE id = ?
	`, string(cr.Status), cr.ReviewerUserID, cr.ReviewerEmail, cr.ReviewerName, cr.ReviewComments, cr.ReviewedAt.Format(time.RFC3339), id)
	if err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *SQLiteStore) ApplyChangeRequest(ctx context.Context, id string, actor string) (*domain.FeatureFlag, *domain.ChangeRequest, *domain.AuditLogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cr, err := s.GetChangeRequest(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}
	if cr.Status != domain.ChangeRequestStatusApproved {
		return nil, nil, nil, fmt.Errorf("cannot apply change request with status '%s' (must be APPROVED)", cr.Status)
	}

	flag, err := s.GetFlagByProject(ctx, cr.ProjectID, cr.FlagKey)
	if err != nil {
		return nil, nil, nil, err
	}

	if flag.Environments == nil {
		flag.Environments = make(map[domain.Environment]domain.EnvironmentConfig)
	}
	flag.Environments[cr.Environment] = cr.ProposedConfig
	flag.UpdatedAt = time.Now().UTC()
	flag.ConfigVersion++

	envsBytes, _ := json.Marshal(flag.Environments)
	_, err = s.db.ExecContext(ctx, `
		UPDATE feature_flags
		SET environments = ?, config_version = config_version + 1, updated_at = ?
		WHERE id = ?
	`, string(envsBytes), flag.UpdatedAt.Format(time.RFC3339), flag.ID)
	if err != nil {
		return nil, nil, nil, err
	}

	now := time.Now().UTC()
	cr.Status = domain.ChangeRequestStatusApplied
	cr.AppliedAt = &now

	_, _ = s.db.ExecContext(ctx, "UPDATE change_requests SET status = ?, applied_at = ? WHERE id = ?",
		string(domain.ChangeRequestStatusApplied), now.Format(time.RFC3339), id)

	audit := domain.AuditLogEntry{
		ID:          "audit_" + generateHexToken(12),
		ProjectID:   cr.ProjectID,
		FlagKey:     cr.FlagKey,
		Action:      "APPLY_CHANGE_REQUEST",
		Environment: cr.Environment,
		Actor:       actor,
		Details:     fmt.Sprintf("Applied Change Request #%s for %q [%s]", id, cr.FlagKey, cr.Environment),
		Timestamp:   now,
	}
	_ = s.insertAuditLog(ctx, audit)

	return flag, cr, &audit, nil
}

// --- API KEYS ---

func (s *SQLiteStore) CreateAPIKey(ctx context.Context, key domain.APIKey) (*domain.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key.ID == "" {
		key.ID = fmt.Sprintf("key_%d_%s", time.Now().UnixNano(), generateHexToken(4))
	}
	if key.ProjectID == "" {
		key.ProjectID = DefaultProjectID
	}
	if key.Environment == "" {
		key.Environment = "production"
	}
	now := time.Now().UTC()
	key.CreatedAt = now
	key.Revoked = false

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO api_keys (id, project_id, environment, key_prefix, key_hash, name, role, created_by, created_at, revoked)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
	`, key.ID, key.ProjectID, key.Environment, key.KeyPrefix, key.KeyHash, key.Name, key.Role, key.CreatedBy, now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return &key, nil
}

func (s *SQLiteStore) ListAPIKeys(ctx context.Context) ([]domain.APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, environment, key_prefix, name, role, created_by, created_at, last_used_at, revoked
		FROM api_keys
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.APIKey
	for rows.Next() {
		var k domain.APIKey
		var lastUsedStr sql.NullString
		var revokedInt int
		var envStr string
		var createdStr string
		if err := rows.Scan(
			&k.ID, &k.ProjectID, &envStr, &k.KeyPrefix, &k.Name,
			&k.Role, &k.CreatedBy, &createdStr, &lastUsedStr, &revokedInt,
		); err != nil {
			return nil, err
		}
		k.Environment = envStr
		k.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		k.Revoked = revokedInt == 1
		if lastUsedStr.Valid {
			t, _ := time.Parse(time.RFC3339, lastUsedStr.String)
			k.LastUsedAt = &t
		}
		result = append(result, k)
	}
	return result, nil
}

func (s *SQLiteStore) ListAPIKeysByProject(ctx context.Context, projectID string) ([]domain.APIKey, error) {
	if projectID == "" {
		projectID = DefaultProjectID
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, environment, key_prefix, name, role, created_by, created_at, last_used_at, revoked
		FROM api_keys
		WHERE project_id = ? AND revoked = 0
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.APIKey
	for rows.Next() {
		var k domain.APIKey
		var lastUsedStr sql.NullString
		var revokedInt int
		var envStr string
		var createdStr string
		if err := rows.Scan(
			&k.ID, &k.ProjectID, &envStr, &k.KeyPrefix, &k.Name,
			&k.Role, &k.CreatedBy, &createdStr, &lastUsedStr, &revokedInt,
		); err != nil {
			return nil, err
		}
		k.Environment = envStr
		k.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		k.Revoked = revokedInt == 1
		if lastUsedStr.Valid {
			t, _ := time.Parse(time.RFC3339, lastUsedStr.String)
			k.LastUsedAt = &t
		}
		result = append(result, k)
	}
	return result, nil
}

func (s *SQLiteStore) GetAPIKeyByHash(ctx context.Context, hash string) (*domain.APIKey, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, environment, key_prefix, key_hash, name, role, created_by, created_at, last_used_at, revoked
		FROM api_keys
		WHERE key_hash = ? AND revoked = 0
	`, hash)

	var k domain.APIKey
	var envStr, createdStr string
	var lastUsedStr sql.NullString
	var revokedInt int
	if err := row.Scan(
		&k.ID, &k.ProjectID, &envStr, &k.KeyPrefix, &k.KeyHash, &k.Name,
		&k.Role, &k.CreatedBy, &createdStr, &lastUsedStr, &revokedInt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("invalid API key")
		}
		return nil, err
	}
	k.Environment = envStr
	k.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	k.Revoked = revokedInt == 1

	now := time.Now().UTC()
	k.LastUsedAt = &now
	_, _ = s.db.ExecContext(ctx, "UPDATE api_keys SET last_used_at = ? WHERE id = ?", now.Format(time.RFC3339), k.ID)

	return &k, nil
}

func (s *SQLiteStore) RevokeAPIKey(ctx context.Context, id string, actor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, "UPDATE api_keys SET revoked = 1 WHERE id = ?", id)
	if err != nil {
		return err
	}

	audit := domain.AuditLogEntry{
		ID:          fmt.Sprintf("audit_%d", time.Now().UnixNano()),
		FlagKey:     "api-keys",
		Action:      "API_KEY_REVOKED",
		Environment: domain.Environment("all"),
		Actor:       actor,
		Timestamp:   time.Now().UTC(),
		Details:     fmt.Sprintf("Revoked API Key %s", id),
	}
	_ = s.insertAuditLog(ctx, audit)

	return nil
}

func generateHexToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
