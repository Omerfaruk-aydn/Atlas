package tools

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/cicdx"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
)

const CICDPipelineDebuggerToolName = "ci_cd_pipeline_debugger"

//go:embed ci_cd_pipeline_debugger.md
var ciCDPipelineDebuggerDescription string

type CICDPipelineDebuggerParams struct {
	Path string `json:"path" description:"Path to the GitHub Actions workflow file. Required."`
}

type CICDPipelineDebuggerResponseMetadata struct {
	JobsFound int            `json:"jobs_found"`
	Total     int            `json:"total"`
	ByKind    map[string]int `json:"by_kind"`
}

func NewCICDPipelineDebuggerTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		CICDPipelineDebuggerToolName,
		ciCDPipelineDebuggerDescription,
		func(ctx context.Context, params CICDPipelineDebuggerParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.Path) == "" {
				return fantasy.NewTextErrorResponse("path is required"), nil
			}
			path := params.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(workingDir, path)
			}

			result, err := cicdx.Parse(path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("no workflow file found at %s", relOrAbs(path, workingDir))), nil
				}
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if result.JobsFound == 0 {
				return fantasy.NewTextResponse("No jobs found in this workflow -- nothing to check."), nil
			}

			byKind := map[string]int{}
			for _, f := range result.Findings {
				byKind[f.Kind]++
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatCICDPipelineDebugger(result)),
				CICDPipelineDebuggerResponseMetadata{
					JobsFound: result.JobsFound,
					Total:     len(result.Findings),
					ByKind:    byKind,
				},
			), nil
		},
	)
}

func formatCICDPipelineDebugger(r cicdx.Result) string {
	var b strings.Builder

	if len(r.Findings) == 0 {
		fmt.Fprintf(&b, "No issues found across %d job(s).\n", r.JobsFound)
		return b.String()
	}

	fmt.Fprintf(&b, "%d issue(s) across %d job(s).\n", len(r.Findings), r.JobsFound)
	currentJob := ""
	for _, f := range r.Findings {
		if f.Job != currentJob {
			currentJob = f.Job
			fmt.Fprintf(&b, "\n%s\n", f.Job)
		}
		if f.Step != "" {
			fmt.Fprintf(&b, "  [%s] %s: %s\n", f.Kind, f.Step, f.Message)
		} else {
			fmt.Fprintf(&b, "  [%s] %s\n", f.Kind, f.Message)
		}
	}
	return b.String()
}
