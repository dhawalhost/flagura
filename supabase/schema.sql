-- Flagura Platform - Supabase PostgreSQL Schema (v1.7.0)

-- 1. Organizations & Projects Multi-Tenancy Hierarchy
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(organization_id, slug)
);
CREATE INDEX IF NOT EXISTS idx_projects_org ON projects(organization_id);
CREATE INDEX IF NOT EXISTS idx_projects_slug ON projects(slug);

-- Seed initial default organization and project
INSERT INTO organizations (id, name, slug, description)
VALUES ('org_default', 'Default Organization', 'default-org', 'Primary workspace organization')
ON CONFLICT (id) DO NOTHING;

INSERT INTO projects (id, organization_id, name, slug, description)
VALUES ('proj_default', 'org_default', 'Default Project', 'default-project', 'Primary feature flag project')
ON CONFLICT (id) DO NOTHING;

-- 2. User Accounts & RBAC
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

-- 3. Sessions
CREATE TABLE IF NOT EXISTS sessions (
    token TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

-- 4. Feature Flags
CREATE TABLE IF NOT EXISTS feature_flags (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL DEFAULT 'proj_default',
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
CREATE INDEX IF NOT EXISTS idx_feature_flags_proj ON feature_flags(project_id, key);

-- 5. Audit Logs
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
CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_proj ON audit_logs(project_id, timestamp DESC);

-- 6. A/B Experimentation Telemetry Events
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
CREATE INDEX IF NOT EXISTS idx_exp_events_flag ON experiment_events(flag_key, metric_name, timestamp DESC);

-- 7. 4-Eyes Change Governance Requests
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
CREATE INDEX IF NOT EXISTS idx_change_requests_status ON change_requests(status, created_at DESC);

-- 8. Programmatic Service API Keys
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
CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_proj ON api_keys(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_api_keys_created ON api_keys(created_at DESC);

-- 9. Password Reset Tokens
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    token TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_reset_tokens_email ON password_reset_tokens(email);
CREATE INDEX IF NOT EXISTS idx_reset_tokens_expires ON password_reset_tokens(expires_at);
