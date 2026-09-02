package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeAtlasConfig(t *testing.T, workingDir, contents string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "atlas.json"), []byte(contents), 0o644))
}

func TestMCPListWithNoServers(t *testing.T) {
	c := newSkillTestCmd(t, runMCPList, t.TempDir(), t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "No MCP servers configured.")
}

func TestMCPListShowsEachTransport(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"mcp":{
		"files":{"type":"stdio","command":"npx","args":["-y","server-filesystem"]},
		"docs":{"type":"http","url":"https://mcp.example.com/mcp","disabled":true}
	}}`)

	c := newSkillTestCmd(t, runMCPList, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	got := out.String()
	require.Contains(t, got, "files (stdio, enabled)")
	require.Contains(t, got, "npx -y server-filesystem")
	require.Contains(t, got, "docs (http, disabled)")
	require.Contains(t, got, "https://mcp.example.com/mcp")
}

// Env values regularly hold API keys, so listing must never print them.
func TestMCPListDoesNotPrintEnv(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"mcp":{
		"secretive":{"type":"stdio","command":"srv","env":{"TOKEN":"sk-do-not-print"}}
	}}`)

	c := newSkillTestCmd(t, runMCPList, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "secretive (stdio, enabled)")
	require.NotContains(t, out.String(), "sk-do-not-print")
	require.NotContains(t, out.String(), "TOKEN")
}

func TestMCPListShowsToolFilters(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"mcp":{
		"filtered":{"type":"stdio","command":"srv","enabled_tools":["a","b"],"disabled_tools":["c"]}
	}}`)

	c := newSkillTestCmd(t, runMCPList, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "only: a, b; disabled: c")
}
