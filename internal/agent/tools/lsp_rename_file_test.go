package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/csync"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/lsp"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/permission"
	"github.com/stretchr/testify/require"
)

func newEmptyLSPManager(t *testing.T) *lsp.Manager {
	t.Helper()
	return lsp.NewManager(config.NewTestStore(&config.Config{
		Providers: csync.NewMap[string, config.ProviderConfig](),
	}))
}

func runLspRenameFile(t *testing.T, workingDir string, mgr *lsp.Manager, params LspRenameFileParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewLspRenameFileTool(mgr, permission.NewPermissionService(t.TempDir(), true, nil), nil, nil, workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	res, err := tool.Run(context.WithValue(context.Background(), SessionIDContextKey, "test-session"), fantasy.ToolCall{
		ID: "test-call", Name: LspRenameFileToolName, Input: string(input),
	})
	require.NoError(t, err)
	return res
}

func TestLspRenameFileRequiresOldPath(t *testing.T) {
	dir := t.TempDir()
	res := runLspRenameFile(t, dir, newEmptyLSPManager(t), LspRenameFileParams{NewPath: "b.go"})
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "old_path is required")
}

func TestLspRenameFileRequiresNewPath(t *testing.T) {
	dir := t.TempDir()
	res := runLspRenameFile(t, dir, newEmptyLSPManager(t), LspRenameFileParams{OldPath: "a.go"})
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "new_path is required")
}

func TestLspRenameFileRejectsAMissingSource(t *testing.T) {
	dir := t.TempDir()
	res := runLspRenameFile(t, dir, newEmptyLSPManager(t), LspRenameFileParams{OldPath: "nope.go", NewPath: "b.go"})
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "does not exist")
}

func TestLspRenameFileRejectsAnExistingDestination(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.go"), []byte("package a\n"), 0o644))

	res := runLspRenameFile(t, dir, newEmptyLSPManager(t), LspRenameFileParams{OldPath: "a.go", NewPath: "b.go"})
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "already exists")
}

func TestLspRenameFileRefusesWithNoLSPClient(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644))

	res := runLspRenameFile(t, dir, newEmptyLSPManager(t), LspRenameFileParams{OldPath: "a.go", NewPath: "b.go"})
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "no LSP client handles this file")

	// Refusing to guess must mean the file was never touched.
	_, err := os.Stat(filepath.Join(dir, "a.go"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "b.go"))
	require.True(t, os.IsNotExist(err))
}

func TestIsMethodNotFoundError(t *testing.T) {
	require.True(t, isMethodNotFoundError(errors.New("willRenameFiles request failed: Method not found")))
	require.False(t, isMethodNotFoundError(errors.New("some other failure")))
	require.False(t, isMethodNotFoundError(nil))
}

func TestResolvePath(t *testing.T) {
	work := t.TempDir()
	require.Equal(t, filepath.Join(work, "a.go"), resolvePath(work, "a.go"))

	abs := filepath.Join(t.TempDir(), "a.go")
	require.Equal(t, abs, resolvePath(work, abs))
}
