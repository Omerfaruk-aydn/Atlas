package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/event"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/message"
	"github.com/spf13/cobra"
)

var sessionToolsJSON bool

var sessionToolsCmd = &cobra.Command{
	Use:   "tools <id>",
	Short: "Count what tools a session used",
	Long: "Count each tool a session called, and how many of those calls came back as errors. " +
		"Use --json for machine-readable output. ID can be a UUID, full hash, or hash prefix.",
	Args: cobra.ExactArgs(1),
	RunE: runSessionTools,
}

func init() {
	sessionToolsCmd.Flags().BoolVar(&sessionToolsJSON, "json", false, "output in JSON format")
	sessionCmd.AddCommand(sessionToolsCmd)
}

type toolUsage struct {
	Name   string `json:"name"`
	Calls  int    `json:"calls"`
	Errors int    `json:"errors"`
}

func runSessionTools(cmd *cobra.Command, args []string) error {
	event.SetNonInteractive(true)

	ctx, svc, cleanup, err := sessionSetup(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	sess, err := resolveSessionID(ctx, svc.sessions, args[0])
	if err != nil {
		return err
	}

	msgs, err := svc.messages.List(ctx, sess.ID)
	if err != nil {
		return fmt.Errorf("failed to list messages: %w", err)
	}

	return printToolUsage(cmd.OutOrStdout(), countToolUsage(msgs))
}

// countToolUsage tallies calls per tool, and how many of those came back as
// errors. Errors are counted from the results rather than the calls: a call
// the turn was cancelled before answering has no result at all, and
// counting it as a failure would misreport a cancellation as a broken tool.
func countToolUsage(msgs []message.Message) []toolUsage {
	calls := map[string]int{}
	errs := map[string]int{}
	names := map[string]string{}

	for _, msg := range msgs {
		for _, c := range msg.ToolCalls() {
			calls[c.Name]++
			names[c.ID] = c.Name
		}
		for _, r := range msg.ToolResults() {
			if !r.IsError {
				continue
			}
			name := r.Name
			if name == "" {
				name = names[r.ToolCallID]
			}
			if name == "" {
				continue
			}
			errs[name]++
		}
	}

	out := make([]toolUsage, 0, len(calls))
	for name, n := range calls {
		out = append(out, toolUsage{Name: name, Calls: n, Errors: errs[name]})
	}
	// Busiest first, then by name so equal counts have a stable order.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Calls != out[j].Calls {
			return out[i].Calls > out[j].Calls
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func printToolUsage(out io.Writer, usage []toolUsage) error {
	if sessionToolsJSON {
		enc := json.NewEncoder(out)
		enc.SetEscapeHTML(false)
		return enc.Encode(usage)
	}

	if len(usage) == 0 {
		_, err := fmt.Fprintln(out, "This session called no tools.")
		return err
	}
	for _, u := range usage {
		if u.Errors > 0 {
			fmt.Fprintf(out, "%s: %d (%d failed)\n", u.Name, u.Calls, u.Errors)
			continue
		}
		fmt.Fprintf(out, "%s: %d\n", u.Name, u.Calls)
	}
	return nil
}
