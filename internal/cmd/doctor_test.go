package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

	require.Contains(t, got, "[ok] data directory: "+dataDir)
	require.Contains(t, got, "[ok] database: "+filepath.Join(dataDir, "atlas.db"))

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

	got, err := doctorOutput(t, workingDir, t.TempDir())
	require.NoError(t, err)
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

	got, err := doctorOutput(t, workingDir, t.TempDir())
	require.NoError(t, err)
	require.NotContains(t, got, "mcp remote")
	require.NotContains(t, got, "mcp off")
}

func TestDoctorFindsACommandThatExists(t *testing.T) {
	workingDir := t.TempDir()
	// go is what runs this test, so it is on PATH by definition.
	writeAtlasConfig(t, workingDir, `{"lsp":{"selfhost":{"command":"go"}}}`)

	got, err := doctorOutput(t, workingDir, t.TempDir())
	require.NoError(t, err)
	require.Contains(t, got, "[ok] lsp selfhost:")
}

// A failing check has to make the command itself fail, so a script can rely
// on the exit status instead of parsing the report.
func TestDoctorFailsWhenTheDataDirectoryIsNotWritable(t *testing.T) {
	workingDir := t.TempDir()

	// A file where the data directory should be: MkdirAll cannot create a
	// directory over it on any platform, unlike permission bits.
	blocked := filepath.Join(t.TempDir(), "in-the-way")
	require.NoError(t, os.WriteFile(blocked, []byte("not a directory"), 0o600))

	got, err := doctorOutput(t, workingDir, blocked)
	require.Error(t, err)
	require.Contains(t, err.Error(), "check(s) failed")
	require.True(t, strings.Contains(got, "[fail] data directory:"), got)
}

func TestCheckStatusNames(t *testing.T) {
	require.Equal(t, "ok", statusOK.String())
	require.Equal(t, "warn", statusWarn.String())
	require.Equal(t, "fail", statusFail.String())
}
