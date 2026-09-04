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

const PRDescribeToolName = "pr_describe"

//go:embed pr_describe.md
var prDescribeDescription string

const maxPRFilesShown = 30

type PRDescribeParams struct {
	Base string `json:"base" description:"The branch or ref the PR would merge into, e.g. 'main'. Required."`
	Head string `json:"head,omitempty" description:"The branch being described. Defaults to the current branch."`
	Dir  string `json:"dir,omitempty" description:"A directory inside the repository. Defaults to the working directory."`
}

type PRDescribeResponseMetadata struct {
	Commits      int  `json:"commits"`
	FilesChanged int  `json:"files_changed"`
	Insertions   int  `json:"insertions"`
	Deletions    int  `json:"deletions"`
	HasTests     bool `json:"has_tests"`
	Tickets      int  `json:"tickets"`
}

func NewPRDescribeTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		PRDescribeToolName,
		prDescribeDescription,
		func(ctx context.Context, params PRDescribeParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.Base) == "" {
				return fantasy.NewTextErrorResponse("base is required, e.g. 'main'"), nil
			}

			dir := cmp.Or(params.Dir, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			head := params.Head
			if head == "" {
				current, err := gitx.CurrentBranch(ctx, dir)
				if err != nil {
					return gitErrorResponse(err), nil
				}
				head = current
			}

			if head == params.Base {
				return fantasy.NewTextErrorResponse(
					fmt.Sprintf("head and base are both %q -- there is nothing to compare. Pass head explicitly if you meant a different branch.", head)), nil
			}

			summary, err := gitx.SummarisePR(ctx, dir, params.Base, head)
			if err != nil {
				return gitErrorResponse(err), nil
			}

			if len(summary.Commits) == 0 {
				return fantasy.NewTextResponse(fmt.Sprintf(
					"No commits found in %s..%s. Nothing to describe.", params.Base, head)), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatPRDescription(summary)),
				PRDescribeResponseMetadata{
					Commits:      len(summary.Commits),
					FilesChanged: len(summary.Diff.Files),
					Insertions:   summary.Diff.Insertions,
					Deletions:    summary.Diff.Deletions,
					HasTests:     summary.HasTests,
					Tickets:      len(summary.TicketRefs),
				},
			), nil
		},
	)
}

func formatPRDescription(s gitx.PRSummary) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Draft PR description for %s -> %s (not posted anywhere).\n\n", s.Head, s.Base)

	// The summary line leads with the most impactful thing, which is
	// usually the single feature or fix, not a restatement of the branch
	// name.
	b.WriteString("## Summary\n")
	sections := gitx.BuildChangelog(s.Commits)
	if len(sections) > 0 && len(sections[0].Entries) > 0 {
		lead := sections[0].Entries[0]
		if lead.Scope != "" {
			fmt.Fprintf(&b, "%s (%s)\n", lead.Description, lead.Scope)
		} else {
			fmt.Fprintf(&b, "%s\n", lead.Description)
		}
	} else {
		fmt.Fprintf(&b, "%d commit(s) with no Conventional Commits structure -- summarise in your own words.\n", len(s.Commits))
	}

	b.WriteString("\n## Changes\n")
	for _, section := range sections {
		fmt.Fprintf(&b, "\n**%s**\n", section.Title)
		for _, e := range section.Entries {
			b.WriteString("- ")
			if e.Scope != "" {
				fmt.Fprintf(&b, "**%s:** ", e.Scope)
			}
			fmt.Fprintf(&b, "%s\n", e.Description)
		}
	}

	fmt.Fprintf(&b, "\n## Diff\n%d file(s) changed, +%d -%d\n",
		len(s.Diff.Files), s.Diff.Insertions, s.Diff.Deletions)

	if len(s.TopLevelDirs) > 0 {
		dirs := gitx.SortedDirs(s.TopLevelDirs)
		parts := make([]string, 0, len(dirs))
		for _, d := range dirs {
			parts = append(parts, fmt.Sprintf("%s (%d)", d, s.TopLevelDirs[d]))
		}
		fmt.Fprintf(&b, "Touches: %s\n", strings.Join(parts, ", "))
	}

	if s.HasTests {
		b.WriteString("Includes test changes.\n")
	} else {
		b.WriteString("No test files changed -- worth a second look if this touches logic.\n")
	}

	if len(s.TicketRefs) > 0 {
		fmt.Fprintf(&b, "\nReferences: %s\n", strings.Join(s.TicketRefs, ", "))
	}

	if len(s.Diff.Files) > 0 {
		b.WriteString("\n## Files\n")
		files := s.Diff.Files
		if len(files) > maxPRFilesShown {
			files = files[:maxPRFilesShown]
		}
		for _, f := range files {
			fmt.Fprintf(&b, "  +%-4d -%-4d %s\n", f.Insertions, f.Deletions, f.Path)
		}
		if len(files) < len(s.Diff.Files) {
			fmt.Fprintf(&b, "  ... and %d more\n", len(s.Diff.Files)-len(files))
		}
	}

	b.WriteString("\nThis is a draft built from commit messages, not from reading the diff for intent. Rewrite the summary if the commits don't tell the real story, and confirm the missing-tests note before it goes out.\n")
	return b.String()
}
