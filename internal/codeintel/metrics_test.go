package codeintel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func metricFor(t *testing.T, r MetricsResult, name string) FuncMetrics {
	t.Helper()
	for _, m := range r.Functions {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("no metrics for %q", name)
	return FuncMetrics{}
}

func TestMetricsScoresAStraightLineFunctionAsOne(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func flat() int {
	x := 1
	y := 2
	return x + y
}
`)

	got, err := Metrics(dir, false)
	require.NoError(t, err)
	require.Equal(t, 1, metricFor(t, got, "flat").Complexity)
}

func TestMetricsCountsEachBranchPoint(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func branchy(n int) int {
	if n > 0 {
		n++
	}
	for range 3 {
		n++
	}
	return n
}
`)

	// 1 base + 1 if + 1 for.
	require.Equal(t, 3, complexityOf(t, dir, "branchy"))
}

// && and || each introduce a path, which is why gocyclo counts them and
// a report that ignores them understates a guard-heavy function.
func TestMetricsCountsLogicalOperators(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func guard(a, b, c bool) bool {
	if a && b || c {
		return true
	}
	return false
}
`)

	// 1 base + 1 if + 1 && + 1 ||.
	require.Equal(t, 4, complexityOf(t, dir, "guard"))
}

// default is not a decision: it is where control lands when nothing else
// matched. Counting it inflates every switch by one.
func TestMetricsDoesNotCountTheDefaultClause(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func sw(n int) string {
	switch n {
	case 1:
		return "one"
	case 2:
		return "two"
	default:
		return "other"
	}
}
`)

	// 1 base + 2 real cases.
	require.Equal(t, 3, complexityOf(t, dir, "sw"))
}

// Two functions can share a complexity score while one is flat and the
// other is a staircase; nesting is what separates them.
func TestMetricsMeasuresNestingDepth(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func deep(n int) int {
	if n > 0 {
		for range 3 {
			if n > 5 {
				n++
			}
		}
	}
	return n
}

func shallow(a, b, c bool) int {
	if a {
		return 1
	}
	if b {
		return 2
	}
	if c {
		return 3
	}
	return 0
}
`)

	got, err := Metrics(dir, false)
	require.NoError(t, err)
	require.Equal(t, 3, metricFor(t, got, "deep").Nesting)
	require.Equal(t, 1, metricFor(t, got, "shallow").Nesting)
}

func TestMetricsCountsReturns(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func many(n int) int {
	if n == 1 {
		return 1
	}
	if n == 2 {
		return 2
	}
	return 0
}
`)

	got, err := Metrics(dir, false)
	require.NoError(t, err)
	require.Equal(t, 3, metricFor(t, got, "many").Returns)
}

// "a, b int" is two parameters. Counting field groups instead would let a
// long signature hide behind grouped names.
func TestMetricsCountsGroupedParametersIndividually(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func sig(a, b, c int, d string) (int, error) { return 0, nil }
`)

	got, err := Metrics(dir, false)
	require.NoError(t, err)
	m := metricFor(t, got, "sig")
	require.Equal(t, 4, m.Params)
	require.Equal(t, 2, m.Results)
}

func TestMetricsMeasuresFunctionLength(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func five() int {
	x := 1
	y := 2
	return x + y
}
`)

	got, err := Metrics(dir, false)
	require.NoError(t, err)
	require.Equal(t, 5, metricFor(t, got, "five").Lines)
}

func TestMetricsRecordsTheReceiver(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type T struct{}

func (t *T) Method() {}
`)

	got, err := Metrics(dir, false)
	require.NoError(t, err)
	require.Equal(t, "T", metricFor(t, got, "Method").Recv)
}

func TestMetricsSortsTheMostComplexFirst(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func simple() int { return 1 }

func complex_(n int) int {
	if n > 0 {
		for range 3 {
			if n > 5 {
				n++
			}
		}
	}
	return n
}
`)

	got, err := Metrics(dir, false)
	require.NoError(t, err)
	require.Equal(t, "complex_", got.Functions[0].Name)
}

func TestMetricsCountsAFunctionLiteralAsNesting(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func outer() func() int {
	return func() int {
		if true {
			return 1
		}
		return 0
	}
}
`)

	got, err := Metrics(dir, false)
	require.NoError(t, err)
	// The literal is one level, the if inside it a second.
	require.Equal(t, 2, metricFor(t, got, "outer").Nesting)
}

func TestMetricsAggregatesFiles(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func one() {}
func two() {}
`)

	got, err := Metrics(dir, false)
	require.NoError(t, err)
	require.Len(t, got.Files, 1)
	require.Equal(t, 2, got.Files[0].Functions)
	require.Positive(t, got.TotalLines)
}

func TestMetricsHandlesADeclarationWithNoBody(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

//go:linkname stub
func stub()
`)

	got, err := Metrics(dir, false)
	require.NoError(t, err)
	m := metricFor(t, got, "stub")
	require.Equal(t, 1, m.Complexity)
	require.Zero(t, m.Lines)
}

func TestMetricsSkipsFilesThatDoNotParse(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "good.go", `package a

func ok() {}
`)
	writeGo(t, dir, "bad.go", `package a

func ((( {
`)

	got, err := Metrics(dir, false)
	require.NoError(t, err)
	require.Equal(t, 1, got.FilesScanned)
}

func TestMetricsFailsClearlyOnAMissingPath(t *testing.T) {
	_, err := Metrics(t.TempDir()+"/nope", false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot scan")
}

func complexityOf(t *testing.T, dir, name string) int {
	t.Helper()
	got, err := Metrics(dir, false)
	require.NoError(t, err)
	return metricFor(t, got, name).Complexity
}
