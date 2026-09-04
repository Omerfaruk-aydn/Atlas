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
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/logtail"
)

const LogTailToolName = "log_tail"

//go:embed log_tail.md
var logTailDescription string

type LogTailParams struct {
	Path  string `json:"path" description:"Path to the log file. Required."`
	Lines int    `json:"lines,omitempty" description:"How many trailing (matching) lines to return. Defaults to 100."`
	Grep  string `json:"grep,omitempty" description:"Restrict to lines containing this substring, case-insensitively."`
	Level string `json:"level,omitempty" description:"Restrict to lines carrying this log level (error, warn, info, debug)."`
}

type LogTailResponseMetadata struct {
	LinesReturned int  `json:"lines_returned"`
	TotalLines    int  `json:"total_lines"`
	Truncated     bool `json:"truncated"`
}

func NewLogTailTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		LogTailToolName,
		logTailDescription,
		func(ctx context.Context, params LogTailParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.Path) == "" {
				return fantasy.NewTextErrorResponse("path is required"), nil
			}
			path := params.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(workingDir, path)
			}

			result, err := logtail.Tail(path, logtail.Options{
				Lines: params.Lines,
				Grep:  params.Grep,
				Level: params.Level,
			})
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("no file found at %s", relOrAbs(path, workingDir))), nil
				}
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if len(result.Lines) == 0 {
				if result.TotalLines == 0 {
					return fantasy.NewTextResponse("The file is empty."), nil
				}
				return fantasy.NewTextResponse(fmt.Sprintf("No lines matched, out of %d total.", result.TotalLines)), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatLogTail(result)),
				LogTailResponseMetadata{
					LinesReturned: len(result.Lines),
					TotalLines:    result.TotalLines,
					Truncated:     result.Truncated,
				},
			), nil
		},
	)
}

func formatLogTail(r logtail.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d line(s), out of %d total.\n\n", len(r.Lines), r.TotalLines)
	for _, line := range r.Lines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if r.Truncated {
		b.WriteString("\n(more matching lines exist further back; increase `lines` to see them)\n")
	}
	return b.String()
}
