package lint

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// lintModule writes a real Go module. Testing a linter wrapper against
// canned output tests the fixture, not the wrapper.
func lintModule(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/lintmod\n\ngo 1.24\n"), 0o644))
	for name, src := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(src), 0o644))
	}
	return dir
}

// vetCatchesThis has a Printf verb mismatch, which is one of the few
// things vet reliably reports on every toolchain version.
const vetCatchesThis = `package a

import "fmt"

func Bad() {
	fmt.Printf("%d\n", "not a number")
}
`

func TestRunFindsAVetIssue(t *testing.T) {
	dir := lintModule(t, map[string]string{"a.go": vetCatchesThis})

	got, err := Run(context.Background(), dir, Options{ForceVet: true})
	require.NoError(t, err)
	require.NotEmpty(t, got.Issues)
	require.Equal(t, "go vet", got.Tool)
	require.Equal(t, "a.go", got.Issues[0].File)
	require.Positive(t, got.Issues[0].Line)
	require.Contains(t, got.Issues[0].Message, "%d")
}

func TestRunReportsNothingOnCleanCode(t *testing.T) {
	dir := lintModule(t, map[string]string{
		"a.go": `package a

func Add(x, y int) int { return x + y }
`,
	})

	got, err := Run(context.Background(), dir, Options{ForceVet: true})
	require.NoError(t, err)
	require.Empty(t, got.Issues)
}

// A mixed absolute/relative report is hard to scan, and the two tools
// disagree about which they print.
func TestRunReportsPathsRelativeToTheLintedDirectory(t *testing.T) {
	dir := lintModule(t, map[string]string{"pkg/a.go": vetCatchesThis})

	got, err := Run(context.Background(), dir, Options{ForceVet: true})
	require.NoError(t, err)
	require.NotEmpty(t, got.Issues)
	require.Equal(t, "pkg/a.go", got.Issues[0].File)
	require.NotContains(t, got.Issues[0].File, dir)
}

// A reader fixing one file wants all of its findings together.
func TestRunGroupsIssuesByFileAndPosition(t *testing.T) {
	dir := lintModule(t, map[string]string{
		"zzz.go": `package a

import "fmt"

func BadZ() {
	fmt.Printf("%d\n", "x")
	fmt.Printf("%d\n", "y")
}
`,
		"aaa.go": `package a

import "fmt"

func BadA() {
	fmt.Printf("%d\n", "z")
}
`,
	})

	got, err := Run(context.Background(), dir, Options{ForceVet: true})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(got.Issues), 3)
	require.Equal(t, "aaa.go", got.Issues[0].File)
	// Within a file, ascending by line.
	for i := 1; i < len(got.Issues); i++ {
		if got.Issues[i].File == got.Issues[i-1].File {
			require.GreaterOrEqual(t, got.Issues[i].Line, got.Issues[i-1].Line)
		}
	}
}

func TestRunHonoursTheIssueCap(t *testing.T) {
	dir := lintModule(t, map[string]string{
		"a.go": `package a

import "fmt"

func Bad() {
	fmt.Printf("%d\n", "a")
	fmt.Printf("%d\n", "b")
	fmt.Printf("%d\n", "c")
}
`,
	})

	got, err := Run(context.Background(), dir, Options{ForceVet: true, MaxIssues: 2})
	require.NoError(t, err)
	require.Len(t, got.Issues, 2)
	require.True(t, got.Truncated)
}

func TestRunNarrowsToAPackage(t *testing.T) {
	dir := lintModule(t, map[string]string{
		"clean/a.go": `package clean

func Fine() {}
`,
		"dirty/b.go": `package dirty

import "fmt"

func Bad() { fmt.Printf("%d\n", "x") }
`,
	})

	got, err := Run(context.Background(), dir, Options{ForceVet: true, Packages: "./clean/..."})
	require.NoError(t, err)
	require.Empty(t, got.Issues)
}

// Falling back has to be visible: vet finds far less than a configured
// golangci-lint, and a caller told "no issues" deserves to know which
// tool said so.
func TestRunMarksTheVetFallback(t *testing.T) {
	dir := lintModule(t, map[string]string{"a.go": vetCatchesThis})

	// ForceVet is an explicit choice, not a fallback.
	explicit, err := Run(context.Background(), dir, Options{ForceVet: true})
	require.NoError(t, err)
	require.False(t, explicit.Fallback)
}

func TestRunCountsIssuesByLinter(t *testing.T) {
	dir := lintModule(t, map[string]string{"a.go": vetCatchesThis})

	got, err := Run(context.Background(), dir, Options{ForceVet: true})
	require.NoError(t, err)
	counts := ByLinter(got.Issues)
	require.Positive(t, counts["vet"])
}

// vet prints a "# package" banner between findings; treating it as a
// finding would put noise at the top of every report.
func TestRunSkipsPackageBanners(t *testing.T) {
	dir := lintModule(t, map[string]string{"a.go": vetCatchesThis})

	got, err := Run(context.Background(), dir, Options{ForceVet: true})
	require.NoError(t, err)
	for _, issue := range got.Issues {
		require.NotContains(t, issue.File, "#")
		require.Positive(t, issue.Line)
	}
}

func TestVetLineParsesWithAndWithoutAColumn(t *testing.T) {
	m := vetLine.FindStringSubmatch("a.go:12:5: something is wrong")
	require.NotNil(t, m)
	require.Equal(t, "a.go", m[1])
	require.Equal(t, "12", m[2])
	require.Equal(t, "5", m[3])
	require.Equal(t, "something is wrong", m[4])

	m = vetLine.FindStringSubmatch("a.go:12: something is wrong")
	require.NotNil(t, m)
	require.Equal(t, "a.go", m[1])
	require.Equal(t, "12", m[2])
	require.Empty(t, m[3])
	require.Equal(t, "something is wrong", m[4])
}

func TestNormalisePathLeavesOutsidePathsAbsolute(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere.go")
	require.Equal(t, filepath.ToSlash(outside), normalisePath(dir, outside))
	require.Empty(t, normalisePath(dir, ""))
	require.Equal(t, "rel/a.go", normalisePath(dir, "rel/a.go"))
}

func TestFirstLineHandlesEmptyOutput(t *testing.T) {
	require.Equal(t, "no output", firstLine("   \n  "))
	require.Equal(t, "first", firstLine("first\nsecond"))
}
