-- +goose Up
-- +goose StatementBegin

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS key_suffix VARCHAR(6) NULL;

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin

ALTER TABLE api_keys DROP COLUMN IF EXISTS key_suffix;

-- +goose StatementEnd
