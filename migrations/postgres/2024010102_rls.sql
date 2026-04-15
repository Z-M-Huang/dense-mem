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
    );

-- Policy: profiles - admin can see all profiles (read-only; writes must go through explicit profile context)
CREATE POLICY profiles_admin_access ON profiles
    FOR SELECT
    TO PUBLIC
    USING (
        current_setting('app.role', true) = 'admin'
    );

-- Policy: api_keys - users can see their own api_keys
CREATE POLICY api_keys_self_access ON api_keys
    FOR ALL
    TO PUBLIC
    USING (
        profile_id = nullif(current_setting('app.current_profile_id', true), '')::uuid
    );

-- Policy: api_keys - admin can see all api_keys (read-only; writes must go through explicit profile context)
CREATE POLICY api_keys_admin_access ON api_keys
    FOR SELECT
    TO PUBLIC
    USING (
        current_setting('app.role', true) = 'admin'
    );

-- Policy: audit_log - users can see their own audit_log entries
CREATE POLICY audit_log_self_access ON audit_log
    FOR SELECT
    TO PUBLIC
    USING (
        profile_id = nullif(current_setting('app.current_profile_id', true), '')::uuid
    );

-- Policy: audit_log - admin can see all audit_log entries
CREATE POLICY audit_log_admin_access ON audit_log
    FOR SELECT
    TO PUBLIC
    USING (
        current_setting('app.role', true) = 'admin'
    );

-- Policy: audit_log - all application roles can insert (append-only)
CREATE POLICY audit_log_insert_all ON audit_log
    FOR INSERT
    TO PUBLIC
    WITH CHECK (
        current_setting('app.role', true) IN ('admin', 'standard')
    );

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin

-- Drop audit_log policies
DROP POLICY IF EXISTS audit_log_insert_all ON audit_log;
DROP POLICY IF EXISTS audit_log_admin_access ON audit_log;
DROP POLICY IF EXISTS audit_log_self_access ON audit_log;

-- Drop api_keys policies
DROP POLICY IF EXISTS api_keys_admin_access ON api_keys;
DROP POLICY IF EXISTS api_keys_self_access ON api_keys;

-- Drop profiles policies
DROP POLICY IF EXISTS profiles_admin_access ON profiles;
DROP POLICY IF EXISTS profiles_self_access ON profiles;

-- Disable Row Level Security on audit_log
ALTER TABLE audit_log DISABLE ROW LEVEL SECURITY;

-- Disable Row Level Security on api_keys
ALTER TABLE api_keys DISABLE ROW LEVEL SECURITY;

-- Disable Row Level Security on profiles
ALTER TABLE profiles DISABLE ROW LEVEL SECURITY;

-- +goose StatementEnd
