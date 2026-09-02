package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/permission"
	"github.com/stretchr/testify/require"
)

// noRequestPermissions wraps a real permission.Service and fails the test if
// Request is ever called. It exists to prove that a URL a policy blocks is
// refused before the tool asks the user for anything -- a blocked domain
// should not even reach the approval dialog.
type noRequestPermissions struct {
	permission.Service
	t *testing.T
}

func (n *noRequestPermissions) Request(ctx context.Context, opts permission.CreatePermissionRequest) (bool, error) {
	n.t.Helper()
	n.t.Fatal("permission was requested for a URL the policy should have blocked before this point")
	return false, nil
}

// noDialTransport fails any request that reaches it, so a test using it
// proves the tool never made a network call.
type noDialTransport struct{ t *testing.T }

func (n noDialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	n.t.Helper()
	n.t.Fatalf("unexpected network call to %s", req.URL)
	return nil, nil
}

func TestFetchToolRefusesABlockedDomainBeforeAskingOrCalling(t *testing.T) {
	t.Parallel()

	permissions := &noRequestPermissions{
		Service: permission.NewPermissionService(t.TempDir(), false, nil),
		t:       t,
	}
	client := &http.Client{Transport: noDialTransport{t}}
	tool := NewFetchTool(permissions, t.TempDir(), client, URLPolicy{Deny: []string{"blocked.example.com"}})

	input, err := json.Marshal(FetchParams{URL: "https://blocked.example.com/page", Format: "text"})
	require.NoError(t, err)

	res, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "c", Name: FetchToolName, Input: string(input)})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "blocked domain list")
}

func TestDownloadToolRefusesABlockedDomainBeforeAskingOrCalling(t *testing.T) {
	t.Parallel()

	permissions := &noRequestPermissions{
		Service: permission.NewPermissionService(t.TempDir(), false, nil),
		t:       t,
	}
	client := &http.Client{Transport: noDialTransport{t}}
	dir := t.TempDir()
	tool := NewDownloadTool(permissions, dir, client, URLPolicy{Deny: []string{"blocked.example.com"}}, PathPolicy{})

	input, err := json.Marshal(DownloadParams{URL: "https://blocked.example.com/file.zip", FilePath: "out.zip"})
	require.NoError(t, err)

	res, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "c", Name: DownloadToolName, Input: string(input)})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "blocked domain list")
}

func TestWebFetchToolRefusesADomainNotOnTheAllowList(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: noDialTransport{t}}
	tool := NewWebFetchTool(t.TempDir(), client, URLPolicy{Allow: []string{"docs.example.com"}})

	input, err := json.Marshal(WebFetchParams{URL: "https://elsewhere.example.com/page"})
	require.NoError(t, err)

	res, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "c", Name: WebFetchToolName, Input: string(input)})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "not on the allowed domain list")
}
