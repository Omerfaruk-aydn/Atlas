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
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/gitx"
)

const GitStatusToolName = "git_status"

//go:embed git_status.md
var gitStatusDescription string

// maxStatusFiles bounds the listing. A tree with thousands of untracked
// files -- a fresh clone with no .gitignore, say -- would otherwise fill
// the context with a directory listing.
const maxStatusFiles = 100

type GitStatusParams struct {
	Path string `json:"path,omitempty" description:"A directory inside the repository. Defaults to the working directory."`
}

type GitStatusResponseMetadata struct {
	Branch    string `json:"branch"`
	Clean     bool   `json:"clean"`
	Staged    int    `json:"staged"`
	Unstaged  int    `json:"unstaged"`
	Conflicts int    `json:"conflicts"`
	Ahead     int    `json:"ahead"`
	Behind    int    `json:"behind"`
}

func NewGitStatusTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		GitStatusToolName,
		gitStatusDescription,
		func(ctx context.Context, params GitStatusParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			dir := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			status, err := gitx.GetStatus(ctx, dir)
			if err != nil {
				return gitErrorResponse(err), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatGitStatus(status)),
				GitStatusResponseMetadata{
					Branch:    status.Branch,
					Clean:     status.Clean(),
					Staged:    status.StagedCount(),
					Unstaged:  status.UnstagedCount(),
					Conflicts: len(status.Conflicts),
					Ahead:     status.Ahead,
					Behind:    status.Behind,
				},
			), nil
		},
	)
}

// gitErrorResponse turns the two conditions that are answers rather than
// failures -- no repository, no git -- into plain text the model can act
// on, and everything else into an error.
func gitErrorResponse(err error) fantasy.ToolResponse {
	switch {
	case errors.Is(err, gitx.ErrNotARepository):
		return fantasy.NewTextResponse("Not a git repository (and no parent directory is one).")
	case errors.Is(err, gitx.ErrGitMissing):
		return fantasy.NewTextResponse("git is not installed or not on PATH, so nothing about version control can be read here.")
	}
	return fantasy.NewTextErrorResponse(err.Error())
}

func formatGitStatus(s gitx.Status) string {
	var b strings.Builder

	if s.Detached {
		fmt.Fprintf(&b, "HEAD detached at %s\n", s.Branch)
	} else {
		fmt.Fprintf(&b, "On branch %s", s.Branch)
		if s.Upstream != "" {
			fmt.Fprintf(&b, " (tracking %s", s.Upstream)
			switch {
			case s.Ahead > 0 && s.Behind > 0:
				fmt.Fprintf(&b, ", %d ahead and %d behind -- diverged", s.Ahead, s.Behind)
			case s.Ahead > 0:
				fmt.Fprintf(&b, ", %d ahead", s.Ahead)
			case s.Behind > 0:
				fmt.Fprintf(&b, ", %d behind", s.Behind)
			}
			b.WriteString(")")
		}
		b.WriteString("\n")
	}

	if s.Clean() {
		b.WriteString("\nWorking tree clean.\n")
		return b.String()
	}

	// Conflicts first. Nothing else in the tree can be committed while
	// they are open, so burying them under the file list would be
	// leading the reader past the only blocking problem.
	if len(s.Conflicts) > 0 {
		fmt.Fprintf(&b, "\n%d conflicted path(s) -- resolve these before anything else:\n", len(s.Conflicts))
		for _, path := range s.Conflicts {
			fmt.Fprintf(&b, "  %s\n", path)
		}
	}

	var staged, unstaged, untracked []gitx.FileStatus
	for _, f := range s.Files {
		if f.Staged == "unmerged" {
			continue // Already listed above.
		}
		if f.Unstaged == "untracked" {
			untracked = append(untracked, f)
			continue
		}
		if f.Staged != "" {
			staged = append(staged, f)
		}
		if f.Unstaged != "" {
			unstaged = append(unstaged, f)
		}
	}

	writeSection(&b, "staged (would be committed)", staged, true)
	writeSection(&b, "not staged (would NOT be committed)", unstaged, false)
	writeSection(&b, "untracked", untracked, false)

	return b.String()
}

func writeSection(b *strings.Builder, title string, files []gitx.FileStatus, staged bool) {
	if len(files) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s, %d:\n", title, len(files))

	shown := files
	if len(shown) > maxStatusFiles {
		shown = shown[:maxStatusFiles]
	}
	for _, f := range shown {
		state := f.Unstaged
		if staged {
			state = f.Staged
		}
		if f.OrigPath != "" {
			fmt.Fprintf(b, "  %-11s %s (from %s)\n", state, f.Path, f.OrigPath)
		} else {
			fmt.Fprintf(b, "  %-11s %s\n", state, f.Path)
		}
	}
	if len(shown) < len(files) {
		fmt.Fprintf(b, "  ... and %d more\n", len(files)-len(shown))
	}
}
