// Package gotest runs Go tests and reads the result.
//
// It drives `go test -json`, which emits one JSON event per line and is
// the only stable machine interface the toolchain offers. Scraping the
// human output means re-deriving structure that is already there, and
// getting it subtly wrong on subtests, parallel output interleaving, and
// build failures.
package gotest

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Status is how one test ended.
type Status string

const (
	StatusPass Status = "pass"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// Test is one test's outcome.
type Test struct {
	Package string
	Name    string
	Status  Status
	Elapsed time.Duration
	// Output holds the test's own output, kept only for failures --
	// a passing test's log is noise, and keeping all of it is how a run
	// over a large repository turns into megabytes.
	Output string
}

// PackageResult summarises one package.
type PackageResult struct {
	Package string
	Status  Status
	Elapsed time.Duration
	// BuildError holds a compile failure. A package that does not build
	// reports no tests at all, and reporting that as "0 failures" is the
	// most dangerous wrong answer this package can give.
	BuildError string
}

// Result is a whole run.
type Result struct {
	Tests    []Test
	Packages []PackageResult
	Passed   int
	Failed   int
	Skipped  int
	Elapsed  time.Duration
	// NoTests reports that the run completed without executing anything,
	// which usually means the pattern matched nothing.
	NoTests bool
	// Timeout reports that the run was cut short by its deadline, so the
	// counts below are partial.
	Timeout bool
}

// OK reports whether everything that ran passed and everything compiled.
func (r Result) OK() bool {
	if r.Failed > 0 || r.Timeout {
		return false
	}
	for _, p := range r.Packages {
		if p.BuildError != "" {
			return false
		}
	}
	return true
}

// Options select what to run.
type Options struct {
	// Packages is a go package pattern such as "./..." or "./internal/x".
	// Empty means "./...".
	Packages string
	// Run is a regexp passed to -run, selecting tests by name.
	Run string
	// Timeout bounds the whole run. Zero means five minutes.
	Timeout time.Duration
	// Verbose keeps output from passing tests too.
	Verbose bool
	// Count, when non-zero, is passed to -count, which is how a cached
	// result is bypassed.
	Count int
	// Race enables the race detector. It needs cgo, and the run fails
	// with a clear toolchain message when that is unavailable.
	Race bool
	// Cover collects coverage into the named profile file.
	CoverProfile string
}

// event is one line of `go test -json`.
//
// Build diagnostics do not use the Package field at all: they arrive as
// "build-output"/"build-fail" actions keyed by ImportPath (which carries
// a ".test" suffix), and the package's own "fail" event points back at
// them through FailedBuild. A parser that only reads Package therefore
// sees a failing package with no failing test and no explanation.
type event struct {
	Action      string  `json:"Action"`
	Package     string  `json:"Package"`
	ImportPath  string  `json:"ImportPath"`
	FailedBuild string  `json:"FailedBuild"`
	Test        string  `json:"Test"`
	Output      string  `json:"Output"`
	Elapsed     float64 `json:"Elapsed"`
}

// Run executes the tests and parses the result.
//
// A non-zero exit status from `go test` is the normal way a failing test
// suite reports itself, so it is not treated as an error: the parsed
// result already says what failed. An error is returned only when the
// toolchain could not be run at all.
func Run(ctx context.Context, dir string, opts Options) (Result, error) {
	if _, err := exec.LookPath("go"); err != nil {
		return Result{}, fmt.Errorf("the go toolchain is not on PATH")
	}
	if opts.Packages == "" {
		opts.Packages = "./..."
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Minute
	}

	args := []string{"test", "-json"}
	// -timeout is given to the test binary as well as bounding the
	// context, so a hung test produces a panic with a stack -- which
	// names the hang -- instead of a killed process that says nothing.
	args = append(args, "-timeout", opts.Timeout.String())
	if opts.Count > 0 {
		args = append(args, fmt.Sprintf("-count=%d", opts.Count))
	}
	if opts.Race {
		args = append(args, "-race")
	}
	if opts.CoverProfile != "" {
		args = append(args, "-coverprofile="+opts.CoverProfile, "-covermode=atomic")
	}
	if opts.Run != "" {
		args = append(args, "-run", opts.Run)
	}
	args = append(args, opts.Packages)

	// The context gets slack beyond the test timeout so the binary's own
	// deadline fires first and produces a diagnosable panic.
	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout+30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "go", args...)
	cmd.Dir = dir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("could not start go test: %w", err)
	}

	result := parseEvents(stdout, opts.Verbose)
	waitErr := cmd.Wait()

	if runCtx.Err() != nil {
		result.Timeout = true
	}

	// A failure with no parsed events at all means the toolchain itself
	// refused -- an unknown flag, a missing race toolchain, a bad
	// pattern. That message is the answer, so it must not be swallowed.
	if waitErr != nil && len(result.Tests) == 0 && len(result.Packages) == 0 {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return result, fmt.Errorf("%s", firstLines(msg, 5))
	}

	finalise(&result)
	return result, nil
}

func parseEvents(stdout interface{ Read([]byte) (int, error) }, verbose bool) Result {
	var result Result

	output := map[key]*strings.Builder{}
	tests := map[key]*Test{}
	packages := map[string]*PackageResult{}
	// buildOutput is keyed by ImportPath, which is a different namespace
	// from Package -- the package's fail event names the import path to
	// join them.
	buildOutput := map[string]*strings.Builder{}
	// failedBuild remembers which import path each package blamed, so a
	// build-fail arriving before the package event is still matched.
	failedBuild := map[string]string{}

	scanner := bufio.NewScanner(stdout)
	// Test output lines can be long -- a failing assertion printing a
	// large struct, say -- and the default 64 KiB limit would truncate
	// mid-line and desynchronise the JSON parse.
	scanner.Buffer(make([]byte, 0, 256*1024), 4*1024*1024)

	for scanner.Scan() {
		var ev event
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			// A non-JSON line is toolchain noise (a build error banner,
			// for one). It is captured against the package below rather
			// than dropped.
			continue
		}

		k := key{ev.Package, ev.Test}

		switch ev.Action {
		case "build-output":
			buf, ok := buildOutput[ev.ImportPath]
			if !ok {
				buf = &strings.Builder{}
				buildOutput[ev.ImportPath] = buf
			}
			if buf.Len() < 32*1024 {
				buf.WriteString(ev.Output)
			}

		case "output":
			buf, ok := output[k]
			if !ok {
				buf = &strings.Builder{}
				output[k] = buf
			}
			// Cap per-test output. One test printing a megabyte should
			// not decide how much of the run is reportable.
			if buf.Len() < 32*1024 {
				buf.WriteString(ev.Output)
			}

		case "pass", "fail", "skip":
			status := Status(ev.Action)
			elapsed := time.Duration(ev.Elapsed * float64(time.Second))

			if ev.Test == "" {
				pkg, ok := packages[ev.Package]
				if !ok {
					pkg = &PackageResult{Package: ev.Package}
					packages[ev.Package] = pkg
				}
				pkg.Status = status
				pkg.Elapsed = elapsed
				if ev.FailedBuild != "" {
					failedBuild[ev.Package] = ev.FailedBuild
				}
				continue
			}

			t := &Test{
				Package: ev.Package,
				Name:    ev.Test,
				Status:  status,
				Elapsed: elapsed,
			}
			if status == StatusFail || verbose {
				if buf, ok := output[k]; ok {
					t.Output = strings.TrimRight(buf.String(), "\n")
				}
			}
			tests[k] = t

		case "build-fail":
			// Recorded by import path; joined to a package below. There
			// may be no package event at all when the pattern itself did
			// not resolve, so a placeholder is created for it.
			name := strings.TrimSuffix(ev.ImportPath, ".test")
			if _, ok := packages[name]; !ok {
				packages[name] = &PackageResult{Package: name, Status: StatusFail}
			}
			failedBuild[name] = ev.ImportPath
		}
	}

	for _, t := range tests {
		result.Tests = append(result.Tests, *t)
	}
	for _, p := range packages {
		if importPath, ok := failedBuild[p.Package]; ok {
			p.Status = StatusFail
			if buf, ok := buildOutput[importPath]; ok {
				p.BuildError = strings.TrimSpace(buf.String())
			}
		}
		// A package that failed with no failing test in it did not run
		// its tests -- a setup failure, a panic outside a test, a
		// TestMain that exited. Calling that "0 failures" is the worst
		// answer available, so the package output is carried as the
		// explanation.
		if p.Status == StatusFail && p.BuildError == "" && !hasFailingTest(tests, p.Package) {
			if buf, ok := output[key{p.Package, ""}]; ok {
				p.BuildError = strings.TrimSpace(buf.String())
			}
		}
		result.Packages = append(result.Packages, *p)
	}
	return result
}

// key identifies one test within one package. Package-level rather than
// local so the helpers below can take it.
type key struct{ pkg, test string }

func hasFailingTest(tests map[key]*Test, pkg string) bool {
	for k, t := range tests {
		if k.pkg == pkg && t.Status == StatusFail {
			return true
		}
	}
	return false
}

func finalise(r *Result) {
	for _, t := range r.Tests {
		switch t.Status {
		case StatusPass:
			r.Passed++
		case StatusFail:
			r.Failed++
		case StatusSkip:
			r.Skipped++
		}
	}
	for _, p := range r.Packages {
		r.Elapsed += p.Elapsed
	}
	r.NoTests = len(r.Tests) == 0 && r.Failed == 0

	// Failures first, then slowest, so the two things a reader wants are
	// both at the top.
	sort.Slice(r.Tests, func(i, j int) bool {
		a, b := r.Tests[i], r.Tests[j]
		if (a.Status == StatusFail) != (b.Status == StatusFail) {
			return a.Status == StatusFail
		}
		if a.Package != b.Package {
			return a.Package < b.Package
		}
		return a.Name < b.Name
	})
	sort.Slice(r.Packages, func(i, j int) bool {
		return r.Packages[i].Package < r.Packages[j].Package
	})
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
