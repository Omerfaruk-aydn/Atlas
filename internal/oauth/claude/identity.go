package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth"
)

// claudeCodeVersion is the Claude Code release version reported in
// the bootstrap-endpoint User-Agent. It is intentionally duplicated
// from internal/deps/atlas-llm/providers/claude/claudeCodeVersion
// because the OAuth and provider packages cannot share a constant
// without an import cycle, and Anthropic's User-Agent check on the
// bootstrap endpoint is permissive enough that a small drift here
// does not break the flow. Update both together.
const claudeCodeVersion = "2.1.257"

// GrantTTL is the absolute lifetime of an Anthropic OAuth grant family,
// anchored at the interactive login. Refresh-token rotation does NOT
// extend it: ~30 days after authorization the token endpoint returns
// `invalid_grant: "Refresh token expired"` for the latest rotated
// token, and only a fresh interactive login recovers the account.
// Observed against production; matches Claude Code's documented
// monthly re-login. Consumers use this to warn before the deadline --
// it is a display heuristic, not a wire contract.
const GrantTTL = 30 * 24 * time.Hour

// bootstrapURL is the Claude Code CLI bootstrap endpoint. It is the
// same path Claude Code itself uses on first launch to resolve the
// signed-in account and the subscription workspace (organization)
// that the access token is scoped to. Token-exchange responses do
// not always include this data; the identity hook uses bootstrap to
// fill the gap.
//
// Source: oh-my-pi `registry/oauth/anthropic.ts` (BOOTSTRAP_URL).
//
// Declared as a `var` (not `const`) so tests can redirect it at a
// local httptest server. Production callers must not mutate it.
var bootstrapURL = "https://api.anthropic.com/api/claude_cli/bootstrap"

// bootstrapModel is the model Claude Code passes on the bootstrap
// request. The endpoint is keyed by the model the user is about to
// use, and `claude-opus-4-8` is what Claude Code's current release
// advertises. Other values still return a valid identity response,
// but matching Claude Code's own choice avoids gratuitous drift.
//
// Source: oh-my-pi `registry/oauth/anthropic.ts`
// (CLAUDE_CODE_BOOTSTRAP_MODEL).
const bootstrapModel = "claude-opus-4-8"

// bootstrapUserAgent is the User-Agent Claude Code sends to the
// bootstrap endpoint. The wire value is just `claude-code/<ver>`,
// not the full request-side fingerprint (no `X-Stainless-*` block,
// no billing header) -- the endpoint serves Claude Code's first
// launch and is permissive about caller identity.
const bootstrapUserAgent = "claude-code/" + claudeCodeVersion

// bootstrapTimeout caps the bootstrap call. The endpoint is local
// to api.anthropic.com and responds in a few hundred milliseconds
// in normal operation; 30s is the same ceiling oh-my-pi uses and
// leaves room for cold starts without hanging the login flow.
const bootstrapTimeout = 30 * time.Second

// Identity is the signed-in account + organization slice resolved
// from the Claude Code CLI bootstrap endpoint.
//
// The organization is the subscription workspace the token draws
// limits from -- one account email can hold several (a Team seat
// plus a personal Max plan, for example), so the org that was
// active at login time is the one that stays in effect for the
// lifetime of the grant.
type Identity struct {
	AccountID string
	Email     string
	OrgID     string
	OrgName   string
}

// bootstrapResponse is the wire shape of the bootstrap endpoint.
// The fields are all optional: the server is free to omit any
// sub-object, and a non-Claude-subscription token can come back
// with `oauth_account` missing entirely. Callers must treat
// missing fields as "unknown, not empty-string".
type bootstrapResponse struct {
	OAuthAccount *struct {
		AccountUUID      string `json:"account_uuid"`
		AccountEmail     string `json:"account_email"`
		OrganizationUUID string `json:"organization_uuid"`
		OrganizationName string `json:"organization_name"`
	} `json:"oauth_account"`
}

// FetchIdentity calls the Claude Code CLI bootstrap endpoint and
// returns the account and organization identity the access token
// is scoped to.
//
// It is intended to be called once at login (when the user is
// interactively authorizing) and never on refresh: the
// organization the token is scoped to is captured at login and
// re-resolving it later could silently re-key stored credentials
// if the user has switched workspaces in the meantime.
//
// The function is best-effort: an error here does not invalidate
// a successful token exchange, so callers should log and continue
// rather than failing the login.
func FetchIdentity(ctx context.Context, accessToken string) (*Identity, error) {
	if accessToken == "" {
		return nil, errors.New("claude: bootstrap identity requires a non-empty access token")
	}
	u := bootstrapURL +
		"?entrypoint=cli" +
		"&model=" + url.QueryEscape(bootstrapModel)

	ctx, cancel := context.WithTimeout(ctx, bootstrapTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("claude: build bootstrap request: %w", err)
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", bootstrapUserAgent)
	req.Header.Set("anthropic-beta", anthropicBeta)

	client := &http.Client{Timeout: bootstrapTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude: bootstrap request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("claude: read bootstrap response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude: bootstrap returned status %d: %s",
			resp.StatusCode, truncateForError(body))
	}

	var parsed bootstrapResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("claude: decode bootstrap response: %w", err)
	}

	acct := parsed.OAuthAccount
	if acct == nil {
		// A token that is not a Claude subscription does not have an
		// `oauth_account` block at all. Return an empty identity
		// rather than an error so the caller can still proceed.
		return &Identity{}, nil
	}
	return &Identity{
		AccountID: nonEmpty(acct.AccountUUID),
		Email:     nonEmpty(acct.AccountEmail),
		OrgID:     nonEmpty(acct.OrganizationUUID),
		OrgName:   nonEmpty(acct.OrganizationName),
	}, nil
}

// EnrichToken folds a fetched identity into a token, only filling
// in fields the token does not already have. The pattern matches
// oh-my-pi's `anthropic-identity` hook: `credentials.accountId ??
// bootstrap.accountId` -- the token-exchange response is treated
// as the more authoritative value when both sides have data, so a
// bootstrap reply that disagrees with an already-stored field is
// never allowed to silently re-key a credential.
//
// IssuedAt is set to the current Unix seconds if it was zero, so
// the 30-day grant TTL can be computed from it later.
func EnrichToken(tok *oauth.Token, id *Identity) {
	if tok == nil || id == nil {
		return
	}
	if tok.AccountID == "" && id.AccountID != "" {
		tok.AccountID = id.AccountID
	}
	if tok.Email == "" && id.Email != "" {
		tok.Email = id.Email
	}
	if tok.OrgID == "" && id.OrgID != "" {
		tok.OrgID = id.OrgID
	}
	if tok.OrgName == "" && id.OrgName != "" {
		tok.OrgName = id.OrgName
	}
	if tok.IssuedAt == 0 {
		tok.IssuedAt = time.Now().Unix()
	}
}

// GrantExpiresAt returns the absolute Unix-seconds deadline at
// which the grant family dies, computed as IssuedAt + GrantTTL.
// Returns 0 when IssuedAt is unset, signalling "unknown" to the
// caller.
func GrantExpiresAt(tok *oauth.Token) int64 {
	if tok == nil || tok.IssuedAt == 0 {
		return 0
	}
	return tok.IssuedAt + int64(GrantTTL.Seconds())
}

// DaysUntilGrantExpiry reports whole days remaining until the
// grant family expires. Negative values mean the deadline has
// already passed. The result is a display heuristic -- the wire
// behavior is binary (refresh works or returns
// `invalid_grant`) -- and consumers should round down rather
// than up so a deadline shown as "0 days" still triggers a
// re-login prompt.
func DaysUntilGrantExpiry(tok *oauth.Token) int {
	deadline := GrantExpiresAt(tok)
	if deadline == 0 {
		return 0
	}
	remaining := deadline - time.Now().Unix()
	days := int(remaining / int64((24 * time.Hour).Seconds()))
	return days
}

func nonEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return s
}

// truncateForError keeps the error message bounded; bootstrap
// responses are normally tiny but a misconfigured server could
// return a long HTML error page, and dumping the whole thing
// into a returned error makes log lines unreadable.
func truncateForError(body []byte) string {
	const max = 200
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "..."
}
