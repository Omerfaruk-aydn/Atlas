package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/event"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/spf13/cobra"
)

var (
	sessionTagJSON  bool
	sessionTagClear bool
)

var sessionTagCmd = &cobra.Command{
	Use:   "tag <id> [tags...]",
	Short: "Show or set a session's tags",
	Long: "Show a session's tags, or replace them wholesale with the given list. " +
		"Use --clear to remove all tags. Use --json for machine-readable output. " +
		"ID can be a UUID, full hash, or hash prefix.",
	Args: cobra.MinimumNArgs(1),
	RunE: runSessionTag,
}

func init() {
	sessionTagCmd.Flags().BoolVar(&sessionTagJSON, "json", false, "output in JSON format")
	sessionTagCmd.Flags().BoolVar(&sessionTagClear, "clear", false, "remove all tags from the session")
	sessionCmd.AddCommand(sessionTagCmd)
}

type sessionTagResult struct {
	ID   string   `json:"id"`
	UUID string   `json:"uuid"`
	Tags []string `json:"tags"`
}

func runSessionTag(cmd *cobra.Command, args []string) error {
	event.SetNonInteractive(true)

	ctx, svc, cleanup, err := sessionSetup(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	event.SessionTagged(sessionTagJSON)

	sess, err := resolveSessionID(ctx, svc.sessions, args[0])
	if err != nil {
		return err
	}

	// Neither new tags nor --clear: just report the current tags rather
	// than treating a bare `session tag <id>` as "set to nothing."
	if len(args) == 1 && !sessionTagClear {
		return printSessionTags(cmd, sess.ID, sess.Tags)
	}

	tags := normalizeTags(args[1:])
	if err := svc.sessions.SetTags(ctx, sess.ID, tags); err != nil {
		return fmt.Errorf("failed to set tags: %w", err)
	}

	return printSessionTags(cmd, sess.ID, tags)
}

// normalizeTags trims whitespace and drops empties and duplicates, keeping
// the first occurrence's order -- a tag list is a set the user reads back,
// not a log of what they typed.
func normalizeTags(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// filterSessionsByTag keeps only sessions carrying the given tag. Matching
// is exact: tags are a small, user-chosen vocabulary, and a substring match
// would make "wip" also return "wip-2" sessions the user never asked for.
func filterSessionsByTag(sessions []session.Session, tag string) []session.Session {
	out := make([]session.Session, 0, len(sessions))
	for _, s := range sessions {
		for _, t := range s.Tags {
			if t == tag {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

func printSessionTags(cmd *cobra.Command, sessionID string, tags []string) error {
	out := cmd.OutOrStdout()
	if sessionTagJSON {
		enc := json.NewEncoder(out)
		enc.SetEscapeHTML(false)
		return enc.Encode(sessionTagResult{
			ID:   session.HashID(sessionID),
			UUID: sessionID,
			Tags: tags,
		})
	}

	if len(tags) == 0 {
		fmt.Fprintf(out, "%s: no tags\n", session.HashID(sessionID)[:12])
		return nil
	}
	fmt.Fprintf(out, "%s: %s\n", session.HashID(sessionID)[:12], strings.Join(tags, ", "))
	return nil
}
