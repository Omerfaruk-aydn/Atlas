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

// newSkillTestCmd builds a standalone command running the given RunE
// against an isolated working directory and data directory, without
// touching the package-level skillCmd/rootCmd singletons other tests in
// this package also use.
func newSkillTestCmd(t *testing.T, runE func(*cobra.Command, []string) error, cwd, dataDir string) *cobra.Command {
	t.Helper()

	// ResolveCwd os.Chdir's the whole process when --cwd is set, so this
	// restores the real working directory once the test is done rather
	// than leaving every later test in this package running from a
	// deleted temp dir.
	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(orig) })

	c := &cobra.Command{RunE: runE}
	c.Flags().String("cwd", cwd, "")
	c.Flags().String("data-dir", dataDir, "")
	c.Flags().Bool("debug", false, "")
	c.SetContext(t.Context())
	return c
}

func writeProjectSkill(t *testing.T, workingDir, name, description, instructions string) {
	t.Helper()
	dir := filepath.Join(workingDir, ".atlas", "skills", name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + instructions + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
}

func TestSkillListShowsProjectSkills(t *testing.T) {
	workingDir := t.TempDir()
	writeProjectSkill(t, workingDir, "deploy-steps", "Use when deploying.", "1. Build.\n2. Ship.")

	c := newSkillTestCmd(t, runSkillList, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "deploy-steps (project, enabled)")
	require.Contains(t, out.String(), "Use when deploying.")
}

func TestSkillListMarksDisabledSkills(t *testing.T) {
	workingDir := t.TempDir()
	writeProjectSkill(t, workingDir, "turned-off", "d", "b")

	dataDir := t.TempDir()
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	// Disable it via the project atlas.json rather than fighting config
	// precedence with a data-dir file.
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "atlas.json"),
		[]byte(`{"options":{"disabled_skills":["turned-off"]}}`), 0o644))

	c := newSkillTestCmd(t, runSkillList, workingDir, dataDir)
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "turned-off (project, disabled)")
}

func TestSkillListJSON(t *testing.T) {
	workingDir := t.TempDir()
	writeProjectSkill(t, workingDir, "deploy-steps", "Use when deploying.", "1. Build.\n2. Ship.")

	c := newSkillTestCmd(t, runSkillList, workingDir, t.TempDir())
	c.Flags().BoolVar(&skillListJSON, "json", true, "")
	t.Cleanup(func() { skillListJSON = false })
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))

	var got []jsonSkill
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))

	var found bool
	for _, s := range got {
		if s.Name == "deploy-steps" {
			found = true
			require.Equal(t, "project", s.Origin)
			require.True(t, s.Enabled)
			require.Equal(t, "Use when deploying.", s.Description)
		}
	}
	require.True(t, found, "deploy-steps should be in the JSON listing")
}

// TestSkillListWithNoProjectSkillsStillShowsBuiltins covers the
// "No skills found." message's other side: with no project or user skills
// at all, discovery still finds the builtins, so that message is only ever
// reachable if a workspace somehow has none of those either.
func TestSkillListWithNoProjectSkillsStillShowsBuiltins(t *testing.T) {
	workingDir := t.TempDir()
	c := newSkillTestCmd(t, runSkillList, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "(builtin, enabled)")
}

func TestSkillShowRendersTheSkill(t *testing.T) {
	workingDir := t.TempDir()
	writeProjectSkill(t, workingDir, "release-checklist", "Use before releasing.", "Run the tests first.")

	c := newSkillTestCmd(t, runSkillShow, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"release-checklist"}))
	require.Contains(t, out.String(), "name: release-checklist")
	require.Contains(t, out.String(), "Run the tests first.")
}

func TestSkillShowUnknown(t *testing.T) {
	workingDir := t.TempDir()
	c := newSkillTestCmd(t, runSkillShow, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	err := c.RunE(c, []string{"no-such-skill"})
	require.Error(t, err)
}
