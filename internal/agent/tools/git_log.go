package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/gitx"
)

const GitLogToolName = "git_log"

//go:embed git_log.md
var gitLogDescription string

const (
	defaultLogLimit = 20
	maxLogLimit     = 200
	// maxBodyLines bounds how much of a commit body is printed. A commit
	// with a 200-line message exists, and reprinting it whole would spend
	// the whole budget on one entry.
	maxBodyLines = 8
	// maxStatFiles bounds the per-commit file list under with_stats.
	maxStatFiles = 12
)

type GitLogParams struct {
	Dir       string `json:"dir,omitempty" description:"A directory inside the repository. Defaults to the working directory."`
	Path      string `json:"path,omitempty" description:"Restrict to commits touching this file or directory."`
	Limit     int    `json:"limit,omitempty" description:"How many commits to return. Default 20."`
	Author    string `json:"author,omitempty" description:"Filter by author name or email substring."`
	Grep      string `json:"grep,omitempty" description:"Filter by commit message substring. Matched literally, not as a regular expression."`
	Since     string `json:"since,omitempty" description:"Only commits after this time. Accepts anything git accepts, including '2 weeks ago'."`
	Until     string `json:"until,omitempty" description:"Only commits before this time."`
	Ref       string `json:"ref,omitempty" description:"A branch, tag, or revision range such as 'main..feature'. Empty means the current HEAD."`
	WithStats *bool  `json:"with_stats,omitempty" description:"Include the files each commit touched and its line counts. Costs an extra git call per commit."`
	NoMerges  *bool  `json:"no_merges,omitempty" description:"Drop merge commits."`
}

type GitLogResponseMetadata struct {
	Commits int    `json:"commits"`
	Oldest  string `json:"oldest,omitempty"`
	Newest  string `json:"newest,omitempty"`
}

func NewGitLogTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		GitLogToolName,
		gitLogDescription,
		func(ctx context.Context, params GitLogParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			dir := cmp.Or(params.Dir, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			limit := cmp.Or(params.Limit, defaultLogLimit)
			limit = min(max(limit, 1), maxLogLimit)

			commits, err := gitx.Log(ctx, dir, gitx.LogOptions{
				Limit:     limit,
				Path:      params.Path,
				Author:    params.Author,
				Grep:      params.Grep,
				Since:     params.Since,
				Until:     params.Until,
				Ref:       params.Ref,
				WithStats: params.WithStats != nil && *params.WithStats,
				NoMerges:  params.NoMerges != nil && *params.NoMerges,
			})
			if err != nil {
				return gitErrorResponse(err), nil
			}

			meta := GitLogResponseMetadata{Commits: len(commits)}
			if len(commits) > 0 {
				meta.Newest = commits[0].Short
				meta.Oldest = commits[len(commits)-1].Short
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatGitLog(commits, params)),
				meta,
			), nil
		},
	)
}

func formatGitLog(commits []gitx.Commit, params GitLogParams) string {
	if len(commits) == 0 {
		return "No commits matched." + describeFilters(params)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d commit(s)%s\n", len(commits), describeFilters(params))

	for _, c := range commits {
		b.WriteString("\n")
		fmt.Fprintf(&b, "%s  %s  %s\n", c.Short, formatCommitDate(c.Date), c.Author)
		fmt.Fprintf(&b, "  %s", c.Subject)
		if c.Merge() {
			b.WriteString("  [merge]")
		}
		b.WriteString("\n")

		if c.Body != "" {
			lines := strings.Split(c.Body, "\n")
			truncated := false
			if len(lines) > maxBodyLines {
				lines = lines[:maxBodyLines]
				truncated = true
			}
			for _, line := range lines {
				fmt.Fprintf(&b, "  | %s\n", line)
			}
			if truncated {
				b.WriteString("  | ...\n")
			}
		}

		if len(c.Files) > 0 {
			fmt.Fprintf(&b, "  %d file(s), +%d -%d\n", len(c.Files), c.Insertions, c.Deletions)
			files := c.Files
			if len(files) > maxStatFiles {
				files = files[:maxStatFiles]
			}
			for _, f := range files {
				fmt.Fprintf(&b, "    %s\n", f)
			}
			if len(files) < len(c.Files) {
				fmt.Fprintf(&b, "    ... and %d more\n", len(c.Files)-len(files))
			}
		}
	}

	return b.String()
}

// describeFilters restates what was actually asked for. Without it a
// short result reads as "this file has barely changed" when it really
// means "your date range was narrow".
func describeFilters(params GitLogParams) string {
	var parts []string
	if params.Path != "" {
		parts = append(parts, "touching "+params.Path)
	}
	if params.Ref != "" {
		parts = append(parts, "on "+params.Ref)
	}
	if params.Author != "" {
		parts = append(parts, "by "+params.Author)
	}
	if params.Grep != "" {
		parts = append(parts, "mentioning "+params.Grep)
	}
	if params.Since != "" {
		parts = append(parts, "since "+params.Since)
	}
	if params.Until != "" {
		parts = append(parts, "until "+params.Until)
	}
	if params.NoMerges != nil && *params.NoMerges {
		parts = append(parts, "excluding merges")
	}
	if len(parts) == 0 {
		return "."
	}
	return " " + strings.Join(parts, ", ") + "."
}

// formatCommitDate shows a relative age, which is what a reader actually
// wants ("last week"), alongside the absolute date for precision.
func formatCommitDate(t time.Time) string {
	if t.IsZero() {
		return "unknown date"
	}
	return fmt.Sprintf("%s (%s)", t.Format("2006-01-02"), relativeAge(t))
}

func relativeAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/24/30))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/24/365))
	}
}
