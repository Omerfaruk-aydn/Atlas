-- +goose Up
-- +goose StatementBegin
ALTER TABLE sessions ADD COLUMN tags TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE sessions DROP COLUMN tags;
-- +goose StatementEnd
