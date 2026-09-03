// Package subagents implements named, model-routable agent definitions:
// a Markdown file with a "model" field a session can hand a task to, running
// on whatever model that field's role resolves to instead of the session's
// primary model. See internal/config's ModelRoles and ResolveRole.
package subagents

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// FileExt is the extension a subagent definition file must have.
	FileExt = ".md"

	MaxNameLength        = 64
	MaxDescriptionLength = 1024
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9]+(-[a-zA-Z0-9]+)*$`)

// Subagent represents a parsed subagent definition file.
type Subagent struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	// Model is a role reference ("research", "@research", "large") that
	// Config.ResolveRole resolves to a concrete provider/model pair. Empty
	// means the subagent runs on whatever model the parent session uses.
	Model        string `yaml:"model,omitempty" json:"model,omitempty"`
	Instructions string `yaml:"-" json:"instructions"`
	// Path is the file this subagent was parsed from. Empty for a
	// Subagent built in memory rather than discovered from disk.
	Path string `yaml:"-" json:"path"`
}

// Validate checks that the subagent has the fields required to be saved or
// used: a name that is safe as a filename and matches the file it came
// from (when it came from one), and a non-empty description.
func (s *Subagent) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("name is required"))
	} else {
		if len(s.Name) > MaxNameLength {
			errs = append(errs, fmt.Errorf("name exceeds %d characters", MaxNameLength))
		}
		if !namePattern.MatchString(s.Name) {
			errs = append(errs, errors.New("name must be alphanumeric with hyphens, no leading/trailing/consecutive hyphens"))
		}
		if s.Path != "" {
			base := strings.TrimSuffix(filepath.Base(s.Path), FileExt)
			if !strings.EqualFold(base, s.Name) {
				errs = append(errs, fmt.Errorf("name %q must match file name %q", s.Name, base))
			}
		}
	}

	if s.Description == "" {
		errs = append(errs, errors.New("description is required"))
	} else if len(s.Description) > MaxDescriptionLength {
		errs = append(errs, fmt.Errorf("description exceeds %d characters", MaxDescriptionLength))
	}

	return errors.Join(errs...)
}

// Parse parses a subagent definition file from disk.
func Parse(path string) (*Subagent, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	s, err := ParseContent(content)
	if err != nil {
		return nil, err
	}
	s.Path = path
	return s, nil
}

// ParseContent parses a subagent definition from raw bytes: YAML
// frontmatter (name, description, model) followed by a Markdown body that
// becomes Instructions.
func ParseContent(content []byte) (*Subagent, error) {
	frontmatter, body, err := splitFrontmatter(string(content))
	if err != nil {
		return nil, err
	}

	var s Subagent
	if err := yaml.Unmarshal([]byte(frontmatter), &s); err != nil {
		return nil, fmt.Errorf("parsing frontmatter: %w", err)
	}
	s.Instructions = strings.TrimSpace(body)
	return &s, nil
}

// splitFrontmatter extracts YAML frontmatter and body from markdown
// content. It mirrors internal/skills's function of the same name; kept
// separate rather than shared so this package has no dependency on skills.
func splitFrontmatter(content string) (frontmatter, body string, err error) {
	content = strings.TrimPrefix(content, string(rune(0xFEFF)))
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	lines := strings.Split(content, "\n")
	start := slices.IndexFunc(lines, func(line string) bool {
		return strings.TrimSpace(line) != ""
	})
	if start == -1 || strings.TrimSpace(lines[start]) != "---" {
		return "", "", errors.New("no YAML frontmatter found")
	}

	endOffset := slices.IndexFunc(lines[start+1:], func(line string) bool {
		return strings.TrimSpace(line) == "---"
	})
	if endOffset == -1 {
		return "", "", errors.New("unclosed frontmatter")
	}
	end := start + 1 + endOffset

	frontmatter = strings.Join(lines[start+1:end], "\n")
	body = strings.Join(lines[end+1:], "\n")
	return frontmatter, body, nil
}

// Discover finds every subagent definition (a *.md file directly inside one
// of dirs -- unlike skills, a subagent is one file, not a folder) across
// dirs, skipping files that fail to parse or validate. It is not recursive:
// a subdirectory of one of dirs is not scanned, keeping "what counts as a
// subagent" a flat, easy-to-audit convention.
func Discover(dirs []string) []*Subagent {
	var all []*Subagent
	seen := make(map[string]bool)

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), FileExt) {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if seen[path] {
				continue
			}
			seen[path] = true

			s, err := Parse(path)
			if err != nil {
				continue
			}
			if err := s.Validate(); err != nil {
				continue
			}
			all = append(all, s)
		}
	}

	slices.SortStableFunc(all, func(a, b *Subagent) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return Deduplicate(all)
}

// Deduplicate removes duplicate subagents by name. When duplicates exist,
// the last occurrence wins, so a later (e.g. project-level) directory in
// the discovery list overrides an earlier (e.g. user-level) one.
func Deduplicate(all []*Subagent) []*Subagent {
	seen := make(map[string]int, len(all))
	for i, s := range all {
		seen[s.Name] = i
	}

	result := make([]*Subagent, 0, len(seen))
	for i, s := range all {
		if seen[s.Name] == i {
			result = append(result, s)
		}
	}
	return result
}

// Find looks up a subagent by name, case-insensitively.
func Find(all []*Subagent, name string) (*Subagent, bool) {
	for _, s := range all {
		if strings.EqualFold(s.Name, name) {
			return s, true
		}
	}
	return nil, false
}
