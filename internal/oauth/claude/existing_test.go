package claude

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// writeCredentials drops a Claude Code style credential file into a
// temporary config dir and points ConfigDir at it.
func writeCredentials(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, credentialsFile), []byte(body), 0o600))
	t.Setenv(configDirEnv, dir)
	return dir
}

// The whole point of the import path: a machine that already runs the
// official Claude Code CLI has a working subscription grant sitting on
// disk, and reusing it beats re-running a browser consent flow that
// Anthropic's own consent screen rejects for third parties.
func TestImportExistingReadsAClaudeCodeGrant(t *testing.T) {
	expiresAtMillis := time.Now().Add(time.Hour).UnixMilli()
	writeCredentials(t, `{
	  "claudeAiOauth": {
	    "accessToken": "access-123",
	    "refreshToken": "refresh-456",
	    "expiresAt": `+itoa(expiresAtMillis)+`,
	    "scopes": ["user:profile", "user:inference"],
	    "subscriptionType": "max"
	  }
	}`)

	tok, err := ImportExisting()

	require.NoError(t, err)
	require.Equal(t, "access-123", tok.AccessToken)
	require.Equal(t, "refresh-456", tok.RefreshToken)
	require.Equal(t, "max", tok.PlanType)
	require.Equal(t, expiresAtMillis/1000, tok.ExpiresAt, "Claude Code records milliseconds, this codebase stores seconds")
	require.Positive(t, tok.ExpiresIn, "ExpiresIn is derived from ExpiresAt so the refresh scheduler has something to work with")
}

// An expired access token is still worth importing: the refresh token
// outlives it by weeks, so the ordinary refresh path recovers without
// the user touching a browser at all.
func TestImportExistingKeepsAnExpiredAccessToken(t *testing.T) {
	past := time.Now().Add(-time.Hour).UnixMilli()
	writeCredentials(t, `{
	  "claudeAiOauth": {
	    "accessToken": "stale",
	    "refreshToken": "refresh-456",
	    "expiresAt": `+itoa(past)+`,
	    "subscriptionType": "pro"
	  }
	}`)

	tok, err := ImportExisting()

	require.NoError(t, err)
	require.Equal(t, "stale", tok.AccessToken)
	require.Equal(t, "refresh-456", tok.RefreshToken)

	summary, err := DescribeExisting()
	require.NoError(t, err)
	require.True(t, summary.Expired)
	require.Equal(t, "pro", summary.PlanType)
}

// A grant whose refresh token has lapsed cannot be revived, so
// importing it would hand the user a "you're logged in!" message and
// then fail on the very first request -- which is exactly what
// happened before this check existed.
func TestImportExistingRefusesADeadGrant(t *testing.T) {
	deadRefresh := time.Now().Add(-24 * time.Hour).UnixMilli()
	writeCredentials(t, `{
	  "claudeAiOauth": {
	    "accessToken": "stale",
	    "refreshToken": "also-stale",
	    "expiresAt": `+itoa(deadRefresh)+`,
	    "refreshTokenExpiresAt": `+itoa(deadRefresh)+`,
	    "subscriptionType": "pro"
	  }
	}`)

	_, err := ImportExisting()

	require.ErrorIs(t, err, ErrExistingLoginExpired)
	require.NotErrorIs(t, err, ErrNoExistingLogin, "'expired' and 'absent' need different advice: re-login vs fall back")
	require.Contains(t, err.Error(), "claude", "the message should name the command that fixes it")
}

// The access token lapsing is routine and must not be confused with
// the grant itself dying.
func TestImportExistingAcceptsALiveRefreshToken(t *testing.T) {
	writeCredentials(t, `{
	  "claudeAiOauth": {
	    "accessToken": "stale",
	    "refreshToken": "still-good",
	    "expiresAt": `+itoa(time.Now().Add(-time.Hour).UnixMilli())+`,
	    "refreshTokenExpiresAt": `+itoa(time.Now().Add(72*time.Hour).UnixMilli())+`
	  }
	}`)

	tok, err := ImportExisting()

	require.NoError(t, err)
	require.Equal(t, "still-good", tok.RefreshToken)
}

func TestImportExistingWithNoFileIsNotAnError(t *testing.T) {
	t.Setenv(configDirEnv, t.TempDir())

	_, err := ImportExisting()

	require.ErrorIs(t, err, ErrNoExistingLogin, "a missing credential file means 'fall back to the browser flow', not 'fail the login'")
}

// A credential file that exists but holds only a console/API-key grant
// (no claudeAiOauth block) is the same situation as no file at all.
func TestImportExistingWithoutASubscriptionGrantFallsBack(t *testing.T) {
	writeCredentials(t, `{"someOtherProvider": {"accessToken": "nope"}}`)

	_, err := ImportExisting()

	require.ErrorIs(t, err, ErrNoExistingLogin)
}

func TestImportExistingRejectsMalformedJSON(t *testing.T) {
	writeCredentials(t, `{"claudeAiOauth": `)

	_, err := ImportExisting()

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrNoExistingLogin, "a corrupt file is a real problem worth reporting, not a silent fallback")
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
