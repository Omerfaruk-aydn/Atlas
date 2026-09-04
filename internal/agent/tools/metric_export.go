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

const MetricExportToolName = "metric_export"

//go:embed metric_export.md
var metricExportDescription string

type MetricExportParams struct {
	Path         string `json:"path,omitempty" description:"Directory or single file to scan. Defaults to the working directory."`
	IncludeTests *bool  `json:"include_tests,omitempty" description:"Also scan test files. Off by default."`
}

type MetricExportResponseMetadata struct {
	Total        int            `json:"total"`
	ByType       map[string]int `json:"by_type"`
	FilesScanned int            `json:"files_scanned"`
}

func NewMetricExportTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		MetricExportToolName,
		metricExportDescription,
		func(ctx context.Context, params MetricExportParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}

			result, err := codeintel.IndexMetrics(root, params.IncludeTests != nil && *params.IncludeTests)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if result.FilesScanned == 0 {
				return fantasy.NewTextResponse("No Go files found to scan. Check the path."), nil
			}
			if len(result.Metrics) == 0 {
				return fantasy.NewTextResponse(fmt.Sprintf("No Prometheus metrics found across %d Go file(s).", result.FilesScanned)), nil
			}

			byType := map[string]int{}
			for _, m := range result.Metrics {
				byType[m.Type]++
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatMetricIndex(result, workingDir)),
				MetricExportResponseMetadata{
					Total:        len(result.Metrics),
					ByType:       byType,
					FilesScanned: result.FilesScanned,
				},
			), nil
		},
	)
}

func formatMetricIndex(r codeintel.MetricIndexResult, workingDir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d metric(s) across %d Go file(s).\n", len(r.Metrics), r.FilesScanned)

	for _, m := range r.Metrics {
		rel := relOrAbs(m.File, workingDir)
		fmt.Fprintf(&b, "\n%s (%s)  %s:%d\n", m.Name, m.Type, rel, m.Line)
		if m.Help != "" {
			fmt.Fprintf(&b, "  %s\n", m.Help)
		}
		if len(m.Labels) > 0 {
			fmt.Fprintf(&b, "  labels: %s\n", strings.Join(m.Labels, ", "))
		}
	}
	return b.String()
}
