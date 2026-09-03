package cmd

import (
	"bytes"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/message"
	"github.com/stretchr/testify/require"
)

func assistantFromModel(model string) message.Message {
	return message.Message{Role: message.Assistant, Model: model}
}

func TestCountModelUsageWithNoMessages(t *testing.T) {
	require.Empty(t, countModelUsage(nil))
}

func TestCountModelUsageTalliesPerModel(t *testing.T) {
	got := countModelUsage([]message.Message{
		assistantFromModel("claude-sonnet-5"),
		assistantFromModel("claude-sonnet-5"),
		assistantFromModel("claude-opus-5"),
	})

	require.Equal(t, []modelUsage{
		{Name: "claude-sonnet-5", Calls: 2},
		{Name: "claude-opus-5", Calls: 1},
	}, got)
}

// Non-assistant messages carry no model of their own and must not be
// counted, even if the field happens to be populated on the struct.
func TestCountModelUsageSkipsNonAssistantMessages(t *testing.T) {
	got := countModelUsage([]message.Message{
		{Role: message.User, Model: "claude-sonnet-5"},
		{Role: message.Tool, Model: "claude-sonnet-5"},
		assistantFromModel("claude-sonnet-5"),
	})
	require.Equal(t, []modelUsage{{Name: "claude-sonnet-5", Calls: 1}}, got)
}

// A summary message or a turn that never reached a provider leaves Model
// blank; it should not show up as a usage entry with an empty name.
func TestCountModelUsageSkipsBlankModel(t *testing.T) {
	got := countModelUsage([]message.Message{assistantFromModel("")})
	require.Empty(t, got)
}

func TestPrintModelUsageHuman(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, printModelUsage(&out, []modelUsage{
		{Name: "claude-sonnet-5", Calls: 3},
	}))
	require.Contains(t, out.String(), "claude-sonnet-5")
	require.Contains(t, out.String(), "3")
}

func TestPrintModelUsageWithNone(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, printModelUsage(&out, nil))
	require.Contains(t, out.String(), "No model usage recorded.")
}

func TestPrintModelUsageJSON(t *testing.T) {
	sessionModelsJSON = true
	t.Cleanup(func() { sessionModelsJSON = false })

	var out bytes.Buffer
	require.NoError(t, printModelUsage(&out, []modelUsage{{Name: "claude-sonnet-5", Calls: 2}}))
	require.Contains(t, out.String(), `"name": "claude-sonnet-5"`)
	require.Contains(t, out.String(), `"calls": 2`)
}
