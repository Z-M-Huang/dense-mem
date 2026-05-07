-- +goose Up
-- +goose StatementBegin

-- audit_log is append-only. Foreign-key ON DELETE SET NULL would mutate
-- historical audit rows when profiles or API keys are hard-deleted, which
-- conflicts with the audit_log_append_only trigger.
ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_profile_id_fkey;
ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_actor_key_id_fkey;

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin

-- Recreate the original constraints for rollback. NOT VALID avoids failing if
-- hard-deleted profiles or keys have left historical audit references behind.
ALTER TABLE audit_log
    ADD CONSTRAINT audit_log_profile_id_fkey
    FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE SET NULL NOT VALID;

ALTER TABLE audit_log
    ADD CONSTRAINT audit_log_actor_key_id_fkey
    FOREIGN KEY (actor_key_id) REFERENCES api_keys(id) ON DELETE SET NULL NOT VALID;

-- +goose StatementEnd
