package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func writeProjectSubagent(t *testing.T, workingDir, name, description, model, instructions string) {
	t.Helper()
	dir := filepath.Join(workingDir, ".atlas", "agents")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	modelLine := ""
	if model != "" {
		modelLine = "model: " + model + "\n"
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n" + modelLine + "---\n\n" + instructions + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".md"), []byte(content), 0o644))
}

func TestAgentListShowsProjectSubagents(t *testing.T) {
	workingDir := t.TempDir()
	writeProjectSubagent(t, workingDir, "frontend", "Handles UI work.", "", "Do UI things.")

	c := newSkillTestCmd(t, runAgentList, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "frontend --")
	require.Contains(t, out.String(), "Handles UI work.")
}

func TestAgentListFlagsAnUnresolvedModelRole(t *testing.T) {
	workingDir := t.TempDir()
	writeProjectSubagent(t, workingDir, "research", "Deep research.", "\"@research\"", "Dig deep.")

	c := newSkillTestCmd(t, runAgentList, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "@research (unresolved)")
}

func TestAgentListWithNoSubagents(t *testing.T) {
	c := newSkillTestCmd(t, runAgentList, t.TempDir(), t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "No subagents found.")
}

func TestAgentListJSON(t *testing.T) {
	workingDir := t.TempDir()
	writeProjectSubagent(t, workingDir, "frontend", "Handles UI work.", "", "Do UI things.")

	c := newSkillTestCmd(t, runAgentList, workingDir, t.TempDir())
	c.Flags().BoolVar(&agentListJSON, "json", true, "")
	t.Cleanup(func() { agentListJSON = false })
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))

	var got []jsonSubagent
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Equal(t, []jsonSubagent{
		{Name: "frontend", Description: "Handles UI work.", ModelResolves: true},
	}, got)
}

func TestAgentShowRendersTheSubagent(t *testing.T) {
	workingDir := t.TempDir()
	writeProjectSubagent(t, workingDir, "frontend", "Handles UI work.", "", "Do UI things.")

	c := newSkillTestCmd(t, runAgentShow, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"frontend"}))
	got := out.String()
	require.Contains(t, got, "name: frontend")
	require.Contains(t, got, "Do UI things.")
}

func TestAgentShowUnknownName(t *testing.T) {
	c := newSkillTestCmd(t, runAgentShow, t.TempDir(), t.TempDir())
	require.Error(t, c.RunE(c, []string{"nope"}))
}

func newAgentNewTestCmd(t *testing.T, workingDir string) *cobra.Command {
	t.Helper()
	c := newSkillTestCmd(t, runAgentNew, workingDir, t.TempDir())
	c.Flags().StringVarP(&agentNewDescription, "description", "d", "", "")
	c.Flags().StringVarP(&agentNewModel, "model", "m", "", "")
	c.Flags().BoolVar(&agentNewUser, "user", false, "")
	t.Cleanup(func() {
		agentNewDescription = ""
		agentNewModel = ""
		agentNewUser = false
	})
	return c
}

func TestAgentNewCreatesAFile(t *testing.T) {
	workingDir := t.TempDir()
	c := newAgentNewTestCmd(t, workingDir)
	require.NoError(t, c.Flags().Set("description", "Backend work."))
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"backend"}))

	path := filepath.Join(workingDir, ".atlas", "agents", "backend.md")
	require.FileExists(t, path)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(content), "Backend work.")
}

func TestAgentNewRefusesToOverwrite(t *testing.T) {
	workingDir := t.TempDir()
	writeProjectSubagent(t, workingDir, "backend", "Existing.", "", "Body.")

	c := newAgentNewTestCmd(t, workingDir)
	err := c.RunE(c, []string{"backend"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

func TestAgentRemoveDeletesTheFile(t *testing.T) {
	workingDir := t.TempDir()
	writeProjectSubagent(t, workingDir, "old", "Old.", "", "Body.")

	c := newSkillTestCmd(t, runAgentRemove, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"old"}))
	require.NoFileExists(t, filepath.Join(workingDir, ".atlas", "agents", "old.md"))
}

func TestAgentRemoveRejectsUnknownName(t *testing.T) {
	c := newSkillTestCmd(t, runAgentRemove, t.TempDir(), t.TempDir())
	require.Error(t, c.RunE(c, []string{"nope"}))
}
