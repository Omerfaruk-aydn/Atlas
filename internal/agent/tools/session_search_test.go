package tools

import (
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/db"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/message"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/stretchr/testify/require"
)

func newSearchFixture(t *testing.T) (fantasy.AgentTool, message.Service, string) {
	t.Helper()

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)
	sess, err := session.NewService(q, conn).Create(t.Context(), "a past session")
	require.NoError(t, err)

	messages := message.NewService(q)
	return NewSessionSearchTool(messages), messages, sess.ID
}

func runSearch(t *testing.T, tool fantasy.AgentTool, params SessionSearchParams) fantasy.ToolResponse {
	t.Helper()

	input, err := json.Marshal(params)
	require.NoError(t, err)

	res, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "c", Name: SessionSearchToolName, Input: string(input)})
	require.NoError(t, err)
	return res
}

func TestSessionSearchToolNamesTheSession(t *testing.T) {
	t.Parallel()
	tool, messages, sessionID := newSearchFixture(t)

	_, err := messages.Create(t.Context(), sessionID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "we agreed to keep the flag undocumented"}},
	})
	require.NoError(t, err)

	res := runSearch(t, tool, SessionSearchParams{Query: "undocumented flag"})

	require.False(t, res.IsError)
	require.Contains(t, res.Content, "a past session", "a hit is useless without saying where it came from")
	require.Contains(t, res.Content, sessionID)
	require.Contains(t, res.Content, "undocumented")
}

func TestSessionSearchToolSaysWhatToTryInstead(t *testing.T) {
	t.Parallel()
	tool, _, _ := newSearchFixture(t)

	res := runSearch(t, tool, SessionSearchParams{Query: "nothing like this was ever said"})

	require.False(t, res.IsError, "finding nothing is an answer, not a failure")
	require.Contains(t, res.Content, "No message matches")
	require.Contains(t, res.Content, "try fewer")
}

func TestSessionSearchToolRequiresAQuery(t *testing.T) {
	t.Parallel()
	tool, _, _ := newSearchFixture(t)

	res := runSearch(t, tool, SessionSearchParams{Query: "  "})

	require.True(t, res.IsError)
	require.Contains(t, res.Content, "query is required")
}

func TestSessionSearchToolPutsEachHitOnOneLine(t *testing.T) {
	t.Parallel()
	tool, messages, sessionID := newSearchFixture(t)

	_, err := messages.Create(t.Context(), sessionID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "first line\n\nsecond line about kestrels\n\nthird"}},
	})
	require.NoError(t, err)

	res := runSearch(t, tool, SessionSearchParams{Query: "kestrels"})

	require.False(t, res.IsError)
	require.Contains(t, res.Content, "second line about kestrels")
	require.NotContains(t, res.Content, "\n\n  ", "the snippet is collapsed so one hit stays one block")
}
