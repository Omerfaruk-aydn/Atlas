-- +goose Up
-- +goose StatementBegin
ALTER TABLE files ADD COLUMN message_id TEXT;
CREATE INDEX IF NOT EXISTS idx_files_message_id ON files (message_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_files_message_id;
ALTER TABLE files DROP COLUMN message_id;
-- +goose StatementEnd
