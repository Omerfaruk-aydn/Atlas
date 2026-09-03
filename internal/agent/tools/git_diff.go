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

const GitDiffToolName = "git_diff"

//go:embed git_diff.md
var gitDiffDescription string

const (
	// maxDiffPatchBytes bounds the total patch text. A refactor across
	// two hundred files produces megabytes of diff, and returning it
	// whole spends the entire context on one tool call.
	maxDiffPatchBytes = 60_000
	// maxDiffFiles bounds the summary listing.
	maxDiffFiles = 200
)

type GitDiffParams struct {
	Dir          string `json:"dir,omitempty" description:"A directory inside the repository. Defaults to the working directory."`
	Staged       *bool  `json:"staged,omitempty" description:"Compare the index against HEAD -- exactly what a commit would capture. Default compares the working tree against the index."`
	Ref          string `json:"ref,omitempty" description:"A revision or range to compare against, such as 'main' or 'main..HEAD'."`
	Path         string `json:"path,omitempty" description:"Narrow to one file or directory."`
	WithPatch    *bool  `json:"with_patch,omitempty" description:"Include the unified diff text, not just the file list and line counts."`
	ContextLines int    `json:"context_lines,omitempty" description:"Unchanged lines around each hunk. Default 3."`
}

type GitDiffResponseMetadata struct {
	Files      int  `json:"files"`
	Insertions int  `json:"insertions"`
	Deletions  int  `json:"deletions"`
	Truncated  bool `json:"truncated"`
}

func NewGitDiffTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		GitDiffToolName,
		gitDiffDescription,
		func(ctx context.Context, params GitDiffParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			staged := params.Staged != nil && *params.Staged
			if staged && params.Ref != "" {
				// --cached with a ref means something else entirely
				// (index vs. that ref), and the caller almost certainly
				// meant one or the other.
				return fantasy.NewTextErrorResponse(
					"pass either staged or ref, not both: staged compares the index to HEAD, ref compares against a revision"), nil
			}

			dir := cmp.Or(params.Dir, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			withPatch := params.WithPatch != nil && *params.WithPatch
			diff, err := gitx.GetDiff(ctx, dir, gitx.DiffOptions{
				Staged:        staged,
				Ref:           params.Ref,
				Path:          params.Path,
				WithPatch:     withPatch,
				ContextLines:  params.ContextLines,
				MaxPatchBytes: maxDiffPatchBytes,
			})
			if err != nil {
				return gitErrorResponse(err), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatDiff(diff, params, staged, withPatch)),
				GitDiffResponseMetadata{
					Files:      len(diff.Files),
					Insertions: diff.Insertions,
					Deletions:  diff.Deletions,
					Truncated:  diff.Truncated,
				},
			), nil
		},
	)
}

func formatDiff(d gitx.Diff, params GitDiffParams, staged, withPatch bool) string {
	scope := describeDiffScope(params, staged)

	if len(d.Files) == 0 {
		return "No changes " + scope + "."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d file(s) changed %s, +%d -%d\n\n", len(d.Files), scope, d.Insertions, d.Deletions)

	files := d.Files
	if len(files) > maxDiffFiles {
		files = files[:maxDiffFiles]
	}

	for _, f := range files {
		switch {
		case f.Binary:
			fmt.Fprintf(&b, "  binary   %s\n", f.Path)
		case f.OrigPath != "":
			fmt.Fprintf(&b, "  +%-4d -%-4d %s (renamed from %s)\n", f.Insertions, f.Deletions, f.Path, f.OrigPath)
		default:
			fmt.Fprintf(&b, "  +%-4d -%-4d %s\n", f.Insertions, f.Deletions, f.Path)
		}
	}
	if len(files) < len(d.Files) {
		fmt.Fprintf(&b, "  ... and %d more file(s)\n", len(d.Files)-len(files))
	}

	if withPatch {
		for _, f := range d.Files {
			if f.Patch == "" {
				continue
			}
			fmt.Fprintf(&b, "\n--- %s ---\n%s", f.Path, f.Patch)
		}
		if d.Truncated {
			b.WriteString("\nSome patch text was dropped to stay within budget; the file summary above is complete. Narrow with `path` to see the rest.\n")
		}
	}

	return b.String()
}

// describeDiffScope names what was compared. A diff summary that does not
// say which comparison produced it is how "nothing changed" gets read as
// "my edit did not land" when the real answer is that it is staged.
func describeDiffScope(params GitDiffParams, staged bool) string {
	var scope string
	switch {
	case staged:
		scope = "in the index vs. HEAD (what a commit would capture)"
	case params.Ref != "":
		if strings.Contains(params.Ref, "..") {
			scope = "across " + params.Ref
		} else {
			scope = "vs. " + params.Ref
		}
	default:
		scope = "in the working tree vs. the index (unstaged)"
	}
	if params.Path != "" {
		scope += ", under " + params.Path
	}
	return scope
}
