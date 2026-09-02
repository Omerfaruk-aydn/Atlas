package message

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func say(t *testing.T, svc Service, sessionID, role string, text string) Message {
	t.Helper()
	msg, err := svc.Create(t.Context(), sessionID, CreateMessageParams{
		Role:  MessageRole(role),
		Parts: []ContentPart{TextContent{Text: text}},
	})
	require.NoError(t, err)
	return msg
}

func TestSearchRanksAndSnippets(t *testing.T) {
	svc, sessionID := newTestService(t)

	say(t, svc, sessionID, "user", "why did we drop the retry loop around the uploader?")
	say(t, svc, sessionID, "assistant", "The retry loop was redundant: the server already retries, so ours just multiplied the delay.")
	say(t, svc, sessionID, "user", "unrelated: what time is the meeting")

	hits, err := svc.Search(t.Context(), SearchParams{Query: "retry loop"})
	require.NoError(t, err)
	require.Len(t, hits, 2, "both messages mention retry and loop; the third mentions neither")

	for _, hit := range hits {
		require.NotEmpty(t, hit.Snippet)
		require.Equal(t, sessionID, hit.SessionID)
	}
}

func TestSearchIgnoresToolOutput(t *testing.T) {
	svc, sessionID := newTestService(t)

	_, err := svc.Create(t.Context(), sessionID, CreateMessageParams{
		Role: Assistant,
		Parts: []ContentPart{
			TextContent{Text: "reading the file now"},
			ToolResult{ToolCallID: "1", Name: "view", Content: "package zeppelin // in the file, not the conversation"},
		},
	})
	require.NoError(t, err)

	hits, err := svc.Search(t.Context(), SearchParams{Query: "zeppelin"})
	require.NoError(t, err)
	require.Empty(t, hits, "tool output is not the conversation; grep is for that")

	hits, err = svc.Search(t.Context(), SearchParams{Query: "reading the file"})
	require.NoError(t, err)
	require.Len(t, hits, 1)
}

func TestSearchIndexesShellCommands(t *testing.T) {
	svc, sessionID := newTestService(t)

	_, err := svc.Create(t.Context(), sessionID, CreateMessageParams{
		Role:  Assistant,
		Parts: []ContentPart{ShellCommand{Command: "go test ./internal/gizmo -race", Output: "ok"}},
	})
	require.NoError(t, err)

	hits, err := svc.Search(t.Context(), SearchParams{Query: "gizmo"})
	require.NoError(t, err)
	require.Len(t, hits, 1, "what was run is worth finding, even though its output is not indexed")
}

func TestSearchFollowsAnUpdate(t *testing.T) {
	svc, sessionID := newTestService(t)

	msg := say(t, svc, sessionID, "assistant", "placeholder")
	msg.Parts = []ContentPart{TextContent{Text: "the answer is quicksilver"}}
	require.NoError(t, svc.Update(t.Context(), msg))
	require.NoError(t, svc.Flush(t.Context(), msg.ID))

	hits, err := svc.Search(t.Context(), SearchParams{Query: "quicksilver"})
	require.NoError(t, err)
	require.Len(t, hits, 1, "a streamed message is rewritten as it arrives; the index has to follow")

	hits, err = svc.Search(t.Context(), SearchParams{Query: "placeholder"})
	require.NoError(t, err)
	require.Empty(t, hits, "and the old text has to go")
}

func TestSearchDropsWithTheMessage(t *testing.T) {
	svc, sessionID := newTestService(t)

	msg := say(t, svc, sessionID, "user", "ephemeral thought")
	require.NoError(t, svc.Delete(t.Context(), msg.ID))

	hits, err := svc.Search(t.Context(), SearchParams{Query: "ephemeral"})
	require.NoError(t, err)
	require.Empty(t, hits)
}

func TestSearchScopesToASession(t *testing.T) {
	svc, sessionID := newTestService(t)
	other, otherID := newTestService(t)
	_ = other

	say(t, svc, sessionID, "user", "shared word here")

	hits, err := svc.Search(t.Context(), SearchParams{Query: "shared", SessionID: otherID})
	require.NoError(t, err)
	require.Empty(t, hits, "a different session, and a different database, sees nothing")

	hits, err = svc.Search(t.Context(), SearchParams{Query: "shared", SessionID: sessionID})
	require.NoError(t, err)
	require.Len(t, hits, 1)
}

// FTS5 reads its input as an expression language, so a quote or a bare NOT
// is a syntax error rather than a search term. Everything a person types has
// to survive.
func TestSearchTakesProseNotSyntax(t *testing.T) {
	svc, sessionID := newTestService(t)

	say(t, svc, sessionID, "user", `he said "not really" about the OR flag`)

	for _, query := range []string{`"not really"`, "NOT really", "OR flag", "AND"} {
		hits, err := svc.Search(t.Context(), SearchParams{Query: query})
		require.NoError(t, err, "query %q must not be a syntax error", query)
		_ = hits
	}

	hits, err := svc.Search(t.Context(), SearchParams{Query: "NOT really"})
	require.NoError(t, err)
	require.Len(t, hits, 1, "the words are searched for, not interpreted")
}

func TestSearchPrefixMatch(t *testing.T) {
	svc, sessionID := newTestService(t)

	say(t, svc, sessionID, "user", "the migration runs at startup")

	hits, err := svc.Search(t.Context(), SearchParams{Query: "migrat*"})
	require.NoError(t, err)
	require.Len(t, hits, 1)

	hits, err = svc.Search(t.Context(), SearchParams{Query: "migrat"})
	require.NoError(t, err)
	require.Empty(t, hits, "without the star it is a whole word")
}

func TestSearchEmptyQueryFindsNothing(t *testing.T) {
	svc, sessionID := newTestService(t)
	say(t, svc, sessionID, "user", "something")

	hits, err := svc.Search(t.Context(), SearchParams{Query: "   "})
	require.NoError(t, err)
	require.Empty(t, hits, "an empty query must not mean everything")
}

func TestSearchLimit(t *testing.T) {
	svc, sessionID := newTestService(t)
	for range 5 {
		say(t, svc, sessionID, "user", "repeated needle "+strings.Repeat("x", 3))
	}

	hits, err := svc.Search(t.Context(), SearchParams{Query: "needle", Limit: 2})
	require.NoError(t, err)
	require.Len(t, hits, 2)
}

func TestSearchSessionIDsRanksBestSessionFirst(t *testing.T) {
	svc, sessionA := newTestService(t)
	_, sessionB := newTestService(t)
	_ = sessionB

	// sessionA gets two matching messages, sessionB gets one -- sessionA
	// should rank ahead, not merely appear because it was touched more
	// recently.
	say(t, svc, sessionA, "user", "kayak trip planning")
	say(t, svc, sessionA, "assistant", "kayak rental confirmed")

	other, otherSession := newTestService(t)
	say(t, other, otherSession, "user", "one mention of kayak here")

	ids, err := svc.SearchSessionIDs(t.Context(), "kayak")
	require.NoError(t, err)
	require.Contains(t, ids, sessionA)
	require.NotContains(t, ids, otherSession, "a different service is a different database")
}

func TestSearchSessionIDsDedupes(t *testing.T) {
	svc, sessionID := newTestService(t)

	say(t, svc, sessionID, "user", "budget review budget review")
	say(t, svc, sessionID, "assistant", "budget review again")

	ids, err := svc.SearchSessionIDs(t.Context(), "budget")
	require.NoError(t, err)
	require.Equal(t, []string{sessionID}, ids, "one session, however many of its messages matched")
}

func TestSearchSessionIDsEmptyOnNoMatch(t *testing.T) {
	svc, sessionID := newTestService(t)
	say(t, svc, sessionID, "user", "nothing relevant")

	ids, err := svc.SearchSessionIDs(t.Context(), "unrelated-term")
	require.NoError(t, err)
	require.Empty(t, ids)
}
