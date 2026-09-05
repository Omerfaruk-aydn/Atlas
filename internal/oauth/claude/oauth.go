// Package claude implements the OAuth2 + PKCE login flow claude.ai's
// web console uses to authorize a local tool, so Atlas-Agent can use a
// Claude Pro/Max/Team/Enterprise subscription's flat-rate quota
// instead of a separate pay-per-token Anthropic API key.
//
// This is a SCAFFOLD, not a finished integration. Anthropic does not
// publish a public OAuth flow for claude.ai subscriptions the way
// Google does for Antigravity or OpenAI does for ChatGPT; the client
// id, authorize/token endpoints, and the request envelope the
// claude.ai console backend expects are not part of any official
// documentation and have to be observed against the real client. The
// shape of this package is the same PKCE flow Antigravity ships
// (see internal/oauth/antigravity) so swapping in real values is a
// small change once they are known.
//
// Risk note: claude.ai's terms of service restrict the service to
// first-party clients in the same way Antigravity's do, and Anthropic
// has revoked third-party access before. Using this login is at the
// user's own risk to their Anthropic account standing, not
// Atlas-Agent's.
//
// To finish wiring this up, a developer with a working claude.ai
// session needs to:
//  1. Capture the OAuth client id and (if any) secret the claude.ai
//     web app uses. The values that ship in this file are placeholders.
//  2. Replace authorizeURL / tokenURL with the real endpoints.
//  3. Implement the project/account-id discovery in Wait (the
//     Antigravity equivalent is discoverProject; claude.ai will have
//     a similar step that resolves an "accountId"/"orgId" pair).
//  4. Implement the model call layer in
//     internal/deps/atlas-llm/providers/claude: the request/response
//     envelope claude.ai's console backend speaks.
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

// TODO(claude-oauth): replace these with the real values captured
// from a logged-in claude.ai web session. They are placeholders that
// will make Start() fail at the authorize step; the rest of the
// package is built to be the drop-in replacement once the real
// values are known.
const (
	// clientID is the OAuth client id the official Claude Code CLI
	// registers itself as. This value is not published in any official
	// Anthropic documentation; it is the client id widely observed and
	// reused across open-source reimplementations of Claude Code's own
	// login flow. It may still be wrong or may be rotated by Anthropic
	// without notice.
	clientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	// clientSecret is the (optional) OAuth client secret. Most public
	// PKCE flows for installed applications do not require a secret;
	// the Antigravity/ChatGPT integrations both ship without one. Set
	// to "" if claude.ai's flow is the same.
	clientSecret = ""
	// authorizeURL is the OAuth2 authorization endpoint.
	authorizeURL = "https://claude.ai/oauth/authorize"
	// tokenURL is the OAuth2 token-exchange endpoint. Claude Code's own
	// client exchanges the code against console.anthropic.com, not
	// claude.ai itself.
	tokenURL = "https://console.anthropic.com/v1/oauth/token"
	// redirectPort is the localhost port the registered OAuth
	// client expects the redirect on. Antigravity uses 36742;
	// whatever claude.ai's web app actually uses, that exact value
	// must go here because the redirect URI is part of the registered
	// client.
	redirectPort = 41537
	redirectPath = "/oauth-callback"
	// scopes is the space-separated list of OAuth scopes Claude Code's
	// own client requests. "openid profile email offline_access" is an
	// OIDC-shaped guess that claude.ai's authorize endpoint rejects
	// outright ("Unknown scope: openid"); these three are the scopes
	// observed on Claude Code's real authorize request.
	scopes = "org:create_api_key user:profile user:inference"
)

var redirectURL = fmt.Sprintf("http://localhost:%d%s", redirectPort, redirectPath)

// AuthSession is one in-flight authorization attempt: a PKCE
// verifier/state pair plus the localhost listener waiting for the
// browser redirect. Same shape as Antigravity's AuthSession so the
// CLI login flow does not need to know which provider it is driving.
type AuthSession struct {
	verifier string
	state    string
	url      string

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
	state, err := randomURLSafe(16)
	if err != nil {
		return nil, fmt.Errorf("generate oauth state: %w", err)
	}

	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	lc := &net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", fmt.Sprintf("localhost:%d", redirectPort))
	if err != nil {
		return nil, fmt.Errorf("bind localhost:%d for Claude sign-in (is another login already running?): %w", redirectPort, err)
	}

	sess := &AuthSession{
		verifier: verifier,
		state:    state,
		resultCh: make(chan authResult, 1),
	}

	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", redirectURL)
	q.Set("scope", scopes)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	sess.url = authorizeURL + "?" + q.Encode()

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
// exchanges the code for tokens, and (TODO) discovers the
// account/organization id needed for model calls.
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
		tok, err := exchangeCode(ctx, res.code, s.verifier)
		if err != nil {
			return nil, err
		}
		// TODO(claude-oauth): once the claude.ai account/org
		// discovery step is known, set tok.AccountID and
		// tok.PlanType here, the way Antigravity does from its
		// loadCodeAssist response.
		return tok, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
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

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func exchangeCode(ctx context.Context, code, verifier string) (*oauth.Token, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURL)
	form.Set("client_id", clientID)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	form.Set("code_verifier", verifier)
	return doTokenRequest(ctx, form)
}

// RefreshToken exchanges a refresh token for a new access token.
// The caller (config.ConfigStore) carries forward the previously
// discovered account/org ids; claude.ai does not require them for
// the token endpoint itself.
func RefreshToken(ctx context.Context, refreshToken string) (*oauth.Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	return doTokenRequest(ctx, form)
}

func doTokenRequest(ctx context.Context, form url.Values) (*oauth.Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &oauth.TokenExchangeError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
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
	}
	tok.SetExpiresAt()
	return tok, nil
}
