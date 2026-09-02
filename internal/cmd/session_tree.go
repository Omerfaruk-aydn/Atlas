package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/event"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/spf13/cobra"
)

// maxSessionTreeDepth bounds the walk. Sub-agents can spawn sub-agents, but
// a chain this deep is a data problem (a parent cycle written by an older
// build, say), and printing forever helps nobody.
const maxSessionTreeDepth = 20

var sessionTreeCmd = &cobra.Command{
	Use:   "tree [id]",
	Short: "Show sessions and the sessions they spawned",
	Long: "Show the session hierarchy: each session with the sub-agent and forked sessions beneath it. " +
		"Give an ID to print just that session's subtree. ID can be a UUID, full hash, or hash prefix.",
	Args: cobra.MaximumNArgs(1),
	RunE: runSessionTree,
}

func init() {
	sessionCmd.AddCommand(sessionTreeCmd)
}

func runSessionTree(cmd *cobra.Command, args []string) error {
	event.SetNonInteractive(true)

	ctx, svc, cleanup, err := sessionSetup(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	var roots []session.Session
	if len(args) == 1 {
		sess, err := resolveSessionID(ctx, svc.sessions, args[0])
		if err != nil {
			return err
		}
		roots = []session.Session{sess}
	} else {
		// List returns only parentless sessions, which are exactly the
		// roots of every tree.
		roots, err = svc.sessions.List(ctx)
		if err != nil {
			return fmt.Errorf("failed to list sessions: %w", err)
		}
	}

	return writeSessionTree(ctx, cmd.OutOrStdout(), roots, svc.sessions.ListByParent)
}

// childrenFunc is the seam that lets the rendering be tested without a
// database: production passes session.Service.ListByParent.
type childrenFunc func(ctx context.Context, parentSessionID string) ([]session.Session, error)

func writeSessionTree(ctx context.Context, w io.Writer, roots []session.Session, children childrenFunc) error {
	if len(roots) == 0 {
		_, err := fmt.Fprintln(w, "No sessions found.")
		return err
	}
	for _, root := range roots {
		if err := writeSessionSubtree(ctx, w, root, children, 0); err != nil {
			return err
		}
	}
	return nil
}

func writeSessionSubtree(ctx context.Context, w io.Writer, sess session.Session, children childrenFunc, depth int) error {
	indent := strings.Repeat("  ", depth)
	prefix := ""
	if depth > 0 {
		prefix = "└─ "
	}
	title := strings.ReplaceAll(sess.Title, "\n", " ")
	if _, err := fmt.Fprintf(w, "%s%s%s  %s\n", indent, prefix, session.HashID(sess.ID)[:7], title); err != nil {
		return err
	}

	if depth >= maxSessionTreeDepth {
		_, err := fmt.Fprintf(w, "%s  (deeper sessions not shown)\n", indent)
		return err
	}

	kids, err := children(ctx, sess.ID)
	if err != nil {
		return fmt.Errorf("failed to list child sessions: %w", err)
	}
	for _, kid := range kids {
		if err := writeSessionSubtree(ctx, w, kid, children, depth+1); err != nil {
			return err
		}
	}
	return nil
}
