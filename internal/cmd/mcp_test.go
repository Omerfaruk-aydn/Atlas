package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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

func TestMCPTestUnknownServer(t *testing.T) {
	c := newSkillTestCmd(t, runMCPTest, t.TempDir(), t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	err := c.RunE(c, []string{"no-such-server"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no MCP server named")
}

func TestMCPTestNoServersConfigured(t *testing.T) {
	c := newSkillTestCmd(t, runMCPTest, t.TempDir(), t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	err := c.RunE(c, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no MCP servers are configured")
}

func TestMCPTestStdioCommandFound(t *testing.T) {
	workingDir := t.TempDir()
	// "go" is guaranteed to be on PATH in this test's own build environment.
	writeAtlasConfig(t, workingDir, `{"mcp":{"local":{"type":"stdio","command":"go"}}}`)

	c := newSkillTestCmd(t, runMCPTest, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"local"}))
	require.Contains(t, out.String(), "local: ok")
}

func TestMCPTestStdioCommandNotFound(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"mcp":{"local":{"type":"stdio","command":"no-such-binary-xyz"}}}`)

	c := newSkillTestCmd(t, runMCPTest, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	err := c.RunE(c, []string{"local"})
	require.Error(t, err)
	require.Contains(t, out.String(), "local: failed")
}

func TestMCPTestHTTPReachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// A real MCP endpoint routinely answers a bare GET with an error --
		// only a live server produces one at all, so this still counts as
		// reachable.
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	t.Cleanup(server.Close)

	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"mcp":{"remote":{"type":"http","url":"`+server.URL+`"}}}`)

	c := newSkillTestCmd(t, runMCPTest, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"remote"}))
	require.Contains(t, out.String(), "remote: ok")
}

func TestMCPTestHTTPUnreachable(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"mcp":{"remote":{"type":"http","url":"http://127.0.0.1:1"}}}`)

	c := newSkillTestCmd(t, runMCPTest, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	err := c.RunE(c, []string{"remote"})
	require.Error(t, err)
	require.Contains(t, out.String(), "remote: failed")
}

// Disabled servers are skipped by a bare `mcp test`, same as `mcp list`
// marks them without contacting them.
func TestMCPTestSkipsDisabledServers(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"mcp":{
		"off":{"type":"stdio","command":"no-such-binary-xyz","disabled":true}
	}}`)

	c := newSkillTestCmd(t, runMCPTest, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	err := c.RunE(c, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no MCP servers are configured")
}
