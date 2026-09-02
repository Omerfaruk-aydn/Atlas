package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/skills"
	"github.com/stretchr/testify/require"
)

func TestPrintSkillStatesWithNoFiles(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, printSkillStates(&out, nil))
	require.Contains(t, out.String(), "No skill files found.")
}

func TestPrintSkillStatesReportsOnlyFailures(t *testing.T) {
	var out bytes.Buffer
	err := printSkillStates(&out, []*skills.SkillState{
		{Name: "good", Path: "/skills/good/SKILL.md", State: skills.StateNormal},
		{Name: "bad", Path: "/skills/bad/SKILL.md", State: skills.StateError, Err: errors.New("missing description")},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "1 of 2 skill files failed")
	require.Contains(t, out.String(), "missing description")
	require.NotContains(t, out.String(), "good")
}

func TestPrintSkillStatesSucceedsWhenAllLoad(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, printSkillStates(&out, []*skills.SkillState{
		{Name: "one", Path: "/skills/one/SKILL.md", State: skills.StateNormal},
	}))
	require.Contains(t, out.String(), "All 1 skill files loaded.")
}

// End to end: a skill file whose frontmatter is broken has to be reported
// by name, since that is the only place the failure surfaces at all.
func TestSkillValidateReportsABrokenSkillFile(t *testing.T) {
	workingDir := t.TempDir()
	skillsDir := filepath.Join(workingDir, "skills", "broken")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "SKILL.md"),
		[]byte("---\nname: broken\n---\n\nno description in the frontmatter.\n"), 0o644))
	writeAtlasConfig(t, workingDir, `{"options":{"skills_paths":["skills"]}}`)

	c := newSkillTestCmd(t, runSkillValidate, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	err := c.RunE(c, nil)
	require.Error(t, err)
	require.Contains(t, out.String(), "broken")
}
