package tools

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/permission"
	"github.com/stretchr/testify/require"
)

func TestCopyLimitedWithNoLimitCopiesEverything(t *testing.T) {
	var out bytes.Buffer
	n, err := copyLimited(&out, strings.NewReader(strings.Repeat("x", 5000)), 0)
	require.NoError(t, err)
	require.Equal(t, int64(5000), n)
	require.Equal(t, 5000, out.Len())
}

// A body of exactly the limit is within it; one byte more is not. Reading a
// byte past the cap is what keeps those two distinguishable.
func TestCopyLimitedAtAndOverTheBoundary(t *testing.T) {
	var atLimit bytes.Buffer
	n, err := copyLimited(&atLimit, strings.NewReader(strings.Repeat("x", 100)), 100)
	require.NoError(t, err)
	require.Equal(t, int64(100), n)

	var over bytes.Buffer
	_, err = copyLimited(&over, strings.NewReader(strings.Repeat("x", 101)), 100)
	require.ErrorIs(t, err, ErrDownloadTooLarge)
}

func TestDeclaredTooLarge(t *testing.T) {
	require.True(t, declaredTooLarge(200, 100))
	require.False(t, declaredTooLarge(100, 100))
	require.False(t, declaredTooLarge(1<<40, 0), "no cap configured")
	require.False(t, declaredTooLarge(-1, 100), "server did not say")
}

func downloadToolFor(t *testing.T, workingDir string, maxBytes int64) fantasy.AgentTool {
	t.Helper()
	return NewDownloadTool(
		permission.NewPermissionService(workingDir, true, nil),
		workingDir, nil, URLPolicy{}, PathPolicy{}, maxBytes,
	)
}

func runDownload(t *testing.T, tool fantasy.AgentTool, url, target string) fantasy.ToolResponse {
	t.Helper()
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "s1")
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "c1",
		Input: `{"url":"` + url + `","file_path":"` + filepath.ToSlash(target) + `"}`,
	})
	require.NoError(t, err)
	return resp
}

// A server that declares an oversized body is refused before anything is
// written at all.
func TestDownloadRefusesADeclaredOversizeBody(t *testing.T) {
	body := strings.Repeat("x", 5000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Declared explicitly: a body this size would otherwise be sent
		// chunked, which is the other path (see the overrun test).
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	workingDir := t.TempDir()
	target := filepath.Join(workingDir, "big.bin")

	resp := runDownload(t, downloadToolFor(t, workingDir, 100), srv.URL, target)
	require.True(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "Nothing was written")

	_, err := os.Stat(target)
	require.True(t, os.IsNotExist(err))
}

// A partial file looks like a completed download to everything downstream,
// so an overrun found mid-copy takes the file with it.
func TestDownloadRemovesThePartialFileOnOverrun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Chunked, so there is no Content-Length to check up front.
		w.Header().Set("Transfer-Encoding", "chunked")
		for range 20 {
			_, _ = w.Write([]byte(strings.Repeat("x", 512)))
			w.(http.Flusher).Flush()
		}
	}))
	defer srv.Close()

	workingDir := t.TempDir()
	target := filepath.Join(workingDir, "streamed.bin")

	resp := runDownload(t, downloadToolFor(t, workingDir, 1000), srv.URL, target)
	require.True(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "max_download_bytes")

	_, err := os.Stat(target)
	require.True(t, os.IsNotExist(err), "the partial file must be gone")
}

func TestDownloadWithinTheLimitSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("small enough"))
	}))
	defer srv.Close()

	workingDir := t.TempDir()
	target := filepath.Join(workingDir, "ok.txt")

	resp := runDownload(t, downloadToolFor(t, workingDir, 1000), srv.URL, target)
	require.False(t, resp.IsError, resp.Content)

	content, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "small enough", string(content))
}
