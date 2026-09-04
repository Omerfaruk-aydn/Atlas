package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/datafile"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
)

const InspectFileToolName = "inspect_file"

//go:embed inspect_file.md
var inspectFileDescription string

type InspectFileParams struct {
	Path   string `json:"path" description:"Path to the archive, SQLite database, or .ipynb notebook to inspect."`
	Action string `json:"action,omitempty" description:"list (default), read, or query. See the tool description for what each means per file kind."`
	Entry  string `json:"entry,omitempty" description:"For read on an archive: the entry name to read, as shown by list."`
	Cell   int    `json:"cell,omitempty" description:"For read on a notebook: the 0-based cell index to read, as shown by list."`
	Query  string `json:"query,omitempty" description:"For query on a SQLite database: the read-only SQL to run."`
	Limit  int    `json:"limit,omitempty" description:"For query: maximum rows returned. Defaults to 50."`
}

type InspectFileResponseMetadata struct {
	Kind   string `json:"kind"`
	Action string `json:"action"`
}

// fileKind reports which of the three formats this tool understands
// path names, or "" when it's none of them -- detected the same way
// the format itself is identified: extension for archives and
// notebooks, file header for SQLite, since a database's extension is
// only a convention.
func fileKind(path string) string {
	if datafile.ArchiveKind(path) != "" {
		return "archive"
	}
	if strings.HasSuffix(strings.ToLower(path), ".ipynb") {
		return "notebook"
	}
	if datafile.IsSQLite(path) {
		return "sqlite"
	}
	return ""
}

// NewInspectFileTool looks inside archives, SQLite databases, and
// Jupyter notebooks -- the formats a plain text view can't
// meaningfully show. PDF and anything reached over the network or by
// credential (SSH, remote databases) are deliberately out of scope;
// see inspect_file.md for why.
func NewInspectFileTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		InspectFileToolName,
		inspectFileDescription,
		func(ctx context.Context, params InspectFileParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.Path) == "" {
				return fantasy.NewTextErrorResponse("path is required"), nil
			}

			path := params.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(workingDir, path)
			}

			kind := fileKind(path)
			if kind == "" {
				return fantasy.NewTextErrorResponse(fmt.Sprintf(
					"%s is not a recognised archive (.zip, .tar, .tar.gz, .tgz), SQLite database, or Jupyter notebook (.ipynb)", params.Path)), nil
			}

			action := strings.ToLower(strings.TrimSpace(cmp.Or(params.Action, "list")))

			switch kind {
			case "archive":
				return inspectArchive(path, action, params)
			case "sqlite":
				return inspectSQLite(ctx, path, action, params)
			case "notebook":
				return inspectNotebook(path, action, params)
			default:
				return fantasy.NewTextErrorResponse("unreachable"), nil
			}
		},
	)
}

func inspectArchive(path, action string, params InspectFileParams) (fantasy.ToolResponse, error) {
	switch action {
	case "list":
		entries, err := datafile.ListArchive(path)
		if err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		if len(entries) == 0 {
			return fantasy.NewTextResponse("Empty archive."), nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "%d entries.\n", len(entries))
		for _, e := range entries {
			if e.IsDir {
				fmt.Fprintf(&b, "  %s/\n", e.Name)
			} else {
				fmt.Fprintf(&b, "  %-10d %s\n", e.Size, e.Name)
			}
		}
		return fantasy.WithResponseMetadata(
			fantasy.NewTextResponse(b.String()),
			InspectFileResponseMetadata{Kind: "archive", Action: "list"},
		), nil

	case "read":
		if strings.TrimSpace(params.Entry) == "" {
			return fantasy.NewTextErrorResponse("entry is required for read on an archive"), nil
		}
		content, truncated, err := datafile.ReadArchiveEntry(path, params.Entry)
		if err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		if truncated {
			content += "\n\n[truncated at 1 MiB]"
		}
		return fantasy.WithResponseMetadata(
			fantasy.NewTextResponse(content),
			InspectFileResponseMetadata{Kind: "archive", Action: "read"},
		), nil

	default:
		return fantasy.NewTextErrorResponse(fmt.Sprintf("unknown action %q for an archive: use list or read", action)), nil
	}
}

func inspectSQLite(ctx context.Context, path, action string, params InspectFileParams) (fantasy.ToolResponse, error) {
	switch action {
	case "list":
		tables, err := datafile.InspectSQLiteSchema(ctx, path)
		if err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		if len(tables) == 0 {
			return fantasy.NewTextResponse("No tables."), nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "%d table(s).\n", len(tables))
		for _, t := range tables {
			fmt.Fprintf(&b, "\n%s (%s)\n", t.Name, strings.Join(t.Columns, ", "))
		}
		return fantasy.WithResponseMetadata(
			fantasy.NewTextResponse(b.String()),
			InspectFileResponseMetadata{Kind: "sqlite", Action: "list"},
		), nil

	case "query":
		if strings.TrimSpace(params.Query) == "" {
			return fantasy.NewTextErrorResponse("query is required for query on a SQLite database"), nil
		}
		result, err := datafile.QuerySQLite(ctx, path, params.Query, params.Limit)
		if err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		return fantasy.WithResponseMetadata(
			fantasy.NewTextResponse(formatQueryResult(result)),
			InspectFileResponseMetadata{Kind: "sqlite", Action: "query"},
		), nil

	case "read":
		return fantasy.NewTextErrorResponse("read is not meaningful for a SQLite database -- use query"), nil

	default:
		return fantasy.NewTextErrorResponse(fmt.Sprintf("unknown action %q for a SQLite database: use list or query", action)), nil
	}
}

func formatQueryResult(result datafile.QueryResult) string {
	if len(result.Rows) == 0 {
		return "No rows."
	}

	var b strings.Builder
	fmt.Fprintln(&b, strings.Join(result.Columns, " | "))
	for _, row := range result.Rows {
		fmt.Fprintln(&b, strings.Join(row, " | "))
	}
	fmt.Fprintf(&b, "\n%d row(s).", len(result.Rows))
	if result.Truncated {
		b.WriteString(" [truncated -- raise limit for more]")
	}
	return b.String()
}

func inspectNotebook(path, action string, params InspectFileParams) (fantasy.ToolResponse, error) {
	nb, err := datafile.ReadNotebook(path)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	switch action {
	case "list":
		if len(nb.Cells) == 0 {
			return fantasy.NewTextResponse("Empty notebook."), nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "%d cell(s).\n", len(nb.Cells))
		for i, c := range nb.Cells {
			firstLine := c.Source
			if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
				firstLine = firstLine[:idx]
			}
			fmt.Fprintf(&b, "  [%d] %-8s %s\n", i, c.Type, firstLine)
		}
		return fantasy.WithResponseMetadata(
			fantasy.NewTextResponse(b.String()),
			InspectFileResponseMetadata{Kind: "notebook", Action: "list"},
		), nil

	case "read":
		if params.Cell < 0 || params.Cell >= len(nb.Cells) {
			return fantasy.NewTextErrorResponse(fmt.Sprintf("cell %d is out of range: notebook has %d cell(s)", params.Cell, len(nb.Cells))), nil
		}
		c := nb.Cells[params.Cell]

		var b strings.Builder
		fmt.Fprintf(&b, "[%d] %s\n\n%s\n", params.Cell, c.Type, c.Source)
		if c.OutputSummary != "" {
			fmt.Fprintf(&b, "\n-- output --\n%s\n", c.OutputSummary)
		}
		return fantasy.WithResponseMetadata(
			fantasy.NewTextResponse(b.String()),
			InspectFileResponseMetadata{Kind: "notebook", Action: "read"},
		), nil

	default:
		return fantasy.NewTextErrorResponse(fmt.Sprintf("unknown action %q for a notebook: use list or read", action)), nil
	}
}
