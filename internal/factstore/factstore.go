// Package factstore is a small, queryable fact store the agent can write
// to and search *within the same session* -- the gap the existing
// internal/memory package deliberately leaves open. Memory is loaded
// once at session start and prepended to every request; a fact retained
// mid-session there would not be seen again until the next session. This
// store trades that stability for immediacy: a fact retained now can be
// recalled a moment later in the same conversation, at the cost of
// costing a tool call to look up rather than always being present.
//
// Facts are appended one JSON object per line to a single file. There is
// no locking against other processes -- this is meant for one agent
// session's own scratch facts, not a shared database.
package factstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Fact is one retained piece of information.
type Fact struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ScoredFact is one recall result.
type ScoredFact struct {
	Fact
	// Score is keyword-overlap weight, not a probability -- see Recall.
	// Zero when the query was empty and results are just recency-ordered.
	Score int
}

// ReflectResult summarises the store's contents: how much is in it, how
// it's tagged, which entries look like duplicates of each other, and how
// old the oldest and newest facts are.
type ReflectResult struct {
	Total      int
	ByTag      map[string]int
	Duplicates [][]Fact
	OldestAt   time.Time
	NewestAt   time.Time
}

// Store reads and appends to one facts file.
type Store struct {
	mu   sync.Mutex
	path string
}

// New returns a Store backed by path. The file and its parent directory
// are created on first write, not here.
func New(path string) *Store {
	return &Store{path: path}
}

// Path returns the file this store reads and writes.
func (s *Store) Path() string {
	return s.path
}

// ErrEmptyText is returned when Retain is given nothing to remember.
var ErrEmptyText = errors.New("text is required")

// Retain appends a new fact.
func (s *Store) Retain(text string, tags []string) (Fact, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Fact{}, ErrEmptyText
	}

	id, err := newFactID()
	if err != nil {
		return Fact{}, err
	}
	fact := Fact{
		ID:        id,
		Text:      text,
		Tags:      normalizeTags(tags),
		CreatedAt: time.Now().UTC(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.append(fact); err != nil {
		return Fact{}, err
	}
	return fact, nil
}

// Recall searches retained facts by keyword overlap against a query's
// words -- not an embedding, the same honest keyword search
// semantic_code_search uses for code. A tag match counts for more than
// a text match. An empty query returns the most recently retained facts
// instead of scoring nothing, since "what have I retained" is a
// legitimate question with no words to search for.
func (s *Store) Recall(query string, limit int) ([]ScoredFact, error) {
	if limit <= 0 {
		limit = 10
	}

	s.mu.Lock()
	facts, err := s.readAll()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}

	terms := tokenize(query)
	var results []ScoredFact
	if len(terms) == 0 {
		sort.Slice(facts, func(i, j int) bool { return facts[i].CreatedAt.After(facts[j].CreatedAt) })
		for _, f := range facts {
			results = append(results, ScoredFact{Fact: f})
		}
	} else {
		for _, f := range facts {
			if score := scoreFact(f, terms); score > 0 {
				results = append(results, ScoredFact{Fact: f, Score: score})
			}
		}
		sort.SliceStable(results, func(i, j int) bool {
			if results[i].Score != results[j].Score {
				return results[i].Score > results[j].Score
			}
			return results[i].CreatedAt.After(results[j].CreatedAt)
		})
	}

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// Reflect reports on the store as a whole: it groups and counts, it does
// not summarise. Producing prose from the accumulated facts would mean
// calling a model from inside a tool, which this deliberately does not
// do -- turning a consolidated view into an actual decision (what to
// keep, what belongs in permanent project memory) is left to the agent
// that called this tool.
func (s *Store) Reflect() (ReflectResult, error) {
	s.mu.Lock()
	facts, err := s.readAll()
	s.mu.Unlock()
	if err != nil {
		return ReflectResult{}, err
	}

	result := ReflectResult{Total: len(facts), ByTag: map[string]int{}}
	byText := map[string][]Fact{}
	for _, f := range facts {
		for _, tag := range f.Tags {
			result.ByTag[tag]++
		}
		key := strings.ToLower(strings.TrimSpace(f.Text))
		byText[key] = append(byText[key], f)

		if result.OldestAt.IsZero() || f.CreatedAt.Before(result.OldestAt) {
			result.OldestAt = f.CreatedAt
		}
		if f.CreatedAt.After(result.NewestAt) {
			result.NewestAt = f.CreatedAt
		}
	}

	var dupKeys []string
	for key, group := range byText {
		if len(group) > 1 {
			dupKeys = append(dupKeys, key)
		}
	}
	sort.Strings(dupKeys)
	for _, key := range dupKeys {
		group := byText[key]
		sort.Slice(group, func(i, j int) bool { return group[i].CreatedAt.Before(group[j].CreatedAt) })
		result.Duplicates = append(result.Duplicates, group)
	}

	return result, nil
}

func (s *Store) append(f Fact) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(data, '\n'))
	return err
}

// readAll reads every fact in the store. A store that has never been
// written to reads as empty, not an error -- the caller must hold s.mu.
func (s *Store) readAll() ([]Fact, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var facts []Fact
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var f Fact
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			continue // A corrupt line does not take the rest of the store down with it.
		}
		facts = append(facts, f)
	}
	return facts, nil
}

func newFactID() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func normalizeTags(tags []string) []string {
	var out []string
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func scoreFact(f Fact, terms []string) int {
	textLower := strings.ToLower(f.Text)
	tagSet := map[string]bool{}
	for _, t := range f.Tags {
		tagSet[t] = true
	}

	score := 0
	for _, term := range terms {
		if tagSet[term] {
			score += 3
		} else if strings.Contains(textLower, term) {
			score += 1
		}
	}
	return score
}

func tokenize(s string) []string {
	lower := strings.ToLower(s)
	var out []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			out = append(out, current.String())
			current.Reset()
		}
	}
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			current.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}
