-- name: GetMessage :one
SELECT *
FROM messages
WHERE id = ? LIMIT 1;

-- name: ListMessagesBySession :many
SELECT *
FROM messages
WHERE session_id = ?
ORDER BY created_at ASC;

-- name: CreateMessage :one
INSERT INTO messages (
    id,
    session_id,
    role,
    parts,
    model,
    provider,
    is_summary_message,
    created_at,
    updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now')
)
RETURNING *;

-- name: UpdateMessage :exec
UPDATE messages
SET
    parts = ?,
    finished_at = ?,
    updated_at = strftime('%s', 'now')
WHERE id = ?;


-- name: DeleteMessage :exec
DELETE FROM messages
WHERE id = ?;

-- name: DeleteSessionMessages :exec
DELETE FROM messages
WHERE session_id = ?;

-- name: ListUserMessagesBySession :many
SELECT *
FROM messages
WHERE session_id = ? AND role = 'user'
ORDER BY created_at DESC;

-- name: ListAllUserMessages :many
SELECT *
FROM messages
WHERE role = 'user'
ORDER BY created_at DESC;

-- name: GetLastAssistantMessageBySession :one
SELECT *
FROM messages
WHERE session_id = ? AND role = 'assistant' AND is_summary_message = 0
ORDER BY created_at DESC
LIMIT 1;

-- name: SearchSessionIDsByMessageContent :many
-- Session search by message content: parts is the message's serialized
-- content (JSON), which still contains the readable text, so a plain LIKE
-- over it is a cheap, no-schema-change substring search. The query param
-- must already be wrapped in %...% and have LIKE metacharacters escaped by
-- the caller.
SELECT DISTINCT session_id, MAX(updated_at) AS last_matched_at
FROM messages
WHERE parts LIKE ? ESCAPE '\'
GROUP BY session_id
ORDER BY last_matched_at DESC
LIMIT 50;
