// Package claude implements the OAuth2 + PKCE login flow the official
// Claude Code CLI uses, so Atlas-Agent can use a Claude Pro/Max/Team
// subscription's flat-rate quota instead of a separate pay-per-token
// Anthropic API key.
//
// None of this is officially documented by Anthropic. Every value here
// was confirmed by extracting the literal strings out of the official
// Claude Code CLI's own compiled binary (@anthropic-ai/claude-code):
// the exact authorize-URL-builder and token-exchange functions, byte
// for byte. Three earlier guesses were each rejected differently
// before landing here: an OIDC-shaped scope list ("Unknown scope:
// openid"), then a plausible-looking but wrong redirect_uri/token
// endpoint pair, then the right endpoints missing the "code=true"
// param the real client always sends -- each got further before
// failing, which is what made the wrong assumption non-obvious each
// time. It may still drift if Anthropic changes the client
// registration.
//
// Risk note: claude.ai's terms of service restrict the service to
// first-party clients in the same way Antigravity's do, and Anthropic
// has revoked third-party access before. Using this login is at the
// user's own risk to their Anthropic account standing, not
// Atlas-Agent's.
//
// Still open, per the TODOs below: the model call layer in
// internal/deps/atlas-llm/providers/claude (the request/response
// envelope claude.ai's console backend speaks for chat completions).
package claude

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth/callback"
)

const (
	// clientID is the OAuth client id the official Claude Code CLI
	// registers itself as. Not published in any official Anthropic
	// documentation; observed and reused across open-source
	// reimplementations of Claude Code's own login flow.
	clientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	// authorizeURL is the OAuth2 authorization endpoint for the
	// Claude Pro/Max subscription flow specifically. Claude Code's
	// client has two distinct authorize endpoints depending on which
	// account type is signing in -- CONSOLE_AUTHORIZE_URL
	// (platform.claude.com, for pay-per-token API key accounts) and
	// CLAUDE_AI_AUTHORIZE_URL (this one, for Pro/Max/Team
	// subscriptions). Using claude.ai/oauth/authorize directly, or
	// api.anthropic.com for the token endpoint below, both looked
	// plausible but are wrong: they reached a real login screen and
	// still failed with "Authorization failed: Invalid request
	// format", because that's not the endpoint pair this client_id is
	// registered against for this account type.
	authorizeURL = "https://claude.com/cai/oauth/authorize"
	// redirectPath is the loopback callback path. The port is not
	// fixed: the official client listens on 127.0.0.1:0 and sends
	// whichever port the OS handed it, so the port is not part of the
	// client registration. Binding an ephemeral port here too avoids
	// the failure mode a fixed port has -- an earlier sign-in attempt
	// that ended without closing its listener makes every later
	// attempt fail to start.
	redirectPath = "/callback"
	// authorizeScopes is the scope set the authorize request must ask
	// for, verbatim and in this order. Read out of the official Claude
	// Code binary, where it is built as the deduplicated concatenation
	// of ["org:create_api_key", "user:profile"] and refreshScopes below.
	//
	// "org:create_api_key" looks like it belongs to the console
	// (pay-per-token) flow and not to a subscription login -- a
	// working credential on disk never lists it, because the consent
	// screen strips it back out of what it grants. It is nevertheless
	// required on the way in: omitting it is what makes the authorize
	// page fail with "Authorization failed: Invalid request format".
	authorizeScopes = "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	// refreshScopes is the narrower set restated on every token
	// refresh -- the same list, minus "org:create_api_key". It matches
	// both what the official client sends on refresh and what a
	// granted subscription credential actually holds.
	refreshScopes = "user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	// anthropicBeta is the beta header Claude Code sends on refresh
	// (not on the initial code exchange) so the OAuth-token grant is
	// recognized by api.anthropic.com.
	anthropicBeta = "oauth-2025-04-20"
)

// tokenURL is the OAuth2 token-exchange endpoint. Shared by both
// account types; served from platform.claude.com, not claude.ai
// or api.anthropic.com. Declared as a `var` (not `const`) so
// tests can redirect it at a local httptest server; production
// callers must not mutate it.
var tokenURL = "https://platform.claude.com/v1/oauth/token"

// AuthSession is one in-flight authorization attempt: a PKCE
// verifier/state pair plus the localhost listener waiting for the
// browser redirect. Same shape as Antigravity's AuthSession so the
// CLI login flow does not need to know which provider it is driving.
type AuthSession struct {
	verifier string
	state    string
	url      string
	// redirect is the loopback callback URL for this session,
	// including the port the OS actually assigned. The token exchange
	// has to repeat it verbatim, so it is captured per session rather
	// than derived from a constant.
	redirect string

	server   *http.Server
	resultCh chan authResult
}

type authResult struct {
	code string
	err  error
}

// Start generates a fresh PKCE challenge, binds the localhost callback
// port, and returns a session whose AuthURL should be opened in the
// browser. Call Wait afterwards to block for the redirect, exchange
// the code, and discover the account/organization id needed for model
// calls.
func Start(ctx context.Context) (*AuthSession, error) {
	verifier, err := randomURLSafe(32)
	if err != nil {
		return nil, fmt.Errorf("generate pkce verifier: %w", err)
	}
	// 32 bytes, matching the official client's randomBytes(32) for both
	// the verifier and the state. A shorter state is a plausible-looking
	// but rejected request.
	state, err := randomURLSafe(32)
	if err != nil {
		return nil, fmt.Errorf("generate oauth state: %w", err)
	}

	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	lc := &net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("bind a loopback port for Claude sign-in: %w", err)
	}

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return nil, fmt.Errorf("unexpected listener address %T", listener.Addr())
	}

	sess := &AuthSession{
		verifier: verifier,
		state:    state,
		redirect: fmt.Sprintf("http://localhost:%d%s", addr.Port, redirectPath),
		resultCh: make(chan authResult, 1),
	}

	// The query string is assembled by hand rather than with
	// url.Values.Encode() because that sorts parameters alphabetically.
	// The official client appends them in this exact order, and the
	// authorize endpoint is picky enough about the request shape that
	// it is not worth betting on order being ignored.
	//
	// The leading fixed "code=true" is what the real client always
	// sends -- on the loopback flow too, not just the manual
	// copy-the-code one. Its purpose isn't documented; it is reproduced
	// verbatim rather than guessed at.
	params := [][2]string{
		{"code", "true"},
		{"client_id", clientID},
		{"response_type", "code"},
		{"redirect_uri", sess.redirect},
		{"scope", authorizeScopes},
		{"code_challenge", challenge},
		{"code_challenge_method", "S256"},
		{"state", state},
	}
	var q strings.Builder
	for i, p := range params {
		if i > 0 {
			q.WriteByte('&')
		}
		q.WriteString(url.QueryEscape(p[0]))
		q.WriteByte('=')
		q.WriteString(url.QueryEscape(p[1]))
	}
	sess.url = authorizeURL + "?" + q.String()

	mux := http.NewServeMux()
	mux.HandleFunc(redirectPath, sess.handleCallback)
	sess.server = &http.Server{Handler: mux}
	go func() {
		_ = sess.server.Serve(listener)
	}()

	return sess, nil
}

// AuthURL returns the URL to open in the user's browser.
func (s *AuthSession) AuthURL() string { return s.url }

func (s *AuthSession) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	result := callback.Result{
		Subject:          "Claude",
		ErrorCode:        q.Get("error"),
		ErrorDescription: q.Get("error_description"),
	}
	if err := callback.Serve(w, result); err != nil {
		s.send(authResult{err: fmt.Errorf("render callback page: %w", err)})
		return
	}

	switch {
	case result.Failed():
		s.send(authResult{err: fmt.Errorf("authorization failed: %s: %s", result.ErrorCode, result.ErrorDescription)})
	case q.Get("state") != s.state:
		s.send(authResult{err: errors.New("oauth state mismatch")})
	case q.Get("code") == "":
		s.send(authResult{err: errors.New("no authorization code returned")})
	default:
		s.send(authResult{code: q.Get("code")})
	}
}

func (s *AuthSession) send(r authResult) {
	select {
	case s.resultCh <- r:
	default:
	}
}

// Wait blocks until the browser redirect arrives or ctx is cancelled,
// exchanges the code for tokens, and discovers the account/org id
// needed for model calls.
func (s *AuthSession) Wait(ctx context.Context) (*oauth.Token, error) {
	return s.WaitWithProgress(ctx, nil)
}

// WaitWithProgress is Wait but calls progress with a short status
// string before each network step so a CLI caller can narrate what
// would otherwise be a silent wait.
func (s *AuthSession) WaitWithProgress(ctx context.Context, progress func(string)) (*oauth.Token, error) {
	defer s.close()
	report := func(msg string) {
		if progress != nil {
			progress(msg)
		}
	}

	select {
	case res := <-s.resultCh:
		if res.err != nil {
			return nil, res.err
		}
		report("Exchanging authorization code for tokens...")
		tok, err := exchangeCode(ctx, res.code, s.verifier, s.state, s.redirect)
		if err != nil {
			return nil, err
		}
		// The token-exchange response sometimes omits account email
		// and organization identity; the bootstrap endpoint fills
		// the gap. Called here on the login path only -- RefreshToken
		// deliberately does not call it, so a background refresh can
		// never silently re-key a stored credential.
		report("Resolving account and workspace...")
		enrichWithBootstrap(ctx, tok, progress)
		return tok, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// enrichWithBootstrap is the login-side counterpart to oh-my-pi's
// `anthropic-identity` after-exchange hook. Best-effort: an error
// here logs and continues rather than failing the login, because
// the user has just authorized and the worst case ("we have a
// valid token but no email") is still a working session.
func enrichWithBootstrap(ctx context.Context, tok *oauth.Token, progress func(string)) {
	if tok == nil || tok.AccessToken == "" {
		return
	}
	id, err := FetchIdentity(ctx, tok.AccessToken)
	if err != nil {
		if progress != nil {
			progress("Identity lookup failed; continuing without email/org metadata")
		}
		return
	}
	EnrichToken(tok, id)
}

// Close abandons the session, releasing the callback listener
// without waiting for a redirect. Safe to call after Wait has
// already returned.
func (s *AuthSession) Close() { s.close() }

func (s *AuthSession) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.server.Shutdown(ctx)
}

func randomURLSafe(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// identity is the nested account/organization object Anthropic's
// token endpoint includes in a successful response, used to scope the
// credential to the right account/org without a separate lookup call.
type identity struct {
	Account struct {
		UUID  string `json:"uuid"`
		Email string `json:"email_address"`
	} `json:"account"`
	Organization struct {
		UUID string `json:"uuid"`
		Name string `json:"name"`
	} `json:"organization"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
	identity
}

func exchangeCode(ctx context.Context, code, verifier, state, redirect string) (*oauth.Token, error) {
	body := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  redirect,
		"client_id":     clientID,
		"code_verifier": verifier,
		"state":         state,
	}
	return doTokenRequest(ctx, body, false)
}

// RefreshToken exchanges a refresh token for a new access token.
// The caller (config.ConfigStore) carries forward the previously
// discovered account/org ids; the token endpoint does not require
// them.
//
// Anthropic rotates refresh tokens, and a grant imported from the
// official Claude Code CLI is shared with it, so a successful refresh
// is written back to that CLI's credential file when it still holds
// the token being spent -- otherwise refreshing here would quietly log
// the user out of Claude Code. See syncBack.
func RefreshToken(ctx context.Context, refreshToken string) (*oauth.Token, error) {
	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     clientID,
		// The real client restates the scope set on every refresh; the
		// grant is re-issued against it rather than inherited.
		"scope": refreshScopes,
	}
	tok, err := doTokenRequest(ctx, body, true)
	if err != nil {
		return nil, err
	}
	syncBack(refreshToken, tok)
	return tok, nil
}

func doTokenRequest(ctx context.Context, body map[string]string, refresh bool) (*oauth.Token, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode token request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	// Claude Code sends these on refresh but not on the initial code
	// exchange.
	if refresh {
		req.Header.Set("anthropic-beta", anthropicBeta)
		req.Header.Set("User-Agent", "anthropic-sdk-typescript/atlas-agent userOAuthProvider")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &oauth.TokenExchangeError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	var tr tokenResponse
	if err := json.Unmarshal(respBody, &tr); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if tr.Error != "" {
		return nil, fmt.Errorf("token request failed: %s: %s", tr.Error, tr.ErrorDesc)
	}
	if tr.AccessToken == "" {
		return nil, errors.New("token response missing access_token")
	}

	tok := &oauth.Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresIn:    tr.ExpiresIn,
		AccountID:    tr.Account.UUID,
	}
	tok.SetExpiresAt()
	return tok, nil
}
