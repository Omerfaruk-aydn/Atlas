// Package memory keeps the small amount of prose the agent is allowed to
// carry from one session to the next.
//
// There are two stores, and the split is deliberate. Project memory holds
// what is true about the code in front of it -- build quirks, conventions
// that are not in the repository, decisions whose reasons live in someone's
// head -- and belongs beside the project. The user profile holds what is
// true about the person -- how they want to be talked to, what they always
// ask for -- and follows them between projects.
//
// Both are bounded. The bound is the point: this text is prepended to every
// request, so an unbounded store is an unbounded bill and, past a certain
// size, worse recall rather than better. A write that would exceed the bound
// fails, and says by how much, so the agent consolidates deliberately
// instead of a store silently losing its oldest half.
package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

// Scope names one of the two stores.
type Scope string

const (
	// ScopeProject is what the agent learned about this codebase.
	ScopeProject Scope = "project"
	// ScopeUser is what the agent learned about the person using it.
	ScopeUser Scope = "user"
)

// Scopes lists every scope, in the order they are shown to the model.
var Scopes = []Scope{ScopeProject, ScopeUser}

// Valid reports whether s names a store.
func (s Scope) Valid() bool {
	return s == ScopeProject || s == ScopeUser
}

// Filename is the name the store is written under.
func (s Scope) Filename() string {
	switch s {
	case ScopeUser:
		return "USER.md"
	default:
		return "MEMORY.md"
	}
}

// Default limits, in characters. They are characters rather than tokens
// because a character count is exact, cheap, and does not change when the
// tokenizer does; the token figure it approximates is roughly a quarter of
// it for English prose.
const (
	DefaultProjectLimit = 3200
	DefaultUserLimit    = 2000
)

// ErrTooLong is returned when a write would push a store past its limit. It
// carries the numbers so the caller can tell the model exactly how much has
// to go.
type ErrTooLong struct {
	Scope Scope
	Size  int
	Limit int
}

func (e *ErrTooLong) Error() string {
	return fmt.Sprintf(
		"%s memory would be %d characters, over the %d limit by %d: remove or consolidate an entry, then write again",
		e.Scope, e.Size, e.Limit, e.Size-e.Limit,
	)
}

// ErrNotFound is returned when replace or remove is given text that is not
// in the store.
var ErrNotFound = errors.New("that text is not in memory")

// ErrAmbiguous is returned when replace or remove is given text that appears
// more than once, so there is no single entry it could mean.
var ErrAmbiguous = errors.New("that text appears more than once in memory; include enough surrounding text to make it unique")

// Store reads and writes the two memory files.
//
// It is safe for concurrent use. Reads come off disk every time rather than
// from a cache: a session that has been open for hours would otherwise go on
// serving text that another session has since rewritten.
type Store struct {
	mu sync.Mutex

	projectDir string
	userDir    string

	projectLimit int
	userLimit    int
}

// Options configures a Store.
type Options struct {
	// ProjectDir is where MEMORY.md lives, normally the project's data
	// directory.
	ProjectDir string
	// UserDir is where USER.md lives, normally the global config directory.
	UserDir string
	// ProjectLimit and UserLimit override the default character bounds. Zero
	// means the default; a negative value means unbounded, which is offered
	// for people who know what they are paying for.
	ProjectLimit int
	UserLimit    int
}

// New returns a Store. It does not touch the filesystem: the directories are
// created when something is first written, so a project that never uses
// memory never grows the files.
func New(opts Options) *Store {
	s := &Store{
		projectDir:   opts.ProjectDir,
		userDir:      opts.UserDir,
		projectLimit: opts.ProjectLimit,
		userLimit:    opts.UserLimit,
	}
	if s.projectLimit == 0 {
		s.projectLimit = DefaultProjectLimit
	}
	if s.userLimit == 0 {
		s.userLimit = DefaultUserLimit
	}
	return s
}

// Limit returns the character bound for a scope. A negative result means the
// scope is unbounded.
func (s *Store) Limit(scope Scope) int {
	if scope == ScopeUser {
		return s.userLimit
	}
	return s.projectLimit
}

// Path returns the file a scope is stored in.
func (s *Store) Path(scope Scope) string {
	if scope == ScopeUser {
		return filepath.Join(s.userDir, scope.Filename())
	}
	return filepath.Join(s.projectDir, scope.Filename())
}

// Read returns the contents of a scope. A store that has never been written
// reads as empty rather than as an error: not having learned anything yet is
// the normal state, not a failure.
func (s *Store) Read(scope Scope) (string, error) {
	if !scope.Valid() {
		return "", fmt.Errorf("unknown memory scope %q", scope)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(scope)
}

func (s *Store) read(scope Scope) (string, error) {
	data, err := os.ReadFile(s.Path(scope))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to read %s memory: %w", scope, err)
	}
	return string(data), nil
}

// Add appends an entry. Entries are one per line, so that replace and remove
// have something to aim at, and so the whole store reads as a list rather
// than as prose that has to be re-derived every time.
func (s *Store) Add(scope Scope, entry string) (string, error) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return "", errors.New("cannot add an empty entry")
	}
	if strings.Contains(entry, "\n") {
		return "", errors.New("an entry is a single line; add several entries instead of one with line breaks")
	}
	if !scope.Valid() {
		return "", fmt.Errorf("unknown memory scope %q", scope)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.read(scope)
	if err != nil {
		return "", err
	}
	if lineExists(current, entry) {
		// Already known. Reporting this rather than writing a second copy
		// keeps a store from filling with the same fact in five wordings.
		return current, nil
	}

	next := current
	if next != "" && !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	next += "- " + entry + "\n"

	return next, s.write(scope, next)
}

// Replace swaps one piece of text for another. The old text has to appear
// exactly once: a substring that matches twice has no single meaning, and
// guessing which was meant is how a store quietly loses an entry.
func (s *Store) Replace(scope Scope, old, new string) (string, error) {
	if !scope.Valid() {
		return "", fmt.Errorf("unknown memory scope %q", scope)
	}
	if old == "" {
		return "", errors.New("cannot replace empty text")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.read(scope)
	if err != nil {
		return "", err
	}
	switch strings.Count(current, old) {
	case 0:
		return "", ErrNotFound
	case 1:
	default:
		return "", ErrAmbiguous
	}

	next := strings.Replace(current, old, new, 1)
	return next, s.write(scope, next)
}

// Remove deletes an entry. The text has to appear exactly once, for the same
// reason as Replace. The line it sits on goes with it, so removing an entry
// does not leave an empty bullet behind.
func (s *Store) Remove(scope Scope, text string) (string, error) {
	if !scope.Valid() {
		return "", fmt.Errorf("unknown memory scope %q", scope)
	}
	if text == "" {
		return "", errors.New("cannot remove empty text")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.read(scope)
	if err != nil {
		return "", err
	}
	switch strings.Count(current, text) {
	case 0:
		return "", ErrNotFound
	case 1:
	default:
		return "", ErrAmbiguous
	}

	lines := strings.Split(current, "\n")
	kept := make([]string, 0, len(lines))
	dropped := false
	for _, line := range lines {
		if !dropped && strings.Contains(line, text) {
			dropped = true
			continue
		}
		kept = append(kept, line)
	}
	next := strings.Join(kept, "\n")
	next = strings.TrimLeft(next, "\n")
	if next != "" && !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	if strings.TrimSpace(next) == "" {
		next = ""
	}

	return next, s.write(scope, next)
}

// Set replaces a whole store. It is what a consolidation looks like: the
// agent rewrites the list shorter rather than deleting entries one at a
// time.
func (s *Store) Set(scope Scope, content string) (string, error) {
	if !scope.Valid() {
		return "", fmt.Errorf("unknown memory scope %q", scope)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	content = strings.TrimSpace(content)
	if content != "" {
		content += "\n"
	}
	return content, s.write(scope, content)
}

// write enforces the bound and then puts the file down atomically. The bound
// is checked here, once, so no caller can route around it.
func (s *Store) write(scope Scope, content string) error {
	limit := s.Limit(scope)
	if limit >= 0 {
		if size := utf8.RuneCountInString(content); size > limit {
			return &ErrTooLong{Scope: scope, Size: size, Limit: limit}
		}
	}

	path := s.Path(scope)
	if content == "" {
		// An empty store is no file. Leaving a zero-byte MEMORY.md behind
		// would read as "there is memory here" to anyone looking.
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to clear %s memory: %w", scope, err)
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create the %s memory directory: %w", scope, err)
	}
	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write %s memory: %w", scope, err)
	}
	return nil
}

// Used returns how much of a scope's bound is spent, for callers that want
// to show it.
func (s *Store) Used(scope Scope) (used, limit int, err error) {
	content, err := s.Read(scope)
	if err != nil {
		return 0, 0, err
	}
	return utf8.RuneCountInString(content), s.Limit(scope), nil
}

func lineExists(content, entry string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-")) == entry {
			return true
		}
	}
	return false
}
