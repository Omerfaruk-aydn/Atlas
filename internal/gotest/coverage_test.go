package gotest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeProfile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cover.out")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func fileCov(t *testing.T, c Coverage, name string) FileCoverage {
	t.Helper()
	for _, f := range c.Files {
		if f.File == name {
			return f
		}
	}
	t.Fatalf("no coverage for %q", name)
	return FileCoverage{}
}

func TestParseCoverageCountsStatementsNotLines(t *testing.T) {
	path := writeProfile(t, `mode: set
example.com/m/a.go:3.20,5.10 4 1
example.com/m/a.go:7.20,9.10 6 0
`)

	got, err := ParseCoverageFile(path)
	require.NoError(t, err)
	require.Equal(t, "set", got.Mode)
	require.Equal(t, 10, got.Statements)
	require.Equal(t, 4, got.Covered)
	require.InDelta(t, 40.0, got.Percent(), 0.01)
}

// "What is not covered" is the actual question; a percentage alone does
// not answer it.
func TestParseCoverageListsUncoveredBlocks(t *testing.T) {
	path := writeProfile(t, `mode: set
example.com/m/a.go:3.20,5.10 4 1
example.com/m/a.go:7.20,9.10 6 0
example.com/m/a.go:11.2,12.3 1 0
`)

	got, err := ParseCoverageFile(path)
	require.NoError(t, err)
	blocks := fileCov(t, got, "example.com/m/a.go").UncoveredBlocks
	require.Len(t, blocks, 2)
	require.Equal(t, 7, blocks[0].StartLine)
	require.Equal(t, 9, blocks[0].EndLine)
	require.Equal(t, 11, blocks[1].StartLine)
}

// Several test binaries can each emit a record for the same block. The
// last one to run may not have reached it, and overwriting would then
// report covered code as uncovered.
func TestParseCoverageAccumulatesRepeatedBlocks(t *testing.T) {
	path := writeProfile(t, `mode: atomic
example.com/m/a.go:3.20,5.10 4 1
example.com/m/a.go:3.20,5.10 4 0
`)

	got, err := ParseCoverageFile(path)
	require.NoError(t, err)
	require.Equal(t, 4, got.Statements)
	require.Equal(t, 4, got.Covered)
	require.Empty(t, fileCov(t, got, "example.com/m/a.go").UncoveredBlocks)
}

func TestParseCoverageAggregatesPerPackage(t *testing.T) {
	path := writeProfile(t, `mode: set
example.com/m/pkg1/a.go:1.1,2.2 10 1
example.com/m/pkg2/b.go:1.1,2.2 10 0
`)

	got, err := ParseCoverageFile(path)
	require.NoError(t, err)
	require.Len(t, got.Packages, 2)
	// Least covered first.
	require.Equal(t, "example.com/m/pkg2", got.Packages[0].Package)
	require.InDelta(t, 0.0, got.Packages[0].Percent(), 0.01)
	require.InDelta(t, 100.0, got.Packages[1].Percent(), 0.01)
}

// A list sorted by name buries the answer the report exists to give.
func TestParseCoverageSortsLeastCoveredFirst(t *testing.T) {
	path := writeProfile(t, `mode: set
example.com/m/aaa.go:1.1,2.2 10 1
example.com/m/zzz.go:1.1,2.2 10 0
`)

	got, err := ParseCoverageFile(path)
	require.NoError(t, err)
	require.Equal(t, "example.com/m/zzz.go", got.Files[0].File)
}

// A file with nothing to cover is not 0% covered, and reporting it that
// way drags the average down for no reason.
func TestFilePercentTreatsNoStatementsAsFullyCovered(t *testing.T) {
	require.InDelta(t, 100.0, FileCoverage{}.Percent(), 0.01)
	require.InDelta(t, 100.0, PackageCoverage{}.Percent(), 0.01)
}

// A Windows path or a host:port-shaped segment contains colons; splitting
// on the first would cut the filename in half.
func TestParseBlockLineSplitsOnTheLastColon(t *testing.T) {
	got, err := parseBlockLine(`C:/work/repo/a.go:3.20,5.10 4 1`)
	require.NoError(t, err)
	require.Equal(t, "C:/work/repo/a.go", got.File)
	require.Equal(t, 3, got.StartLine)
	require.Equal(t, 20, got.StartCol)
	require.Equal(t, 5, got.EndLine)
	require.Equal(t, 10, got.EndCol)
	require.Equal(t, 4, got.Statements)
	require.Equal(t, 1, got.Count)
}

func TestParseBlockLineRejectsMalformedRecords(t *testing.T) {
	for _, bad := range []string{
		"no-colon-here",
		"a.go:3.20,5.10 4",
		"a.go:garbage 4 1",
		"a.go:3.20 4 1",
		"a.go:3.20,5.10 x 1",
		"a.go:3.20,5.10 4 x",
	} {
		_, err := parseBlockLine(bad)
		require.Error(t, err, "expected %q to be rejected", bad)
	}
}

// One malformed line in a profile must not lose the rest of it.
func TestParseCoverageSkipsMalformedLines(t *testing.T) {
	path := writeProfile(t, `mode: set
this line is nonsense
example.com/m/a.go:3.20,5.10 4 1
`)

	got, err := ParseCoverageFile(path)
	require.NoError(t, err)
	require.Equal(t, 4, got.Statements)
}

func TestParseCoverageFailsClearlyOnAMissingFile(t *testing.T) {
	_, err := ParseCoverageFile(filepath.Join(t.TempDir(), "nope.out"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot read coverage profile")
}

func TestParseCoverageOnAnEmptyProfile(t *testing.T) {
	path := writeProfile(t, "mode: set\n")

	got, err := ParseCoverageFile(path)
	require.NoError(t, err)
	require.Empty(t, got.Files)
	require.Zero(t, got.Percent())
}

// The parser has to agree with what the toolchain actually writes, not
// with a format remembered from documentation.
func TestParseCoverageAgainstARealProfile(t *testing.T) {
	dir := testModule(t, map[string]string{
		"a.go": `package a

func Covered(x int) int {
	if x > 0 {
		return x
	}
	return -x
}

func NeverCalled() string {
	return "nobody runs this"
}
`,
		"a_test.go": `package a

import "testing"

func TestCovered(t *testing.T) {
	if Covered(1) != 1 {
		t.Fatal("wrong")
	}
}
`,
	})

	profile := filepath.Join(dir, "cover.out")
	_, err := Run(context.Background(), dir, Options{CoverProfile: profile})
	require.NoError(t, err)

	got, err := ParseCoverageFile(profile)
	require.NoError(t, err)
	require.NotEmpty(t, got.Files)
	require.Positive(t, got.Statements)
	// Something ran and something did not, so coverage is strictly
	// between the extremes.
	require.Greater(t, got.Percent(), 0.0)
	require.Less(t, got.Percent(), 100.0)

	// NeverCalled's body must show up as an uncovered block.
	var uncovered int
	for _, f := range got.Files {
		uncovered += len(f.UncoveredBlocks)
	}
	require.Positive(t, uncovered)
}
