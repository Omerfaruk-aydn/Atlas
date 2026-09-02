package cmd

import (
	"bytes"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/db"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/message"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func newSearchTestCmd(t *testing.T, dataDir string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{RunE: runSessionSearch}
	c.Flags().String("data-dir", dataDir, "")
	c.Flags().IntVarP(&sessionSearchLimit, "limit", "n", message.DefaultSearchLimit, "")
	c.Flags().StringVarP(&sessionSearchSession, "session", "s", "", "")
	c.SetContext(t.Context())
	t.Cleanup(func() {
		sessionSearchLimit = message.DefaultSearchLimit
		sessionSearchSession = ""
	})
	return c
}

// seedSearchSessions writes two sessions with one user message each and
// returns them in creation order.
func seedSearchSessions(t *testing.T, dataDir string) (session.Session, session.Session) {
	t.Helper()

	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)
	sessions := session.NewService(q, conn)
	messages := message.NewService(q)

	write := func(title, text string) session.Session {
		sess, err := sessions.Create(t.Context(), title)
		require.NoError(t, err)
		_, err = messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
			Role:  message.User,
			Parts: []message.ContentPart{message.TextContent{Text: text}},
		})
		require.NoError(t, err)
		return sess
	}

	return write("migrations work", "how do the goose migrations get applied"),
		write("parser work", "the tokenizer drops diacritics")
}

func TestSessionSearchFindsAMatch(t *testing.T) {
	dataDir := t.TempDir()
	seedSearchSessions(t, dataDir)

	c := newSearchTestCmd(t, dataDir)
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"tokenizer"}))
	require.Contains(t, out.String(), "parser work")
	require.Contains(t, out.String(), "diacritics")
	require.NotContains(t, out.String(), "migrations work")
}

// Extra arguments are words of one query, not a usage error.
func TestSessionSearchJoinsItsArguments(t *testing.T) {
	dataDir := t.TempDir()
	seedSearchSessions(t, dataDir)

	c := newSearchTestCmd(t, dataDir)
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"goose", "migrations"}))
	require.Contains(t, out.String(), "migrations work")
}

func TestSessionSearchReportsNoMatches(t *testing.T) {
	dataDir := t.TempDir()
	seedSearchSessions(t, dataDir)

	c := newSearchTestCmd(t, dataDir)
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"kubernetes"}))
	require.Contains(t, out.String(), "No matches.")
}

func TestSessionSearchCanBeLimitedToOneSession(t *testing.T) {
	dataDir := t.TempDir()
	first, second := seedSearchSessions(t, dataDir)

	// Both seeded messages contain "the", so an unscoped search would
	// return both and this assertion would be vacuous.
	c := newSearchTestCmd(t, dataDir)
	require.NoError(t, c.Flags().Set("session", second.ID))
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"the"}))
	require.NotContains(t, out.String(), first.ID)
	require.NotContains(t, out.String(), "migrations work")
}

func TestSessionSearchUnknownSessionID(t *testing.T) {
	dataDir := t.TempDir()
	seedSearchSessions(t, dataDir)

	c := newSearchTestCmd(t, dataDir)
	require.NoError(t, c.Flags().Set("session", "not-a-real-session"))
	var out bytes.Buffer
	c.SetOut(&out)

	require.Error(t, c.RunE(c, []string{"anything"}))
}

// A snippet cut out of a multi-line message must not break up the listing.
func TestSingleLineFlattensWhitespace(t *testing.T) {
	require.Equal(t, "a b c", singleLine("a\n  b\tc\n"))
	require.Equal(t, "", singleLine("   \n "))
}
