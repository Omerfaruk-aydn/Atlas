package tools

import (
	"cmp"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ghpr"
)

const GithubPRViewToolName = "github_pr_view"

//go:embed github_pr_view.md
var githubPRViewDescription string

// maxPRDiffBytes bounds how much diff text is inlined -- a PR touching
// hundreds of files could otherwise spend the whole response budget on
// one tool call.
const maxPRDiffBytes = 60_000

type GithubPRViewParams struct {
	Ref string `json:"ref" description:"PR number, 'owner/repo#number', or a full GitHub PR URL. Required."`
	Dir string `json:"dir,omitempty" description:"A directory inside the repository, used to resolve a bare number. Defaults to the working directory."`
}

type GithubPRViewResponseMetadata struct {
	Number       int    `json:"number"`
	State        string `json:"state"`
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	ChangedFiles int    `json:"changed_files"`
}

func NewGithubPRViewTool(workingDir string) fantasy.AgentTool {
	client := ghpr.New()
	return fantasy.NewAgentTool(
		GithubPRViewToolName,
		githubPRViewDescription,
		func(ctx context.Context, params GithubPRViewParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.Ref) == "" {
				return fantasy.NewTextErrorResponse("ref is required"), nil
			}

			dir := cmp.Or(params.Dir, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			pr, err := client.View(ctx, dir, params.Ref)
			if err != nil {
				return ghErrorResponse(err), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatPRView(pr)),
				GithubPRViewResponseMetadata{
					Number: pr.Number, State: pr.State,
					Additions: pr.Additions, Deletions: pr.Deletions, ChangedFiles: pr.ChangedFiles,
				},
			), nil
		},
	)
}

func ghErrorResponse(err error) fantasy.ToolResponse {
	switch {
	case errors.Is(err, ghpr.ErrGHMissing):
		return fantasy.NewTextResponse("gh is not installed or not on PATH, so a pull request cannot be read here.")
	case errors.Is(err, ghpr.ErrNotAuthenticated):
		return fantasy.NewTextResponse("gh is installed but not authenticated. Run `gh auth login`, then try again.")
	case errors.Is(err, ghpr.ErrBadRef):
		return fantasy.NewTextErrorResponse(err.Error())
	}
	return fantasy.NewTextErrorResponse(err.Error())
}

func formatPRView(pr ghpr.PR) string {
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s [%s]\n", pr.Number, pr.Title, pr.State)
	fmt.Fprintf(&b, "%s -> %s, by %s\n", pr.HeadRefName, pr.BaseRefName, pr.Author)
	fmt.Fprintf(&b, "+%d/-%d across %d file(s)\n", pr.Additions, pr.Deletions, pr.ChangedFiles)
	fmt.Fprintf(&b, "%s\n", pr.URL)

	if strings.TrimSpace(pr.Body) != "" {
		b.WriteString("\n")
		b.WriteString(pr.Body)
		b.WriteString("\n")
	}

	switch {
	case pr.Diff == "":
		b.WriteString("\n(diff unavailable -- metadata only)\n")
	case len(pr.Diff) > maxPRDiffBytes:
		b.WriteString("\n--- diff (truncated) ---\n")
		b.WriteString(pr.Diff[:maxPRDiffBytes])
		b.WriteString("\n... truncated ...\n")
	default:
		b.WriteString("\n--- diff ---\n")
		b.WriteString(pr.Diff)
	}
	return b.String()
}
