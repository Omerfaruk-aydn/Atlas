package codeintel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeTestGenFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestGenerateTestSkeletonNoParamsNoResult(t *testing.T) {
	dir := t.TempDir()
	writeTestGenFile(t, dir, "a.go", "package a\n\nfunc Foo() {}\n")

	got, err := GenerateTestSkeleton(dir, "Foo")
	require.NoError(t, err)
	require.Contains(t, got.Skeleton, "func TestFoo(t *testing.T) {")
	require.Contains(t, got.Skeleton, "Foo()")
	require.NotContains(t, got.Skeleton, "want")
	require.Equal(t, []string{"testing"}, got.Imports)
}

func TestGenerateTestSkeletonWithParamsAndError(t *testing.T) {
	dir := t.TempDir()
	writeTestGenFile(t, dir, "a.go", "package a\n\nfunc Divide(a, b int) (int, error) { return a / b, nil }\n")

	got, err := GenerateTestSkeleton(dir, "Divide")
	require.NoError(t, err)
	require.Contains(t, got.Skeleton, "a int")
	require.Contains(t, got.Skeleton, "b int")
	require.Contains(t, got.Skeleton, "want int")
	require.Contains(t, got.Skeleton, "wantErr bool")
	require.Contains(t, got.Skeleton, "got, err := Divide(tt.a, tt.b)")
	require.Contains(t, got.Skeleton, "require.Equal(t, tt.wantErr, err != nil)")
	require.Contains(t, got.Imports, "github.com/stretchr/testify/require")
}

func TestGenerateTestSkeletonErrorOnlyResult(t *testing.T) {
	dir := t.TempDir()
	writeTestGenFile(t, dir, "a.go", "package a\n\nfunc Validate(s string) error { return nil }\n")

	got, err := GenerateTestSkeleton(dir, "Validate")
	require.NoError(t, err)
	require.Contains(t, got.Skeleton, "err := Validate(tt.s)")
	require.Contains(t, got.Skeleton, "wantErr bool")
	require.NotContains(t, got.Skeleton, "want ")
}

func TestGenerateTestSkeletonSingleNonErrorResult(t *testing.T) {
	dir := t.TempDir()
	writeTestGenFile(t, dir, "a.go", "package a\n\nfunc Double(n int) int { return n * 2 }\n")

	got, err := GenerateTestSkeleton(dir, "Double")
	require.NoError(t, err)
	require.Contains(t, got.Skeleton, "want int")
	require.Contains(t, got.Skeleton, "got := Double(tt.n)")
	require.Contains(t, got.Skeleton, "require.Equal(t, tt.want, got)")
	require.NotContains(t, got.Skeleton, "wantErr")
}

func TestGenerateTestSkeletonUsesContextBackground(t *testing.T) {
	dir := t.TempDir()
	writeTestGenFile(t, dir, "a.go", "package a\n\nimport \"context\"\n\nfunc Fetch(ctx context.Context, id string) error { return nil }\n")

	got, err := GenerateTestSkeleton(dir, "Fetch")
	require.NoError(t, err)
	require.Contains(t, got.Skeleton, "Fetch(context.Background(), tt.id)")
	require.NotContains(t, got.Skeleton, "ctx string")
	require.Contains(t, got.Imports, "context")
}

func TestGenerateTestSkeletonHandlesUnexportedFunc(t *testing.T) {
	dir := t.TempDir()
	writeTestGenFile(t, dir, "a.go", "package a\n\nfunc helper() {}\n")

	got, err := GenerateTestSkeleton(dir, "helper")
	require.NoError(t, err)
	require.Contains(t, got.Skeleton, "func TestHelper(t *testing.T) {")
}

func TestGenerateTestSkeletonRejectsAMethod(t *testing.T) {
	dir := t.TempDir()
	writeTestGenFile(t, dir, "a.go", "package a\n\ntype Client struct{}\n\nfunc (c *Client) Do() error { return nil }\n")

	_, err := GenerateTestSkeleton(dir, "Do")
	require.Error(t, err)
	require.Contains(t, err.Error(), "method")
}

func TestGenerateTestSkeletonReportsNotFound(t *testing.T) {
	dir := t.TempDir()
	writeTestGenFile(t, dir, "a.go", "package a\n\nfunc Foo() {}\n")

	_, err := GenerateTestSkeleton(dir, "Missing")
	require.Error(t, err)
}

func TestGenerateTestSkeletonHandlesManyReturnValues(t *testing.T) {
	dir := t.TempDir()
	writeTestGenFile(t, dir, "a.go", "package a\n\nfunc Split(s string) (string, string, error) { return s, s, nil }\n")

	got, err := GenerateTestSkeleton(dir, "Split")
	require.NoError(t, err)
	require.Contains(t, got.Skeleton, "TODO: capture and assert 3 return values")
}
