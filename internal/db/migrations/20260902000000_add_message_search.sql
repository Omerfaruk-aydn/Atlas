-- +goose Up
-- +goose StatementBegin
-- A full-text index over what was actually said in a session.
--
-- Only prose is indexed: the text parts of a message and the command line of a
-- shell call. Tool calls and their results are deliberately left out. They are
-- the bulk of a coding session's bytes and almost none of its meaning -- a file
-- listing or a diff would swamp every real answer, and someone looking for
-- "why did we drop the retry loop" wants the sentence that said so, not the
-- forty files that were read around it.
CREATE VIRTUAL TABLE IF NOT EXISTS message_search USING fts5(
    body,
    message_id UNINDEXED,
    session_id UNINDEXED,
    tokenize = 'unicode61 remove_diacritics 2'
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS message_search_insert
AFTER INSERT ON messages
BEGIN
INSERT INTO message_search (rowid, body, message_id, session_id)
SELECT
    new.rowid,
    COALESCE((
        SELECT group_concat(
            COALESCE(json_extract(value, '$.data.text'), json_extract(value, '$.data.command')),
            char(10)
        )
        FROM json_each(new.parts)
        WHERE json_extract(value, '$.type') IN ('text', 'shell_command')
    ), ''),
    new.id,
    new.session_id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS message_search_delete
AFTER DELETE ON messages
BEGIN
DELETE FROM message_search WHERE rowid = old.rowid;
END;
-- +goose StatementEnd

-- +goose StatementBegin
-- Parts are rewritten as a response streams in, so this fires often. Deleting
-- and re-inserting the one row is what FTS5 wants; there is no cheaper update.
CREATE TRIGGER IF NOT EXISTS message_search_update
AFTER UPDATE OF parts ON messages
BEGIN
DELETE FROM message_search WHERE rowid = old.rowid;
INSERT INTO message_search (rowid, body, message_id, session_id)
SELECT
    new.rowid,
    COALESCE((
        SELECT group_concat(
            COALESCE(json_extract(value, '$.data.text'), json_extract(value, '$.data.command')),
            char(10)
        )
        FROM json_each(new.parts)
        WHERE json_extract(value, '$.type') IN ('text', 'shell_command')
    ), ''),
    new.id,
    new.session_id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
-- Backfill whatever is already on disk, so search is not blind to every
-- session that happened before this migration.
INSERT INTO message_search (rowid, body, message_id, session_id)
SELECT
    m.rowid,
    COALESCE((
        SELECT group_concat(
            COALESCE(json_extract(value, '$.data.text'), json_extract(value, '$.data.command')),
            char(10)
        )
        FROM json_each(m.parts)
        WHERE json_extract(value, '$.type') IN ('text', 'shell_command')
    ), ''),
    m.id,
    m.session_id
FROM messages m;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS message_search_update;
DROP TRIGGER IF EXISTS message_search_delete;
DROP TRIGGER IF EXISTS message_search_insert;
DROP TABLE IF EXISTS message_search;
-- +goose StatementEnd
