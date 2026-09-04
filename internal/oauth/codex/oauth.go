// Package codex implements the OAuth2 + PKCE login flow ChatGPT's Codex
// CLI uses to let a ChatGPT Plus/Pro/Business subscription authorize a
// local tool, so Atlas-Agent can use that subscription's flat-rate quota
// instead of a separate pay-per-token OpenAI API key.
//
// The redirect port (1455) and client id are not configurable: they are
// the values registered against auth.openai.com for this flow, the same
// ones the official Codex CLI uses. A local listener on that exact port is
// required because the redirect URI is part of the registered client.
package codex

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
	clientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	authorizeURL = "https://auth.openai.com/oauth/authorize"
	tokenURL     = "https://auth.openai.com/oauth/token"
	redirectPort = 1455
	redirectPath = "/auth/callback"
	scope        = "openid profile email offline_access"
)

var redirectURL = fmt.Sprintf("http://localhost:%d%s", redirectPort, redirectPath)

// AuthSession is one in-flight authorization attempt: a PKCE verifier/state
// pair plus the localhost listener waiting for the browser redirect.
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

// Start generates a fresh PKCE challenge, binds the fixed localhost
// callback port, and returns a session whose AuthURL should be opened in
// the browser. Call Wait afterwards to block for the redirect and exchange
// the resulting code for tokens.
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
		return nil, fmt.Errorf("bind localhost:%d for ChatGPT sign-in (is another login already running?): %w", redirectPort, err)
	}

	sess := &AuthSession{
		verifier: verifier,
		state:    state,
		resultCh: make(chan authResult, 1),
	}

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURL)
	q.Set("scope", scope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	q.Set("id_token_add_organizations", "true")
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
		Subject:          "ChatGPT",
		ErrorCode:        q.Get("error"),
		ErrorDescription: q.Get("error_description"),
	}

	// Render the page before settling the result, same reasoning as the
	// MCP OAuth receiver: settling first can close the connection out from
	// under an in-flight write.
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
		// A retried/duplicate redirect (reloaded tab); the first result
		// already won and is being consumed by Wait.
	}
}

// Wait blocks until the browser redirect arrives or ctx is cancelled,
// exchanges the authorization code for tokens, and shuts the local
// listener down either way.
func (s *AuthSession) Wait(ctx context.Context) (*oauth.Token, error) {
	defer s.close()

	select {
	case res := <-s.resultCh:
		if res.err != nil {
			return nil, res.err
		}
		return exchangeCode(ctx, res.code, s.verifier)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close abandons the session, releasing the callback listener without
// waiting for a redirect. Safe to call after Wait has already returned.
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
	IDToken      string `json:"id_token"`
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
	form.Set("code_verifier", verifier)
	return doTokenRequest(ctx, form)
}

// RefreshToken exchanges a refresh token for a new access token. Used both
// for the periodic background refresh and for retrying after a 401.
func RefreshToken(ctx context.Context, refreshToken string) (*oauth.Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
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

	accountID, planType := parseAccountClaims(tr.IDToken)

	tok := &oauth.Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresIn:    tr.ExpiresIn,
		AccountID:    accountID,
		PlanType:     planType,
	}
	tok.SetExpiresAt()
	return tok, nil
}

// parseAccountClaims decodes the ChatGPT account id and plan type out of
// the id_token's payload segment. Not signature-verified: the token was
// handed to us directly by auth.openai.com over TLS in the token exchange
// response, so there is no untrusted party in a position to forge it here.
func parseAccountClaims(idToken string) (accountID, planType string) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return "", ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ""
	}
	var claims struct {
		Auth struct {
			ChatGPTAccountID string `json:"chatgpt_account_id"`
			ChatGPTPlanType  string `json:"chatgpt_plan_type"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", ""
	}
	return claims.Auth.ChatGPTAccountID, claims.Auth.ChatGPTPlanType
}
