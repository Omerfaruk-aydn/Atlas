package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func newHooksTestCmd(t *testing.T, workingDir string) *cobra.Command {
	t.Helper()
	c := newSkillTestCmd(t, runHooksList, workingDir, t.TempDir())
	c.Flags().StringVar(&hooksListTool, "tool", "", "")
	t.Cleanup(func() { hooksListTool = "" })
	return c
}

func TestHooksListWithNoneConfigured(t *testing.T) {
	c := newHooksTestCmd(t, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "No hooks configured.")
}

func TestHooksListShowsEachEvent(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"hooks":{
		"PreToolUse":[{"name":"guard","matcher":"^bash$","command":"./guard.sh"}],
		"post_tool_use":[{"command":"./format.sh"}]
	}}`)

	c := newHooksTestCmd(t, workingDir)
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	got := out.String()

	require.Contains(t, got, "PreToolUse")
	require.Contains(t, got, "guard [^bash$]")
	require.Contains(t, got, "./guard.sh")

	// The snake_case event name is normalized at load time.
	require.Contains(t, got, "PostToolUse")
	require.Contains(t, got, "(all tools)")
	require.Contains(t, got, "./format.sh")
}

func TestHooksListFiltersByTool(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"hooks":{"PreToolUse":[
		{"name":"bash-only","matcher":"^bash$","command":"./bash.sh"},
		{"name":"everything","command":"./all.sh"}
	]}}`)

	c := newHooksTestCmd(t, workingDir)
	require.NoError(t, c.Flags().Set("tool", "view"))
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "everything")
	require.NotContains(t, out.String(), "bash-only")
}

func TestHooksListReportsWhenNothingMatchesTheTool(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"hooks":{"PreToolUse":[
		{"name":"bash-only","matcher":"^bash$","command":"./bash.sh"}
	]}}`)

	c := newHooksTestCmd(t, workingDir)
	require.NoError(t, c.Flags().Set("tool", "view"))
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "No hooks would fire for view.")
}

func TestHooksListJSON(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"hooks":{
		"PreToolUse":[{"name":"guard","matcher":"^bash$","command":"./guard.sh"}],
		"post_tool_use":[{"command":"./format.sh"}]
	}}`)

	c := newHooksTestCmd(t, workingDir)
	c.Flags().BoolVar(&hooksListJSON, "json", true, "")
	t.Cleanup(func() { hooksListJSON = false })
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))

	var got []jsonHook
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Equal(t, []jsonHook{
		{Event: "PostToolUse", Name: "./format.sh", Command: "./format.sh"},
		{Event: "PreToolUse", Name: "guard", Matcher: "^bash$", Command: "./guard.sh"},
	}, got)
}

// A hook with no name is listed by its command, the same fallback the TUI
// uses.
func TestHooksListNamesAnUnnamedHookByItsCommand(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"hooks":{"PreToolUse":[{"command":"./unnamed.sh"}]}}`)

	c := newHooksTestCmd(t, workingDir)
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "./unnamed.sh")
}
