// Package claude implements the OAuth2 + PKCE login flow the official
// Claude Code CLI uses, so Atlas-Agent can use a Claude Pro/Max/Team
// subscription's flat-rate quota instead of a separate pay-per-token
// Anthropic API key.
//
// None of this is officially documented by Anthropic. The client id,
// endpoints, scopes, and request shapes below were confirmed against
// the real authorize/token endpoints (two earlier guesses were
// rejected outright: an OIDC-shaped scope list with "Unknown scope:
// openid", then a wrong redirect_uri with "Authorization failed:
// Invalid request format") and cross-checked against Claude Code's own
// auth configuration as reverse-engineered by other open-source
// projects. It may still drift if Anthropic changes the client
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
	// authorizeURL is the OAuth2 authorization endpoint.
	authorizeURL = "https://claude.ai/oauth/authorize"
	// tokenURL is the OAuth2 token-exchange endpoint. Claude Code's own
	// client exchanges the code against api.anthropic.com, not
	// claude.ai or console.anthropic.com.
	tokenURL = "https://api.anthropic.com/v1/oauth/token"
	// redirectPort/redirectPath are the loopback callback address
	// registered for clientID. Claude Code listens on this exact
	// port/path; a different one is why an earlier attempt at this
	// login failed with "Invalid request format" (a fixed
	// console.anthropic.com redirect_uri isn't what's registered).
	redirectPort = 54545
	redirectPath = "/callback"
	// scopes is the space-separated list of OAuth scopes Claude Code's
	// own client requests. Two earlier, smaller guesses were rejected
	// or incomplete; this is the full set Claude Code sends.
	scopes = "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	// anthropicBeta is the beta header Claude Code sends on refresh
	// (not on the initial code exchange) so the OAuth-token grant is
	// recognized by api.anthropic.com.
	anthropicBeta = "oauth-2025-04-20"
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
	// Claude Code's own authorize request carries this fixed
	// "code=true" param; its meaning isn't documented but omitting it
	// is not known to be safe, so it's reproduced as observed.
	q.Set("code", "true")
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
		return exchangeCode(ctx, res.code, s.verifier, s.state)
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

func exchangeCode(ctx context.Context, code, verifier, state string) (*oauth.Token, error) {
	body := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  redirectURL,
		"client_id":     clientID,
		"code_verifier": verifier,
		"state":         state,
	}
	return doTokenRequest(ctx, body, false)
}

// RefreshToken exchanges a refresh token for a new access token.
// The caller (config.ConfigStore) carries forward the previously
// discovered account/org ids; claude.ai does not require them for
// the token endpoint itself.
func RefreshToken(ctx context.Context, refreshToken string) (*oauth.Token, error) {
	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     clientID,
	}
	return doTokenRequest(ctx, body, true)
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
