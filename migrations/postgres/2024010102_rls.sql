-- +goose Up
-- +goose StatementBegin

-- Enable and force Row Level Security on profiles table
ALTER TABLE profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE profiles FORCE ROW LEVEL SECURITY;

-- Enable and force Row Level Security on api_keys table
ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_keys FORCE ROW LEVEL SECURITY;

-- Enable and force Row Level Security on audit_log table
ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE ROW LEVEL SECURITY;

-- Policy: profiles - users can see their own profile
CREATE POLICY profiles_self_access ON profiles
    FOR ALL
    TO PUBLIC
    USING (
        id = nullif(current_setting('app.current_profile_id', true), '')::uuid
    )
    WITH CHECK (
        id = nullif(current_setting('app.current_profile_id', true), '')::uuid
    );

-- Policy: profiles - internal/system transactions can read all profiles
CREATE POLICY profiles_system_read_access ON profiles
    FOR SELECT
    TO PUBLIC
    USING (
        current_setting('app.tx_mode', true) = 'system'
    );

-- Policy: api_keys - users can see their own api_keys
CREATE POLICY api_keys_self_access ON api_keys
    FOR ALL
    TO PUBLIC
    USING (
        profile_id = nullif(current_setting('app.current_profile_id', true), '')::uuid
    )
    WITH CHECK (
        profile_id = nullif(current_setting('app.current_profile_id', true), '')::uuid
    );

-- Policy: api_keys - internal/system transactions can read all keys
CREATE POLICY api_keys_system_read_access ON api_keys
    FOR SELECT
    TO PUBLIC
    USING (
        current_setting('app.tx_mode', true) = 'system'
    );

-- Policy: api_keys - internal/system transactions can perform internal updates
CREATE POLICY api_keys_system_update_access ON api_keys
    FOR UPDATE
    TO PUBLIC
    USING (
        current_setting('app.tx_mode', true) = 'system'
    )
    WITH CHECK (
        current_setting('app.tx_mode', true) = 'system'
    );

-- Policy: audit_log - users can see their own audit_log entries
CREATE POLICY audit_log_self_access ON audit_log
    FOR SELECT
    TO PUBLIC
    USING (
        profile_id = nullif(current_setting('app.current_profile_id', true), '')::uuid
    );

-- Policy: audit_log - internal/system transactions can read all entries
CREATE POLICY audit_log_system_read_access ON audit_log
    FOR SELECT
    TO PUBLIC
    USING (
        current_setting('app.tx_mode', true) = 'system'
    );

-- Policy: audit_log - profile and internal/system transactions can insert
CREATE POLICY audit_log_insert_all ON audit_log
    FOR INSERT
    TO PUBLIC
    WITH CHECK (
        current_setting('app.tx_mode', true) IN ('profile', 'system')
        AND (
            current_setting('app.tx_mode', true) = 'system'
            OR profile_id IS NULL
            OR profile_id = nullif(current_setting('app.current_profile_id', true), '')::uuid
        )
    );

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin

-- Drop audit_log policies
DROP POLICY IF EXISTS audit_log_insert_all ON audit_log;
DROP POLICY IF EXISTS audit_log_system_read_access ON audit_log;
DROP POLICY IF EXISTS audit_log_self_access ON audit_log;

-- Drop api_keys policies
DROP POLICY IF EXISTS api_keys_system_update_access ON api_keys;
DROP POLICY IF EXISTS api_keys_system_read_access ON api_keys;
DROP POLICY IF EXISTS api_keys_self_access ON api_keys;

-- Drop profiles policies
DROP POLICY IF EXISTS profiles_system_read_access ON profiles;
DROP POLICY IF EXISTS profiles_self_access ON profiles;

-- Disable Row Level Security on audit_log
ALTER TABLE audit_log DISABLE ROW LEVEL SECURITY;

-- Disable Row Level Security on api_keys
ALTER TABLE api_keys DISABLE ROW LEVEL SECURITY;

-- Disable Row Level Security on profiles
ALTER TABLE profiles DISABLE ROW LEVEL SECURITY;

-- +goose StatementEnd
