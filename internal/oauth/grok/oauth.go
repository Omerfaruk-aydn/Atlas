// Package grok implements the OAuth2 + PKCE login flow grok.com's web
// console uses to authorize a local tool, so Atlas-Agent can use a
// xAI SuperGrok/SuperGrok Heavy subscription's flat-rate quota
// instead of a separate pay-per-token XAI_API_KEY.
//
// This is a SCAFFOLD, not a finished integration. xAI does not publish
// a public OAuth flow for grok.com subscriptions the way Google does
// for Antigravity. The client id, authorize/token endpoints, and the
// request envelope the grok.com backend expects are not part of any
// official documentation and have to be observed against the real
// client. The shape of this package is the same PKCE flow Antigravity
// ships so swapping in real values is a small change once they are
// known.
//
// Risk note: xAI's terms of service restrict the service to
// first-party clients. Using this login is at the user's own risk to
// their xAI account standing, not Atlas-Agent's.
//
// To finish wiring this up:
//  1. Capture the OAuth client id from a logged-in grok.com session
//     (browser DevTools network tab on a fresh sign-in).
//  2. Replace authorizeURL / tokenURL with the real endpoints.
//  3. Implement the account-id discovery in Wait (similar to
//     Antigravity's discoverProject).
//  4. Implement the model call layer in
//     internal/deps/atlas-llm/providers/grokweb.
package grok

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
	// clientID is a placeholder; replace with the real value captured
	// from a logged-in grok.com session.
	clientID     = "REPLACE_WITH_REAL_GROK_OAUTH_CLIENT_ID"
	clientSecret = ""
	authorizeURL = "https://grok.com/oauth/authorize"
	tokenURL     = "https://grok.com/oauth/token"
	redirectPort = 41538
	redirectPath = "/oauth-callback"
	scopes       = "openid profile email offline_access"
)

var redirectURL = fmt.Sprintf("http://localhost:%d%s", redirectPort, redirectPath)

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
		return nil, fmt.Errorf("bind localhost:%d for Grok sign-in (is another login already running?): %w", redirectPort, err)
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

func (s *AuthSession) AuthURL() string { return s.url }

func (s *AuthSession) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	result := callback.Result{
		Subject:          "Grok",
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

func (s *AuthSession) Wait(ctx context.Context) (*oauth.Token, error) {
	return s.WaitWithProgress(ctx, nil)
}

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
		// TODO(grok-oauth): discover the account id and set
		// tok.AccountID / tok.PlanType here.
		return tok, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

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
