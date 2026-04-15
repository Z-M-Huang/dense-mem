-- +goose Up
-- +goose StatementBegin

-- Enable pgcrypto extension for gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Profiles table: stores tenant-like entities for multi-tenancy
CREATE TABLE IF NOT EXISTS profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived', 'deleted')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);

-- Partial unique index: only active (non-deleted) profiles must have unique names
CREATE UNIQUE INDEX IF NOT EXISTS idx_profiles_name_unique_active
    ON profiles (lower(name))
    WHERE deleted_at IS NULL;

-- API keys table: stores authentication keys with role-based access
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id UUID NULL REFERENCES profiles(id) ON DELETE RESTRICT,
    key_hash TEXT NOT NULL,
    key_prefix VARCHAR(12) NOT NULL,
    label VARCHAR(100) NOT NULL DEFAULT '',
    role VARCHAR(20) NOT NULL CHECK (role IN ('standard', 'admin')),
    scopes TEXT[] NOT NULL DEFAULT ARRAY['read']::text[],
    rate_limit INTEGER NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NULL,
    revoked_at TIMESTAMPTZ NULL,
    last_used_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Check constraint: admin keys must NOT have a profile_id; standard keys MUST have a profile_id
    CONSTRAINT chk_api_keys_role_profile CHECK (
        (role = 'admin' AND profile_id IS NULL) OR
        (role = 'standard' AND profile_id IS NOT NULL)
    )
);

-- Indexes for api_keys
CREATE INDEX IF NOT EXISTS idx_api_keys_profile_id ON api_keys(profile_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_prefix ON api_keys(key_prefix);

-- Audit log table: append-only record of all operations
CREATE TABLE IF NOT EXISTS audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id UUID NULL REFERENCES profiles(id) ON DELETE SET NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    operation VARCHAR(64) NOT NULL,
    entity_type VARCHAR(64) NOT NULL,
    entity_id TEXT NOT NULL,
    before_payload JSONB NULL,
    after_payload JSONB NULL,
    actor_key_id UUID NULL REFERENCES api_keys(id) ON DELETE SET NULL,
    actor_role VARCHAR(20) NULL,
    client_ip INET NULL,
    correlation_id TEXT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

-- Indexes for audit_log
CREATE INDEX IF NOT EXISTS idx_audit_log_profile_timestamp ON audit_log(profile_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_timestamp ON audit_log(timestamp DESC);

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin

-- Drop tables in reverse dependency order to avoid FK constraint errors
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS profiles;

-- +goose StatementEnd
