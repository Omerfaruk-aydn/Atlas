// Package jetbrains implements the JWT-exchange login flow JetBrains'
// AI Assistant exposes for its Pro/Ultimate subscription, so
// Atlas-Agent can use that subscription's flat-rate quota instead of
// a separate Anthropic/OpenAI API key.
//
// JetBrains AI is unusual among the coding-plan integrations: the
// real backend is an internal gateway at api.jetbrains.ai that
// accepts a signed JWT obtained by exchanging a long-lived
// "JB-ACCESS-TOKEN" cookie set by account.jetbrains.com after a
// browser sign-in. The flow is therefore not a vanilla OAuth2 PKCE
// dance, but the shape from the user's perspective is the same: open
// a browser, sign in, paste the resulting token back into the CLI.
//
// This package SCAFFOLDS that exchange. To finish wiring it up:
//  1. Capture the actual account.jetbrains.com endpoints and JWT
//     signing parameters from a logged-in JetBrains AI session
//     (browser DevTools network tab on a fresh sign-in).
//  2. Replace exchangeURL and the JWT-issuing endpoint with the real
//     values.
//  3. Implement the model call layer in
//     internal/deps/atlas-llm/providers/jetbrains.
package jetbrains

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth"
)

// exchangeURL is a placeholder; replace with the real value once
// known. The token JWT-issuing endpoint is typically something like
// https://account.jetbrains.com/api/jba/jwt or similar; the call
// pattern is POST a refresh-token, receive a Bearer JWT.
const exchangeURL = "https://account.jetbrains.com/REPLACE_WITH_REAL_ENDPOINT"

// AuthSession is one in-flight authorization attempt. Unlike the
// Antigravity/Claude/Grok scaffolds, JetBrains' flow is initiated by
// the user pasting a "JB-ACCESS-TOKEN" cookie value (captured from
// their browser session) rather than driving a redirect, so the
// session is a thin wrapper around the exchange call.
type AuthSession struct {
	jbToken string
}

// Start begins a JetBrains AI sign-in. The user must first sign in
// at account.jetbrains.com in their browser, then copy the value of
// the "JB-ACCESS-TOKEN" cookie into the prompt.
func Start(ctx context.Context) (*AuthSession, error) {
	fmt.Println()
	fmt.Println("To sign in to JetBrains AI:")
	fmt.Println("  1. Open https://account.jetbrains.com in your browser")
	fmt.Println("  2. Sign in with your JetBrains account (Pro/Ultimate)")
	fmt.Println("  3. Open DevTools -> Application -> Cookies")
	fmt.Println("  4. Copy the value of the 'JB-ACCESS-TOKEN' cookie")
	fmt.Println()
	fmt.Print("Paste JB-ACCESS-TOKEN here: ")
	var tok string
	if _, err := fmt.Scanln(&tok); err != nil {
		return nil, fmt.Errorf("read JB-ACCESS-TOKEN: %w", err)
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return nil, errors.New("JB-ACCESS-TOKEN is required")
	}
	return &AuthSession{jbToken: tok}, nil
}

// Wait exchanges the JB-ACCESS-TOKEN for a Bearer JWT usable against
// api.jetbrains.ai and returns it as an oauth.Token.
func (s *AuthSession) Wait(ctx context.Context) (*oauth.Token, error) {
	return s.WaitWithProgress(ctx, nil)
}

// WaitWithProgress is Wait with a progress callback for the CLI.
func (s *AuthSession) WaitWithProgress(ctx context.Context, progress func(string)) (*oauth.Token, error) {
	report := func(msg string) {
		if progress != nil {
			progress(msg)
		}
	}

	report("Exchanging JB-ACCESS-TOKEN for a Bearer JWT...")
	tok, err := exchangeForJWT(ctx, s.jbToken)
	if err != nil {
		return nil, err
	}
	return tok, nil
}

// Close is a no-op for JetBrains; there is no listener to release.
func (s *AuthSession) Close() {}

type jwtResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

func exchangeForJWT(ctx context.Context, jbToken string) (*oauth.Token, error) {
	form := strings.NewReader("jb-access-token=" + jbToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, exchangeURL, form)
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

	var jr jwtResponse
	if err := json.Unmarshal(body, &jr); err != nil {
		return nil, fmt.Errorf("decode jwt response: %w", err)
	}
	if jr.Error != "" {
		return nil, fmt.Errorf("jwt request failed: %s: %s", jr.Error, jr.ErrorDesc)
	}
	if jr.AccessToken == "" {
		return nil, errors.New("jwt response missing access_token")
	}

	tok := &oauth.Token{
		AccessToken: jr.AccessToken,
		ExpiresIn:   jr.ExpiresIn,
	}
	tok.SetExpiresAt()
	return tok, nil
}
