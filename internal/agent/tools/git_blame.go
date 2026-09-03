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

const GitBlameToolName = "git_blame"

//go:embed git_blame.md
var gitBlameDescription string

const (
	// maxBlameSpans bounds the block listing. Blaming a 5000-line file
	// produces hundreds of blocks, which is a wall of text rather than an
	// answer -- the description points at narrowing the range instead.
	maxBlameSpans = 60
	// maxBlameAuthors bounds the ownership summary.
	maxBlameAuthors = 10
)

type GitBlameParams struct {
	Path             string `json:"path" description:"The file to blame. Required."`
	Dir              string `json:"dir,omitempty" description:"A directory inside the repository. Defaults to the working directory."`
	StartLine        int    `json:"start_line,omitempty" description:"First line to blame, 1-based and inclusive."`
	EndLine          int    `json:"end_line,omitempty" description:"Last line to blame, inclusive."`
	Rev              string `json:"rev,omitempty" description:"Blame the file as of this revision instead of the working tree."`
	IgnoreWhitespace *bool  `json:"ignore_whitespace,omitempty" description:"Ignore whitespace-only changes, so a reformatting commit does not claim every line it reindented."`
	ShowLines        *bool  `json:"show_lines,omitempty" description:"Include the source text beside each block. Off by default."`
}

type GitBlameResponseMetadata struct {
	Lines   int `json:"lines"`
	Spans   int `json:"spans"`
	Authors int `json:"authors"`
}

func NewGitBlameTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		GitBlameToolName,
		gitBlameDescription,
		func(ctx context.Context, params GitBlameParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.Path) == "" {
				return fantasy.NewTextErrorResponse("path is required"), nil
			}
			// An end without a start silently blames one line at line 0,
			// which returns nothing and looks like an empty file.
			if params.EndLine > 0 && params.StartLine <= 0 {
				return fantasy.NewTextErrorResponse("end_line needs a start_line"), nil
			}
			if params.StartLine > 0 && params.EndLine > 0 && params.EndLine < params.StartLine {
				return fantasy.NewTextErrorResponse(
					fmt.Sprintf("end_line (%d) is before start_line (%d)", params.EndLine, params.StartLine)), nil
			}

			dir := cmp.Or(params.Dir, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			lines, err := gitx.Blame(ctx, dir, params.Path, gitx.BlameOptions{
				StartLine:        params.StartLine,
				EndLine:          params.EndLine,
				Rev:              params.Rev,
				IgnoreWhitespace: params.IgnoreWhitespace != nil && *params.IgnoreWhitespace,
			})
			if err != nil {
				return gitErrorResponse(err), nil
			}

			spans := gitx.Spans(lines)
			authors := gitx.Authors(lines)

			showLines := params.ShowLines != nil && *params.ShowLines
			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatBlame(params.Path, lines, spans, authors, showLines)),
				GitBlameResponseMetadata{
					Lines:   len(lines),
					Spans:   len(spans),
					Authors: len(authors),
				},
			), nil
		},
	)
}

func formatBlame(path string, lines []gitx.BlameLine, spans []gitx.BlameSpan, authors []gitx.AuthorStat, showLines bool) string {
	if len(lines) == 0 {
		return fmt.Sprintf("No blame information for %s -- the file may be empty, untracked, or outside the requested line range.", path)
	}

	byLine := make(map[int]string, len(lines))
	for _, l := range lines {
		byLine[l.Line] = l.Content
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d line(s) from %d commit(s).\n\n", path, len(lines), len(spans))

	shown := spans
	if len(shown) > maxBlameSpans {
		shown = shown[:maxBlameSpans]
	}

	for _, s := range shown {
		if s.Lines() == 1 {
			fmt.Fprintf(&b, "line %d", s.Start)
		} else {
			fmt.Fprintf(&b, "lines %d-%d (%d)", s.Start, s.End, s.Lines())
		}
		fmt.Fprintf(&b, "  %s  %s  %s\n", s.Short, formatCommitDate(s.Date), s.Author)
		fmt.Fprintf(&b, "  %s\n", s.Summary)

		if showLines {
			for n := s.Start; n <= s.End; n++ {
				if content, ok := byLine[n]; ok {
					fmt.Fprintf(&b, "  %5d| %s\n", n, content)
				}
			}
		}
	}

	if len(shown) < len(spans) {
		fmt.Fprintf(&b, "\n... and %d more block(s). Narrow with start_line/end_line.\n", len(spans)-len(shown))
	}

	if len(authors) > 0 {
		b.WriteString("\nownership:\n")
		top := authors
		if len(top) > maxBlameAuthors {
			top = top[:maxBlameAuthors]
		}
		for _, a := range top {
			pct := float64(a.Lines) * 100 / float64(len(lines))
			fmt.Fprintf(&b, "  %4d line(s) (%2.0f%%)  %s, last touched %s\n",
				a.Lines, pct, a.Author, relativeAge(a.Latest))
		}
	}

	// Without this the reader treats the newest commit on a line as the
	// origin of the logic, which a rename or a reformat makes false.
	b.WriteString("\nThe last commit to touch a line is not always the one that introduced the logic: a rename, a move, or a reformat resets attribution. Follow the file's history with git_log if a message does not explain the line.\n")
	return b.String()
}
