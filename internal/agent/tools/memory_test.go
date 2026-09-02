package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/memory"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/permission"
	"github.com/stretchr/testify/require"
)

func newMemoryStore(t *testing.T, opts memory.Options) *memory.Store {
	t.Helper()
	dir := t.TempDir()
	if opts.ProjectDir == "" {
		opts.ProjectDir = filepath.Join(dir, "project")
	}
	if opts.UserDir == "" {
		opts.UserDir = filepath.Join(dir, "user")
	}
	return memory.New(opts)
}

// newMemoryTool builds the tool with approval already given, so the tests
// that are about the store are not also about the dialog.
func newMemoryTool(t *testing.T, opts memory.Options) fantasy.AgentTool {
	t.Helper()
	return NewMemoryTool(newMemoryStore(t, opts), permission.NewPermissionService(t.TempDir(), true, nil))
}

func runMemory(t *testing.T, tool fantasy.AgentTool, params MemoryParams) fantasy.ToolResponse {
	t.Helper()

	input, err := json.Marshal(params)
	require.NoError(t, err)

	res, err := tool.Run(context.WithValue(t.Context(), SessionIDContextKey, "test-session"), fantasy.ToolCall{
		ID:    "test-call",
		Name:  MemoryToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return res
}

func TestMemoryToolAddsAndReadsBack(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{})

	res := runMemory(t, tool, MemoryParams{
		Action: "add",
		Scope:  "project",
		Entry:  "the race detector needs a C compiler on Windows",
	})

	require.False(t, res.IsError)
	require.Contains(t, res.Content, "the race detector needs a C compiler")
	require.Contains(t, res.Content, "next session",
		"the model is told the write is not visible in this conversation")
}

func TestMemoryToolReportsAnUnknownScopeToTheModel(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{})

	res := runMemory(t, tool, MemoryParams{Action: "add", Scope: "everything", Entry: "x"})

	require.True(t, res.IsError, "a fixable mistake comes back as a result, not as a failed turn")
	require.Contains(t, res.Content, "unknown scope")
}

func TestMemoryToolReportsAnUnknownAction(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{})

	res := runMemory(t, tool, MemoryParams{Action: "append", Scope: "project", Entry: "x"})

	require.True(t, res.IsError)
	require.Contains(t, res.Content, "unknown action")
}

func TestMemoryToolTellsTheModelHowMuchToCut(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{ProjectLimit: 20})

	res := runMemory(t, tool, MemoryParams{
		Action: "add",
		Scope:  "project",
		Entry:  strings.Repeat("x", 100),
	})

	require.True(t, res.IsError)
	require.Contains(t, res.Content, "over the 20 limit by",
		"the model is told the size of the problem, not just that there is one")
}

func TestMemoryToolRefusesAnAmbiguousReplace(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{})

	runMemory(t, tool, MemoryParams{Action: "add", Scope: "project", Entry: "go 1.26"})
	runMemory(t, tool, MemoryParams{Action: "add", Scope: "project", Entry: "go 1.26 is pinned"})

	res := runMemory(t, tool, MemoryParams{
		Action: "replace", Scope: "project", Old: "go 1.26", New: "go 1.27",
	})

	require.True(t, res.IsError)
	require.Contains(t, res.Content, "more than once")
}

func TestMemoryToolSetConsolidates(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{})

	runMemory(t, tool, MemoryParams{Action: "add", Scope: "user", Entry: "one"})
	runMemory(t, tool, MemoryParams{Action: "add", Scope: "user", Entry: "two"})

	res := runMemory(t, tool, MemoryParams{
		Action: "set", Scope: "user", Entry: "- one and two, together",
	})

	require.False(t, res.IsError)
	require.Contains(t, res.Content, "one and two, together")
	require.NotContains(t, res.Content, "- one\n")
}

func TestMemoryToolShowsAnEmptyStoreAsEmpty(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{})

	runMemory(t, tool, MemoryParams{Action: "add", Scope: "project", Entry: "only"})
	res := runMemory(t, tool, MemoryParams{Action: "remove", Scope: "project", Old: "only"})

	require.False(t, res.IsError)
	require.Contains(t, res.Content, "(empty)")
}

func TestMemoryToolReportsUsage(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{})

	res := runMemory(t, tool, MemoryParams{Action: "add", Scope: "project", Entry: "abc"})

	require.Contains(t, res.Content, "Using 6 of")
}

// answerPermission runs a tool call with a subscriber standing in for the
// user, and reports what the dialog was asked to show.
func answerPermission(t *testing.T, tool fantasy.AgentTool, permissions permission.Service, params MemoryParams, allow bool) (fantasy.ToolResponse, []permission.PermissionRequest) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	events := permissions.Subscribe(ctx)

	var (
		mu   sync.Mutex
		seen []permission.PermissionRequest
		done = make(chan struct{})
	)
	go func() {
		defer close(done)
		for event := range events {
			mu.Lock()
			seen = append(seen, event.Payload)
			mu.Unlock()
			if allow {
				permissions.Grant(event.Payload)
			} else {
				permissions.Deny(event.Payload)
			}
		}
	}()

	input, err := json.Marshal(params)
	require.NoError(t, err)
	res, err := tool.Run(context.WithValue(ctx, SessionIDContextKey, "test-session"), fantasy.ToolCall{
		ID: "test-call", Name: MemoryToolName, Input: string(input),
	})
	require.NoError(t, err)

	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	return res, seen
}

func TestMemoryToolAsksBeforeWriting(t *testing.T) {
	t.Parallel()
	store := newMemoryStore(t, memory.Options{})
	permissions := permission.NewPermissionService(t.TempDir(), false, nil)
	tool := NewMemoryTool(store, permissions)

	res, asked := answerPermission(t, tool, permissions, MemoryParams{
		Action: "add", Scope: "project", Entry: "the linter is pinned to v2.13.1",
	}, true)

	require.False(t, res.IsError)
	require.Len(t, asked, 1, "a write the user never sees is the thing this prevents")
	require.Equal(t, MemoryToolName, asked[0].ToolName)
	require.Equal(t, store.Path(memory.ScopeProject), asked[0].Path)

	written, err := store.Read(memory.ScopeProject)
	require.NoError(t, err)
	require.Contains(t, written, "v2.13.1")
}

func TestMemoryToolWritesNothingWhenDenied(t *testing.T) {
	t.Parallel()
	store := newMemoryStore(t, memory.Options{})
	permissions := permission.NewPermissionService(t.TempDir(), false, nil)
	tool := NewMemoryTool(store, permissions)

	res, asked := answerPermission(t, tool, permissions, MemoryParams{
		Action: "add", Scope: "user", Entry: "prefers tabs",
	}, false)

	require.True(t, res.IsError)
	require.Len(t, asked, 1)

	written, err := store.Read(memory.ScopeUser)
	require.NoError(t, err)
	require.Empty(t, written, "denied means not on disk, not on disk and reported as denied")
}

func TestMemoryToolShowsBothVersions(t *testing.T) {
	t.Parallel()
	store := newMemoryStore(t, memory.Options{})
	_, err := store.Add(memory.ScopeProject, "go 1.26")
	require.NoError(t, err)

	permissions := permission.NewPermissionService(t.TempDir(), false, nil)
	tool := NewMemoryTool(store, permissions)

	_, asked := answerPermission(t, tool, permissions, MemoryParams{
		Action: "replace", Scope: "project", Old: "go 1.26", New: "go 1.27",
	}, true)

	require.Len(t, asked, 1)
	params, ok := asked[0].Params.(MemoryPermissionParams)
	require.True(t, ok, "the dialog needs the versions to render a diff")
	require.Contains(t, params.OldContent, "go 1.26")
	require.Contains(t, params.NewContent, "go 1.27")
}

// Adding what is already there changes nothing, and a question about a
// change that is not happening is just noise.
func TestMemoryToolDoesNotAskWhenNothingChanges(t *testing.T) {
	t.Parallel()
	store := newMemoryStore(t, memory.Options{})
	_, err := store.Add(memory.ScopeProject, "already known")
	require.NoError(t, err)

	permissions := permission.NewPermissionService(t.TempDir(), false, nil)
	tool := NewMemoryTool(store, permissions)

	res, asked := answerPermission(t, tool, permissions, MemoryParams{
		Action: "add", Scope: "project", Entry: "already known",
	}, false)

	require.False(t, res.IsError)
	require.Empty(t, asked)
}
