package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/permission"
	"github.com/stretchr/testify/require"
)

// The refusal has to come before the permission request, not after it: a
// workspace that forbids writing outside itself should not be prompting
// about the write at all.
func TestWriteToolRefusesOutsideTheRootBeforeAsking(t *testing.T) {
	workingDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "escaped.txt")

	permissions := &noRequestPermissions{
		Service: permission.NewPermissionService(t.TempDir(), false, nil),
		t:       t,
	}
	tool := NewWriteTool(nil, permissions, &mockHistoryService{}, mockFileTrackerService{}, workingDir,
		PathPolicy{Root: workingDir, Restrict: true})

	ctx := context.WithValue(context.Background(), SessionIDContextKey, "s1")
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "c1",
		Input: `{"file_path":"` + filepath.ToSlash(outside) + `","content":"nope"}`,
	})
	require.NoError(t, err)
	require.True(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "outside the working directory")

	_, statErr := os.Stat(outside)
	require.True(t, os.IsNotExist(statErr), "the file must not have been written")
}

func TestWriteToolStillWritesInsideTheRoot(t *testing.T) {
	workingDir := t.TempDir()
	target := filepath.Join(workingDir, "inside.txt")

	tool := NewWriteTool(nil, &mockPermissionService{}, &mockHistoryService{}, mockFileTrackerService{}, workingDir,
		PathPolicy{Root: workingDir, Restrict: true})

	ctx := context.WithValue(context.Background(), SessionIDContextKey, "s1")
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "c1",
		Input: `{"file_path":"` + filepath.ToSlash(target) + `","content":"yes"}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError, resp.Content)

	content, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "yes", string(content))
}
