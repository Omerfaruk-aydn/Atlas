package codeintel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func callerNames(r ImpactResult) []string {
	out := make([]string, 0, len(r.Callers))
	for _, c := range r.Callers {
		out = append(out, c.Func.Name)
	}
	return out
}

func TestImpactAnalysisFindsADirectCaller(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func target() int { return 1 }

func caller() int { return target() }

func unrelated() int { return 2 }
`)

	got, err := ImpactAnalysis(dir, "target", 3, false)
	require.NoError(t, err)
	require.Equal(t, []string{"caller"}, callerNames(got))
	require.Equal(t, 1, got.Callers[0].Depth)
	require.Equal(t, "target", got.Callers[0].Via)
}

func TestImpactAnalysisWalksTransitively(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func target() int { return 1 }

func mid() int { return target() }

func top() int { return mid() }
`)

	got, err := ImpactAnalysis(dir, "target", 5, false)
	require.NoError(t, err)
	require.Equal(t, []string{"mid", "top"}, callerNames(got))
	require.Equal(t, 1, got.Callers[0].Depth)
	require.Equal(t, 2, got.Callers[1].Depth)
	// The chain has to be readable: top got here via mid.
	require.Equal(t, "mid", got.Callers[1].Via)
}

func TestImpactAnalysisStopsAtMaxDepth(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func target() int { return 1 }

func d1() int { return target() }
func d2() int { return d1() }
func d3() int { return d2() }
`)

	got, err := ImpactAnalysis(dir, "target", 2, false)
	require.NoError(t, err)
	require.Equal(t, []string{"d1", "d2"}, callerNames(got))
	require.True(t, got.Truncated)

	full, err := ImpactAnalysis(dir, "target", 5, false)
	require.NoError(t, err)
	require.Len(t, full.Callers, 3)
	require.False(t, full.Truncated)
}

// x.Close() carries no type information without a type checker, so a
// method call has to register as an edge on the bare name or method
// callers would be missed entirely.
func TestImpactAnalysisSeesMethodCallsThroughASelector(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type T struct{}

func (t T) Close() error { return nil }

func shutdown(t T) error { return t.Close() }
`)

	got, err := ImpactAnalysis(dir, "Close", 3, false)
	require.NoError(t, err)
	require.Equal(t, []string{"shutdown"}, callerNames(got))
	require.Equal(t, "T", got.Target.Recv)
}

// The flip side of matching on the bare name: two unrelated Close methods
// are indistinguishable, and the caller has to be told so.
func TestImpactAnalysisReportsAmbiguousDeclarations(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type A struct{}
type B struct{}

func (a A) Close() error { return nil }
func (b B) Close() error { return nil }

func shut(a A) error { return a.Close() }
`)

	got, err := ImpactAnalysis(dir, "Close", 3, false)
	require.NoError(t, err)
	require.Len(t, got.Ambiguous, 1)
	require.Equal(t, []string{"shut"}, callerNames(got))
}

// Recursion is a cycle in the reverse graph; visiting each function once
// is what keeps it from looping forever.
func TestImpactAnalysisTerminatesOnRecursion(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func target() int { return helper() }

func helper() int { return target() }
`)

	got, err := ImpactAnalysis(dir, "target", 10, false)
	require.NoError(t, err)
	require.Equal(t, []string{"helper"}, callerNames(got))
}

func TestImpactAnalysisReportsNothingForAnUnknownSymbol(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func f() {}
`)

	got, err := ImpactAnalysis(dir, "nonexistent", 3, false)
	require.NoError(t, err)
	require.Empty(t, got.Callers)
	require.Equal(t, "nonexistent", got.Target.Name)
	require.Zero(t, got.Target.Line)
}

func TestImpactAnalysisRequiresASymbol(t *testing.T) {
	_, err := ImpactAnalysis(t.TempDir(), "", 3, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "symbol is required")
}

func TestImpactAnalysisCrossesFiles(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func target() int { return 1 }
`)
	writeGo(t, dir, "b.go", `package a

func caller() int { return target() }
`)

	got, err := ImpactAnalysis(dir, "target", 3, false)
	require.NoError(t, err)
	require.Equal(t, []string{"caller"}, callerNames(got))
}

func TestImpactAnalysisHonoursIncludeTests(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func target() int { return 1 }
`)
	writeGo(t, dir, "a_test.go", `package a

import "testing"

func TestTarget(t *testing.T) { _ = target() }
`)

	got, err := ImpactAnalysis(dir, "target", 3, false)
	require.NoError(t, err)
	require.Empty(t, got.Callers)

	withTests, err := ImpactAnalysis(dir, "target", 3, true)
	require.NoError(t, err)
	require.Equal(t, []string{"TestTarget"}, callerNames(withTests))
}

// A caller that calls the target ten times in a loop is still one caller.
func TestImpactAnalysisReportsEachCallerOnce(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func target() int { return 1 }

func caller() int {
	n := 0
	for range 10 {
		n += target()
	}
	return n + target()
}
`)

	got, err := ImpactAnalysis(dir, "target", 3, false)
	require.NoError(t, err)
	require.Len(t, got.Callers, 1)
}

func TestImpactAnalysisRecordsTargetLocation(t *testing.T) {
	dir := t.TempDir()
	path := writeGo(t, dir, "a.go", `package a

func target() int { return 1 }
`)

	got, err := ImpactAnalysis(dir, "target", 3, false)
	require.NoError(t, err)
	require.Equal(t, path, got.Target.File)
	require.Equal(t, 3, got.Target.Line)
	require.Equal(t, "a", got.Target.Package)
}

// maxDepth below 1 would return an empty result for a symbol that plainly
// has callers, which reads as "nothing depends on this" -- the most
// dangerous wrong answer this tool can give.
func TestImpactAnalysisClampsANonsensicalDepth(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func target() int { return 1 }

func caller() int { return target() }
`)

	got, err := ImpactAnalysis(dir, "target", 0, false)
	require.NoError(t, err)
	require.Len(t, got.Callers, 1)
}

func TestImpactAnalysisSeesGenericCalls(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func target[T any](v T) T { return v }

func caller() int { return target[int](1) }
`)

	got, err := ImpactAnalysis(dir, "target", 3, false)
	require.NoError(t, err)
	require.Equal(t, []string{"caller"}, callerNames(got))
}
