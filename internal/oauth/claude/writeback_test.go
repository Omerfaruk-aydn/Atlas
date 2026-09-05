package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth"
	"github.com/stretchr/testify/require"
)

// readBlock returns the claudeAiOauth object from the credential file
// the current test has pointed ConfigDir at.
func readBlock(t *testing.T) map[string]any {
	t.Helper()
	dir, err := ConfigDir()
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, credentialsFile))
	require.NoError(t, err)
	var parsed map[string]map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	return parsed["claudeAiOauth"]
}

// Anthropic rotates refresh tokens. Because the imported grant is
// shared with the official Claude Code CLI, a refresh here has to hand
// the rotated pair back, or Claude Code's next refresh spends a dead
// token and logs the user out of their primary tool.
func TestSyncBackWritesTheRotatedGrant(t *testing.T) {
	writeCredentials(t, `{
	  "claudeAiOauth": {
	    "accessToken": "old-access",
	    "refreshToken": "old-refresh",
	    "expiresAt": 1000,
	    "subscriptionType": "max",
	    "rateLimitTier": "default_claude_max"
	  }
	}`)

	syncBack("old-refresh", &oauth.Token{
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		ExpiresAt:    12345,
	})

	block := readBlock(t)
	require.Equal(t, "new-access", block["accessToken"])
	require.Equal(t, "new-refresh", block["refreshToken"])
	require.EqualValues(t, 12345*1000, block["expiresAt"], "Claude Code reads milliseconds")
	require.Equal(t, "max", block["subscriptionType"], "fields this package doesn't model must survive the rewrite")
	require.Equal(t, "default_claude_max", block["rateLimitTier"])
}

// A file holding a different account's grant, or one already rotated
// past ours, must be left completely alone.
func TestSyncBackLeavesSomeoneElsesGrantAlone(t *testing.T) {
	writeCredentials(t, `{
	  "claudeAiOauth": {
	    "accessToken": "their-access",
	    "refreshToken": "their-refresh",
	    "expiresAt": 1000
	  }
	}`)

	syncBack("old-refresh", &oauth.Token{
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		ExpiresAt:    12345,
	})

	block := readBlock(t)
	require.Equal(t, "their-access", block["accessToken"])
	require.Equal(t, "their-refresh", block["refreshToken"])
}

// When the server hands back the same refresh token, nothing rotated
// and the file is already accurate -- don't rewrite it.
func TestSyncBackSkipsWhenNothingRotated(t *testing.T) {
	writeCredentials(t, `{
	  "claudeAiOauth": {
	    "accessToken": "old-access",
	    "refreshToken": "same-refresh",
	    "expiresAt": 1000
	  }
	}`)

	syncBack("same-refresh", &oauth.Token{
		AccessToken:  "new-access",
		RefreshToken: "same-refresh",
		ExpiresAt:    12345,
	})

	block := readBlock(t)
	require.Equal(t, "old-access", block["accessToken"], "an unrotated refresh leaves the shared file untouched")
}

// Sync-back is a courtesy to another tool, never a reason to fail a
// refresh that already succeeded.
func TestSyncBackSurvivesAMissingOrBrokenFile(t *testing.T) {
	t.Setenv(configDirEnv, t.TempDir())
	require.NotPanics(t, func() {
		syncBack("old-refresh", &oauth.Token{AccessToken: "a", RefreshToken: "b", ExpiresAt: 1})
	})

	writeCredentials(t, `{"claudeAiOauth": `)
	require.NotPanics(t, func() {
		syncBack("old-refresh", &oauth.Token{AccessToken: "a", RefreshToken: "b", ExpiresAt: 1})
	})
}
