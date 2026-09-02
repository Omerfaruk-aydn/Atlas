package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderRoundTrips(t *testing.T) {
	t.Parallel()

	in := &Skill{
		Name:          "release-checklist",
		Description:   "Use when cutting a release, before tagging.",
		UserInvocable: true,
		Instructions:  "1. Run the tests.\n2. Tag.\n",
	}

	content, err := Render(in)
	require.NoError(t, err)

	out, err := ParseContent(content)
	require.NoError(t, err)
	require.Equal(t, in.Name, out.Name)
	require.Equal(t, in.Description, out.Description)
	require.True(t, out.UserInvocable)
	require.Equal(t, "1. Run the tests.\n2. Tag.", out.Instructions)
}

func TestRenderOmitsWhatWasNotSet(t *testing.T) {
	t.Parallel()

	content, err := Render(&Skill{Name: "a-skill", Description: "d", Instructions: "body"})
	require.NoError(t, err)

	rendered := string(content)
	require.NotContains(t, rendered, "license")
	require.NotContains(t, rendered, "user-invocable")
	require.NotContains(t, rendered, "metadata")
}

// name has to come first: a generated file that a person then edits should
// not look generated.
func TestRenderPutsNameFirst(t *testing.T) {
	t.Parallel()

	content, err := Render(&Skill{Name: "zeta", Description: "alpha", Instructions: "body"})
	require.NoError(t, err)

	rendered := string(content)
	require.Less(t, strings.Index(rendered, "name:"), strings.Index(rendered, "description:"))
}

func TestSaveWritesWhereDiscoveryLooks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	path, err := Save(dir, &Skill{
		Name:         "build-quirks",
		Description:  "Use when a build fails in a way the README does not explain.",
		Instructions: "Check the vendored toolchain first.",
	})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "build-quirks", SkillFileName), path)

	found := Discover([]string{dir})
	require.Len(t, found, 1)
	require.Equal(t, "build-quirks", found[0].Name)
}

func TestSaveRefusesAnInvalidSkill(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := Save(dir, &Skill{Name: "Not A Name", Description: "d", Instructions: "b"})
	require.Error(t, err)

	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	require.Empty(t, entries, "a skill that does not parse would haunt every later session; nothing should be written")
}

func TestDeleteOnlyRemovesASkill(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	notASkill := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(notASkill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(notASkill, "main.go"), []byte("package main"), 0o644))

	require.Error(t, Delete(dir, "src"), "a directory without a SKILL.md is somebody's code")
	require.DirExists(t, notASkill)

	_, err := Save(dir, &Skill{Name: "temp-skill", Description: "d", Instructions: "b"})
	require.NoError(t, err)
	require.NoError(t, Delete(dir, "temp-skill"))
	require.NoDirExists(t, filepath.Join(dir, "temp-skill"))
}

func TestFindIgnoresCase(t *testing.T) {
	t.Parallel()

	all := []*Skill{{Name: "one"}, {Name: "two"}}
	got, ok := Find(all, "TWO")
	require.True(t, ok)
	require.Equal(t, "two", got.Name)

	_, ok = Find(all, "three")
	require.False(t, ok)
}

func TestPatchSwapsOnlyTheMatchingText(t *testing.T) {
	t.Parallel()

	in := &Skill{
		Name:         "deploy",
		Description:  "d",
		Instructions: "1. Build.\n2. Run the old script.\n3. Verify.",
	}

	out, err := Patch(in, "Run the old script.", "Run deploy.sh.")
	require.NoError(t, err)
	require.Equal(t, "1. Build.\n2. Run deploy.sh.\n3. Verify.", out.Instructions)
	require.Equal(t, in.Description, out.Description, "patch only touches instructions")
	require.Equal(t, "1. Build.\n2. Run the old script.\n3. Verify.", in.Instructions, "the original is not mutated")
}

func TestPatchRequiresExactlyOneMatch(t *testing.T) {
	t.Parallel()

	_, err := Patch(&Skill{Instructions: "no match here"}, "missing", "x")
	require.ErrorIs(t, err, ErrPatchNotFound)

	_, err = Patch(&Skill{Instructions: "dup dup"}, "dup", "x")
	require.ErrorIs(t, err, ErrPatchAmbiguous)
}
