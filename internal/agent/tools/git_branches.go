package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/gitx"
)

const GitBranchesToolName = "git_branches"

//go:embed git_branches.md
var gitBranchesDescription string

const defaultStaleDays = 60

type GitBranchesParams struct {
	Dir           string `json:"dir,omitempty" description:"A directory inside the repository. Defaults to the working directory."`
	IncludeRemote *bool  `json:"include_remote,omitempty" description:"Also list remote-tracking branches. Off by default."`
	MergedBase    string `json:"merged_base,omitempty" description:"The branch to check merges against. Defaults to the detected default branch."`
	StaleDays     int    `json:"stale_days,omitempty" description:"Mark branches whose last commit is older than this many days. Default 60."`
}

type GitBranchesResponseMetadata struct {
	Total    int    `json:"total"`
	Merged   int    `json:"merged"`
	Stale    int    `json:"stale"`
	BaseUsed string `json:"base_used,omitempty"`
}

func NewGitBranchesTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		GitBranchesToolName,
		gitBranchesDescription,
		func(ctx context.Context, params GitBranchesParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			dir := cmp.Or(params.Dir, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			base := params.MergedBase
			if base == "" {
				base = gitx.DefaultBranch(ctx, dir)
			}

			branches, err := gitx.Branches(ctx, dir, gitx.BranchOptions{
				IncludeRemote: params.IncludeRemote != nil && *params.IncludeRemote,
				MergedBase:    base,
			})
			if err != nil {
				return gitErrorResponse(err), nil
			}

			staleDays := cmp.Or(params.StaleDays, defaultStaleDays)
			staleAfter := time.Duration(staleDays) * 24 * time.Hour

			merged, stale := 0, 0
			for _, b := range branches {
				if b.MergedInto != "" {
					merged++
				}
				if b.Age() > staleAfter {
					stale++
				}
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatBranches(branches, base, staleAfter)),
				GitBranchesResponseMetadata{
					Total:    len(branches),
					Merged:   merged,
					Stale:    stale,
					BaseUsed: base,
				},
			), nil
		},
	)
}

func formatBranches(branches []gitx.Branch, base string, staleAfter time.Duration) string {
	if len(branches) == 0 {
		return "No branches found."
	}

	// Current branch first, then most recently active, so the reader's
	// own context and the freshest work are both at the top rather than
	// buried in an alphabetical list.
	sort.SliceStable(branches, func(i, j int) bool {
		if branches[i].Current != branches[j].Current {
			return branches[i].Current
		}
		return branches[i].LastDate.After(branches[j].LastDate)
	})

	var b strings.Builder
	fmt.Fprintf(&b, "%d branch(es).", len(branches))
	if base != "" {
		fmt.Fprintf(&b, " Checked for merges against %q.", base)
	} else {
		b.WriteString(" Could not determine a base branch, so merge status is not shown.")
	}
	b.WriteString("\n\n")

	for _, br := range branches {
		marker := "  "
		if br.Current {
			marker = "* "
		}
		fmt.Fprintf(&b, "%s%s", marker, br.Name)
		if br.Remote {
			b.WriteString("  (remote)")
		}
		b.WriteString("\n")

		fmt.Fprintf(&b, "    %s  %s  %s\n", br.LastCommit, formatCommitDate(br.LastDate), br.LastAuthor)
		if br.LastSubject != "" {
			fmt.Fprintf(&b, "    %s\n", br.LastSubject)
		}

		var tags []string
		if br.MergedInto != "" {
			tags = append(tags, "merged into "+br.MergedInto)
		} else if base != "" && !br.Current && !br.Remote {
			tags = append(tags, "NOT merged")
		}
		if br.Age() > staleAfter {
			tags = append(tags, fmt.Sprintf("stale (%dd)", int(br.Age().Hours()/24)))
		}
		if br.Upstream != "" {
			switch {
			case br.Ahead > 0 && br.Behind > 0:
				tags = append(tags, fmt.Sprintf("%d ahead / %d behind %s", br.Ahead, br.Behind, br.Upstream))
			case br.Ahead > 0:
				tags = append(tags, fmt.Sprintf("%d ahead of %s", br.Ahead, br.Upstream))
			case br.Behind > 0:
				tags = append(tags, fmt.Sprintf("%d behind %s", br.Behind, br.Upstream))
			}
		}
		if len(tags) > 0 {
			fmt.Fprintf(&b, "    [%s]\n", strings.Join(tags, ", "))
		}
	}

	if base != "" {
		b.WriteString("\n\"merged\" means reachable from the base branch. A squash-merged or rebased branch will show as NOT merged even though its work is safe -- confirm before treating an unmerged branch as disposable.\n")
	}
	return b.String()
}
