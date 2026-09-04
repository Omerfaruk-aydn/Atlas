package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/commitsplit"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
)

const GitCommitSplitToolName = "git_commit_split"

//go:embed git_commit_split.md
var gitCommitSplitDescription string

type GitCommitSplitParams struct {
	Dir string `json:"dir,omitempty" description:"A directory inside the repository. Defaults to the working directory."`
}

type GitCommitSplitResponseMetadata struct {
	Groups int `json:"groups"`
	Cycles int `json:"cycles"`
}

func NewGitCommitSplitTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		GitCommitSplitToolName,
		gitCommitSplitDescription,
		func(ctx context.Context, params GitCommitSplitParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			dir := cmp.Or(params.Dir, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			result, err := commitsplit.Split(ctx, dir)
			if err != nil {
				return gitErrorResponse(err), nil
			}
			if len(result.Groups) == 0 {
				return fantasy.NewTextResponse("Nothing has changed. Nothing to split."), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatCommitSplit(result, dir, workingDir)),
				GitCommitSplitResponseMetadata{
					Groups: len(result.Groups),
					Cycles: len(result.Cycles),
				},
			), nil
		},
	)
}

func formatCommitSplit(r commitsplit.Result, repoDir, workingDir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d commit(s) suggested, in this order:\n", len(r.Groups))

	for i, g := range r.Groups {
		fmt.Fprintf(&b, "\n%d. %s (+%d/-%d)\n", i+1, g.Label, g.Insertions, g.Deletions)
		for _, f := range g.Files {
			fmt.Fprintf(&b, "   %s\n", relOrAbs(filepath.Join(repoDir, f), workingDir))
		}
	}

	if len(r.Cycles) > 0 {
		b.WriteString("\nImport cycle(s) among changed packages, merged into one group each:\n")
		for _, cycle := range r.Cycles {
			fmt.Fprintf(&b, "  %s\n", strings.Join(cycle, " <-> "))
		}
	}

	b.WriteString("\nNothing was staged or committed -- stage each group's files and commit them in this order yourself.\n")
	return b.String()
}
