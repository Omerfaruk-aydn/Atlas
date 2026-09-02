package cmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// db.Connect pools one handle per data directory, so a command that closed
// it directly left a closed handle behind for the next one to pick up. Two
// session commands in one process is the shape that catches it -- rare from
// the shell, ordinary in tests and in the server.
func TestSessionSetupCanBeUsedTwiceInOneProcess(t *testing.T) {
	dataDir := t.TempDir()
	run := func() error {
		c := &cobra.Command{RunE: runSessionList}
		c.Flags().String("data-dir", dataDir, "")
		c.Flags().Bool("json", false, "")
		c.SetContext(t.Context())
		var out bytes.Buffer
		c.SetOut(&out)
		return c.RunE(c, nil)
	}
	require.NoError(t, run())
	require.NoError(t, run())
}
