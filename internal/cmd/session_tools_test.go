package cmd

import (
	"bytes"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/message"
	"github.com/stretchr/testify/require"
)

func assistantWithCalls(calls ...message.ToolCall) message.Message {
	parts := make([]message.ContentPart, 0, len(calls))
	for _, c := range calls {
		parts = append(parts, c)
	}
	return message.Message{Role: message.Assistant, Parts: parts}
}

func toolMessage(results ...message.ToolResult) message.Message {
	parts := make([]message.ContentPart, 0, len(results))
	for _, r := range results {
		parts = append(parts, r)
	}
	return message.Message{Role: message.Tool, Parts: parts}
}

func TestCountToolUsageWithNoCalls(t *testing.T) {
	require.Empty(t, countToolUsage(nil))
}

func TestCountToolUsageTalliesPerTool(t *testing.T) {
	got := countToolUsage([]message.Message{
		assistantWithCalls(
			message.ToolCall{ID: "1", Name: "view"},
			message.ToolCall{ID: "2", Name: "bash"},
		),
		assistantWithCalls(message.ToolCall{ID: "3", Name: "view"}),
	})

	require.Equal(t, []toolUsage{
		{Name: "view", Calls: 2},
		{Name: "bash", Calls: 1},
	}, got)
}

func TestCountToolUsageCountsErrors(t *testing.T) {
	got := countToolUsage([]message.Message{
		assistantWithCalls(message.ToolCall{ID: "1", Name: "view"}),
		toolMessage(message.ToolResult{ToolCallID: "1", Name: "view", IsError: true}),
	})
	require.Equal(t, []toolUsage{{Name: "view", Calls: 1, Errors: 1}}, got)
}

// A result that carries no tool name still has to be attributed, via the
// call it answers.
func TestCountToolUsageAttributesAnUnnamedResult(t *testing.T) {
	got := countToolUsage([]message.Message{
		assistantWithCalls(message.ToolCall{ID: "abc", Name: "grep"}),
		toolMessage(message.ToolResult{ToolCallID: "abc", IsError: true}),
	})
	require.Equal(t, []toolUsage{{Name: "grep", Calls: 1, Errors: 1}}, got)
}

// A call the turn was cancelled before answering has no result; it counts
// as a call, never as a failure.
func TestCountToolUsageDoesNotInventFailures(t *testing.T) {
	got := countToolUsage([]message.Message{
		assistantWithCalls(message.ToolCall{ID: "1", Name: "bash"}),
	})
	require.Equal(t, []toolUsage{{Name: "bash", Calls: 1, Errors: 0}}, got)
}

func TestPrintToolUsageHuman(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, printToolUsage(&out, []toolUsage{
		{Name: "view", Calls: 3},
		{Name: "bash", Calls: 2, Errors: 1},
	}))
	require.Contains(t, out.String(), "view: 3\n")
	require.Contains(t, out.String(), "bash: 2 (1 failed)\n")
}

func TestPrintToolUsageWithNoTools(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, printToolUsage(&out, nil))
	require.Contains(t, out.String(), "called no tools")
}
