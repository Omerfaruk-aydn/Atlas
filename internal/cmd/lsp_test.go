package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLSPListWithNoServers(t *testing.T) {
	c := newSkillTestCmd(t, runLSPList, t.TempDir(), t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "No LSP servers configured.")
}

func TestLSPListShowsFileTypesAndDisabled(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"lsp":{
		"gopls":{"command":"go","args":["run","golang.org/x/tools/gopls@latest"],"filetypes":["go","mod"]},
		"off":{"command":"no-such-binary-xyz","disabled":true}
	}}`)

	c := newSkillTestCmd(t, runLSPList, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	got := out.String()
	require.Contains(t, got, "gopls (enabled)")
	require.Contains(t, got, "go run golang.org/x/tools/gopls@latest")
	require.Contains(t, got, "filetypes: go, mod")
	require.Contains(t, got, "off (disabled)")
}

// A configured command that isn't on PATH shows up as such, the same
// judgment doctor makes, so this list explains why a server's tools never
// show up without requiring a full doctor run.
func TestLSPListFlagsAnUnresolvableCommand(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"lsp":{
		"missing":{"command":"no-such-binary-xyz"}
	}}`)

	c := newSkillTestCmd(t, runLSPList, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "missing (enabled, not found on PATH)")
}

func TestLSPListJSON(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"lsp":{
		"gopls":{"command":"go","args":["run","golang.org/x/tools/gopls@latest"],"filetypes":["go","mod"]},
		"off":{"command":"no-such-binary-xyz","disabled":true}
	}}`)

	c := newSkillTestCmd(t, runLSPList, workingDir, t.TempDir())
	c.Flags().BoolVar(&lspListJSON, "json", true, "")
	t.Cleanup(func() { lspListJSON = false })
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))

	var got []jsonLSPServer
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Equal(t, []jsonLSPServer{
		{Name: "gopls", Status: "enabled", Command: "go run golang.org/x/tools/gopls@latest", FileTypes: []string{"go", "mod"}},
		{Name: "off", Status: "disabled", Command: "no-such-binary-xyz"},
	}, got)
}

// Env values regularly hold secrets, so listing must never print them.
func TestLSPListDoesNotPrintEnv(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"lsp":{
		"secretive":{"command":"go","env":{"TOKEN":"sk-do-not-print"}}
	}}`)

	c := newSkillTestCmd(t, runLSPList, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.NotContains(t, out.String(), "sk-do-not-print")
	require.NotContains(t, out.String(), "TOKEN")
}
