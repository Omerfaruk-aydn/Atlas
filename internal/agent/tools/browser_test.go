package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/browser"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/permission"
	"github.com/stretchr/testify/require"
)

// fakeBrowserSession is a browser.Session double so these tests exercise
// the tool's dispatch/validation/permission logic without launching a real
// browser.
type fakeBrowserSession struct {
	navigated  string
	clicked    string
	typedSel   string
	typedText  string
	pressedKey string
	evalScript string
	evalErr    error
	textSel    string
	htmlSel    string
	screenshot []byte
	url        string
}

func (f *fakeBrowserSession) Navigate(url string) error { f.navigated = url; return nil }
func (f *fakeBrowserSession) Click(selector string) error {
	f.clicked = selector
	return nil
}

func (f *fakeBrowserSession) Type(selector, text string) error {
	f.typedSel, f.typedText = selector, text
	return nil
}

func (f *fakeBrowserSession) PressKey(name string) error {
	if _, ok := browser.ResolveKey(name); !ok {
		return fmt.Errorf("unsupported key %q", name)
	}
	f.pressedKey = name
	return nil
}

func (f *fakeBrowserSession) Eval(script string) (string, error) {
	f.evalScript = script
	if f.evalErr != nil {
		return "", f.evalErr
	}
	return "42", nil
}

func (f *fakeBrowserSession) Text(selector string) (string, error) {
	f.textSel = selector
	return "hello world", nil
}

func (f *fakeBrowserSession) HTML(selector string) (string, error) {
	f.htmlSel = selector
	return "<p>hi</p>", nil
}

func (f *fakeBrowserSession) Screenshot(bool) ([]byte, error) {
	return []byte("png-bytes"), nil
}

func (f *fakeBrowserSession) URL() (string, error) { return f.url, nil }
func (f *fakeBrowserSession) Close()               {}

// fakeBrowserSessions is the browserSessions double: one session per ID,
// created lazily, tracking Close calls.
type fakeBrowserSessions struct {
	sessions  map[string]*fakeBrowserSession
	closed    []string
	launchErr error
}

func newFakeBrowserSessions() *fakeBrowserSessions {
	return &fakeBrowserSessions{sessions: map[string]*fakeBrowserSession{}}
}

func (f *fakeBrowserSessions) Session(id string) (browser.Session, error) {
	if f.launchErr != nil {
		return nil, f.launchErr
	}
	s, ok := f.sessions[id]
	if !ok {
		s = &fakeBrowserSession{}
		f.sessions[id] = s
	}
	return s, nil
}

func (f *fakeBrowserSessions) Close(id string) {
	f.closed = append(f.closed, id)
	delete(f.sessions, id)
}

func runBrowserTool(t *testing.T, sessions *fakeBrowserSessions, perms permission.Service, params BrowserParams) fantasy.ToolResponse {
	t.Helper()

	input, err := json.Marshal(params)
	require.NoError(t, err)

	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	resp, err := newBrowserTool(perms, t.TempDir(), sessions).Run(ctx, fantasy.ToolCall{
		ID:    "test-call",
		Name:  BrowserToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func TestBrowserToolNavigateRequiresURL(t *testing.T) {
	t.Parallel()
	resp := runBrowserTool(t, newFakeBrowserSessions(), &mockPermissionService{}, BrowserParams{Action: "navigate"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "url is required")
}

func TestBrowserToolNavigateRejectsNonHTTPScheme(t *testing.T) {
	t.Parallel()
	resp := runBrowserTool(t, newFakeBrowserSessions(), &mockPermissionService{}, BrowserParams{Action: "navigate", URL: "file:///etc/passwd"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "http")
}

func TestBrowserToolNavigateDrivesTheSession(t *testing.T) {
	t.Parallel()
	sessions := newFakeBrowserSessions()
	resp := runBrowserTool(t, sessions, &mockPermissionService{}, BrowserParams{Action: "navigate", URL: "https://example.com"})
	require.False(t, resp.IsError)
	require.Equal(t, "https://example.com", sessions.sessions["test-session"].navigated)
}

func TestBrowserToolClickRequiresSelector(t *testing.T) {
	t.Parallel()
	resp := runBrowserTool(t, newFakeBrowserSessions(), &mockPermissionService{}, BrowserParams{Action: "click"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "selector is required")
}

func TestBrowserToolTypeReplacesFieldContents(t *testing.T) {
	t.Parallel()
	sessions := newFakeBrowserSessions()
	resp := runBrowserTool(t, sessions, &mockPermissionService{}, BrowserParams{Action: "type", Selector: "#q", Text: "hello"})
	require.False(t, resp.IsError)
	require.Equal(t, "#q", sessions.sessions["test-session"].typedSel)
	require.Equal(t, "hello", sessions.sessions["test-session"].typedText)
}

func TestBrowserToolKeyRejectsUnknownKeyName(t *testing.T) {
	t.Parallel()
	resp := runBrowserTool(t, newFakeBrowserSessions(), &mockPermissionService{}, BrowserParams{Action: "key", Key: "super-delete"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "key press failed")
}

func TestBrowserToolKeyAcceptsKnownKeyName(t *testing.T) {
	t.Parallel()
	sessions := newFakeBrowserSessions()
	resp := runBrowserTool(t, sessions, &mockPermissionService{}, BrowserParams{Action: "key", Key: "Enter"})
	require.False(t, resp.IsError)
	require.Equal(t, "enter", sessions.sessions["test-session"].pressedKey)
}

func TestBrowserToolEvalReturnsTheScriptResult(t *testing.T) {
	t.Parallel()
	resp := runBrowserTool(t, newFakeBrowserSessions(), &mockPermissionService{}, BrowserParams{Action: "eval", Script: "1+41"})
	require.False(t, resp.IsError)
	require.Equal(t, "42", resp.Content)
}

func TestBrowserToolTextReturnsElementText(t *testing.T) {
	t.Parallel()
	resp := runBrowserTool(t, newFakeBrowserSessions(), &mockPermissionService{}, BrowserParams{Action: "text", Selector: "body"})
	require.False(t, resp.IsError)
	require.Equal(t, "hello world", resp.Content)
}

func TestBrowserToolScreenshotReturnsAnImageResponse(t *testing.T) {
	t.Parallel()
	resp := runBrowserTool(t, newFakeBrowserSessions(), &mockPermissionService{}, BrowserParams{Action: "screenshot"})
	require.False(t, resp.IsError)
	require.Equal(t, "image", resp.Type)
	require.Equal(t, "image/png", resp.MediaType)
	require.Equal(t, []byte("png-bytes"), resp.Data)
}

func TestBrowserToolCloseClosesTheSessionWithoutLaunchingOne(t *testing.T) {
	t.Parallel()
	sessions := newFakeBrowserSessions()
	sessions.sessions["test-session"] = &fakeBrowserSession{}

	resp := runBrowserTool(t, sessions, &mockPermissionService{}, BrowserParams{Action: "close"})
	require.False(t, resp.IsError)
	require.Contains(t, sessions.closed, "test-session")
}

func TestBrowserToolRejectsUnknownAction(t *testing.T) {
	t.Parallel()
	resp := runBrowserTool(t, newFakeBrowserSessions(), &mockPermissionService{}, BrowserParams{Action: "teleport"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "unknown action")
}

func TestBrowserToolReportsLaunchFailureAsAToolError(t *testing.T) {
	t.Parallel()
	sessions := newFakeBrowserSessions()
	sessions.launchErr = fmt.Errorf("no chrome binary found")

	resp := runBrowserTool(t, sessions, &mockPermissionService{}, BrowserParams{Action: "navigate", URL: "https://example.com"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "failed to start browser")
}

// denyingPermissionService always denies, to verify the tool never touches
// the browser manager when permission is refused.
type denyingPermissionService struct {
	mockPermissionService
}

func (d *denyingPermissionService) Request(ctx context.Context, req permission.CreatePermissionRequest) (bool, error) {
	return false, nil
}

func TestBrowserToolNeverLaunchesASessionWhenPermissionIsDenied(t *testing.T) {
	t.Parallel()
	sessions := newFakeBrowserSessions()

	resp := runBrowserTool(t, sessions, &denyingPermissionService{}, BrowserParams{Action: "navigate", URL: "https://example.com"})
	require.True(t, resp.IsError)
	require.Empty(t, sessions.sessions, "a denied call must not create a browser session")
}
