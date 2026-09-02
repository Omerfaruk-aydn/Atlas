package cmd

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/event"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/spf13/cobra"
)

var (
	sessionPruneOlderThan int
	sessionPruneKeep      int
	sessionPruneDryRun    bool
)

var sessionPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Delete old sessions",
	Long: "Delete old sessions and their messages. Choose what to keep with --older-than (days) " +
		"or --keep (most recently updated sessions); give both and a session must be discarded by " +
		"both to go. Nothing is deleted unless one of them is given. Use --dry-run to see the list first.",
	Args: cobra.NoArgs,
	RunE: runSessionPrune,
}

func init() {
	sessionPruneCmd.Flags().IntVar(&sessionPruneOlderThan, "older-than", 0, "delete sessions not updated in this many days")
	sessionPruneCmd.Flags().IntVar(&sessionPruneKeep, "keep", 0, "keep only this many most recently updated sessions")
	sessionPruneCmd.Flags().BoolVar(&sessionPruneDryRun, "dry-run", false, "list what would be deleted without deleting it")
	sessionCmd.AddCommand(sessionPruneCmd)
}

// pruneCriteria says which sessions to discard. A zero field is "no opinion":
// with neither set nothing is selected, so an accidental bare `session prune`
// cannot wipe the history.
type pruneCriteria struct {
	// OlderThanDays discards sessions not updated within this many days.
	OlderThanDays int
	// Keep discards everything past the first Keep sessions, most recently
	// updated first.
	Keep int
}

// sessionsToPrune returns the sessions both criteria agree to discard,
// ordered oldest first. Sessions are sorted here rather than relying on the
// caller's ordering so --keep means the same thing whatever order the store
// hands them back in.
func sessionsToPrune(sessions []session.Session, c pruneCriteria, now time.Time) []session.Session {
	if c.OlderThanDays <= 0 && c.Keep <= 0 {
		return nil
	}

	byRecency := make([]session.Session, len(sessions))
	copy(byRecency, sessions)
	sort.SliceStable(byRecency, func(i, j int) bool {
		return byRecency[i].UpdatedAt > byRecency[j].UpdatedAt
	})

	// UpdatedAt is Unix seconds -- see session.Session.
	cutoff := now.AddDate(0, 0, -c.OlderThanDays).Unix()

	var out []session.Session
	for i, sess := range byRecency {
		tooOld := c.OlderThanDays > 0 && sess.UpdatedAt < cutoff
		pastKeep := c.Keep > 0 && i >= c.Keep

		// With both given, a session must fail both tests: "keep the 20
		// newest, and anything from the last 30 days" is the useful
		// reading of the two together.
		discard := tooOld || pastKeep
		if c.OlderThanDays > 0 && c.Keep > 0 {
			discard = tooOld && pastKeep
		}
		if discard {
			out = append(out, sess)
		}
	}

	// Oldest first, so the printed list reads as a history being trimmed
	// from its far end.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].UpdatedAt < out[j].UpdatedAt
	})
	return out
}

func runSessionPrune(cmd *cobra.Command, _ []string) error {
	event.SetNonInteractive(true)

	if sessionPruneOlderThan <= 0 && sessionPruneKeep <= 0 {
		return errors.New("nothing to do: pass --older-than and/or --keep")
	}

	ctx, svc, cleanup, err := sessionSetup(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	all, err := svc.sessions.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	doomed := sessionsToPrune(all, pruneCriteria{
		OlderThanDays: sessionPruneOlderThan,
		Keep:          sessionPruneKeep,
	}, time.Now())

	out := cmd.OutOrStdout()
	if len(doomed) == 0 {
		fmt.Fprintln(out, "Nothing to prune.")
		return nil
	}

	for _, sess := range doomed {
		fmt.Fprintf(out, "%s  %s  %s\n",
			session.HashID(sess.ID),
			time.Unix(sess.UpdatedAt, 0).Format(time.DateOnly),
			sess.Title,
		)
	}

	if sessionPruneDryRun {
		fmt.Fprintf(out, "\n%d session(s) would be deleted.\n", len(doomed))
		return nil
	}

	for _, sess := range doomed {
		if err := svc.sessions.Delete(ctx, sess.ID); err != nil {
			return fmt.Errorf("failed to delete session %s: %w", session.HashID(sess.ID), err)
		}
	}
	fmt.Fprintf(out, "\nDeleted %d session(s).\n", len(doomed))
	return nil
}
