package hooks

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPayloadCarriesTheToolResponseForPostHooks(t *testing.T) {
	raw := BuildPayloadWithResponse(EventPostToolUse, "s1", "/work", "view", `{"file_path":"a.go"}`, "the output")

	var p Payload
	require.NoError(t, json.Unmarshal(raw, &p))
	require.Equal(t, EventPostToolUse, p.Event)
	require.Equal(t, "the output", p.ToolResponse)
}

// A hook reads this off a pipe, so a tool that returned megabytes must not
// stall the turn feeding them to a shell script.
func TestLongToolResponsesAreTruncated(t *testing.T) {
	huge := strings.Repeat("x", MaxToolResponseInPayload*2)
	raw := BuildPayloadWithResponse(EventPostToolUse, "s1", "/work", "bash", "{}", huge)

	var p Payload
	require.NoError(t, json.Unmarshal(raw, &p))
	require.Less(t, len(p.ToolResponse), len(huge))
	require.Contains(t, p.ToolResponse, "[truncated]")
	require.True(t, strings.HasPrefix(p.ToolResponse, strings.Repeat("x", 100)))
}

// PreToolUse payloads are unchanged: the field is omitted, not empty.
func TestPreToolUsePayloadHasNoToolResponseField(t *testing.T) {
	raw := BuildPayload(EventPreToolUse, "s1", "/work", "view", `{"file_path":"a.go"}`)
	require.NotContains(t, string(raw), "tool_response")
}
