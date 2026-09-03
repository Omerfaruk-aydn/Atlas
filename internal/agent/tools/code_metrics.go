package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/codeintel"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
)

const CodeMetricsToolName = "code_metrics"

//go:embed code_metrics.md
var codeMetricsDescription string

const (
	defaultMetricsTop = 20
	maxMetricsTop     = 100
	// maxMetricsFiles bounds the per-file summary at the end of the
	// report.
	maxMetricsFiles = 10
)

type CodeMetricsParams struct {
	Path          string `json:"path,omitempty" description:"Directory or single .go file to measure. Defaults to the working directory."`
	Top           int    `json:"top,omitempty" description:"How many functions to list, most complex first. Default 20."`
	MinComplexity int    `json:"min_complexity,omitempty" description:"Only report functions at or above this cyclomatic complexity."`
	IncludeTests  *bool  `json:"include_tests,omitempty" description:"Also measure _test.go files. Default false: table-driven tests score high for reasons that are not a problem."`
}

type CodeMetricsResponseMetadata struct {
	Functions     int `json:"functions"`
	Reported      int `json:"reported"`
	FilesScanned  int `json:"files_scanned"`
	TotalLines    int `json:"total_lines"`
	MaxComplexity int `json:"max_complexity"`
}

func NewCodeMetricsTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		CodeMetricsToolName,
		codeMetricsDescription,
		func(ctx context.Context, params CodeMetricsParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}
			includeTests := params.IncludeTests != nil && *params.IncludeTests

			result, err := codeintel.Metrics(root, includeTests)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			top := cmp.Or(params.Top, defaultMetricsTop)
			top = min(max(top, 1), maxMetricsTop)

			out, reported := formatMetrics(result, top, params.MinComplexity, workingDir)

			maxComplexity := 0
			if len(result.Functions) > 0 {
				maxComplexity = result.Functions[0].Complexity
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(out),
				CodeMetricsResponseMetadata{
					Functions:     len(result.Functions),
					Reported:      reported,
					FilesScanned:  result.FilesScanned,
					TotalLines:    result.TotalLines,
					MaxComplexity: maxComplexity,
				},
			), nil
		},
	)
}

func formatMetrics(r codeintel.MetricsResult, top, minComplexity int, workingDir string) (string, int) {
	if r.FilesScanned == 0 {
		return "No Go files found to measure.", 0
	}
	if len(r.Functions) == 0 {
		return fmt.Sprintf("No functions found across %d file(s).", r.FilesScanned), 0
	}

	// The list is already sorted most-complex-first by the analysis.
	candidates := r.Functions
	if minComplexity > 0 {
		filtered := candidates[:0:0]
		for _, m := range candidates {
			if m.Complexity >= minComplexity {
				filtered = append(filtered, m)
			}
		}
		candidates = filtered
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d function(s) across %d file(s), %d lines total.\n",
		len(r.Functions), r.FilesScanned, r.TotalLines)

	if minComplexity > 0 {
		fmt.Fprintf(&b, "%d function(s) at complexity >= %d.\n", len(candidates), minComplexity)
	}
	if len(candidates) == 0 {
		return b.String(), 0
	}

	shown := candidates
	if len(shown) > top {
		shown = shown[:top]
	}

	fmt.Fprintf(&b, "\ncplx  lines  nest  sig     function\n")
	for _, m := range shown {
		fmt.Fprintf(&b, "%4d  %5d  %4d  %d->%d   %s  (%s:%d)\n",
			m.Complexity, m.Lines, m.Nesting, m.Params, m.Results,
			metricsLabel(m), relOrAbs(m.File, workingDir), m.Line)
	}
	if len(shown) < len(candidates) {
		fmt.Fprintf(&b, "\n... and %d more above the cut. Raise `top` to see them.\n",
			len(candidates)-len(shown))
	}

	if len(r.Files) > 1 {
		b.WriteString("\nLargest files:\n")
		files := r.Files
		if len(files) > maxMetricsFiles {
			files = files[:maxMetricsFiles]
		}
		for _, f := range files {
			fmt.Fprintf(&b, "  %5d lines  %2d func(s)  %s\n",
				f.Lines, f.Functions, relOrAbs(f.Path, workingDir))
		}
	}

	b.WriteString("\nA score is not a verdict: a wide switch over an enum reads fine, a wide nest of conditionals does not, and the number is the same. Read the function before splitting it.\n")
	return b.String(), len(shown)
}

func metricsLabel(m codeintel.FuncMetrics) string {
	if m.Recv != "" {
		return fmt.Sprintf("(%s).%s", m.Recv, m.Name)
	}
	return m.Name
}
