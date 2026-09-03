package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func doctorOutput(t *testing.T, workingDir, dataDir string) (string, error) {
	t.Helper()
	c := newSkillTestCmd(t, runDoctor, workingDir, dataDir)
	var out bytes.Buffer
	c.SetOut(&out)
	err := c.RunE(c, nil)
	return out.String(), err
}

func TestDoctorChecksTheEssentials(t *testing.T) {
	got, _ := doctorOutput(t, t.TempDir(), t.TempDir())
	require.Contains(t, got, "data directory:")
	require.Contains(t, got, "database:")
	require.Contains(t, got, "providers:")
	require.Contains(t, got, "models:")
}

func TestDoctorReportsAWritableDataDirectoryAndWorkingDatabase(t *testing.T) {
	dataDir := t.TempDir()
	got, _ := doctorOutput(t, t.TempDir(), dataDir)

	// Only the status is asserted, not the path: macOS hands out a
	// symlinked temp directory and reports its resolved form.
	require.Contains(t, got, "[ok] data directory:")
	require.Contains(t, got, "[ok] database:")
	require.Contains(t, got, "atlas.db")

	// The writability probe cleans up after itself.
	entries, err := os.ReadDir(dataDir)
	require.NoError(t, err)
	for _, e := range entries {
		require.NotEqual(t, ".atlas-doctor", e.Name())
	}
}

// A missing server binary loses that server's tools but does not stop a
// session starting, so it is a warning and the command still succeeds.
func TestDoctorWarnsAboutAMissingServerCommand(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"mcp":{"ghost":{"type":"stdio","command":"definitely-not-a-real-binary-xyz"}}}`)

	// The error is ignored, not asserted on: a machine with no provider
	// or model configured -- CI, for one -- fails those checks and so
	// fails the command, which says nothing about the check under test.
	got, _ := doctorOutput(t, workingDir, t.TempDir())
	require.Contains(t, got, "[warn] mcp ghost: definitely-not-a-real-binary-xyz not found on PATH")
}

// An HTTP server has no command to look for, so it is not reported at all
// rather than reported as missing.
func TestDoctorSkipsNonStdioAndDisabledServers(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"mcp":{
		"remote":{"type":"http","url":"https://mcp.example.com"},
		"off":{"type":"stdio","command":"definitely-not-a-real-binary-xyz","disabled":true}
	}}`)

	got, _ := doctorOutput(t, workingDir, t.TempDir())
	require.NotContains(t, got, "mcp remote")
	require.NotContains(t, got, "mcp off")
}

func TestDoctorFindsACommandThatExists(t *testing.T) {
	workingDir := t.TempDir()
	// go is what runs this test, so it is on PATH by definition.
	writeAtlasConfig(t, workingDir, `{"lsp":{"selfhost":{"command":"go"}}}`)

	got, _ := doctorOutput(t, workingDir, t.TempDir())
	require.Contains(t, got, "[ok] lsp selfhost:")
}

// A failing check has to make the command itself fail, so a script can rely
// on the exit status instead of parsing the report.
func TestPrintChecksFailsOnAFailedCheck(t *testing.T) {
	var out bytes.Buffer
	err := printChecks(&out, []checkResult{
		{"data directory", statusFail, "not writable"},
		{"database", statusOK, "fine"},
		{"mcp ghost", statusWarn, "not found on PATH"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "1 check(s) failed")
	require.Contains(t, out.String(), "[fail] data directory: not writable")
	require.Contains(t, out.String(), "[warn] mcp ghost: not found on PATH")
}

// A warning is something a session starts without, so it must not fail the
// command.
func TestPrintChecksSucceedsOnWarningsAlone(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, printChecks(&out, []checkResult{
		{"providers", statusWarn, "none with an API key"},
	}))
}

func TestCheckStatusNames(t *testing.T) {
	require.Equal(t, "ok", statusOK.String())
	require.Equal(t, "warn", statusWarn.String())
	require.Equal(t, "fail", statusFail.String())
}

func TestPrintChecksJSON(t *testing.T) {
	doctorJSON = true
	t.Cleanup(func() { doctorJSON = false })

	var out bytes.Buffer
	err := printChecks(&out, []checkResult{
		{"data directory", statusFail, "not writable"},
		{"database", statusOK, "fine"},
	})
	require.Error(t, err)

	var got []jsonCheckResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Equal(t, []jsonCheckResult{
		{Name: "data directory", Status: "fail", Detail: "not writable"},
		{Name: "database", Status: "ok", Detail: "fine"},
	}, got)
}
