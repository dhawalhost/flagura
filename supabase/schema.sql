-- Flagura Platform - Supabase PostgreSQL Schema (v1.6.6)
-- Production multi-tenant feature flagging, 4-eyes governance, experiments, and team access.

-- ============================================================================
-- 1. Organizations & Projects Multi-Tenancy Hierarchy
-- ============================================================================
CREATE TABLE IF NOT EXISTS organizations (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    description TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_organizations_slug ON organizations(slug);

CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT DEFAULT '',
    config_version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(organization_id, slug)
);
ALTER TABLE projects ADD COLUMN IF NOT EXISTS config_version BIGINT NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_projects_org ON projects(organization_id);
CREATE INDEX IF NOT EXISTS idx_projects_slug ON projects(slug);

-- Seed initial default organization and project
INSERT INTO organizations (id, name, slug, description)
VALUES ('org_default', 'Default Organization', 'default-org', 'Primary workspace organization')
ON CONFLICT (id) DO NOTHING;

INSERT INTO projects (id, organization_id, name, slug, description)
VALUES ('proj_default', 'org_default', 'Default Project', 'default-project', 'Primary feature flag project')
ON CONFLICT (id) DO NOTHING;

-- ============================================================================
-- 2. User Accounts & RBAC
-- ============================================================================
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
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url TEXT DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'developer';

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- ============================================================================
-- 3. Sessions
-- ============================================================================
CREATE TABLE IF NOT EXISTS sessions (
    token TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

-- ============================================================================
-- 4. Organization Team Members & Invitations
-- ============================================================================
CREATE TABLE IF NOT EXISTS org_members (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'developer',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(organization_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_org_members_user ON org_members(user_id);
CREATE INDEX IF NOT EXISTS idx_org_members_org ON org_members(organization_id);

CREATE TABLE IF NOT EXISTS org_invitations (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    org_name TEXT NOT NULL,
    email TEXT NOT NULL,
    token TEXT UNIQUE NOT NULL,
    role TEXT NOT NULL DEFAULT 'developer',
    invited_by TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_org_invitations_token ON org_invitations(token);
CREATE INDEX IF NOT EXISTS idx_org_invitations_org ON org_invitations(organization_id);

-- ============================================================================
-- 5. Feature Flags (Scoped per Project with Monotonic Config Version)
-- ============================================================================
CREATE TABLE IF NOT EXISTS feature_flags (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL DEFAULT 'proj_default',
    key TEXT NOT NULL,
    config_version BIGINT NOT NULL DEFAULT 1,
    name TEXT NOT NULL,
    description TEXT,
    type TEXT NOT NULL DEFAULT 'boolean',
    tags TEXT[] DEFAULT '{}',
    variants JSONB DEFAULT '[]'::jsonb,
    environments JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- Ensure columns exist before creating indexes (for existing pre-v1.7 databases)
ALTER TABLE feature_flags ADD COLUMN IF NOT EXISTS project_id TEXT NOT NULL DEFAULT 'proj_default';
ALTER TABLE feature_flags ADD COLUMN IF NOT EXISTS config_version BIGINT NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_feature_flags_key ON feature_flags(key);
CREATE INDEX IF NOT EXISTS idx_feature_flags_proj ON feature_flags(project_id, key);
CREATE UNIQUE INDEX IF NOT EXISTS idx_feature_flags_project_key ON feature_flags(project_id, key);

-- ============================================================================
-- 6. Audit Logs
-- ============================================================================
CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL DEFAULT 'proj_default',
    flag_key TEXT NOT NULL,
    action TEXT NOT NULL,
    environment TEXT NOT NULL,
    actor TEXT NOT NULL,
    details TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- Ensure columns exist before creating indexes
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS project_id TEXT NOT NULL DEFAULT 'proj_default';

CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_proj ON audit_logs(project_id, timestamp DESC);

-- ============================================================================
-- 7. A/B Experimentation Telemetry Events
-- ============================================================================
CREATE TABLE IF NOT EXISTS experiment_events (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL DEFAULT 'proj_default',
    flag_key TEXT NOT NULL,
    variant TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    event_type TEXT NOT NULL DEFAULT 'conversion',
    value DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    user_id TEXT DEFAULT '',
    environment TEXT NOT NULL DEFAULT 'production',
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- Ensure columns exist before creating indexes
ALTER TABLE experiment_events ADD COLUMN IF NOT EXISTS project_id TEXT NOT NULL DEFAULT 'proj_default';

CREATE INDEX IF NOT EXISTS idx_exp_events_flag ON experiment_events(flag_key, metric_name, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_exp_events_proj ON experiment_events(project_id, timestamp DESC);

-- ============================================================================
-- 8. 4-Eyes Change Governance Requests
-- ============================================================================
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
    proposed_config JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    reviewer_user_id TEXT DEFAULT '',
    reviewer_email TEXT DEFAULT '',
    reviewer_name TEXT DEFAULT '',
    review_comments TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at TIMESTAMPTZ,
    applied_at TIMESTAMPTZ
);
-- Ensure columns exist before creating indexes
ALTER TABLE change_requests ADD COLUMN IF NOT EXISTS project_id TEXT NOT NULL DEFAULT 'proj_default';

CREATE INDEX IF NOT EXISTS idx_change_requests_status ON change_requests(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_change_requests_proj ON change_requests(project_id, created_at DESC);

-- ============================================================================
-- 9. Programmatic Service API Keys
-- ============================================================================
CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL DEFAULT 'proj_default',
    environment TEXT NOT NULL DEFAULT 'production',
    key_prefix TEXT NOT NULL,
    key_hash TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'developer',
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    revoked BOOLEAN NOT NULL DEFAULT FALSE
);
-- Ensure columns exist before creating indexes (for databases upgraded from legacy schemas)
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS project_id TEXT NOT NULL DEFAULT 'proj_default';
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS environment TEXT NOT NULL DEFAULT 'production';
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS key_prefix TEXT NOT NULL DEFAULT '';
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS key_hash TEXT;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'developer';
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS created_by TEXT NOT NULL DEFAULT 'admin';
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS revoked BOOLEAN NOT NULL DEFAULT FALSE;

-- If legacy plaintext 'key' column exists, backfill key_hash & key_prefix
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'api_keys' AND column_name = 'key'
    ) THEN
        UPDATE api_keys
        SET 
            key_hash = encode(sha256(key::bytea), 'hex'),
            key_prefix = LEFT(key, 8)
        WHERE key_hash IS NULL AND key IS NOT NULL;
    END IF;
END $$;

-- Guarantee any remaining null key_hash values have a unique fallback hash
UPDATE api_keys SET key_hash = md5(id || clock_timestamp()::text) WHERE key_hash IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_proj ON api_keys(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_api_keys_created ON api_keys(created_at DESC);

-- ============================================================================
-- 10. Password Reset Tokens
-- ============================================================================
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    token TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_reset_tokens_email ON password_reset_tokens(email);
CREATE INDEX IF NOT EXISTS idx_reset_tokens_expires ON password_reset_tokens(expires_at);
