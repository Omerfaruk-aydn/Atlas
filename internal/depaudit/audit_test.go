package depaudit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func auditModule(t *testing.T, goMod string, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644))
	for name, src := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(src), 0o644))
	}
	return dir
}

func TestRunListsTheMainModule(t *testing.T) {
	dir := auditModule(t, "module example.com/auditmod\n\ngo 1.24\n", map[string]string{
		"a.go": "package a\n",
	})

	got, err := Run(context.Background(), dir, Options{SkipUpdates: true, SkipVulns: true})
	require.NoError(t, err)
	require.NotEmpty(t, got.Modules)

	var main Module
	for _, m := range got.Modules {
		if m.Main {
			main = m
		}
	}
	require.Equal(t, "example.com/auditmod", main.Path)
}

// The main module is not an outdated dependency, and listing it as one
// would put a nonsense entry at the top of every report.
func TestOutdatedExcludesTheMainModule(t *testing.T) {
	r := Result{Modules: []Module{
		{Path: "example.com/main", Main: true, Update: "v2.0.0"},
		{Path: "example.com/dep", Version: "v1.0.0", Update: "v1.1.0"},
		{Path: "example.com/current", Version: "v1.0.0"},
	}}

	outdated := r.Outdated()
	require.Len(t, outdated, 1)
	require.Equal(t, "example.com/dep", outdated[0].Path)
}

// "No vulnerabilities" and "vulnerabilities were not checked" are
// different answers, and confusing them is the worst failure this
// package has available.
func TestRunDistinguishesNotCheckedFromNoneFound(t *testing.T) {
	dir := auditModule(t, "module example.com/auditmod\n\ngo 1.24\n", map[string]string{
		"a.go": "package a\n",
	})

	got, err := Run(context.Background(), dir, Options{SkipUpdates: true, SkipVulns: true})
	require.NoError(t, err)
	require.False(t, got.VulnToolAvailable)
	require.Empty(t, got.Vulnerabilities)
}

func TestRunReportsAMissingGovulncheckAsAToolProblem(t *testing.T) {
	if _, err := exec.LookPath("govulncheck"); err == nil {
		t.Skip("govulncheck is installed, so there is no missing-tool case here")
	}
	dir := auditModule(t, "module example.com/auditmod\n\ngo 1.24\n", map[string]string{
		"a.go": "package a\n",
	})

	got, err := Run(context.Background(), dir, Options{SkipUpdates: true})
	require.NoError(t, err)
	require.False(t, got.VulnToolAvailable)
	require.Contains(t, got.VulnToolError, "govulncheck is not installed")
}

// A vulnerability whose code is never called is real but not urgent, and
// a report that cannot tell the two apart stops being read.
func TestParseGovulncheckSeparatesCalledFromPresent(t *testing.T) {
	stream := `
{"osv":{"id":"GO-2024-0001","aliases":["CVE-2024-1111"],"summary":"Reachable problem"}}
{"osv":{"id":"GO-2024-0002","aliases":["CVE-2024-2222"],"summary":"Present but unused"}}
{"finding":{"osv":"GO-2024-0001","fixed_version":"v1.2.3","trace":[{"module":"example.com/vuln","version":"v1.0.0","package":"example.com/vuln/pkg","function":"Exploitable"}]}}
{"finding":{"osv":"GO-2024-0002","fixed_version":"v2.0.0","trace":[{"module":"example.com/other","version":"v1.0.0"}]}}
`
	got, err := parseGovulncheck([]byte(stream))
	require.NoError(t, err)
	require.Len(t, got, 2)

	// Reachable sorts first: that is what needs action today.
	require.Equal(t, "GO-2024-0001", got[0].ID)
	require.True(t, got[0].Called)
	require.Equal(t, "example.com/vuln/pkg.Exploitable", got[0].Trace)
	require.Equal(t, "v1.2.3", got[0].Fixed)
	require.Equal(t, "example.com/vuln", got[0].Module)
	require.Equal(t, "v1.0.0", got[0].Found)
	require.Contains(t, got[0].Aliases, "CVE-2024-1111")

	require.Equal(t, "GO-2024-0002", got[1].ID)
	require.False(t, got[1].Called)
	require.Empty(t, got[1].Trace)
}

func TestCalledCountsOnlyReachableVulnerabilities(t *testing.T) {
	r := Result{Vulnerabilities: []Vulnerability{
		{ID: "a", Called: true},
		{ID: "b", Called: false},
		{ID: "c", Called: true},
	}}
	require.Equal(t, 2, r.Called())
}

// govulncheck emits a stream of single-key objects, not one document,
// so a decoder that stops at the first value would see only one message.
func TestParseGovulncheckReadsTheWholeStream(t *testing.T) {
	stream := `{"osv":{"id":"GO-1","summary":"one"}}
{"osv":{"id":"GO-2","summary":"two"}}
{"osv":{"id":"GO-3","summary":"three"}}
`
	got, err := parseGovulncheck([]byte(stream))
	require.NoError(t, err)
	require.Len(t, got, 3)
}

// A finding can arrive before the advisory that describes it.
func TestParseGovulncheckToleratesFindingsBeforeAdvisories(t *testing.T) {
	stream := `{"finding":{"osv":"GO-1","fixed_version":"v1.0.1","trace":[{"module":"m","version":"v1.0.0","package":"p","function":"F"}]}}
{"osv":{"id":"GO-1","summary":"described later"}}
`
	got, err := parseGovulncheck([]byte(stream))
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "described later", got[0].Summary)
	require.True(t, got[0].Called)
}

func TestParseGovulncheckHandlesEmptyOutput(t *testing.T) {
	got, err := parseGovulncheck(nil)
	require.NoError(t, err)
	require.Empty(t, got)

	got, err = parseGovulncheck([]byte("   \n"))
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestParseGovulncheckRejectsUnparsableOutput(t *testing.T) {
	_, err := parseGovulncheck([]byte("this is not json at all"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "could not be parsed")
}

// A vulnerability with no published fix must not be reported as fixed in
// the empty version.
func TestParseGovulncheckLeavesFixedEmptyWhenThereIsNone(t *testing.T) {
	stream := `{"osv":{"id":"GO-1","summary":"unfixed"}}
{"finding":{"osv":"GO-1","trace":[{"module":"m","version":"v1.0.0","package":"p","function":"F"}]}}
`
	got, err := parseGovulncheck([]byte(stream))
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Empty(t, got[0].Fixed)
}

// Captured verbatim from govulncheck v1.7.0 run against a module using
// gopkg.in/yaml.v2 v2.2.2, so the parser is pinned to what the tool
// actually emits rather than to a reading of its documentation.
//
// Two properties of the real stream that the synthetic cases above do
// not exercise: govulncheck emits SEVERAL findings for one advisory --
// a module-level one with no function, and a symbol-level one with the
// call chain -- and the trace runs from the vulnerable symbol outward to
// the caller, so trace[0] is the affected code and not the entry point.
const realGovulncheckStream = `
{"config":{"protocol_version":"v1.0.0","scanner_name":"govulncheck","scanner_version":"v1.7.0"}}
{"SBOM":{"go_version":"go1.27.0","modules":[{"path":"example.com/vm"},{"path":"gopkg.in/yaml.v2","version":"v2.2.2"}]}}
{"progress":{"message":"Fetching vulnerabilities from the database..."}}
{"osv":{"schema_version":"1.3.1","id":"GO-2020-0036","aliases":["CVE-2019-11253"],"summary":"Excessive resource consumption in YAML parsing"}}
{"osv":{"schema_version":"1.3.1","id":"GO-2021-0061","aliases":["CVE-2019-11254"],"summary":"Panic in yaml parsing"}}
{"osv":{"schema_version":"1.3.1","id":"GO-2022-0956","aliases":["CVE-2022-3064"],"summary":"Denial of service"}}
{"finding":{"osv":"GO-2020-0036","fixed_version":"v2.2.8","trace":[{"module":"gopkg.in/yaml.v2","version":"v2.2.2"}]}}
{"finding":{"osv":"GO-2021-0061","fixed_version":"v2.2.3","trace":[{"module":"gopkg.in/yaml.v2","version":"v2.2.2"}]}}
{"finding":{"osv":"GO-2022-0956","fixed_version":"v2.2.4","trace":[{"module":"gopkg.in/yaml.v2","version":"v2.2.2"}]}}
{"finding":{"osv":"GO-2020-0036","fixed_version":"v2.2.8","trace":[{"module":"gopkg.in/yaml.v2","version":"v2.2.2","package":"gopkg.in/yaml.v2","function":"Unmarshal"},{"module":"example.com/vm","package":"example.com/vm","function":"main"}]}}
{"finding":{"osv":"GO-2021-0061","fixed_version":"v2.2.3","trace":[{"module":"gopkg.in/yaml.v2","version":"v2.2.2","package":"gopkg.in/yaml.v2","function":"Unmarshal"},{"module":"example.com/vm","package":"example.com/vm","function":"main"}]}}
`

func TestParseGovulncheckAgainstRealOutput(t *testing.T) {
	got, err := parseGovulncheck([]byte(realGovulncheckStream))
	require.NoError(t, err)
	require.Len(t, got, 3, "three advisories, despite five findings")

	byID := map[string]Vulnerability{}
	for _, v := range got {
		byID[v.ID] = v
	}

	// Reached through yaml.Unmarshal, so urgent.
	reachable := byID["GO-2020-0036"]
	require.True(t, reachable.Called)
	require.Equal(t, "gopkg.in/yaml.v2.Unmarshal", reachable.Trace,
		"trace[0] is the vulnerable symbol, not the entry point")
	require.Equal(t, "gopkg.in/yaml.v2", reachable.Module)
	require.Equal(t, "v2.2.2", reachable.Found)
	require.Equal(t, "v2.2.8", reachable.Fixed)
	require.Contains(t, reachable.Aliases, "CVE-2019-11253")

	// Present in the module graph but never called.
	unreached := byID["GO-2022-0956"]
	require.False(t, unreached.Called)
	require.Empty(t, unreached.Trace)
	require.Equal(t, "v2.2.4", unreached.Fixed)

	// The two reachable ones must sort ahead of the unreachable one.
	require.True(t, got[0].Called)
	require.True(t, got[1].Called)
	require.False(t, got[2].Called)

	require.Equal(t, 2, Result{Vulnerabilities: got}.Called())
}

// The stream carries config, SBOM and progress messages that are neither
// advisories nor findings; treating one as either would corrupt the
// report.
func TestParseGovulncheckIgnoresNonVulnerabilityMessages(t *testing.T) {
	got, err := parseGovulncheck([]byte(`
{"config":{"scanner_name":"govulncheck"}}
{"SBOM":{"go_version":"go1.27.0"}}
{"progress":{"message":"working"}}
`))
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestRunFailsWhenTheModuleDirectoryIsNotAModule(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	_, err := Run(context.Background(), t.TempDir(), Options{SkipUpdates: true, SkipVulns: true})
	require.Error(t, err)
}
