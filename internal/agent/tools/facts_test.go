package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/factstore"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/permission"
	"github.com/stretchr/testify/require"
)

func newFactsTool(t *testing.T) fantasy.AgentTool {
	t.Helper()
	store := factstore.New(filepath.Join(t.TempDir(), "facts.jsonl"))
	return NewFactsTool(store, permission.NewPermissionService(t.TempDir(), true, nil))
}

func runFacts(t *testing.T, tool fantasy.AgentTool, params FactsParams) fantasy.ToolResponse {
	t.Helper()
	input, err := json.Marshal(params)
	require.NoError(t, err)
	res, err := tool.Run(context.WithValue(t.Context(), SessionIDContextKey, "test-session"), fantasy.ToolCall{
		ID:    "test-call",
		Name:  FactsToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return res
}

func TestFactsToolRetainsAndRecalls(t *testing.T) {
	tool := newFactsTool(t)

	res := runFacts(t, tool, FactsParams{Action: "retain", Text: "the queue worker retries three times"})
	require.False(t, res.IsError)
	require.Contains(t, res.Content, "Retained")

	res = runFacts(t, tool, FactsParams{Action: "recall", Query: "queue worker"})
	require.False(t, res.IsError)
	require.Contains(t, res.Content, "1 fact(s)")
	require.Contains(t, res.Content, "retries three times")
}

func TestFactsToolRetainRequiresText(t *testing.T) {
	tool := newFactsTool(t)
	res := runFacts(t, tool, FactsParams{Action: "retain"})
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "text is required")
}

func TestFactsToolRecallWithNoQueryReturnsRecent(t *testing.T) {
	tool := newFactsTool(t)
	runFacts(t, tool, FactsParams{Action: "retain", Text: "first fact"})
	runFacts(t, tool, FactsParams{Action: "retain", Text: "second fact"})

	res := runFacts(t, tool, FactsParams{Action: "recall"})
	require.Contains(t, res.Content, "2 fact(s)")
}

func TestFactsToolRecallReportsNoResults(t *testing.T) {
	tool := newFactsTool(t)
	res := runFacts(t, tool, FactsParams{Action: "recall", Query: "nothing here"})
	require.Contains(t, res.Content, "No retained fact matches")
}

func TestFactsToolRecallOnEmptyStore(t *testing.T) {
	tool := newFactsTool(t)
	res := runFacts(t, tool, FactsParams{Action: "recall"})
	require.Contains(t, res.Content, "Nothing has been retained yet")
}

func TestFactsToolReflectSummarises(t *testing.T) {
	tool := newFactsTool(t)
	runFacts(t, tool, FactsParams{Action: "retain", Text: "fact a", Tags: []string{"build"}})
	runFacts(t, tool, FactsParams{Action: "retain", Text: "fact a", Tags: []string{"build"}})

	res := runFacts(t, tool, FactsParams{Action: "reflect"})
	require.Contains(t, res.Content, "2 fact(s) retained")
	require.Contains(t, res.Content, "build: 2")
	require.Contains(t, res.Content, "near-duplicate")
}

func TestFactsToolReflectOnEmptyStore(t *testing.T) {
	tool := newFactsTool(t)
	res := runFacts(t, tool, FactsParams{Action: "reflect"})
	require.Contains(t, res.Content, "Nothing has been retained yet")
}

func TestFactsToolRejectsUnknownAction(t *testing.T) {
	tool := newFactsTool(t)
	res := runFacts(t, tool, FactsParams{Action: "delete"})
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "unknown action")
}

func answerFactsPermission(t *testing.T, tool fantasy.AgentTool, permissions permission.Service, params FactsParams, allow bool) (fantasy.ToolResponse, []permission.PermissionRequest) {
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
		ID: "test-call", Name: FactsToolName, Input: string(input),
	})
	require.NoError(t, err)

	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	return res, seen
}

func TestFactsToolAsksBeforeRetaining(t *testing.T) {
	store := factstore.New(filepath.Join(t.TempDir(), "facts.jsonl"))
	permissions := permission.NewPermissionService(t.TempDir(), false, nil)
	tool := NewFactsTool(store, permissions)

	res, asked := answerFactsPermission(t, tool, permissions, FactsParams{Action: "retain", Text: "needs approval first"}, true)
	require.False(t, res.IsError)
	require.Len(t, asked, 1)
	require.Equal(t, FactsToolName, asked[0].ToolName)

	got, err := store.Recall("", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestFactsToolRetainDeniedByPermission(t *testing.T) {
	store := factstore.New(filepath.Join(t.TempDir(), "facts.jsonl"))
	permissions := permission.NewPermissionService(t.TempDir(), false, nil)
	tool := NewFactsTool(store, permissions)

	res, asked := answerFactsPermission(t, tool, permissions, FactsParams{Action: "retain", Text: "should not be saved"}, false)
	require.True(t, res.IsError)
	require.Len(t, asked, 1)

	got, err := store.Recall("", 10)
	require.NoError(t, err)
	require.Empty(t, got)
}
