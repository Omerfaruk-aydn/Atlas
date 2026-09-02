package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/memory"
	"github.com/spf13/cobra"
)

var memorySearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Find lines in the agent's memory",
	Long: "Search the project and user memory stores for a substring, case-insensitively, and print the " +
		"matching lines with their scope and line number. A memory store grows past what is comfortable to " +
		"read whole, which is what `memory show` gives you.",
	Args: cobra.MinimumNArgs(1),
	RunE: runMemorySearch,
}

func init() {
	memoryCmd.AddCommand(memorySearchCmd)
}

type memoryMatch struct {
	Scope memory.Scope
	Line  int
	Text  string
}

func runMemorySearch(cmd *cobra.Command, args []string) error {
	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return err
	}
	dataDir, _ := cmd.Flags().GetString("data-dir")
	debug, _ := cmd.Flags().GetBool("debug")

	cfg, err := config.Init(cwd, dataDir, debug)
	if err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}
	store := buildMemoryStore(cfg)

	// Join rather than requiring quotes: `memory search deploy steps` is
	// what people type, and a multi-word query is the common case.
	query := strings.Join(args, " ")

	var matches []memoryMatch
	for _, scope := range memory.Scopes {
		content, err := store.Read(scope)
		if err != nil {
			return fmt.Errorf("reading %s memory: %w", scope, err)
		}
		matches = append(matches, searchMemory(scope, content, query)...)
	}

	return printMemoryMatches(cmd.OutOrStdout(), matches, query)
}

// searchMemory returns the lines of one store containing query, matched
// case-insensitively -- memory is prose the agent wrote, not identifiers,
// so case is noise rather than signal.
func searchMemory(scope memory.Scope, content, query string) []memoryMatch {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	needle := strings.ToLower(query)

	var out []memoryMatch
	for i, line := range strings.Split(content, "\n") {
		if !strings.Contains(strings.ToLower(line), needle) {
			continue
		}
		out = append(out, memoryMatch{Scope: scope, Line: i + 1, Text: strings.TrimSpace(line)})
	}
	return out
}

func printMemoryMatches(out io.Writer, matches []memoryMatch, query string) error {
	if len(matches) == 0 {
		_, err := fmt.Fprintf(out, "No memory lines match %q.\n", query)
		return err
	}
	for _, m := range matches {
		if _, err := fmt.Fprintf(out, "%s:%d: %s\n", m.Scope, m.Line, m.Text); err != nil {
			return err
		}
	}
	return nil
}
