package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// TestSessionShowJSONReportsARealTimestamp locks in that
// session.Session.CreatedAt/UpdatedAt is Unix seconds -- what the database
// layer actually writes (strftime('%s', 'now')), despite a migration
// comment on the column calling it milliseconds. A regression that started
// treating the value as milliseconds would report a session created moments
// ago as created in 1970.
func TestSessionShowJSONReportsARealTimestamp(t *testing.T) {
	dataDir := t.TempDir()
	sess := seedExportSession(t, dataDir)

	c := &cobra.Command{RunE: runSessionShow}
	c.Flags().String("data-dir", dataDir, "")
	sessionShowJSON = true
	t.Cleanup(func() { sessionShowJSON = false })
	c.SetContext(t.Context())

	var out bytes.Buffer
	c.SetOut(&out)
	require.NoError(t, c.RunE(c, []string{sess.ID}))

	var parsed sessionShowOutput
	require.NoError(t, json.Unmarshal(out.Bytes(), &parsed))

	created, err := time.Parse(time.RFC3339, parsed.Meta.Created)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), created, time.Hour,
		"a session created moments ago must show as created moments ago, not decades off")
}
