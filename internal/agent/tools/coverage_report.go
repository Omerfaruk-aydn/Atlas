package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/gotest"
)

const CoverageReportToolName = "coverage_report"

//go:embed coverage_report.md
var coverageReportDescription string

const (
	maxCoverageFiles = 20
	// maxUncoveredPerFile bounds the line ranges shown for one file. A
	// file at 0% has a block per function, and listing all of them says
	// no more than "this file has no tests".
	maxUncoveredPerFile = 12
	maxCoveragePackages = 15
)

type CoverageReportParams struct {
	Dir           string  `json:"dir,omitempty" description:"A directory inside the module. Defaults to the working directory."`
	Packages      string  `json:"packages,omitempty" description:"Go package pattern. Default './...'."`
	Run           string  `json:"run,omitempty" description:"Regexp selecting tests by name, to measure the coverage of specific tests."`
	Timeout       string  `json:"timeout,omitempty" description:"How long the run may take, as a Go duration. Default 5m."`
	ShowUncovered *bool   `json:"show_uncovered,omitempty" description:"List the uncovered line ranges. On by default."`
	MinPercent    float64 `json:"min_percent,omitempty" description:"Report whether coverage meets this threshold."`
}

type CoverageReportResponseMetadata struct {
	Percent    float64 `json:"percent"`
	Statements int     `json:"statements"`
	Covered    int     `json:"covered"`
	Files      int     `json:"files"`
	TestsOK    bool    `json:"tests_ok"`
	MetMinimum *bool   `json:"met_minimum,omitempty"`
}

func NewCoverageReportTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		CoverageReportToolName,
		coverageReportDescription,
		func(ctx context.Context, params CoverageReportParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			timeout := defaultTestTimeout
			if params.Timeout != "" {
				parsed, err := time.ParseDuration(params.Timeout)
				if err != nil || parsed <= 0 {
					return fantasy.NewTextErrorResponse(
						fmt.Sprintf("timeout %q is not a positive duration (try '90s' or '5m')", params.Timeout)), nil
				}
				timeout = min(parsed, maxTestTimeout)
			}
			if params.MinPercent < 0 || params.MinPercent > 100 {
				return fantasy.NewTextErrorResponse("min_percent must be between 0 and 100"), nil
			}

			dir := cmp.Or(params.Dir, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			// The profile goes to a temp file rather than the working
			// tree: writing cover.out into somebody's repository is a
			// side effect they did not ask for and would then have to
			// gitignore.
			profile, err := os.CreateTemp("", "atlas-cover-*.out")
			if err != nil {
				return fantasy.NewTextErrorResponse("could not create a coverage profile: " + err.Error()), nil
			}
			profilePath := profile.Name()
			profile.Close()
			defer os.Remove(profilePath)

			run, err := gotest.Run(ctx, dir, gotest.Options{
				Packages:     params.Packages,
				Run:          params.Run,
				Timeout:      timeout,
				CoverProfile: profilePath,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			cov, covErr := gotest.ParseCoverageFile(profilePath)

			showUncovered := params.ShowUncovered == nil || *params.ShowUncovered
			out := formatCoverage(run, cov, covErr, params, showUncovered)

			meta := CoverageReportResponseMetadata{
				Percent:    cov.Percent(),
				Statements: cov.Statements,
				Covered:    cov.Covered,
				Files:      len(cov.Files),
				TestsOK:    run.OK(),
			}
			if params.MinPercent > 0 {
				met := cov.Percent() >= params.MinPercent
				meta.MetMinimum = &met
			}

			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(out), meta), nil
		},
	)
}

func formatCoverage(run gotest.Result, cov gotest.Coverage, covErr error, params CoverageReportParams, showUncovered bool) string {
	var b strings.Builder

	// A failing suite is stated first. Coverage measured against tests
	// that did not pass means less than the number suggests, and a
	// reader who sees only the percentage will not know that.
	if !run.OK() {
		if run.Failed > 0 {
			fmt.Fprintf(&b, "WARNING: %d test(s) failed during this run. Coverage below is still real, but it was measured against a suite that is not passing.\n\n", run.Failed)
		} else {
			b.WriteString("WARNING: the test run did not complete cleanly (a package failed to build, or the run timed out). Coverage below may be partial.\n\n")
		}
	}

	if covErr != nil || cov.Statements == 0 {
		b.WriteString("No coverage data was produced. ")
		if run.NoTests {
			b.WriteString("No tests ran, so there was nothing to measure.")
		} else {
			b.WriteString("The selected packages may have no test files.")
		}
		return b.String()
	}

	fmt.Fprintf(&b, "coverage: %.1f%% of statements (%d of %d) across %d file(s)\n",
		cov.Percent(), cov.Covered, cov.Statements, len(cov.Files))

	if params.MinPercent > 0 {
		if cov.Percent() >= params.MinPercent {
			fmt.Fprintf(&b, "Meets the %.1f%% threshold.\n", params.MinPercent)
		} else {
			fmt.Fprintf(&b, "BELOW the %.1f%% threshold, by %.1f points.\n",
				params.MinPercent, params.MinPercent-cov.Percent())
		}
	}

	if len(cov.Packages) > 1 {
		b.WriteString("\nleast-covered packages:\n")
		pkgs := cov.Packages
		if len(pkgs) > maxCoveragePackages {
			pkgs = pkgs[:maxCoveragePackages]
		}
		for _, p := range pkgs {
			fmt.Fprintf(&b, "  %5.1f%%  %4d stmt  %s\n", p.Percent(), p.Statements, p.Package)
		}
	}

	b.WriteString("\nleast-covered files:\n")
	files := cov.Files
	if len(files) > maxCoverageFiles {
		files = files[:maxCoverageFiles]
	}
	for _, f := range files {
		fmt.Fprintf(&b, "  %5.1f%%  %4d stmt  %s\n", f.Percent(), f.Statements, f.File)
		if !showUncovered || len(f.UncoveredBlocks) == 0 {
			continue
		}
		blocks := f.UncoveredBlocks
		truncated := false
		if len(blocks) > maxUncoveredPerFile {
			blocks = blocks[:maxUncoveredPerFile]
			truncated = true
		}
		var ranges []string
		for _, blk := range blocks {
			if blk.StartLine == blk.EndLine {
				ranges = append(ranges, fmt.Sprintf("%d", blk.StartLine))
			} else {
				ranges = append(ranges, fmt.Sprintf("%d-%d", blk.StartLine, blk.EndLine))
			}
		}
		fmt.Fprintf(&b, "         uncovered lines: %s", strings.Join(ranges, ", "))
		if truncated {
			fmt.Fprintf(&b, ", and %d more block(s)", len(f.UncoveredBlocks)-len(blocks))
		}
		b.WriteString("\n")
	}
	if len(files) < len(cov.Files) {
		fmt.Fprintf(&b, "\n... and %d more file(s), better covered than these.\n", len(cov.Files)-len(files))
	}

	// Both of these are routinely misread, and a percentage quoted
	// without them turns into a target rather than a diagnostic.
	b.WriteString("\nCoverage counts statements, not lines: a long branchless function is one block, so a high percentage does not mean most lines ran individually.\n")
	b.WriteString("Covered means executed, not verified -- a test that calls code and asserts nothing scores the same as one that checks the result.\n")
	return b.String()
}
