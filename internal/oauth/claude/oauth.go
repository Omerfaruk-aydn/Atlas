// Package claude implements the OAuth2 + PKCE login flow claude.ai's
// web console uses to authorize a local tool, so Atlas-Agent can use a
// Claude Pro/Max/Team/Enterprise subscription's flat-rate quota
// instead of a separate pay-per-token Anthropic API key.
//
// This is a SCAFFOLD, not a finished integration. Anthropic does not
// publish a public OAuth flow for claude.ai subscriptions the way
// Google does for Antigravity or OpenAI does for ChatGPT; the client
// id, endpoints, and scopes below are not part of any official
// documentation. They were confirmed against the real authorize
// endpoint during development (an earlier guess at the scope value
// was rejected outright with "Unknown scope: openid"; the values here
// get past that and reach the user's actual account) but the request
// envelope the token endpoint expects on the model-call side is still
// unconfirmed.
//
// Unlike Antigravity/ChatGPT, claude.ai's registered client does not
// accept a loopback (localhost) redirect_uri -- authorizing with one
// fails with "Authorization failed: Invalid request format". Its
// redirect_uri is a fixed console.anthropic.com page that displays a
// "{code}#{state}" string for the user to copy back into the CLI by
// hand, so this package has no local callback listener; Exchange
// takes that pasted string directly.
//
// Risk note: claude.ai's terms of service restrict the service to
// first-party clients in the same way Antigravity's do, and Anthropic
// has revoked third-party access before. Using this login is at the
// user's own risk to their Anthropic account standing, not
// Atlas-Agent's.
//
// To finish wiring this up, a developer with a working claude.ai
// session needs to:
//  1. Confirm the account/organization id discovery step (the
//     Antigravity equivalent is discoverProject); claude.ai may need a
//     similar step that resolves an "accountId"/"orgId" pair before
//     model calls will work.
//  2. Implement the model call layer in
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
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth"
)

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
	// redirectURL is fixed: claude.ai's registered client for this
	// client_id does not accept an arbitrary loopback redirect_uri the
	// way Antigravity/ChatGPT's clients do. This console.anthropic.com
	// page shows a "{code}#{state}" string for the user to paste back.
	redirectURL = "https://console.anthropic.com/oauth/code/callback"
	// scopes is the space-separated list of OAuth scopes Claude Code's
	// own client requests. "openid profile email offline_access" is an
	// OIDC-shaped guess that claude.ai's authorize endpoint rejects
	// outright ("Unknown scope: openid"); these three are the scopes
	// observed on Claude Code's real authorize request.
	scopes = "org:create_api_key user:profile user:inference"
)

// AuthSession is one in-flight authorization attempt: a PKCE
// verifier/state pair and the authorize URL to open in the browser.
// There is no local callback listener (see the package doc) -- call
// Exchange with the code the user pastes back from the browser.
type AuthSession struct {
	verifier string
	state    string
	url      string
}

// Start generates a fresh PKCE challenge and returns a session whose
// AuthURL should be opened in the browser. The browser page shows a
// "{code}#{state}" string once the user approves access; pass that
// string to Exchange to complete the login.
func Start(_ context.Context) (*AuthSession, error) {
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

	sess := &AuthSession{
		verifier: verifier,
		state:    state,
	}

	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", redirectURL)
	q.Set("scope", scopes)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	sess.url = authorizeURL + "?" + q.Encode()

	return sess, nil
}

// AuthURL returns the URL to open in the user's browser.
func (s *AuthSession) AuthURL() string { return s.url }

// Exchange takes the string the user copied from the browser page
// after approving access -- either a bare authorization code or a
// "{code}#{state}" pair -- and exchanges it for tokens. When a state
// half is present it is verified against the one generated by Start.
func (s *AuthSession) Exchange(ctx context.Context, pasted string) (*oauth.Token, error) {
	pasted = strings.TrimSpace(pasted)
	if pasted == "" {
		return nil, errors.New("no authorization code provided")
	}

	code := pasted
	if idx := strings.IndexByte(pasted, '#'); idx != -1 {
		code = pasted[:idx]
		if returnedState := pasted[idx+1:]; returnedState != "" && returnedState != s.state {
			return nil, errors.New("oauth state mismatch")
		}
	}
	if code == "" {
		return nil, errors.New("no authorization code provided")
	}

	tok, err := exchangeCode(ctx, code, s.verifier)
	if err != nil {
		return nil, err
	}
	// TODO(claude-oauth): once the claude.ai account/org discovery
	// step is known, set tok.AccountID and tok.PlanType here, the way
	// Antigravity does from its loadCodeAssist response.
	return tok, nil
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
