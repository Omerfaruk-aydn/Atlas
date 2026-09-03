package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/gotest"
)

const TestRunToolName = "test_run"

//go:embed test_run.md
var testRunDescription string

const (
	defaultTestTimeout = 5 * time.Minute
	maxTestTimeout     = 30 * time.Minute
	// maxReportedFailures bounds how many failures are printed with
	// their output. Beyond this the pattern is usually one cause, and
	// the remaining names are enough to see it.
	maxReportedFailures = 15
	// maxFailureOutputLines bounds each failure's captured output. A
	// test dumping a large struct should not crowd out the other
	// failures.
	maxFailureOutputLines = 40
	// slowTestThreshold is when a test is worth mentioning as slow.
	slowTestThreshold = 2 * time.Second
	maxSlowTests      = 5
)

type TestRunParams struct {
	Dir      string `json:"dir,omitempty" description:"A directory inside the module. Defaults to the working directory."`
	Packages string `json:"packages,omitempty" description:"Go package pattern such as './...' or './internal/foo/...'. Default './...'."`
	Run      string `json:"run,omitempty" description:"Regexp selecting tests by name, as with 'go test -run'."`
	Timeout  string `json:"timeout,omitempty" description:"How long the run may take, as a Go duration such as '90s' or '5m'. Default 5m."`
	Count    int    `json:"count,omitempty" description:"Pass 1 to bypass the test cache and force a real re-run."`
	Verbose  *bool  `json:"verbose,omitempty" description:"Keep output from passing tests too. Off by default."`
	Race     *bool  `json:"race,omitempty" description:"Enable the race detector. Requires cgo."`
}

type TestRunResponseMetadata struct {
	Passed        int  `json:"passed"`
	Failed        int  `json:"failed"`
	Skipped       int  `json:"skipped"`
	BuildFailures int  `json:"build_failures"`
	OK            bool `json:"ok"`
	TimedOut      bool `json:"timed_out"`
}

func NewTestRunTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		TestRunToolName,
		testRunDescription,
		func(ctx context.Context, params TestRunParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			timeout := defaultTestTimeout
			if params.Timeout != "" {
				parsed, err := time.ParseDuration(params.Timeout)
				if err != nil {
					return fantasy.NewTextErrorResponse(
						fmt.Sprintf("timeout %q is not a duration (try '90s' or '5m')", params.Timeout)), nil
				}
				if parsed <= 0 {
					return fantasy.NewTextErrorResponse("timeout must be positive"), nil
				}
				timeout = min(parsed, maxTestTimeout)
			}

			dir := cmp.Or(params.Dir, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			result, err := gotest.Run(ctx, dir, gotest.Options{
				Packages: params.Packages,
				Run:      params.Run,
				Timeout:  timeout,
				Count:    params.Count,
				Verbose:  params.Verbose != nil && *params.Verbose,
				Race:     params.Race != nil && *params.Race,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			buildFailures := 0
			for _, p := range result.Packages {
				if p.BuildError != "" {
					buildFailures++
				}
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatTestRun(result, params)),
				TestRunResponseMetadata{
					Passed:        result.Passed,
					Failed:        result.Failed,
					Skipped:       result.Skipped,
					BuildFailures: buildFailures,
					OK:            result.OK(),
					TimedOut:      result.Timeout,
				},
			), nil
		},
	)
}

func formatTestRun(r gotest.Result, params TestRunParams) string {
	var b strings.Builder

	// Build failures lead. A package that did not compile ran no tests,
	// and a headline of "0 failed" above a silent compile error is the
	// single most misleading thing this tool could print.
	var broken []gotest.PackageResult
	for _, p := range r.Packages {
		if p.BuildError != "" {
			broken = append(broken, p)
		}
	}
	if len(broken) > 0 {
		fmt.Fprintf(&b, "%d package(s) DID NOT COMPILE, so their tests never ran:\n\n", len(broken))
		for _, p := range broken {
			fmt.Fprintf(&b, "  %s\n", p.Package)
			for _, line := range limitLines(p.BuildError, maxFailureOutputLines) {
				fmt.Fprintf(&b, "    %s\n", line)
			}
			b.WriteString("\n")
		}
	}

	scope := cmp.Or(params.Packages, "./...")
	if params.Run != "" {
		scope += " matching " + params.Run
	}

	switch {
	case r.Timeout:
		fmt.Fprintf(&b, "TIMED OUT after the deadline. %d passed, %d failed so far -- these counts are partial.\n",
			r.Passed, r.Failed)
	case r.NoTests && len(broken) == 0:
		fmt.Fprintf(&b, "No tests ran for %s. The pattern may match no packages, or those packages may have no test files.\n", scope)
		return b.String()
	case r.OK():
		fmt.Fprintf(&b, "PASS: %d passed", r.Passed)
		if r.Skipped > 0 {
			fmt.Fprintf(&b, ", %d skipped", r.Skipped)
		}
		fmt.Fprintf(&b, " (%s)\n", scope)
	default:
		fmt.Fprintf(&b, "FAIL: %d failed, %d passed", r.Failed, r.Passed)
		if r.Skipped > 0 {
			fmt.Fprintf(&b, ", %d skipped", r.Skipped)
		}
		fmt.Fprintf(&b, " (%s)\n", scope)
	}

	shown := 0
	for _, tc := range r.Tests {
		if tc.Status != gotest.StatusFail {
			continue
		}
		if shown >= maxReportedFailures {
			fmt.Fprintf(&b, "\n... and %d more failure(s) not shown.\n", r.Failed-shown)
			break
		}
		shown++
		fmt.Fprintf(&b, "\n--- FAIL %s\n    %s\n", tc.Name, shortPackage(tc.Package))
		for _, line := range limitLines(tc.Output, maxFailureOutputLines) {
			fmt.Fprintf(&b, "    %s\n", line)
		}
	}

	if r.OK() {
		writeSlowTests(&b, r)
	}

	return b.String()
}

// writeSlowTests surfaces the few tests worth knowing about on a green
// run, where there is nothing else to report and the alternative is a
// one-line answer that hides a suite quietly getting slower.
func writeSlowTests(b *strings.Builder, r gotest.Result) {
	var slow []gotest.Test
	for _, tc := range r.Tests {
		if tc.Elapsed >= slowTestThreshold {
			slow = append(slow, tc)
		}
	}
	if len(slow) == 0 {
		return
	}
	for i := range slow {
		for j := i + 1; j < len(slow); j++ {
			if slow[j].Elapsed > slow[i].Elapsed {
				slow[i], slow[j] = slow[j], slow[i]
			}
		}
	}
	if len(slow) > maxSlowTests {
		slow = slow[:maxSlowTests]
	}
	b.WriteString("\nslowest:\n")
	for _, tc := range slow {
		fmt.Fprintf(b, "  %6.1fs  %s\n", tc.Elapsed.Seconds(), tc.Name)
	}
}

func limitLines(s string, n int) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = append(lines[:n], fmt.Sprintf("... (%d more lines)", len(lines)-n))
	}
	return lines
}

// shortPackage drops the module prefix, which is identical on every line
// and pushes the part that differs off the edge.
func shortPackage(pkg string) string {
	if i := strings.Index(pkg, "/internal/"); i >= 0 {
		return pkg[i+1:]
	}
	if i := strings.LastIndex(pkg, "/"); i >= 0 && strings.Count(pkg, "/") > 2 {
		return "..." + pkg[i:]
	}
	return pkg
}
