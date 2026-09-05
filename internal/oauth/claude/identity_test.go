package claude

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth"
	"github.com/stretchr/testify/require"
)

// withBootstrapURL replaces bootstrapURL for the duration of a test
// and restores it afterwards. Tests that hit a local httptest
// server can pin the package-level var without exposing test-only
// configuration in the production path.
func withBootstrapURL(t *testing.T, url string) {
	t.Helper()
	original := bootstrapURL
	bootstrapURL = url
	t.Cleanup(func() { bootstrapURL = original })
}

// TestFetchIdentityHitsBootstrapEndpoint pins the wire contract
// FetchIdentity enforces: HTTP method, path, query parameters, and
// header block. Catching a change here is what protects the next
// Anthropic protocol shift from silently breaking login.
func TestFetchIdentityHitsBootstrapEndpoint(t *testing.T) {
	var (
		gotMethod atomic.Value
		gotPath   atomic.Value
		gotQuery  atomic.Value
		gotAuth   atomic.Value
		gotUA     atomic.Value
		gotBeta   atomic.Value
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod.Store(r.Method)
		gotPath.Store(r.URL.Path)
		gotQuery.Store(r.URL.RawQuery)
		gotAuth.Store(r.Header.Get("Authorization"))
		gotUA.Store(r.Header.Get("User-Agent"))
		gotBeta.Store(r.Header.Get("anthropic-beta"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"oauth_account":{"account_uuid":"acct-1","account_email":"a@b.c","organization_uuid":"org-1","organization_name":"Org"}}`))
	}))
	defer srv.Close()

	// Pin the production path on the local server so the test sees
	// the same path the production code targets. The default
	// bootstrapURL points at api.anthropic.com; the override
	// appends the production path to the test server's root.
	withBootstrapURL(t, srv.URL+"/api/claude_cli/bootstrap")

	_, err := FetchIdentity(context.Background(), "tok-abc")
	require.NoError(t, err)

	require.Equal(t, http.MethodGet, gotMethod.Load())
	require.Equal(t, "/api/claude_cli/bootstrap", gotPath.Load())
	require.Contains(t, gotQuery.Load(), "entrypoint=cli",
		"the entrypoint=cli query param identifies us as Claude Code's CLI bootstrap, not a generic api call")
	require.Contains(t, gotQuery.Load(), "model=claude-opus-4-8",
		"the model query param must match what Claude Code itself sends")
	require.Equal(t, "Bearer tok-abc", gotAuth.Load())
	require.Equal(t, "claude-code/"+claudeCodeVersion, gotUA.Load())
	require.Equal(t, "oauth-2025-04-20", gotBeta.Load())
}

// TestFetchIdentityReadsFields exercises the JSON decoder: every
// field the bootstrap endpoint returns must be available to the
// caller.
func TestFetchIdentityReadsFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"oauth_account": {
				"account_uuid": "acct-uuid-1",
				"account_email": "user@example.com",
				"organization_uuid": "org-uuid-1",
				"organization_name": "Personal Max"
			}
		}`))
	}))
	defer srv.Close()

	withBootstrapURL(t, srv.URL)

	id, err := FetchIdentity(context.Background(), "tok-abc")
	require.NoError(t, err)
	require.Equal(t, "acct-uuid-1", id.AccountID)
	require.Equal(t, "user@example.com", id.Email)
	require.Equal(t, "org-uuid-1", id.OrgID)
	require.Equal(t, "Personal Max", id.OrgName)
}

// TestFetchIdentityHandlesMissingOAuthAccount covers the case
// where the access token is not a Claude subscription. The
// bootstrap endpoint returns a payload without `oauth_account` at
// all in that case; FetchIdentity must return an empty identity
// (not an error) so the caller can still proceed.
func TestFetchIdentityHandlesMissingOAuthAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	withBootstrapURL(t, srv.URL)

	id, err := FetchIdentity(context.Background(), "tok-abc")
	require.NoError(t, err,
		"missing oauth_account is a normal response, not an error")
	require.NotNil(t, id)
	require.Empty(t, id.AccountID)
	require.Empty(t, id.Email)
}

// TestFetchIdentityRejectsEmptyToken enforces the boundary check
// so an empty token never reaches the server.
func TestFetchIdentityRejectsEmptyToken(t *testing.T) {
	_, err := FetchIdentity(context.Background(), "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-empty access token")
}

// TestFetchIdentityReportsHTTPError covers the non-200 path. The
// status code and a truncated body must land in the error message
// for diagnostics.
func TestFetchIdentityReportsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"type":"permission_error","message":"nope"}}`))
	}))
	defer srv.Close()

	withBootstrapURL(t, srv.URL)

	_, err := FetchIdentity(context.Background(), "tok-abc")
	require.Error(t, err)
	require.Contains(t, err.Error(), "403")
	require.Contains(t, err.Error(), "permission_error",
		"error body must be surfaced for diagnostics")
}

// TestFetchIdentityReportsInvalidJSON covers the case where the
// endpoint returns HTML or another non-JSON payload (for example,
// a captive portal response or a Cloudflare error page).
func TestFetchIdentityReportsInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>nope</body></html>`))
	}))
	defer srv.Close()

	withBootstrapURL(t, srv.URL)

	_, err := FetchIdentity(context.Background(), "tok-abc")
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode bootstrap response")
}

// TestTruncateForErrorBoundsLongBodies pins the error-message
// truncation policy. A 500-char body must come back bounded so
// log lines stay readable.
func TestTruncateForErrorBoundsLongBodies(t *testing.T) {
	body := []byte(strings.Repeat("x", 500))
	got := truncateForError(body)
	require.Contains(t, got, "...")
	require.LessOrEqual(t, len(got), 250)

	// Short bodies are returned verbatim.
	short := []byte("ok")
	require.Equal(t, "ok", truncateForError(short))
}

// TestEnrichTokenFillsMissingFields: bootstrap may return a
// partial identity; the token-exchange response may already
// carry the account UUID. EnrichToken must never overwrite an
// already-set field with empty or different data.
func TestEnrichTokenFillsMissingFields(t *testing.T) {
	tok := &oauth.Token{
		AccountID: "preset-acct",
		// Email, OrgID, OrgName, IssuedAt all empty.
	}
	id := &Identity{
		AccountID: "bs-acct",
		Email:     "user@example.com",
		OrgID:     "org-1",
		OrgName:   "Org",
	}
	EnrichToken(tok, id)
	require.Equal(t, "preset-acct", tok.AccountID,
		"existing account id must not be overwritten by bootstrap")
	require.Equal(t, "user@example.com", tok.Email)
	require.Equal(t, "org-1", tok.OrgID)
	require.Equal(t, "Org", tok.OrgName)
	require.NotZero(t, tok.IssuedAt,
		"EnrichToken must set IssuedAt when it is unset")
}

// TestEnrichTokenDoesNotResetIssuedAt: repeated calls must not
// advance the IssuedAt timestamp, or the 30-day TTL would reset
// on every refresh.
func TestEnrichTokenDoesNotResetIssuedAt(t *testing.T) {
	preset := time.Now().Unix() - 1000
	tok := &oauth.Token{IssuedAt: preset}
	EnrichToken(tok, &Identity{Email: "a@b.c"})
	require.Equal(t, preset, tok.IssuedAt,
		"existing IssuedAt must not move")

	tok2 := &oauth.Token{}
	EnrichToken(tok2, &Identity{Email: "a@b.c"})
	require.NotZero(t, tok2.IssuedAt,
		"empty IssuedAt must be set to now")
}

// TestEnrichTokenHandlesNilArguments: a nil safety net makes the
// hook safe to call from a deferred "best effort" path.
func TestEnrichTokenHandlesNilArguments(t *testing.T) {
	require.NotPanics(t, func() { EnrichToken(nil, &Identity{}) })
	require.NotPanics(t, func() { EnrichToken(&oauth.Token{}, nil) })
	require.NotPanics(t, func() { EnrichToken(nil, nil) })
}

// TestDaysUntilGrantExpiryRoundsDown: the day count is a display
// heuristic, not a wire contract. Rounding down so a sub-day
// window shows as 0 (not 1) is what triggers a re-login prompt
// before the deadline slips past silently.
func TestDaysUntilGrantExpiryRoundsDown(t *testing.T) {
	// Issued 1 hour ago: ~29d 23h remain. Whole-day floor: 29.
	tok := &oauth.Token{IssuedAt: time.Now().Add(-time.Hour).Unix()}
	require.Equal(t, 29, DaysUntilGrantExpiry(tok),
		"a fresh token must report 29 days remaining, not 30")

	// Issued 29 days, 23 hours ago: 1 hour left. Whole-day floor: 0.
	// This is the "trigger re-login now" case the round-down
	// behaviour is for.
	tok = &oauth.Token{IssuedAt: time.Now().Add(-(29*24*time.Hour + 23*time.Hour)).Unix()}
	require.Equal(t, 0, DaysUntilGrantExpiry(tok),
		"a sub-day window must round down to 0 so the UI prompts re-login")

	// Issued 30 days, 25 hours ago: ~25h past the deadline.
	// Whole-day floor: -1.
	tok = &oauth.Token{IssuedAt: time.Now().Add(-(30*24*time.Hour + 25*time.Hour)).Unix()}
	require.Less(t, DaysUntilGrantExpiry(tok), 0,
		"past-deadline tokens must report a negative remainder")
}

// TestDaysUntilGrantExpiryUnknownWhenUnset: an unset IssuedAt
// must report 0 (unknown), not the time-since-zero fallback which
// would be a deadline of "year 1970 + 30 days" -- a clearly
// wrong display.
func TestDaysUntilGrantExpiryUnknownWhenUnset(t *testing.T) {
	require.Equal(t, 0, DaysUntilGrantExpiry(&oauth.Token{}),
		"unset IssuedAt must report 0, not the time-since-zero fallback")
	require.Equal(t, 0, DaysUntilGrantExpiry(nil),
		"nil token must report 0")
}

// TestGrantExpiresAtIsIssuedAtPlusTTL: the deadline is the
// stored IssuedAt plus the constant 30-day TTL. Pinning this
// means a future change to GrantTTL is a deliberate edit.
func TestGrantExpiresAtIsIssuedAtPlusTTL(t *testing.T) {
	issued := int64(1700000000)
	tok := &oauth.Token{IssuedAt: issued}
	want := issued + int64(GrantTTL.Seconds())
	require.Equal(t, want, GrantExpiresAt(tok))

	require.Equal(t, int64(0), GrantExpiresAt(nil),
		"nil token must report 0, not panic")
}

// TestGrantTTLIs30Days pins the 30-day constant. Anthropic's
// observed behavior is that the refresh-token family dies 30 days
// after the original authorization regardless of rotation; a
// future "let's bump it to 60 days" change is a deliberate edit,
// not a silent drift.
func TestGrantTTLIs30Days(t *testing.T) {
	require.Equal(t, 30*24*time.Hour, GrantTTL)
}
