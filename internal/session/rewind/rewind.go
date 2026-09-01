// Package rewind lets the user jump back to an earlier point in a
// conversation and have the working directory's files restored to their
// state as of that point, without losing anything: the target session is
// never modified or deleted, only forked.
package rewind

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maincodss/atlas-agent/internal/history"
	"github.com/maincodss/atlas-agent/internal/message"
	"github.com/maincodss/atlas-agent/internal/session"
)

// Result is what ForkAt produced: the new session plus a summary of the
// disk changes it made, so callers can show the user what happened.
type Result struct {
	Session      session.Session
	FilesWritten int
	FilesDeleted int
}

// Service forks a session at a chosen checkpoint message and restores the
// working directory's files to their state as of that message.
type Service interface {
	// Preview reports how many files ForkAt would write and delete for the
	// given checkpoint, without creating a session or touching any file on
	// disk. Used to show an accurate confirmation before the (hard to
	// reverse) apply.
	Preview(ctx context.Context, sourceSessionID, upToMessageID string) (filesToWrite, filesToDelete int, err error)

	// ForkAt creates a child session (ParentSessionID = sourceSessionID)
	// containing a verbatim copy of sourceSessionID's messages up to and
	// including upToMessageID, then restores the working directory's files
	// to their content as of that message. sourceSessionID itself is never
	// modified or deleted — this is non-destructive by design.
	ForkAt(ctx context.Context, sourceSessionID, upToMessageID string) (Result, error)
}

type service struct {
	sessions session.Service
	messages message.Service
	files    history.Service
}

func NewService(sessions session.Service, messages message.Service, files history.Service) Service {
	return &service{sessions: sessions, messages: messages, files: files}
}

// resolveUpToTarget returns the ordered slice of messages in sourceSessionID
// up to and including upToMessageID, and the resolved file state as of that
// point. Shared by Preview (read-only) and ForkAt (applies the result).
func (s *service) resolveUpToTarget(ctx context.Context, sourceSessionID, upToMessageID string) ([]message.Message, []history.ResolvedFile, error) {
	// message.Service.List flushes nothing itself; callers that need the
	// latest debounced state must flush first (mirrors the session-switch
	// read pattern documented on message.Service).
	if err := s.messages.FlushAll(ctx); err != nil {
		return nil, nil, fmt.Errorf("rewind: flushing pending message state: %w", err)
	}

	all, err := s.messages.List(ctx, sourceSessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("rewind: listing messages: %w", err)
	}

	targetIdx := -1
	for i, m := range all {
		if m.ID == upToMessageID {
			targetIdx = i
			break
		}
	}
	if targetIdx == -1 {
		return nil, nil, fmt.Errorf("rewind: message %q not found in session %q", upToMessageID, sourceSessionID)
	}
	upToTarget := all[:targetIdx+1]

	ids := make([]string, len(upToTarget))
	for i, m := range upToTarget {
		ids[i] = m.ID
	}
	resolved, err := s.files.ResolveAsOf(ctx, sourceSessionID, ids)
	if err != nil {
		return nil, nil, fmt.Errorf("rewind: resolving file state: %w", err)
	}

	return upToTarget, resolved, nil
}

func (s *service) Preview(ctx context.Context, sourceSessionID, upToMessageID string) (int, int, error) {
	_, resolved, err := s.resolveUpToTarget(ctx, sourceSessionID, upToMessageID)
	if err != nil {
		return 0, 0, err
	}
	written, deleted := countChanges(resolved)
	return written, deleted, nil
}

func (s *service) ForkAt(ctx context.Context, sourceSessionID, upToMessageID string) (Result, error) {
	source, err := s.sessions.Get(ctx, sourceSessionID)
	if err != nil {
		return Result{}, fmt.Errorf("rewind: loading source session: %w", err)
	}

	upToTarget, resolved, err := s.resolveUpToTarget(ctx, sourceSessionID, upToMessageID)
	if err != nil {
		return Result{}, err
	}

	child, err := s.sessions.CreateChild(ctx, sourceSessionID, rewindTitle(source.Title))
	if err != nil {
		return Result{}, fmt.Errorf("rewind: creating forked session: %w", err)
	}

	for _, m := range upToTarget {
		if _, err := s.messages.Create(ctx, child.ID, message.CreateMessageParams{
			Role:             m.Role,
			Parts:            m.Parts,
			Model:            m.Model,
			Provider:         m.Provider,
			IsSummaryMessage: m.IsSummaryMessage,
		}); err != nil {
			return Result{}, fmt.Errorf("rewind: copying message %q: %w", m.ID, err)
		}
	}

	result := Result{Session: child}
	for _, rf := range resolved {
		if rf.Content == nil {
			if err := os.Remove(rf.Path); err != nil && !os.IsNotExist(err) {
				return result, fmt.Errorf("rewind: removing %q: %w", rf.Path, err)
			}
			result.FilesDeleted++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(rf.Path), 0o755); err != nil {
			return result, fmt.Errorf("rewind: creating parent dir for %q: %w", rf.Path, err)
		}
		if err := os.WriteFile(rf.Path, []byte(*rf.Content), 0o644); err != nil {
			return result, fmt.Errorf("rewind: writing %q: %w", rf.Path, err)
		}
		result.FilesWritten++
	}

	return result, nil
}

func countChanges(resolved []history.ResolvedFile) (written, deleted int) {
	for _, rf := range resolved {
		if rf.Content == nil {
			deleted++
		} else {
			written++
		}
	}
	return written, deleted
}

func rewindTitle(sourceTitle string) string {
	if sourceTitle == "" {
		return "Rewind"
	}
	return "Rewind: " + sourceTitle
}
