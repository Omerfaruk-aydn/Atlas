package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/gitx"
)

const GitConventionalCommitToolName = "git_conventional_commit"

//go:embed git_conventional_commit.md
var gitConventionalCommitDescription string

type GitConventionalCommitParams struct {
	Dir string `json:"dir,omitempty" description:"A directory inside the repository. Defaults to the working directory."`
}

type GitConventionalCommitResponseMetadata struct {
	Type       string `json:"type"`
	Scope      string `json:"scope"`
	Confidence string `json:"confidence"`
	FilesCount int    `json:"files_count"`
}

func NewGitConventionalCommitTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		GitConventionalCommitToolName,
		gitConventionalCommitDescription,
		func(ctx context.Context, params GitConventionalCommitParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			dir := cmp.Or(params.Dir, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			suggestion, err := gitx.SuggestCommitType(ctx, dir)
			if err != nil {
				return gitErrorResponse(err), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatCommitTypeSuggestion(suggestion)),
				GitConventionalCommitResponseMetadata{
					Type:       suggestion.Type,
					Scope:      suggestion.Scope,
					Confidence: suggestion.Confidence,
					FilesCount: suggestion.FilesCount,
				},
			), nil
		},
	)
}

func formatCommitTypeSuggestion(s gitx.CommitTypeSuggestion) string {
	if s.FilesCount == 0 {
		return "Nothing is staged. Stage the changes you intend to commit and run this again."
	}

	var b strings.Builder

	header := s.Type
	if s.Scope != "" {
		header = fmt.Sprintf("%s(%s)", s.Type, s.Scope)
	}
	fmt.Fprintf(&b, "Suggested prefix: %s: <description>\n", header)
	fmt.Fprintf(&b, "Confidence: %s -- %s\n", s.Confidence, s.Rationale)

	if len(s.ByCategory) > 0 {
		type entry struct {
			category string
			n        int
		}
		entries := make([]entry, 0, len(s.ByCategory))
		for c, n := range s.ByCategory {
			entries = append(entries, entry{c, n})
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].n != entries[j].n {
				return entries[i].n > entries[j].n
			}
			return entries[i].category < entries[j].category
		})
		parts := make([]string, 0, len(entries))
		for _, e := range entries {
			parts = append(parts, fmt.Sprintf("%s %d", e.category, e.n))
		}
		fmt.Fprintf(&b, "Files by category: %s\n", strings.Join(parts, ", "))
	}

	if s.Confidence != "high" {
		b.WriteString("\nThis is a starting point, not the answer -- read the diff and override the type if it's wrong.\n")
	}
	return b.String()
}
