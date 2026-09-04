package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/docindex"
)

const DocIndexToolName = "doc_index"

//go:embed doc_index.md
var docIndexDescription string

// maxDocIndexFilesShown bounds the printed list the same way the
// codeintel tools bound theirs -- a repository with hundreds of Markdown
// files would otherwise produce an unreadable wall of output.
const maxDocIndexFilesShown = 50

type DocIndexParams struct {
	Path     string `json:"path,omitempty" description:"Directory or single Markdown file to scan. Defaults to the working directory."`
	Query    string `json:"query,omitempty" description:"Restrict results to files whose title or any heading contains this text."`
	MaxDepth int    `json:"max_depth,omitempty" description:"Only include headings at or above this level (1-6). Defaults to 6."`
}

type DocIndexResponseMetadata struct {
	FilesMatched int `json:"files_matched"`
	FilesScanned int `json:"files_scanned"`
}

func NewDocIndexTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		DocIndexToolName,
		docIndexDescription,
		func(ctx context.Context, params DocIndexParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}

			result, err := docindex.Build(root, docindex.Options{
				Query:    params.Query,
				MaxDepth: params.MaxDepth,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatDocIndex(result, workingDir, params.Query)),
				DocIndexResponseMetadata{
					FilesMatched: len(result.Files),
					FilesScanned: result.FilesScanned,
				},
			), nil
		},
	)
}

func formatDocIndex(r docindex.Result, workingDir, query string) string {
	if r.FilesScanned == 0 {
		return "No Markdown files found to scan. Check the path."
	}
	if len(r.Files) == 0 {
		return fmt.Sprintf("No file's title or headings matched %q, across %d Markdown file(s) scanned.", query, r.FilesScanned)
	}

	var b strings.Builder
	if query != "" {
		fmt.Fprintf(&b, "%d of %d Markdown file(s) match %q.\n", len(r.Files), r.FilesScanned, query)
	} else {
		fmt.Fprintf(&b, "%d Markdown file(s).\n", len(r.Files))
	}

	shown := r.Files
	if len(shown) > maxDocIndexFilesShown {
		shown = shown[:maxDocIndexFilesShown]
	}

	for _, doc := range shown {
		rel := relOrAbs(doc.Path, workingDir)
		fmt.Fprintf(&b, "\n%s -- %s\n", rel, doc.Title)
		for _, h := range doc.Headings {
			fmt.Fprintf(&b, "%s%s:%d  %s\n", strings.Repeat("  ", h.Level-1), rel, h.Line, h.Text)
		}
	}

	if len(shown) < len(r.Files) {
		fmt.Fprintf(&b, "\n... and %d more file(s). Narrow with `path` or `query`.\n", len(r.Files)-len(shown))
	}
	return b.String()
}
