package claude

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth"
)

// syncBack keeps Claude Code's credential file in step after this
// package refreshes a grant that was imported from it.
//
// Anthropic rotates refresh tokens: a successful refresh can invalidate
// the token that was sent. Since ImportExisting deliberately shares one
// grant with the official CLI, refreshing here would otherwise log the
// user out of Claude Code the next time it tried to refresh with the
// now-dead token. Writing the rotated pair back keeps both tools on the
// live grant.
//
// It only writes when the file's stored refresh token is the one that
// was just spent, so an unrelated Claude Code login (a different
// account, or a grant this codebase never imported) is left alone.
// Every failure is logged and swallowed: the refresh itself already
// succeeded, and Atlas-Agent's own credential store is the source of
// truth for Atlas-Agent.
func syncBack(spentRefreshToken string, tok *oauth.Token) {
	if spentRefreshToken == "" || tok == nil || tok.RefreshToken == "" {
		return
	}
	if tok.RefreshToken == spentRefreshToken {
		// No rotation happened, so the file is still accurate.
		return
	}

	dir, err := ConfigDir()
	if err != nil {
		slog.Debug("Claude credential sync-back skipped", "error", err)
		return
	}
	path := filepath.Join(dir, credentialsFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Debug("Claude credential sync-back skipped", "path", path, "error", err)
		}
		return
	}

	// Decode into a generic map so fields this package doesn't model --
	// subscriptionType, rateLimitTier, other providers' blocks -- survive
	// the rewrite untouched.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		slog.Debug("Claude credential sync-back skipped: unparseable file", "path", path, "error", err)
		return
	}
	blockRaw, ok := raw["claudeAiOauth"]
	if !ok {
		return
	}
	var block map[string]any
	if err := json.Unmarshal(blockRaw, &block); err != nil {
		slog.Debug("Claude credential sync-back skipped: unparseable grant", "path", path, "error", err)
		return
	}

	stored, _ := block["refreshToken"].(string)
	if stored != spentRefreshToken {
		// Someone else's grant, or one already rotated past ours.
		return
	}

	block["accessToken"] = tok.AccessToken
	block["refreshToken"] = tok.RefreshToken
	// Claude Code records expiry in milliseconds.
	block["expiresAt"] = tok.ExpiresAt * 1000

	updatedBlock, err := json.Marshal(block)
	if err != nil {
		slog.Debug("Claude credential sync-back failed to encode", "error", err)
		return
	}
	raw["claudeAiOauth"] = updatedBlock

	updated, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		slog.Debug("Claude credential sync-back failed to encode", "error", err)
		return
	}

	// Write through a temp file in the same directory so a crash
	// mid-write cannot leave Claude Code with a truncated credential.
	tmp, err := os.CreateTemp(dir, ".credentials-*.tmp")
	if err != nil {
		slog.Debug("Claude credential sync-back failed to stage", "error", err)
		return
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(updated); err != nil {
		_ = tmp.Close()
		slog.Debug("Claude credential sync-back failed to write", "error", err)
		return
	}
	if err := tmp.Close(); err != nil {
		slog.Debug("Claude credential sync-back failed to close", "error", err)
		return
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		slog.Debug("Claude credential sync-back could not tighten permissions", "error", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		slog.Debug("Claude credential sync-back failed to replace", "path", path, "error", err)
		return
	}
	slog.Debug("Rotated Claude grant written back to Claude Code's credential file", "path", path)
}
