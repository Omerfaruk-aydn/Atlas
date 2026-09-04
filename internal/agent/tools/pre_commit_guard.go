package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/gitx"
)

const PreCommitGuardToolName = "pre_commit_guard"

//go:embed pre_commit_guard.md
var preCommitGuardDescription string

type PreCommitGuardParams struct {
	Dir          string `json:"dir,omitempty" description:"A directory inside the repository. Defaults to the working directory."`
	MaxFileBytes int64  `json:"max_file_bytes,omitempty" description:"Size threshold in bytes for the large-file check. Defaults to 1 MiB."`
}

type PreCommitGuardResponseMetadata struct {
	Total       int `json:"total"`
	FilesStaged int `json:"files_staged"`
}

func NewPreCommitGuardTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		PreCommitGuardToolName,
		preCommitGuardDescription,
		func(ctx context.Context, params PreCommitGuardParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			dir := cmp.Or(params.Dir, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			result, err := gitx.PreCommitCheck(ctx, dir, gitx.PreCommitOptions{
				MaxFileBytes: params.MaxFileBytes,
			})
			if err != nil {
				return gitErrorResponse(err), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatPreCommitGuard(result, dir, workingDir)),
				PreCommitGuardResponseMetadata{
					Total:       len(result.Findings),
					FilesStaged: result.FilesStaged,
				},
			), nil
		},
	)
}

func formatPreCommitGuard(r gitx.PreCommitResult, repoDir, workingDir string) string {
	if r.FilesStaged == 0 {
		return "Nothing is staged. Stage the changes you intend to commit and run this again."
	}
	if len(r.Findings) == 0 {
		return fmt.Sprintf("No issues found across %d staged file(s).", r.FilesStaged)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d issue(s) across %d staged file(s).\n", len(r.Findings), r.FilesStaged)

	currentFile := ""
	for _, f := range r.Findings {
		rel := relOrAbs(filepath.Join(repoDir, f.File), workingDir)
		if rel != currentFile {
			currentFile = rel
			fmt.Fprintf(&b, "\n%s\n", rel)
		}
		if f.Line > 0 {
			fmt.Fprintf(&b, "  %d  [%s] %s\n", f.Line, f.Kind, f.Message)
		} else {
			fmt.Fprintf(&b, "  [%s] %s\n", f.Kind, f.Message)
		}
	}

	b.WriteString("\nReview each before committing -- a merge marker or debug print here would ship as-is.\n")
	return b.String()
}
