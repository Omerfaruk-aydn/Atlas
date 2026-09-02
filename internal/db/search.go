package db

import (
	"context"
	"database/sql"
	"strings"
)

// This query is hand-written rather than generated. sqlc's SQLite parser does
// not recognise a virtual table, so `message_search MATCH ?` -- the whole
// point of the FTS5 index -- comes back as "column message_search does not
// exist". Everything else in this package stays generated.
const searchMessages = `
SELECT
    ms.message_id,
    ms.session_id,
    s.title,
    m.role,
    m.created_at,
    snippet(message_search, 0, '', '', '…', 24)
FROM message_search ms
JOIN messages m ON m.id = ms.message_id
JOIN sessions s ON s.id = ms.session_id
WHERE message_search MATCH ?1
    AND (?2 = '' OR ms.session_id = ?2)
ORDER BY bm25(message_search)
LIMIT ?3
`

// FullQuerier is [Querier] plus the queries sqlc cannot generate. Callers
// that need search take this instead of [Querier]; *Queries satisfies both.
type FullQuerier interface {
	Querier
	SearchMessages(ctx context.Context, arg SearchMessagesParams) ([]SearchMessagesRow, error)
}

type SearchMessagesParams struct {
	// Query is FTS5 match syntax. Build it with [MatchQuery] unless the
	// caller is deliberately writing the syntax itself.
	Query string
	// SessionID limits the search to one session. Empty searches all.
	SessionID string
	Limit     int64
}

type SearchMessagesRow struct {
	MessageID    string `json:"message_id"`
	SessionID    string `json:"session_id"`
	SessionTitle string `json:"session_title"`
	Role         string `json:"role"`
	CreatedAt    int64  `json:"created_at"`
	// Snippet is the matching stretch of the message, cut to roughly two
	// dozen tokens around the hit.
	Snippet string `json:"snippet"`
}

// SearchMessages returns the best-ranked messages matching an FTS5 query,
// most relevant first.
func (q *Queries) SearchMessages(ctx context.Context, arg SearchMessagesParams) ([]SearchMessagesRow, error) {
	rows, err := q.db.QueryContext(ctx, searchMessages, arg.Query, arg.SessionID, arg.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []SearchMessagesRow{}
	for rows.Next() {
		var i SearchMessagesRow
		var title sql.NullString
		if err := rows.Scan(
			&i.MessageID,
			&i.SessionID,
			&title,
			&i.Role,
			&i.CreatedAt,
			&i.Snippet,
		); err != nil {
			return nil, err
		}
		i.SessionTitle = title.String
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// MatchQuery turns what a person typed into FTS5 match syntax.
//
// FTS5 treats bare input as an expression language: a stray quote or a word
// like NOT is a syntax error, not a search for that word. Since the input
// here is a human's phrase and not a query someone is trying to write, every
// token is quoted as a literal and the results are ANDed. The one piece of
// syntax kept is the trailing `*`, because prefix matching is what people
// mean when they type it.
func MatchQuery(input string) string {
	fields := strings.Fields(input)
	quoted := make([]string, 0, len(fields))
	for _, f := range fields {
		prefix := ""
		if strings.HasSuffix(f, "*") && len(f) > 1 {
			f, prefix = strings.TrimSuffix(f, "*"), "*"
		}
		f = strings.ReplaceAll(f, `"`, `""`)
		if f == "" {
			continue
		}
		quoted = append(quoted, `"`+f+`"`+prefix)
	}
	return strings.Join(quoted, " AND ")
}
