package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth"
)

// The official Claude Code CLI stores its subscription OAuth grant as
// plain JSON under the user's Claude config directory. Reusing it is
// the same trick oh-my-pi uses ("inherits authentication from existing
// dotfiles"), and it is far more robust than re-running the browser
// consent flow: the grant is already issued, already scoped correctly,
// and refreshes through the same token endpoint.
const (
	// credentialsFile is the on-disk name of Claude Code's credential
	// store, inside the config dir resolved by ConfigDir.
	credentialsFile = ".credentials.json"
	// configDirEnv overrides the default ~/.claude location, matching
	// the environment variable Claude Code itself honours.
	configDirEnv = "CLAUDE_CONFIG_DIR"
)

// ErrNoExistingLogin reports that no Claude Code credential was found
// on this machine. Callers fall back to the browser flow.
var ErrNoExistingLogin = errors.New("no existing Claude Code login found")

// ErrExistingLoginExpired reports that a Claude Code credential exists
// but its refresh token has already lapsed, so the grant is dead and
// importing it would only produce authentication failures later. This
// is distinct from ErrNoExistingLogin because the fix is different:
// re-run the official CLI's login rather than look elsewhere.
var ErrExistingLoginExpired = errors.New("the existing Claude Code login has expired; run `claude` and sign in again to refresh it")

// claudeCodeCredentials mirrors the subset of Claude Code's credential
// file this package needs. Unknown fields are ignored so a format
// addition on their side doesn't break the import.
type claudeCodeCredentials struct {
	ClaudeAIOAuth struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		// ExpiresAt is milliseconds since the epoch, unlike
		// oauth.Token.ExpiresAt which is seconds.
		ExpiresAt int64 `json:"expiresAt"`
		// RefreshTokenExpiresAt, also in milliseconds, is the one that
		// actually matters for importing: past it, the whole grant is
		// dead and no refresh can revive it.
		RefreshTokenExpiresAt int64    `json:"refreshTokenExpiresAt"`
		Scopes                []string `json:"scopes"`
		SubscriptionType      string   `json:"subscriptionType"`
	} `json:"claudeAiOauth"`
}

// ConfigDir returns the directory Claude Code keeps its credentials in.
func ConfigDir() (string, error) {
	if dir := os.Getenv(configDirEnv); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// ImportExisting reads the OAuth grant the official Claude Code CLI
// already holds on this machine and converts it into a token this
// codebase can persist and refresh.
//
// It returns ErrNoExistingLogin when there is nothing to import, which
// callers should treat as "fall back to the browser flow" rather than
// as a failure. An expired access token is still returned: the refresh
// token outlives it and the ordinary refresh path renews it on first
// use.
//
// On success the function also calls the bootstrap endpoint
// best-effort to fill in account email and organization identity
// that Claude Code's credential file does not record. A failure
// here is logged and ignored -- the imported grant is still valid
// for model calls without email/org metadata, and a later
// interactive login can recover the metadata cleanly.
func ImportExisting() (*oauth.Token, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, credentialsFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoExistingLogin
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var creds claudeCodeCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	grant := creds.ClaudeAIOAuth
	if grant.AccessToken == "" {
		return nil, ErrNoExistingLogin
	}
	// An expired *access* token is fine -- the refresh token renews it.
	// An expired *refresh* token is not: the grant is gone, and
	// importing it would report a successful login that fails on the
	// first request instead.
	if grant.RefreshTokenExpiresAt > 0 && time.Now().UnixMilli() >= grant.RefreshTokenExpiresAt {
		return nil, ErrExistingLoginExpired
	}

	tok := &oauth.Token{
		AccessToken:  grant.AccessToken,
		RefreshToken: grant.RefreshToken,
		// Claude Code records expiry in milliseconds; this codebase
		// stores seconds.
		ExpiresAt: grant.ExpiresAt / 1000,
		PlanType:  grant.SubscriptionType,
	}
	tok.SetExpiresIn()

	// Best-effort identity enrichment. The access token may be
	// already expired here; in that case bootstrap will 401, and
	// the caller will see a token without email/org. A later
	// refresh + login resolves that.
	enrichWithBootstrap(context.Background(), tok, nil)
	return tok, nil
}

// ExistingSummary describes an importable Claude Code login well enough
// for a CLI to tell the user what it found, without exposing the token
// itself.
type ExistingSummary struct {
	// PlanType is the subscription tier Claude Code recorded ("max",
	// "pro", "team", ...). Empty when the credential predates that
	// field.
	PlanType string
	// Expired reports whether the access token has already lapsed. A
	// refresh renews it, so this is informational, not a blocker.
	Expired bool
	// Path is the credential file the summary came from.
	Path string
}

// DescribeExisting reports what ImportExisting would pick up, for a
// caller that wants to say so before importing.
func DescribeExisting() (*ExistingSummary, error) {
	tok, err := ImportExisting()
	if err != nil {
		return nil, err
	}
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	return &ExistingSummary{
		PlanType: tok.PlanType,
		Expired:  time.Now().Unix() >= tok.ExpiresAt,
		Path:     filepath.Join(dir, credentialsFile),
	}, nil
}
