package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/dhawalhost/flagura/pkg/domain"

	"github.com/lib/pq"
)

type PostgresStore struct {
	db         *sql.DB
	driverName string
}

func detectDriverName(databaseURL string) string {
	lower := strings.ToLower(databaseURL)
	if strings.Contains(lower, "supabase.co") || strings.Contains(lower, "supabase.com") {
		return "Supabase PostgreSQL"
	}
	if strings.Contains(lower, "neon.tech") {
		return "Neon PostgreSQL"
	}
	if strings.Contains(lower, "rds.amazonaws.com") {
		return "AWS RDS PostgreSQL"
	}
	if strings.Contains(lower, "localhost") || strings.Contains(lower, "127.0.0.1") || strings.Contains(lower, "postgres:5432") {
		return "Local PostgreSQL"
	}
	return "PostgreSQL"
}

func NewPostgresStore(databaseURL string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	store := &PostgresStore{
		db:         db,
		driverName: detectDriverName(databaseURL),
	}
	if err := store.autoMigrate(ctx); err != nil {
		return nil, fmt.Errorf("auto-migration failed: %w", err)
	}

	return store, nil
}

func (s *PostgresStore) DriverName() string {
	if s.driverName != "" {
		return s.driverName
	}
	return "PostgreSQL"
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *PostgresStore) autoMigrate(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		name TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'developer',
		avatar_url TEXT DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

	CREATE TABLE IF NOT EXISTS sessions (
		token TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at TIMESTAMPTZ NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

	CREATE TABLE IF NOT EXISTS feature_flags (
		id TEXT PRIMARY KEY,
		key TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		description TEXT,
		type TEXT NOT NULL DEFAULT 'boolean',
		tags TEXT[] DEFAULT '{}',
		variants JSONB DEFAULT '[]'::jsonb,
		environments JSONB NOT NULL DEFAULT '{}'::jsonb,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_feature_flags_key ON feature_flags(key);

	CREATE TABLE IF NOT EXISTS audit_logs (
		id TEXT PRIMARY KEY,
		flag_key TEXT NOT NULL,
		action TEXT NOT NULL,
		environment TEXT NOT NULL,
		actor TEXT NOT NULL,
		details TEXT NOT NULL,
		timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp DESC);
	`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return err
	}

	// Seed default administrator if users table is empty
	var userCount int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount); err == nil && userCount == 0 {
		hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
		now := time.Now().UTC()
		_, _ = s.db.ExecContext(ctx, `
			INSERT INTO users (id, email, password_hash, name, role, avatar_url, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, "usr_admin_default", "dhawal@flagura.dev", string(hash), "Dhawal Dyavanpalli", "admin", "", now, now)
	}

	// Seed if empty
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM feature_flags").Scan(&count)
	if err == nil && count == 0 {
		mem := NewMemoryStore()
		flags, _ := mem.ListFlags(ctx)
		for _, f := range flags {
			_, _ = s.SaveFlag(ctx, f, "system-seeder")
		}
		logs, _ := mem.ListAuditLogs(ctx, 10)
		for _, l := range logs {
			_, _ = s.db.ExecContext(ctx,
				"INSERT INTO audit_logs (id, flag_key, action, environment, actor, details, timestamp) VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT DO NOTHING",
				l.ID, l.FlagKey, l.Action, l.Environment, l.Actor, l.Details, l.Timestamp,
			)
		}
	}

	return nil
}

func (s *PostgresStore) ListFlags(ctx context.Context) ([]domain.FeatureFlag, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, key, name, description, type, tags, environments, created_at, updated_at
		FROM feature_flags
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flags []domain.FeatureFlag
	for rows.Next() {
		var f domain.FeatureFlag
		var envsJSON []byte
		var tags []string

		if err := rows.Scan(&f.ID, &f.Key, &f.Name, &f.Description, &f.Type, pq.Array(&tags), &envsJSON, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		f.Tags = tags
		if len(envsJSON) > 0 {
			_ = json.Unmarshal(envsJSON, &f.Environments)
		}
		if f.Environments == nil {
			f.Environments = make(map[domain.Environment]domain.EnvironmentConfig)
		}
		flags = append(flags, f)
	}

	return flags, nil
}

func (s *PostgresStore) GetFlag(ctx context.Context, keyOrID string) (*domain.FeatureFlag, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, key, name, description, type, tags, environments, created_at, updated_at
		FROM feature_flags
		WHERE key = $1 OR id = $1
	`, keyOrID)

	var f domain.FeatureFlag
	var envsJSON []byte
	var tags []string

	if err := row.Scan(&f.ID, &f.Key, &f.Name, &f.Description, &f.Type, pq.Array(&tags), &envsJSON, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return nil, err
	}
	f.Tags = tags
	if len(envsJSON) > 0 {
		_ = json.Unmarshal(envsJSON, &f.Environments)
	}
	if f.Environments == nil {
		f.Environments = make(map[domain.Environment]domain.EnvironmentConfig)
	}

	return &f, nil
}

func (s *PostgresStore) SaveFlag(ctx context.Context, flag domain.FeatureFlag, actor string) (*domain.AuditLogEntry, error) {
	if actor == "" {
		actor = "developer@flagura.dev"
	}
	if flag.ID == "" {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		flag.ID = fmt.Sprintf("flag_%d_%s", time.Now().Unix(), hex.EncodeToString(b))
	}
	now := time.Now().UTC()
	flag.UpdatedAt = now

	envsJSON, err := json.Marshal(flag.Environments)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal environments: %w", err)
	}

	query := `
		INSERT INTO feature_flags (id, key, name, description, type, tags, environments, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (key) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			type = EXCLUDED.type,
			tags = EXCLUDED.tags,
			environments = EXCLUDED.environments,
			updated_at = EXCLUDED.updated_at
	`
	_, err = s.db.ExecContext(ctx, query,
		flag.ID, flag.Key, flag.Name, flag.Description, flag.Type,
		pq.Array(flag.Tags), envsJSON, now, now,
	)
	if err != nil {
		return nil, err
	}

	log := domain.AuditLogEntry{
		ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
		Timestamp:   now,
		Actor:       actor,
		Action:      "FLAG_UPDATED",
		FlagKey:     flag.Key,
		Environment: "all",
		Details:     fmt.Sprintf("Saved feature flag '%s'.", flag.Key),
	}

	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (id, flag_key, action, environment, actor, details, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, log.ID, log.FlagKey, log.Action, log.Environment, log.Actor, log.Details, log.Timestamp)

	return &log, nil
}

func (s *PostgresStore) DeleteFlag(ctx context.Context, keyOrID string, actor string) (*domain.AuditLogEntry, error) {
	if actor == "" {
		actor = "admin@flagura.dev"
	}

	f, err := s.GetFlag(ctx, keyOrID)
	if err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx, "DELETE FROM feature_flags WHERE id = $1 OR key = $1", keyOrID)
	if err != nil {
		return nil, err
	}

	log := domain.AuditLogEntry{
		ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
		Timestamp:   time.Now().UTC(),
		Actor:       actor,
		Action:      "FLAG_DELETED",
		FlagKey:     f.Key,
		Environment: "all",
		Details:     fmt.Sprintf("Deleted feature flag '%s'.", f.Key),
	}

	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (id, flag_key, action, environment, actor, details, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, log.ID, log.FlagKey, log.Action, log.Environment, log.Actor, log.Details, log.Timestamp)

	return &log, nil
}

func (s *PostgresStore) ToggleFlag(ctx context.Context, keyOrID string, env domain.Environment, enabled *bool, actor string) (*domain.FeatureFlag, *domain.AuditLogEntry, error) {
	f, err := s.GetFlag(ctx, keyOrID)
	if err != nil {
		return nil, nil, err
	}

	cfg := f.Environments[env]
	if enabled != nil {
		cfg.Enabled = *enabled
	} else {
		cfg.Enabled = !cfg.Enabled
	}
	f.Environments[env] = cfg
	f.UpdatedAt = time.Now().UTC()

	envsJSON, err := json.Marshal(f.Environments)
	if err != nil {
		return nil, nil, err
	}

	_, err = s.db.ExecContext(ctx, "UPDATE feature_flags SET environments = $1, updated_at = $2 WHERE id = $3", envsJSON, f.UpdatedAt, f.ID)
	if err != nil {
		return nil, nil, err
	}

	statusText := "Disabled (Kill Switch)"
	if cfg.Enabled {
		statusText = "Enabled"
	}
	log := domain.AuditLogEntry{
		ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
		Timestamp:   time.Now().UTC(),
		Actor:       actor,
		Action:      "KILL_SWITCH_TOGGLED",
		FlagKey:     f.Key,
		Environment: env,
		Details:     fmt.Sprintf("%s flag for %s environment.", statusText, env),
	}

	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (id, flag_key, action, environment, actor, details, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, log.ID, log.FlagKey, log.Action, log.Environment, log.Actor, log.Details, log.Timestamp)

	return f, &log, nil
}

func (s *PostgresStore) UpdateRollout(ctx context.Context, keyOrID string, env domain.Environment, pct float64, actor string) (*domain.FeatureFlag, *domain.AuditLogEntry, error) {
	f, err := s.GetFlag(ctx, keyOrID)
	if err != nil {
		return nil, nil, err
	}

	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}

	cfg := f.Environments[env]
	oldPct := cfg.Percentage
	cfg.Percentage = pct
	if cfg.Strategy == domain.StrategyBoolean && pct < 100 {
		cfg.Strategy = domain.StrategyPercentage
	}
	f.Environments[env] = cfg
	f.UpdatedAt = time.Now().UTC()

	envsJSON, err := json.Marshal(f.Environments)
	if err != nil {
		return nil, nil, err
	}

	_, err = s.db.ExecContext(ctx, "UPDATE feature_flags SET environments = $1, updated_at = $2 WHERE id = $3", envsJSON, f.UpdatedAt, f.ID)
	if err != nil {
		return nil, nil, err
	}

	log := domain.AuditLogEntry{
		ID:          fmt.Sprintf("log_%d", time.Now().UnixNano()),
		Timestamp:   time.Now().UTC(),
		Actor:       actor,
		Action:      "ROLLOUT_CHANGED",
		FlagKey:     f.Key,
		Environment: env,
		Details:     fmt.Sprintf("Shifted percentage rollout from %.0f%% to %.0f%% in %s.", oldPct, pct, env),
	}

	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (id, flag_key, action, environment, actor, details, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, log.ID, log.FlagKey, log.Action, log.Environment, log.Actor, log.Details, log.Timestamp)

	return f, &log, nil
}

func (s *PostgresStore) ListAuditLogs(ctx context.Context, limit int) ([]domain.AuditLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, flag_key, action, environment, actor, details, timestamp
		FROM audit_logs
		ORDER BY timestamp DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []domain.AuditLogEntry
	for rows.Next() {
		var l domain.AuditLogEntry
		var envStr string
		if err := rows.Scan(&l.ID, &l.FlagKey, &l.Action, &envStr, &l.Actor, &l.Details, &l.Timestamp); err != nil {
			return nil, err
		}
		l.Environment = domain.Environment(envStr)
		logs = append(logs, l)
	}

	return logs, nil
}

func (s *PostgresStore) Reset(ctx context.Context) error {
	_, _ = s.db.ExecContext(ctx, "TRUNCATE feature_flags, audit_logs")
	return s.autoMigrate(ctx)
}

func (s *PostgresStore) CreateUser(ctx context.Context, user domain.User) (*domain.User, error) {
	if user.ID == "" {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		user.ID = fmt.Sprintf("usr_%d_%s", time.Now().UnixNano(), hex.EncodeToString(b))
	}
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, name, role, avatar_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, user.ID, user.Email, user.PasswordHash, user.Name, string(user.Role), user.AvatarURL, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert user: %w", err)
	}

	return &user, nil
}

func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	var u domain.User
	var roleStr string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, name, role, avatar_url, created_at, updated_at
		FROM users
		WHERE email = $1
	`, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &roleStr, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found with email: %s", email)
		}
		return nil, err
	}
	u.Role = domain.UserRole(roleStr)
	return &u, nil
}

func (s *PostgresStore) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	var u domain.User
	var roleStr string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, name, role, avatar_url, created_at, updated_at
		FROM users
		WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &roleStr, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found with ID: %s", id)
		}
		return nil, err
	}
	u.Role = domain.UserRole(roleStr)
	return &u, nil
}

func (s *PostgresStore) CreateSession(ctx context.Context, session domain.Session) error {
	now := time.Now().UTC()
	session.CreatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (token, user_id, expires_at, created_at)
		VALUES ($1, $2, $3, $4)
	`, session.Token, session.UserID, session.ExpiresAt, session.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetSession(ctx context.Context, token string) (*domain.Session, error) {
	var sess domain.Session
	err := s.db.QueryRowContext(ctx, `
		SELECT token, user_id, expires_at, created_at
		FROM sessions
		WHERE token = $1
	`, token).Scan(&sess.Token, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found")
		}
		return nil, err
	}

	if sess.IsExpired() {
		_ = s.DeleteSession(ctx, token)
		return nil, fmt.Errorf("session expired")
	}

	// Fetch user
	user, err := s.GetUserByID(ctx, sess.UserID)
	if err == nil {
		sess.User = user
	}

	return &sess, nil
}

func (s *PostgresStore) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token = $1", token)
	return err
}
