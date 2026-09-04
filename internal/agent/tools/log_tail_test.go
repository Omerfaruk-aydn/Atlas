package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runLogTailTool(t *testing.T, workingDir string, params LogTailParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewLogTailTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  LogTailToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func TestLogTailToolReportsLines(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.log"), []byte("line1\nline2\nline3\n"), 0o644))

	resp := runLogTailTool(t, dir, LogTailParams{Path: "app.log", Lines: 2})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "2 line(s), out of 3 total")
	require.Contains(t, resp.Content, "line2")
	require.Contains(t, resp.Content, "line3")
	require.NotContains(t, resp.Content, "line1")
}

func TestLogTailToolFiltersByLevel(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.log"), []byte("INFO ok\nERROR boom\n"), 0o644))

	resp := runLogTailTool(t, dir, LogTailParams{Path: "app.log", Level: "error"})
	require.Contains(t, resp.Content, "ERROR boom")
	require.NotContains(t, resp.Content, "INFO ok")
}

func TestLogTailToolReportsNoMatches(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.log"), []byte("INFO ok\n"), 0o644))

	resp := runLogTailTool(t, dir, LogTailParams{Path: "app.log", Grep: "nonexistent"})
	require.Contains(t, resp.Content, "No lines matched, out of 1 total")
}

func TestLogTailToolReportsEmptyFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.log"), []byte(""), 0o644))

	resp := runLogTailTool(t, dir, LogTailParams{Path: "app.log"})
	require.Contains(t, resp.Content, "The file is empty")
}

func TestLogTailToolRequiresPath(t *testing.T) {
	dir := t.TempDir()
	resp := runLogTailTool(t, dir, LogTailParams{})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "path is required")
}

func TestLogTailToolReportsMissingFile(t *testing.T) {
	dir := t.TempDir()
	resp := runLogTailTool(t, dir, LogTailParams{Path: "nope.log"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "no file found")
}

func TestLogTailToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.log"), []byte("line1\nline2\n"), 0o644))

	resp := runLogTailTool(t, dir, LogTailParams{Path: "app.log"})
	var meta LogTailResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 2, meta.LinesReturned)
	require.Equal(t, 2, meta.TotalLines)
	require.False(t, meta.Truncated)
}
