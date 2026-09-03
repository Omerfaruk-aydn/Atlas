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

var sessionModelsJSON bool

var sessionModelsCmd = &cobra.Command{
	Use:   "models <id>",
	Short: "Count which models a session used",
	Long: "Count each model that answered in a session, and how many assistant messages came from it. " +
		"Useful after switching models mid-session to see how the split actually landed. " +
		"Use --json for machine-readable output. ID can be a UUID, full hash, or hash prefix.",
	Args: cobra.ExactArgs(1),
	RunE: runSessionModels,
}

func init() {
	sessionModelsCmd.Flags().BoolVar(&sessionModelsJSON, "json", false, "output in JSON format")
	sessionCmd.AddCommand(sessionModelsCmd)
}

type modelUsage struct {
	Name  string `json:"name"`
	Calls int    `json:"calls"`
}

func runSessionModels(cmd *cobra.Command, args []string) error {
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

	return printModelUsage(cmd.OutOrStdout(), countModelUsage(msgs))
}

// countModelUsage tallies assistant messages per model. User, system, and
// tool messages carry no model of their own and are skipped; a blank model
// on an assistant message (a summary message, or a turn that never reached
// a provider) is skipped too rather than reported as an empty name.
func countModelUsage(msgs []message.Message) []modelUsage {
	counts := make(map[string]int)
	for _, m := range msgs {
		if m.Role != message.Assistant || m.Model == "" {
			continue
		}
		counts[m.Model]++
	}

	usage := make([]modelUsage, 0, len(counts))
	for name, calls := range counts {
		usage = append(usage, modelUsage{Name: name, Calls: calls})
	}
	sort.Slice(usage, func(i, j int) bool {
		if usage[i].Calls != usage[j].Calls {
			return usage[i].Calls > usage[j].Calls
		}
		return usage[i].Name < usage[j].Name
	})
	return usage
}

func printModelUsage(out io.Writer, usage []modelUsage) error {
	if sessionModelsJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(usage)
	}

	if len(usage) == 0 {
		_, err := fmt.Fprintln(out, "No model usage recorded.")
		return err
	}

	for _, u := range usage {
		if _, err := fmt.Fprintf(out, "%-30s %d\n", u.Name, u.Calls); err != nil {
			return err
		}
	}
	return nil
}
