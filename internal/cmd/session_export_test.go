package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/db"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/message"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// newExportTestCmd builds a standalone command running the real
// runSessionExport against an isolated data directory, without touching the
// package-level sessionCmd/rootCmd singletons other tests in this package
// also use. sessionExportOutput is reset via the "output" flag's own
// default, so each test gets a clean value without needing to set the
// package var directly.
func newExportTestCmd(t *testing.T, dataDir string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{RunE: runSessionExport}
	c.Flags().String("data-dir", dataDir, "")
	c.Flags().StringVarP(&sessionExportOutput, "output", "o", "", "")
	c.SetContext(t.Context())
	t.Cleanup(func() { sessionExportOutput = "" })
	return c
}

func seedExportSession(t *testing.T, dataDir string) session.Session {
	t.Helper()

	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)
	sessions := session.NewService(q, conn)
	messages := message.NewService(q)

	sess, err := sessions.Create(t.Context(), "exported session")
	require.NoError(t, err)

	_, err = messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "what does this function do"}},
	})
	require.NoError(t, err)

	return sess
}

func TestSessionExportToStdout(t *testing.T) {
	dataDir := t.TempDir()
	sess := seedExportSession(t, dataDir)

	c := newExportTestCmd(t, dataDir)
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{sess.ID}))

	require.Contains(t, out.String(), "# exported session")
	require.Contains(t, out.String(), "what does this function do")
}

func TestSessionExportToFile(t *testing.T) {
	dataDir := t.TempDir()
	sess := seedExportSession(t, dataDir)

	c := newExportTestCmd(t, dataDir)
	outPath := filepath.Join(t.TempDir(), "out.md")
	require.NoError(t, c.Flags().Set("output", outPath))

	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{sess.ID}))
	require.Contains(t, out.String(), "Exported to "+outPath)

	content, err := os.ReadFile(outPath)
	require.NoError(t, err)
	require.Contains(t, string(content), "what does this function do")
}

func TestSessionExportUnknownID(t *testing.T) {
	dataDir := t.TempDir()
	seedExportSession(t, dataDir) // establish the DB, but not the ID we ask for

	c := newExportTestCmd(t, dataDir)
	var out bytes.Buffer
	c.SetOut(&out)

	err := c.RunE(c, []string{"not-a-real-session"})
	require.Error(t, err)
}
