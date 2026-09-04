package gitx

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFunctionHistoryFindsCommitsTouchingAFunction(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.go", "package a\n\nfunc Foo() int {\n\treturn 1\n}\n")
	commit(t, dir, "add Foo")
	writeFile(t, dir, "a.go", "package a\n\nfunc Foo() int {\n\treturn 2\n}\n")
	commit(t, dir, "change Foo")

	got, err := FunctionHistory(context.Background(), dir, "a.go", "Foo", 0)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "change Foo", got[0].Subject)
	require.Contains(t, got[0].Diff, "return 2")
	require.Equal(t, "add Foo", got[1].Subject)
}

func TestFunctionHistoryRespectsLimit(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.go", "package a\n\nfunc Foo() int {\n\treturn 1\n}\n")
	commit(t, dir, "add Foo")
	writeFile(t, dir, "a.go", "package a\n\nfunc Foo() int {\n\treturn 2\n}\n")
	commit(t, dir, "change Foo")

	got, err := FunctionHistory(context.Background(), dir, "a.go", "Foo", 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "change Foo", got[0].Subject)
}

func TestFunctionHistoryReportsFunctionNotFound(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.go", "package a\n\nfunc Foo() int {\n\treturn 1\n}\n")
	commit(t, dir, "add Foo")

	_, err := FunctionHistory(context.Background(), dir, "a.go", "NoSuchFunc", 0)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrFunctionNotFound))
}

func TestFunctionHistoryReportsANonRepository(t *testing.T) {
	_, err := FunctionHistory(context.Background(), t.TempDir(), "a.go", "Foo", 0)
	require.ErrorIs(t, err, ErrNotARepository)
}

func TestFunctionHistoryPopulatesAuthorAndHash(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.go", "package a\n\nfunc Foo() int {\n\treturn 1\n}\n")
	commit(t, dir, "add Foo")

	got, err := FunctionHistory(context.Background(), dir, "a.go", "Foo", 0)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "Test <test@example.com>", got[0].Author)
	require.Len(t, got[0].Short, 7)
	require.NotEmpty(t, got[0].Date)
}
