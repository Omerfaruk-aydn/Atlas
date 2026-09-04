package gitx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSuggestCommitTypeAllTestFiles(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a_test.go", "package a\n")
	git(t, dir, "add", "a_test.go")

	got, err := SuggestCommitType(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "test", got.Type)
	require.Equal(t, "high", got.Confidence)
}

func TestSuggestCommitTypeAllDocs(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "README.md", "# hi\n")
	git(t, dir, "add", "README.md")

	got, err := SuggestCommitType(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "docs", got.Type)
	require.Equal(t, "high", got.Confidence)
}

func TestSuggestCommitTypeAllCI(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, ".github/workflows/ci.yml", "name: ci\n")
	git(t, dir, "add", "-A")

	got, err := SuggestCommitType(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "ci", got.Type)
	require.Equal(t, "high", got.Confidence)
}

func TestSuggestCommitTypeDependencyBump(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "go.mod", "module a\n")
	commit(t, dir, "init")
	writeFile(t, dir, "go.mod", "module a\ngo 1.22\n")
	git(t, dir, "add", "go.mod")

	got, err := SuggestCommitType(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "build", got.Type)
}

func TestSuggestCommitTypeNewSourceFileSuggestsFeat(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "existing.go", "package a\n")
	commit(t, dir, "init")
	writeFile(t, dir, "new.go", "package a\n\nfunc New() {}\n")
	git(t, dir, "add", "new.go")

	got, err := SuggestCommitType(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "feat", got.Type)
	require.Equal(t, "medium", got.Confidence)
}

func TestSuggestCommitTypeDeletedFileSuggestsChore(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "gone.go", "package a\n")
	commit(t, dir, "init")
	git(t, dir, "rm", "gone.go")

	got, err := SuggestCommitType(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "chore", got.Type)
}

func TestSuggestCommitTypeModifiedOnlySuggestsFixAtLowConfidence(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a.go", "package a\n\nfunc F() {}\n")
	git(t, dir, "add", "a.go")

	got, err := SuggestCommitType(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "fix", got.Type)
	require.Equal(t, "low", got.Confidence)
}

func TestSuggestCommitTypeReportsScope(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "internal/gitx/a.go", "package gitx\n")
	writeFile(t, dir, "internal/gitx/b.go", "package gitx\n")
	git(t, dir, "add", "-A")

	got, err := SuggestCommitType(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "gitx", got.Scope)
}

func TestSuggestCommitTypeNoScopeAcrossUnrelatedDirs(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "internal/gitx/a.go", "package gitx\n")
	writeFile(t, dir, "internal/tools/b.go", "package tools\n")
	git(t, dir, "add", "-A")

	got, err := SuggestCommitType(context.Background(), dir)
	require.NoError(t, err)
	require.Empty(t, got.Scope)
}

func TestSuggestCommitTypeReportsNothingStaged(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	commit(t, dir, "init")

	got, err := SuggestCommitType(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, 0, got.FilesCount)
	require.Empty(t, got.Type)
}

func TestSuggestCommitTypeReportsANonRepository(t *testing.T) {
	_, err := SuggestCommitType(context.Background(), t.TempDir())
	require.ErrorIs(t, err, ErrNotARepository)
}
