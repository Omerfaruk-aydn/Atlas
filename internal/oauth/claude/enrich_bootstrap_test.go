package claude

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth"
	"github.com/stretchr/testify/require"
)

// TestEnrichWithBootstrapPopulatesToken exercises the login-side
// hook directly: a token arrives from the token-exchange response
// without email/org, the bootstrap endpoint replies with full
// identity, and the resulting token has those fields populated.
//
// The unit test stops at `enrichWithBootstrap` rather than driving
// `Wait` end-to-end because the latter would require a real loopback
// HTTP callback, which is the part of the flow the existing
// existing_test.go and writeback_test.go already exercise
// separately. The 2-line wiring inside Wait is straightforward
// enough that unit-testing the helper is the better return.
func TestEnrichWithBootstrapPopulatesToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"oauth_account": {
				"account_uuid": "acct-bs",
				"account_email": "u@example.com",
				"organization_uuid": "org-bs",
				"organization_name": "Personal Max"
			}
		}`))
	}))
	defer srv.Close()
	withBootstrapURL(t, srv.URL+"/api/claude_cli/bootstrap")

	tok := &oauth.Token{
		AccessToken: "fresh-access",
		// AccountID/Email/OrgID/OrgName all empty, as if the
		// token-exchange response had left them unset.
	}
	enrichWithBootstrap(context.Background(), tok, nil)

	require.Equal(t, "u@example.com", tok.Email)
	require.Equal(t, "org-bs", tok.OrgID)
	require.Equal(t, "Personal Max", tok.OrgName)
	require.NotZero(t, tok.IssuedAt,
		"IssuedAt must be set so the 30-day TTL is computable")
}

// TestEnrichWithBootstrapSurvivesError covers the "best effort"
// contract: a bootstrap failure must not fail the login. The
// caller is calling `enrichWithBootstrap` after a successful
// exchange, so a non-nil error here would be a regression.
func TestEnrichWithBootstrapSurvivesError(t *testing.T) {
	withBootstrapURL(t, "http://127.0.0.1:1/api/claude_cli/bootstrap")

	tok := &oauth.Token{AccessToken: "fresh-access"}
	// A nil progress callback is the production path; the function
	// must handle it without panicking.
	require.NotPanics(t, func() {
		enrichWithBootstrap(context.Background(), tok, nil)
	})
	// Token is unchanged because bootstrap failed.
	require.Equal(t, "fresh-access", tok.AccessToken)
	require.Empty(t, tok.Email)
}

// TestEnrichWithBootstrapSkipsEmptyToken covers the safety check
// at the top of the helper. A token with no access token would
// produce a meaningless Authorization header to the server; the
// helper short-circuits instead.
func TestEnrichWithBootstrapSkipsEmptyToken(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
	}))
	defer srv.Close()
	withBootstrapURL(t, srv.URL)

	tok := &oauth.Token{} // no access token
	enrichWithBootstrap(context.Background(), tok, nil)

	require.Equal(t, 0, hits,
		"bootstrap must not be called when the token has no access token")
}
