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

const ChangelogGenToolName = "changelog_gen"

//go:embed changelog_gen.md
var changelogGenDescription string

// maxChangelogEntriesPerSection bounds one section's listing. A section
// with hundreds of entries (a whole project history with no range given)
// is not a changelog anyone reads; the description points at narrowing
// the range instead.
const maxChangelogEntriesPerSection = 60

type ChangelogGenParams struct {
	Range string `json:"range" description:"A revision or range, e.g. 'v1.2.0..HEAD' for everything since a tag, or 'HEAD' for the whole history."`
	Dir   string `json:"dir,omitempty" description:"A directory inside the repository. Defaults to the working directory."`
}

type ChangelogGenResponseMetadata struct {
	Commits         int `json:"commits"`
	Conventional    int `json:"conventional"`
	BreakingChanges int `json:"breaking_changes"`
	NonConventional int `json:"non_conventional"`
}

func NewChangelogGenTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ChangelogGenToolName,
		changelogGenDescription,
		func(ctx context.Context, params ChangelogGenParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.Range) == "" {
				return fantasy.NewTextErrorResponse("range is required, e.g. 'v1.2.0..HEAD' or 'HEAD'"), nil
			}

			dir := cmp.Or(params.Dir, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			entries, err := gitx.ChangelogRange(ctx, dir, params.Range)
			if err != nil {
				return gitErrorResponse(err), nil
			}

			if len(entries) == 0 {
				return fantasy.NewTextResponse(fmt.Sprintf("No commits found in range %q.", params.Range)), nil
			}

			conventional, breaking := 0, 0
			for _, e := range entries {
				if e.Type != "" {
					conventional++
				}
				if e.Breaking {
					breaking++
				}
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatChangelog(entries, params.Range)),
				ChangelogGenResponseMetadata{
					Commits:         len(entries),
					Conventional:    conventional,
					BreakingChanges: breaking,
					NonConventional: len(entries) - conventional,
				},
			), nil
		},
	)
}

func formatChangelog(entries []gitx.ChangeEntry, revRange string) string {
	sections := gitx.BuildChangelog(entries)

	var b strings.Builder
	fmt.Fprintf(&b, "Draft changelog for %s (%d commit(s)).\n", revRange, len(entries))
	b.WriteString("This is a draft, not saved to any file -- write it yourself if it should be kept.\n")

	nonConventional := 0
	for _, e := range entries {
		if e.Type == "" {
			nonConventional++
		}
	}
	if nonConventional > 0 {
		fmt.Fprintf(&b, "%d commit(s) did not follow Conventional Commits and are listed under Other.\n", nonConventional)
	}

	for _, section := range sections {
		fmt.Fprintf(&b, "\n## %s\n", section.Title)
		entries := section.Entries
		truncated := false
		if len(entries) > maxChangelogEntriesPerSection {
			entries = entries[:maxChangelogEntriesPerSection]
			truncated = true
		}
		for _, e := range entries {
			b.WriteString("- ")
			if e.Scope != "" {
				fmt.Fprintf(&b, "**%s:** ", e.Scope)
			}
			fmt.Fprintf(&b, "%s (%s)\n", e.Description, e.Hash)
		}
		if truncated {
			fmt.Fprintf(&b, "- ... and %d more in this section\n", len(section.Entries)-len(entries))
		}
	}

	scopeCounts := gitx.ScopeCounts(entries)
	if len(scopeCounts) > 1 {
		scopes := gitx.SortedScopes(scopeCounts)
		if len(scopes) > 8 {
			scopes = scopes[:8]
		}
		parts := make([]string, 0, len(scopes))
		for _, s := range scopes {
			parts = append(parts, fmt.Sprintf("%s (%d)", s, scopeCounts[s]))
		}
		fmt.Fprintf(&b, "\nBusiest scopes: %s\n", strings.Join(parts, ", "))
	}

	b.WriteString("\nCommit subjects are terse by design. Rewrite any description that reads well only to someone already in the diff.\n")
	return b.String()
}
