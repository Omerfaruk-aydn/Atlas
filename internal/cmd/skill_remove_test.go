package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/skills"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func newRemovableSkill(t *testing.T, root, name string) *skills.Skill {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, skills.SkillFileName), []byte("skill"), 0o644))
	return &skills.Skill{Name: name, Path: dir}
}

func TestSkillRemoveDeletesTheDirectory(t *testing.T) {
	root := t.TempDir()
	skill := newRemovableSkill(t, root, "old-deploy")

	c := &cobra.Command{}
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, removeSkill(c, []*skills.Skill{skill}, "old-deploy"))
	require.NoDirExists(t, skill.Path)
	require.Contains(t, out.String(), "Removed")
}

func TestSkillRemoveRejectsUnknownName(t *testing.T) {
	c := &cobra.Command{}
	require.Error(t, removeSkill(c, nil, "nope"))
}

func TestSkillRemoveRefusesBuiltin(t *testing.T) {
	root := t.TempDir()
	skill := newRemovableSkill(t, root, "builtin-one")
	skill.Builtin = true

	c := &cobra.Command{}
	err := removeSkill(c, []*skills.Skill{skill}, "builtin-one")
	require.Error(t, err)
	require.Contains(t, err.Error(), "builtin")
	require.DirExists(t, skill.Path)
}

// If the skill's directory does not match its own name, something is odd
// about how it was discovered; refuse rather than guess what to delete.
func TestSkillRemoveRefusesMismatchedDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "actual-dir")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	skill := &skills.Skill{Name: "renamed-skill", Path: dir}

	c := &cobra.Command{}
	err := removeSkill(c, []*skills.Skill{skill}, "renamed-skill")
	require.Error(t, err)
	require.DirExists(t, dir)
}
