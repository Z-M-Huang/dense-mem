-- +goose Up
-- +goose StatementBegin

-- connector_credentials table: stores encrypted credentials for external connectors
CREATE TABLE IF NOT EXISTS connector_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    connector_type VARCHAR(50) NOT NULL,
    credential_name VARCHAR(100) NOT NULL,
    encrypted_secret BYTEA NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);

-- Partial unique index: only active (non-deleted) credentials must be unique per profile/connector/type
CREATE UNIQUE INDEX IF NOT EXISTS idx_connector_credentials_unique_active
    ON connector_credentials (profile_id, connector_type, credential_name)
    WHERE deleted_at IS NULL;

-- Index for profile lookups
CREATE INDEX IF NOT EXISTS idx_connector_credentials_profile_id ON connector_credentials(profile_id);

-- connector_sync_state table: tracks synchronization state for external connectors
CREATE TABLE IF NOT EXISTS connector_sync_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    connector_type VARCHAR(50) NOT NULL,
    source_id VARCHAR(255) NOT NULL,
    last_sync_at TIMESTAMPTZ NULL,
    cursor TEXT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('idle','syncing','error')),
    error_message TEXT NULL,
    items_synced INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Unique index: one sync state per profile/connector/source combination
CREATE UNIQUE INDEX IF NOT EXISTS idx_connector_sync_state_unique
    ON connector_sync_state (profile_id, connector_type, source_id);

-- Index for profile lookups
CREATE INDEX IF NOT EXISTS idx_connector_sync_state_profile_id ON connector_sync_state(profile_id);

-- Enable and force Row Level Security on connector_credentials table
ALTER TABLE connector_credentials ENABLE ROW LEVEL SECURITY;
ALTER TABLE connector_credentials FORCE ROW LEVEL SECURITY;

-- Enable and force Row Level Security on connector_sync_state table
ALTER TABLE connector_sync_state ENABLE ROW LEVEL SECURITY;
ALTER TABLE connector_sync_state FORCE ROW LEVEL SECURITY;

-- Policy: connector_credentials - users can see their own credentials
CREATE POLICY connector_credentials_self_access ON connector_credentials
    FOR ALL
    TO PUBLIC
    USING (
        profile_id = nullif(current_setting('app.current_profile_id', true), '')::uuid
    );

-- Policy: connector_credentials - admin can see all credentials
CREATE POLICY connector_credentials_admin_access ON connector_credentials
    FOR ALL
    TO PUBLIC
    USING (
        current_setting('app.role', true) = 'admin'
    );

-- Policy: connector_sync_state - users can see their own sync state
CREATE POLICY connector_sync_state_self_access ON connector_sync_state
    FOR ALL
    TO PUBLIC
    USING (
        profile_id = nullif(current_setting('app.current_profile_id', true), '')::uuid
    );

-- Policy: connector_sync_state - admin can see all sync state
CREATE POLICY connector_sync_state_admin_access ON connector_sync_state
    FOR ALL
    TO PUBLIC
    USING (
        current_setting('app.role', true) = 'admin'
    );

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin

-- Drop connector_sync_state policies
DROP POLICY IF EXISTS connector_sync_state_admin_access ON connector_sync_state;
DROP POLICY IF EXISTS connector_sync_state_self_access ON connector_sync_state;

-- Drop connector_credentials policies
DROP POLICY IF EXISTS connector_credentials_admin_access ON connector_credentials;
DROP POLICY IF EXISTS connector_credentials_self_access ON connector_credentials;

-- Disable Row Level Security on connector_sync_state
ALTER TABLE connector_sync_state DISABLE ROW LEVEL SECURITY;

-- Disable Row Level Security on connector_credentials
ALTER TABLE connector_credentials DISABLE ROW LEVEL SECURITY;

-- Drop connector_sync_state table (indexes are dropped automatically)
DROP TABLE IF EXISTS connector_sync_state;

-- Drop connector_credentials table (indexes are dropped automatically)
DROP TABLE IF EXISTS connector_credentials;

-- +goose StatementEnd
