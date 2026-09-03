package cmd

import (
	"fmt"
	"io"
	"sort"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/event"
	"github.com/spf13/cobra"
)

var sessionDiffCmd = &cobra.Command{
	Use:   "diff <id1> <id2>",
	Short: "Compare tool usage between two sessions",
	Long: "Show how tool usage differs between two sessions: which tools only one of them called, " +
		"and how the call counts differ for tools both used. IDs can be a UUID, full hash, or hash prefix.",
	Args: cobra.ExactArgs(2),
	RunE: runSessionDiff,
}

func init() {
	sessionCmd.AddCommand(sessionDiffCmd)
}

func runSessionDiff(cmd *cobra.Command, args []string) error {
	event.SetNonInteractive(true)

	ctx, svc, cleanup, err := sessionSetup(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	sessA, err := resolveSessionID(ctx, svc.sessions, args[0])
	if err != nil {
		return err
	}
	sessB, err := resolveSessionID(ctx, svc.sessions, args[1])
	if err != nil {
		return err
	}

	msgsA, err := svc.messages.List(ctx, sessA.ID)
	if err != nil {
		return fmt.Errorf("failed to list messages: %w", err)
	}
	msgsB, err := svc.messages.List(ctx, sessB.ID)
	if err != nil {
		return fmt.Errorf("failed to list messages: %w", err)
	}

	return printToolUsageDiff(cmd.OutOrStdout(), toolCallCounts(countToolUsage(msgsA)), toolCallCounts(countToolUsage(msgsB)))
}

// toolCallCounts reduces a toolUsage slice to name -> call count, dropping
// the error tally: a diff is about what each session touched, not about
// which one failed more.
func toolCallCounts(usage []toolUsage) map[string]int {
	counts := make(map[string]int, len(usage))
	for _, u := range usage {
		counts[u.Name] = u.Calls
	}
	return counts
}

// toolDiffLine is one tool's comparison across two sessions. A and B are
// zero when the tool did not appear in that session at all, which is
// distinguishable from a tool that was called zero times: nothing is
// counted unless the tool appears in at least one of the two maps.
type toolDiffLine struct {
	Name string
	A    int
	B    int
}

func diffToolUsage(a, b map[string]int) []toolDiffLine {
	names := make(map[string]struct{}, len(a)+len(b))
	for name := range a {
		names[name] = struct{}{}
	}
	for name := range b {
		names[name] = struct{}{}
	}

	lines := make([]toolDiffLine, 0, len(names))
	for name := range names {
		lines = append(lines, toolDiffLine{Name: name, A: a[name], B: b[name]})
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].Name < lines[j].Name })
	return lines
}

func printToolUsageDiff(out io.Writer, a, b map[string]int) error {
	lines := diffToolUsage(a, b)
	if len(lines) == 0 {
		_, err := fmt.Fprintln(out, "Neither session called any tools.")
		return err
	}

	for _, l := range lines {
		switch {
		case l.A > 0 && l.B == 0:
			if _, err := fmt.Fprintf(out, "- %s: %d (only in first)\n", l.Name, l.A); err != nil {
				return err
			}
		case l.B > 0 && l.A == 0:
			if _, err := fmt.Fprintf(out, "+ %s: %d (only in second)\n", l.Name, l.B); err != nil {
				return err
			}
		case l.A == l.B:
			if _, err := fmt.Fprintf(out, "  %s: %d\n", l.Name, l.A); err != nil {
				return err
			}
		default:
			if _, err := fmt.Fprintf(out, "~ %s: %d -> %d\n", l.Name, l.A, l.B); err != nil {
				return err
			}
		}
	}
	return nil
}
