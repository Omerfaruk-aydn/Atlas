package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/event"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/message"
	"github.com/spf13/cobra"
)

var (
	sessionSearchLimit   int
	sessionSearchSession string
)

var sessionSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search past conversations",
	Long: "Full-text search across the messages of every session, newest matches first. " +
		"The query is plain words, not FTS5 syntax. Use --session to search within one session; " +
		"ID can be a UUID, full hash, or hash prefix.",
	Args: cobra.MinimumNArgs(1),
	RunE: runSessionSearch,
}

func init() {
	sessionSearchCmd.Flags().IntVarP(&sessionSearchLimit, "limit", "n", message.DefaultSearchLimit, "maximum matches to show")
	sessionSearchCmd.Flags().StringVarP(&sessionSearchSession, "session", "s", "", "limit the search to one session")
	sessionCmd.AddCommand(sessionSearchCmd)
}

func runSessionSearch(cmd *cobra.Command, args []string) error {
	event.SetNonInteractive(true)

	ctx, svc, cleanup, err := sessionSetup(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	// Every argument joined, so `session search auth token` searches for
	// both words rather than failing on an unexpected second argument.
	params := message.SearchParams{
		Query: strings.Join(args, " "),
		Limit: sessionSearchLimit,
	}
	if sessionSearchSession != "" {
		sess, err := resolveSessionID(ctx, svc.sessions, sessionSearchSession)
		if err != nil {
			return err
		}
		params.SessionID = sess.ID
	}

	hits, err := svc.messages.Search(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to search messages: %w", err)
	}

	out := cmd.OutOrStdout()
	if len(hits) == 0 {
		fmt.Fprintln(out, "No matches.")
		return nil
	}

	for _, hit := range hits {
		title := hit.SessionTitle
		if title == "" {
			title = hit.SessionID
		}
		// CreatedAt is Unix seconds -- see message.Message.
		fmt.Fprintf(out, "%s  %s  %s\n  %s\n",
			time.Unix(hit.CreatedAt, 0).Format(time.DateOnly),
			hit.Role,
			title,
			singleLine(hit.Snippet),
		)
	}
	return nil
}

// singleLine keeps one hit to one line: a snippet cut out of a multi-line
// message would otherwise break up the listing.
func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
