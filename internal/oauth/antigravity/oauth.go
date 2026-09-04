// Package antigravity implements the OAuth2 + PKCE login flow Google's
// Antigravity IDE uses, plus the Cloud Code "loadCodeAssist"/"onboardUser"
// project-discovery step every request against its backend needs, so
// Atlas-Agent can authenticate against a Google AI Pro/Ultra (Antigravity)
// plan the same way the official client does.
//
// The client id/secret below are not a secret Atlas-Agent is disclosing:
// they are the public "installed application" OAuth client Antigravity
// itself ships (the same pattern gcloud's own OAuth client uses), captured
// from the community reverse-engineering documented at
// https://github.com/NoeFabris/opencode-antigravity-auth. Google's
// Antigravity Terms of Service restrict the service to first-party
// clients and has revoked third-party access before (see that project's
// issue tracker); using this login is done at the user's own risk to
// their Google account standing, not Atlas-Agent's.
//
// Only login and project discovery live here. The resulting token is used
// for model calls by internal/deps/atlas-llm/providers/antigravity, which
// speaks Google's internal v1internal:generateContent envelope (distinct
// from both the public Gemini API and Vertex AI) for the Gemini-family
// models an Antigravity account exposes. Claude and GPT-OSS models also
// served through Antigravity go through additional per-family request and
// response translation on Google's side that is not reproduced here, so
// they are not usable through this login.
package antigravity

import (
	"bytes"
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
	"runtime"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth/callback"
)

const (
	clientID     = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	clientSecret = "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"

	authorizeURL = "https://accounts.google.com/o/oauth2/auth"
	tokenURL     = "https://oauth2.googleapis.com/token"

	// codeAssistEndpoint is the Cloud Code backend used for both the
	// project-discovery calls here and (eventually) model inference.
	codeAssistEndpoint = "https://cloudcode-pa.googleapis.com"

	redirectPort = 36742
	redirectPath = "/oauth-callback"

	scopes = "https://www.googleapis.com/auth/cloud-platform" +
		" https://www.googleapis.com/auth/userinfo.email" +
		" https://www.googleapis.com/auth/userinfo.profile" +
		" https://www.googleapis.com/auth/cclog" +
		" https://www.googleapis.com/auth/experimentsandconfigs"
)

var redirectURL = fmt.Sprintf("http://localhost:%d%s", redirectPort, redirectPath)

// clientMetadata identifies this login the way Antigravity's own client
// does, required by loadCodeAssist/onboardUser.
type clientMetadata struct {
	IdeType    string `json:"ideType"`
	Platform   string `json:"platform"`
	PluginType string `json:"pluginType"`
}

func metadata() clientMetadata {
	// Google's ClientMetadata.Platform enum rejects OS-specific values
	// like "WINDOWS" or "LINUX" here (confirmed against the real
	// loadCodeAssist endpoint, which 400s on them) -- the only value that
	// works is PLATFORM_UNSPECIFIED, the same one the pi-ai runtime uses.
	return clientMetadata{IdeType: "ANTIGRAVITY", Platform: "PLATFORM_UNSPECIFIED", PluginType: "GEMINI"}
}

// RequestHeaders returns the headers Antigravity's Cloud Code backend
// expects identifying the calling client on every model request: a
// Client-Metadata JSON blob and a matching User-Agent. Used by
// config.ProviderConfig.SetupAntigravity to configure the provider's
// ExtraHeaders once a login is stored.
func RequestHeaders() map[string]string {
	meta, err := json.Marshal(metadata())
	if err != nil {
		// metadata() is a fixed literal struct; marshaling it cannot fail.
		panic(err)
	}
	return map[string]string{
		"Client-Metadata": string(meta),
		"User-Agent":      "antigravity/1.15.8 " + runtime.GOOS + "/" + runtime.GOARCH,
	}
}

// AuthSession is one in-flight authorization attempt.
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
// the browser. Call Wait afterwards to block for the redirect, exchange
// the code, and discover (or provision) a Cloud project to use.
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
		return nil, fmt.Errorf("bind localhost:%d for Antigravity sign-in (is another login already running?): %w", redirectPort, err)
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
		Subject:          "Antigravity",
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
// exchanges the code for tokens, discovers (or provisions) a Cloud
// project for the account, and shuts the local listener down either way.
func (s *AuthSession) Wait(ctx context.Context) (*oauth.Token, error) {
	defer s.close()

	select {
	case res := <-s.resultCh:
		if res.err != nil {
			return nil, res.err
		}
		tok, err := exchangeCode(ctx, res.code, s.verifier)
		if err != nil {
			return nil, err
		}
		project, tier, err := discoverProject(ctx, tok.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("signed in, but could not resolve a Cloud project for this account: %w", err)
		}
		tok.AccountID = project
		tok.PlanType = tier
		return tok, nil
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
	form.Set("client_secret", clientSecret)
	form.Set("code_verifier", verifier)
	return doTokenRequest(ctx, form)
}

// RefreshToken exchanges a refresh token for a new access token. The
// discovered project id and tier are not re-resolved here (the caller
// carries them forward from the previous token); see
// [config.ConfigStore]'s refresh path.
func RefreshToken(ctx context.Context, refreshToken string) (*oauth.Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
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

type loadCodeAssistRequest struct {
	Metadata             clientMetadata `json:"metadata"`
	CloudaicompanionProj string         `json:"cloudaicompanionProject,omitempty"`
}

type tierInfo struct {
	ID string `json:"id"`
}

type loadCodeAssistResponse struct {
	CloudaicompanionProject string     `json:"cloudaicompanionProject"`
	CurrentTier             *tierInfo  `json:"currentTier"`
	AllowedTiers            []tierInfo `json:"allowedTiers"`
}

type onboardUserRequest struct {
	TierID               string         `json:"tierId"`
	Metadata             clientMetadata `json:"metadata"`
	CloudaicompanionProj string         `json:"cloudaicompanionProject,omitempty"`
}

type onboardUserResponse struct {
	Done     bool `json:"done"`
	Response struct {
		CloudaicompanionProject struct {
			ID string `json:"id"`
		} `json:"cloudaicompanionProject"`
	} `json:"response"`
}

// discoverProject resolves the Cloud Code project id backing this
// account's Antigravity access, provisioning one via onboardUser when the
// account does not have one yet (a fresh sign-up). Returns the project id
// and the account's tier ("free" or "paid").
func discoverProject(ctx context.Context, accessToken string) (project, tier string, err error) {
	load, err := postCodeAssist[loadCodeAssistResponse](ctx, accessToken, "loadCodeAssist", loadCodeAssistRequest{
		Metadata: metadata(),
	})
	if err != nil {
		return "", "", fmt.Errorf("loadCodeAssist: %w", err)
	}

	tierID := "free"
	if load.CurrentTier != nil && load.CurrentTier.ID != "" {
		tierID = load.CurrentTier.ID
	} else if len(load.AllowedTiers) > 0 {
		tierID = load.AllowedTiers[0].ID
	}

	if load.CloudaicompanionProject != "" {
		return load.CloudaicompanionProject, tierID, nil
	}

	// No project yet: this account needs onboarding, which provisions one
	// asynchronously. Poll until it reports done with a project id.
	const (
		maxAttempts = 10
		pollDelay   = 5 * time.Second
	)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		onboard, err := postCodeAssist[onboardUserResponse](ctx, accessToken, "onboardUser", onboardUserRequest{
			TierID:   tierID,
			Metadata: metadata(),
		})
		if err != nil {
			return "", "", fmt.Errorf("onboardUser: %w", err)
		}
		if onboard.Done && onboard.Response.CloudaicompanionProject.ID != "" {
			return onboard.Response.CloudaicompanionProject.ID, tierID, nil
		}
		select {
		case <-time.After(pollDelay):
		case <-ctx.Done():
			return "", "", ctx.Err()
		}
	}
	return "", "", errors.New("timed out waiting for Antigravity project provisioning")
}

func postCodeAssist[T any](ctx context.Context, accessToken, method string, body any) (*T, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		codeAssistEndpoint+"/v1internal:"+method, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("%s failed: %s: %s", method, resp.Status, string(respBody))
	}

	var out T
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", method, err)
	}
	return &out, nil
}
