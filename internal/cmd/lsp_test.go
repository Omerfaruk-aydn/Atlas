package cmd

import (
	"bytes"
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
