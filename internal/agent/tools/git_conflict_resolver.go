package tools

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/gitx"
)

const GitConflictResolverToolName = "git_conflict_resolver"

//go:embed git_conflict_resolver.md
var gitConflictResolverDescription string

// maxConflictPreviewLines bounds how much of each side is shown per
// block -- enough to recognise which change is which without reprinting
// a large conflicted function in full.
const maxConflictPreviewLines = 5

type GitConflictResolverParams struct {
	Path string `json:"path" description:"Path to the file to read. Required."`
}

type GitConflictResolverResponseMetadata struct {
	Conflicts int `json:"conflicts"`
}

func NewGitConflictResolverTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		GitConflictResolverToolName,
		gitConflictResolverDescription,
		func(ctx context.Context, params GitConflictResolverParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.Path) == "" {
				return fantasy.NewTextErrorResponse("path is required"), nil
			}
			path := params.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(workingDir, path)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("no file found at %s", relOrAbs(path, workingDir))), nil
				}
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			blocks := gitx.ParseConflicts(string(data))
			if len(blocks) == 0 {
				return fantasy.NewTextResponse("No merge-conflict markers found in this file."), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatConflictBlocks(blocks, relOrAbs(path, workingDir))),
				GitConflictResolverResponseMetadata{Conflicts: len(blocks)},
			), nil
		},
	)
}

func formatConflictBlocks(blocks []gitx.ConflictBlock, relPath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d conflict(s) in %s.\n", len(blocks), relPath)

	for i, block := range blocks {
		fmt.Fprintf(&b, "\nConflict %d, lines %d-%d:\n", i+1, block.StartLine, block.EndLine)
		fmt.Fprintf(&b, "  ours (%s), %d line(s):\n%s", labelOrDefault(block.OursLabel, "HEAD"), len(block.OursLines), previewLines(block.OursLines))
		if len(block.BaseLines) > 0 {
			fmt.Fprintf(&b, "  base (%s), %d line(s):\n%s", labelOrDefault(block.BaseLabel, "common ancestor"), len(block.BaseLines), previewLines(block.BaseLines))
		}
		fmt.Fprintf(&b, "  theirs (%s), %d line(s):\n%s", labelOrDefault(block.TheirsLabel, "incoming"), len(block.TheirsLines), previewLines(block.TheirsLines))
	}

	b.WriteString("\nNothing was changed -- resolve each block by editing the file directly.\n")
	return b.String()
}

func labelOrDefault(label, fallback string) string {
	if label == "" {
		return fallback
	}
	return label
}

func previewLines(lines []string) string {
	shown := lines
	truncated := false
	if len(shown) > maxConflictPreviewLines {
		shown = shown[:maxConflictPreviewLines]
		truncated = true
	}
	var b strings.Builder
	for _, line := range shown {
		fmt.Fprintf(&b, "    %s\n", line)
	}
	if truncated {
		fmt.Fprintf(&b, "    ... %d more line(s)\n", len(lines)-len(shown))
	}
	return b.String()
}
