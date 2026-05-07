-- +goose Up
-- +goose StatementBegin

ALTER TABLE api_keys ALTER COLUMN scopes SET DEFAULT ARRAY['read','write']::text[];
UPDATE api_keys SET scopes = ARRAY['read','write']::text[] WHERE scopes <> ARRAY['read','write']::text[];

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin

ALTER TABLE api_keys ALTER COLUMN scopes SET DEFAULT ARRAY['read']::text[];

-- +goose StatementEnd
