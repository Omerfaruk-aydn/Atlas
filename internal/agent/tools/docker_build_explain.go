package tools

import (
	"cmp"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/dockerx"
)

const DockerBuildExplainToolName = "docker_build_explain"

//go:embed docker_build_explain.md
var dockerBuildExplainDescription string

type DockerBuildExplainParams struct {
	Path string `json:"path,omitempty" description:"Path to the Dockerfile. Defaults to 'Dockerfile' in the working directory."`
}

type DockerBuildExplainResponseMetadata struct {
	Stages int            `json:"stages"`
	Total  int            `json:"total"`
	ByKind map[string]int `json:"by_kind"`
}

func NewDockerBuildExplainTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		DockerBuildExplainToolName,
		dockerBuildExplainDescription,
		func(ctx context.Context, params DockerBuildExplainParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			path := cmp.Or(params.Path, "Dockerfile")
			if !filepath.IsAbs(path) {
				path = filepath.Join(workingDir, path)
			}

			result, err := dockerx.Parse(path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("no Dockerfile found at %s", relOrAbs(path, workingDir))), nil
				}
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if len(result.Stages) == 0 {
				return fantasy.NewTextResponse("No FROM instruction found -- nothing to explain."), nil
			}

			byKind := map[string]int{}
			for _, f := range result.Findings {
				byKind[f.Kind]++
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatDockerBuildExplain(result)),
				DockerBuildExplainResponseMetadata{
					Stages: len(result.Stages),
					Total:  len(result.Findings),
					ByKind: byKind,
				},
			), nil
		},
	)
}

func formatDockerBuildExplain(r dockerx.Result) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%d build stage(s).\n", len(r.Stages))
	for _, s := range r.Stages {
		label := fmt.Sprintf("stage %d", s.Index)
		if s.Name != "" {
			label = s.Name
		}
		fmt.Fprintf(&b, "  %s: FROM %s (%d instruction(s))\n", label, s.BaseImage, len(s.Instructions))
	}

	if len(r.Findings) == 0 {
		b.WriteString("\nNo issues found.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "\n%d issue(s):\n", len(r.Findings))
	for _, f := range r.Findings {
		if f.Line > 0 {
			fmt.Fprintf(&b, "  [%s] %s:%d  %s\n", f.Kind, f.Stage, f.Line, f.Message)
		} else {
			fmt.Fprintf(&b, "  [%s] %s  %s\n", f.Kind, f.Stage, f.Message)
		}
	}
	return b.String()
}
