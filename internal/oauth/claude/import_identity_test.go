package claude

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// writeClaudeCodeCredentials stages a fake Claude Code credential
// file at a path returned by ConfigDir and returns the path. Tests
// that exercise ImportExisting call this first.
func writeClaudeCodeCredentials(t *testing.T, dir string, access, refresh string) {
	t.Helper()
	creds := map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":  access,
			"refreshToken": refresh,
			// 1 hour from now in milliseconds -- a healthy access
			// token but more importantly a healthy refresh token.
			"expiresAt":             (nowMs() + 3600_000),
			"refreshTokenExpiresAt": (nowMs() + 7*24*3600_000),
		},
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, credentialsFile), data, 0o600))
}

func nowMs() int64 { return time.Now().UnixMilli() }

// TestImportExistingFetchesBootstrapIdentity covers the new
// import-time enrichment path: a healthy Claude Code credential
// is imported, the bootstrap endpoint is called with the imported
// access token, and the resulting token has email + org filled in.
func TestImportExistingFetchesBootstrapIdentity(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)

	// Local bootstrap server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"oauth_account": {
				"account_uuid": "imported-acct",
				"account_email": "imported@example.com",
				"organization_uuid": "imported-org",
				"organization_name": "Imported Max"
			}
		}`))
	}))
	defer srv.Close()
	withBootstrapURL(t, srv.URL+"/api/claude_cli/bootstrap")

	writeClaudeCodeCredentials(t, dir, "imported-access", "imported-refresh")

	tok, err := ImportExisting()
	require.NoError(t, err)
	require.NotNil(t, tok)

	// Email + org from bootstrap.
	require.Equal(t, "imported@example.com", tok.Email)
	require.Equal(t, "imported-org", tok.OrgID)
	require.Equal(t, "Imported Max", tok.OrgName)
	require.NotZero(t, tok.IssuedAt,
		"IssuedAt must be set so the 30-day TTL is computable")
}

// TestImportExistingSurvivesBootstrapError covers the best-effort
// contract: a bootstrap failure (network error, 401, 500) must
// not fail the import. The user has a working grant on disk; we
// must surface it even when we cannot enrich it.
func TestImportExistingSurvivesBootstrapError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)

	// Point bootstrap at a closed port so the call fails fast.
	withBootstrapURL(t, "http://127.0.0.1:1/api/claude_cli/bootstrap")

	writeClaudeCodeCredentials(t, dir, "tok-1", "refresh-1")

	tok, err := ImportExisting()
	require.NoError(t, err,
		"a bootstrap failure must not fail the import")
	require.NotNil(t, tok)
	require.Equal(t, "tok-1", tok.AccessToken,
		"the imported token must still be returned")
	require.Empty(t, tok.Email,
		"email stays empty when bootstrap fails")
}

// TestImportExistingReturnsErrWhenFileMissing: no Claude Code
// credential on disk must still surface ErrNoExistingLogin, even
// though we now also call bootstrap on the import path. A
// missing file short-circuits before the bootstrap call.
func TestImportExistingReturnsErrWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)

	// Bootstrap would be called if we reached it, but a missing
	// file returns before any network activity. Wire a server
	// that would explode on contact so any call is loud.
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		t.Fatal("bootstrap must not be called when the file is missing")
	}))
	defer srv.Close()
	withBootstrapURL(t, srv.URL)

	_, err := ImportExisting()
	require.ErrorIs(t, err, ErrNoExistingLogin)
	require.Equal(t, 0, hits)
}
