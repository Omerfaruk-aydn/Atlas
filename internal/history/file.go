package history

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/google/uuid"
)

const (
	InitialVersion = 0
)

type File struct {
	ID        string
	SessionID string
	Path      string
	Content   string
	Version   int64
	MessageID string
	CreatedAt int64
	UpdatedAt int64
}

// ResolvedFile is one path's resolved-as-of-a-message content, as computed
// by ResolveAsOf. Content == nil means the path did not exist yet as of the
// target message and should be deleted when applying the resolution.
type ResolvedFile struct {
	Path    string
	Content *string
}

// Service manages file versions and history for sessions.
type Service interface {
	pubsub.Subscriber[File]
	// Create records a file's version-0 content. Pass an empty messageID
	// for a genuine pre-chat baseline (the file already existed on disk
	// before this session touched it) so ResolveAsOf can fall back to it;
	// pass the current message ID when this call is a placeholder for a
	// brand-new file the tool is about to create, so ResolveAsOf does not
	// mistake it for a baseline that predates the file's existence.
	Create(ctx context.Context, sessionID, path, content, messageID string) (File, error)

	// CreateVersion creates a new version of a file, produced by messageID.
	// messageID may be empty if there is no associated message (e.g. a
	// caller outside the normal chat flow).
	CreateVersion(ctx context.Context, sessionID, path, content, messageID string) (File, error)

	Get(ctx context.Context, id string) (File, error)
	GetByPathAndSession(ctx context.Context, path, sessionID string) (File, error)
	ListBySession(ctx context.Context, sessionID string) ([]File, error)
	ListLatestSessionFiles(ctx context.Context, sessionID string) ([]File, error)
	Delete(ctx context.Context, id string) error
	DeleteSessionFiles(ctx context.Context, sessionID string) error

	// ResolveAsOf computes, for every path this session has touched, the
	// file content as of the target message (inclusive). messageIDsUpToTarget
	// must be the ordered set of every message ID in the session at or
	// before the target message; history has no message-ordering knowledge
	// of its own, so callers (message.Service) own that ordering.
	//
	// For each path, the version with the highest Version number whose
	// MessageID is in messageIDsUpToTarget wins. If none qualifies, the
	// version-0 baseline (no MessageID) is used if one exists. If neither
	// exists, the path did not exist as of the target message and is
	// reported with Content == nil (caller should delete it).
	ResolveAsOf(ctx context.Context, sessionID string, messageIDsUpToTarget []string) ([]ResolvedFile, error)
}

type service struct {
	*pubsub.Broker[File]
	db *sql.DB
	q  *db.Queries
}

func NewService(q *db.Queries, db *sql.DB) Service {
	return &service{
		Broker: pubsub.NewBroker[File](),
		q:      q,
		db:     db,
	}
}

func (s *service) Create(ctx context.Context, sessionID, path, content, messageID string) (File, error) {
	return s.createWithVersion(ctx, sessionID, path, content, messageID, InitialVersion)
}

// CreateVersion creates a new version of a file with auto-incremented version
// number. If no previous versions exist for the path, it creates the initial
// version. The provided content is stored as the new version.
func (s *service) CreateVersion(ctx context.Context, sessionID, path, content, messageID string) (File, error) {
	// Get the latest version for this path
	files, err := s.q.ListFilesByPath(ctx, path)
	if err != nil {
		return File{}, err
	}

	if len(files) == 0 {
		// No previous versions, create initial
		return s.createWithVersion(ctx, sessionID, path, content, messageID, InitialVersion)
	}

	// Get the latest version
	latestFile := files[0] // Files are ordered by version DESC, created_at DESC
	nextVersion := latestFile.Version + 1

	return s.createWithVersion(ctx, sessionID, path, content, messageID, nextVersion)
}

func (s *service) createWithVersion(ctx context.Context, sessionID, path, content, messageID string, version int64) (File, error) {
	// Maximum number of retries for transaction conflicts
	const maxRetries = 3
	var file File
	var err error

	// Retry loop for transaction conflicts
	for attempt := range maxRetries {
		// Start a transaction
		tx, txErr := s.db.BeginTx(ctx, nil)
		if txErr != nil {
			return File{}, fmt.Errorf("failed to begin transaction: %w", txErr)
		}

		// Create a new queries instance with the transaction
		qtx := s.q.WithTx(tx)

		// Try to create the file within the transaction
		dbFile, txErr := qtx.CreateFile(ctx, db.CreateFileParams{
			ID:        uuid.New().String(),
			SessionID: sessionID,
			Path:      path,
			Content:   content,
			Version:   version,
			MessageID: sql.NullString{String: messageID, Valid: messageID != ""},
		})
		if txErr != nil {
			// Rollback the transaction
			tx.Rollback()

			// Check if this is a uniqueness constraint violation
			if strings.Contains(txErr.Error(), "UNIQUE constraint failed") {
				if attempt < maxRetries-1 {
					// If we have retries left, increment version and try again
					version++
					continue
				}
			}
			return File{}, txErr
		}

		// Commit the transaction
		if txErr = tx.Commit(); txErr != nil {
			return File{}, fmt.Errorf("failed to commit transaction: %w", txErr)
		}

		file = s.fromDBItem(dbFile)
		s.Publish(pubsub.CreatedEvent, file)
		return file, nil
	}

	return file, err
}

func (s *service) Get(ctx context.Context, id string) (File, error) {
	dbFile, err := s.q.GetFile(ctx, id)
	if err != nil {
		return File{}, err
	}
	return s.fromDBItem(dbFile), nil
}

func (s *service) GetByPathAndSession(ctx context.Context, path, sessionID string) (File, error) {
	dbFile, err := s.q.GetFileByPathAndSession(ctx, db.GetFileByPathAndSessionParams{
		Path:      path,
		SessionID: sessionID,
	})
	if err != nil {
		return File{}, err
	}
	return s.fromDBItem(dbFile), nil
}

func (s *service) ListBySession(ctx context.Context, sessionID string) ([]File, error) {
	dbFiles, err := s.q.ListFilesBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	files := make([]File, len(dbFiles))
	for i, dbFile := range dbFiles {
		files[i] = s.fromDBItem(dbFile)
	}
	return files, nil
}

func (s *service) ListLatestSessionFiles(ctx context.Context, sessionID string) ([]File, error) {
	dbFiles, err := s.q.ListLatestSessionFiles(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	files := make([]File, len(dbFiles))
	for i, dbFile := range dbFiles {
		files[i] = s.fromDBItem(dbFile)
	}
	return files, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	file, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	err = s.q.DeleteFile(ctx, id)
	if err != nil {
		return err
	}
	s.Publish(pubsub.DeletedEvent, file)
	return nil
}

func (s *service) DeleteSessionFiles(ctx context.Context, sessionID string) error {
	files, err := s.ListBySession(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, file := range files {
		err = s.Delete(ctx, file.ID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *service) fromDBItem(item db.File) File {
	return File{
		ID:        item.ID,
		SessionID: item.SessionID,
		Path:      item.Path,
		Content:   item.Content,
		Version:   item.Version,
		MessageID: item.MessageID.String,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}
}

// ResolveAsOf implements Service.ResolveAsOf. See the interface doc for the
// exact selection semantics.
func (s *service) ResolveAsOf(ctx context.Context, sessionID string, messageIDsUpToTarget []string) ([]ResolvedFile, error) {
	files, err := s.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	allowed := make(map[string]struct{}, len(messageIDsUpToTarget))
	for _, id := range messageIDsUpToTarget {
		allowed[id] = struct{}{}
	}

	// ListBySession is ordered version ASC, created_at ASC. Within each path
	// group we track the highest-version qualifying entry (message_id in
	// allowed) and, separately, the baseline (version 0, no message_id) as a
	// fallback. Because versions are visited ascending, the last qualifying
	// entry seen is always the highest-version one, so a plain overwrite is
	// correct; a qualifying entry always outranks the baseline regardless of
	// visit order.
	type resolution struct {
		qualifying *string
		baseline   *string
	}
	byPath := make(map[string]*resolution)
	order := make([]string, 0)

	for _, f := range files {
		r, ok := byPath[f.Path]
		if !ok {
			r = &resolution{}
			byPath[f.Path] = r
			order = append(order, f.Path)
		}

		content := f.Content
		if _, qualifies := allowed[f.MessageID]; qualifies {
			r.qualifying = &content
		} else if f.Version == InitialVersion && f.MessageID == "" {
			r.baseline = &content
		}
	}

	resolved := make([]ResolvedFile, 0, len(order))
	for _, path := range order {
		r := byPath[path]
		switch {
		case r.qualifying != nil:
			resolved = append(resolved, ResolvedFile{Path: path, Content: r.qualifying})
		case r.baseline != nil:
			resolved = append(resolved, ResolvedFile{Path: path, Content: r.baseline})
		default:
			resolved = append(resolved, ResolvedFile{Path: path, Content: nil})
		}
	}
	return resolved, nil
}
