package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func newSkillNewCmd(t *testing.T, workingDir string) *cobra.Command {
	t.Helper()
	c := newSkillTestCmd(t, runSkillNew, workingDir, t.TempDir())
	c.Flags().StringVarP(&skillNewDescription, "description", "d", "", "")
	c.Flags().BoolVar(&skillNewUser, "user", false, "")
	c.Flags().BoolVar(&skillNewUserInvocable, "user-invocable", false, "")
	t.Cleanup(func() {
		skillNewDescription = ""
		skillNewUser = false
		skillNewUserInvocable = false
	})
	return c
}

func TestSkillNewWritesAProjectSkill(t *testing.T) {
	workingDir := t.TempDir()

	c := newSkillNewCmd(t, workingDir)
	require.NoError(t, c.Flags().Set("description", "Use when deploying."))
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"deploy-steps"}))

	path := filepath.Join(workingDir, ".atlas", "skills", "deploy-steps", "SKILL.md")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(content), "name: deploy-steps")
	require.Contains(t, string(content), "description: Use when deploying.")
	require.Contains(t, string(content), "# deploy-steps")
	require.Contains(t, out.String(), "Created")
}

// A skill is selected by its description alone, so leaving it out has to be
// pointed out rather than quietly accepted.
func TestSkillNewWarnsWhenNoDescriptionIsGiven(t *testing.T) {
	workingDir := t.TempDir()

	c := newSkillNewCmd(t, workingDir)
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"nameless"}))
	require.Contains(t, out.String(), "Set its description")

	content, err := os.ReadFile(filepath.Join(workingDir, ".atlas", "skills", "nameless", "SKILL.md"))
	require.NoError(t, err)
	require.Contains(t, string(content), defaultSkillDescription)
}

func TestSkillNewRefusesToOverwrite(t *testing.T) {
	workingDir := t.TempDir()
	writeProjectSkill(t, workingDir, "existing", "Already here.", "Body.")

	c := newSkillNewCmd(t, workingDir)
	err := c.RunE(c, []string{"existing"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")

	// The original is untouched.
	content, err := os.ReadFile(filepath.Join(workingDir, ".atlas", "skills", "existing", "SKILL.md"))
	require.NoError(t, err)
	require.Contains(t, string(content), "Already here.")
}

func TestSkillNewCanMarkASkillUserInvocable(t *testing.T) {
	workingDir := t.TempDir()

	c := newSkillNewCmd(t, workingDir)
	require.NoError(t, c.Flags().Set("user-invocable", "true"))
	require.NoError(t, c.Flags().Set("description", "Run the checklist."))
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"checklist"}))

	content, err := os.ReadFile(filepath.Join(workingDir, ".atlas", "skills", "checklist", "SKILL.md"))
	require.NoError(t, err)
	require.Contains(t, string(content), "user-invocable: true")
}

// What it writes has to be discoverable, not merely present: the point of
// scaffolding is a file the next session picks up.
func TestSkillNewOutputIsDiscoverable(t *testing.T) {
	workingDir := t.TempDir()

	c := newSkillNewCmd(t, workingDir)
	require.NoError(t, c.Flags().Set("description", "Use when releasing."))
	var out bytes.Buffer
	c.SetOut(&out)
	require.NoError(t, c.RunE(c, []string{"release-checklist"}))

	list := newSkillTestCmd(t, runSkillList, workingDir, t.TempDir())
	var listed bytes.Buffer
	list.SetOut(&listed)
	require.NoError(t, list.RunE(list, nil))
	require.Contains(t, listed.String(), "release-checklist (project, enabled)")
	require.Contains(t, listed.String(), "Use when releasing.")
}
