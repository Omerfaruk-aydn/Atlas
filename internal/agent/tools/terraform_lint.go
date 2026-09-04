package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/terraformx"
)

const TerraformLintToolName = "terraform_lint"

//go:embed terraform_lint.md
var terraformLintDescription string

// maxTerraformFindingsShown bounds the printed list the same way the
// other scanners bound theirs -- a large module tree would otherwise
// produce an unreadable wall of output.
const maxTerraformFindingsShown = 60

type TerraformLintParams struct {
	Path string `json:"path,omitempty" description:"Directory or single .tf file to scan. Defaults to the working directory."`
}

type TerraformLintResponseMetadata struct {
	Total        int            `json:"total"`
	ByKind       map[string]int `json:"by_kind"`
	FilesScanned int            `json:"files_scanned"`
}

func NewTerraformLintTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		TerraformLintToolName,
		terraformLintDescription,
		func(ctx context.Context, params TerraformLintParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}

			result, err := terraformx.Scan(root)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if result.FilesScanned == 0 {
				return fantasy.NewTextResponse("No .tf files found to scan. Check the path."), nil
			}

			byKind := map[string]int{}
			for _, f := range result.Findings {
				byKind[f.Kind]++
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatTerraformLint(result, workingDir)),
				TerraformLintResponseMetadata{
					Total:        len(result.Findings),
					ByKind:       byKind,
					FilesScanned: result.FilesScanned,
				},
			), nil
		},
	)
}

func formatTerraformLint(r terraformx.Result, workingDir string) string {
	if len(r.Findings) == 0 {
		return fmt.Sprintf("No issues found across %d .tf file(s).", r.FilesScanned)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d issue(s) across %d .tf file(s).\n", len(r.Findings), r.FilesScanned)

	shown := r.Findings
	if len(shown) > maxTerraformFindingsShown {
		shown = shown[:maxTerraformFindingsShown]
	}

	currentFile := ""
	for _, f := range shown {
		rel := relOrAbs(f.File, workingDir)
		if rel != currentFile {
			currentFile = rel
			fmt.Fprintf(&b, "\n%s\n", rel)
		}
		if f.Resource != "" {
			fmt.Fprintf(&b, "  %d  [%s] %s: %s\n", f.Line, f.Kind, f.Resource, f.Message)
		} else {
			fmt.Fprintf(&b, "  %d  [%s] %s\n", f.Line, f.Kind, f.Message)
		}
	}

	if len(shown) < len(r.Findings) {
		fmt.Fprintf(&b, "\n... and %d more. Narrow with `path`.\n", len(r.Findings)-len(shown))
	}
	return b.String()
}
