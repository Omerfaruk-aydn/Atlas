package subagents

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// frontmatter is the written form of a subagent's header. It is a struct
// rather than a map so the field order in the written file is stable
// (name, description, model) rather than YAML's alphabetical map order.
type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Model       string `yaml:"model,omitempty"`
}

// Render turns a subagent back into the Markdown file it came from.
func Render(s *Subagent) ([]byte, error) {
	var b strings.Builder
	b.WriteString("---\n")

	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(frontmatter{Name: s.Name, Description: s.Description, Model: s.Model}); err != nil {
		return nil, fmt.Errorf("rendering frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("rendering frontmatter: %w", err)
	}

	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(s.Instructions))
	b.WriteString("\n")
	return []byte(b.String()), nil
}

// Path is where a subagent of the given name lives under dir.
func Path(dir, name string) string {
	return filepath.Join(dir, name+FileExt)
}

// Save validates a subagent and writes it under dir as <dir>/<name>.md.
// Validation happens before anything touches disk, mirroring
// internal/skills.Save, for the same reason: a file that does not parse
// back is worse than useless, since it lingers in discovery diagnostics
// until someone finds and deletes it by hand.
func Save(dir string, s *Subagent) (string, error) {
	path := Path(dir, s.Name)

	check := *s
	check.Path = path
	if err := check.Validate(); err != nil {
		return "", err
	}

	content, err := Render(s)
	if err != nil {
		return "", err
	}
	if _, err := ParseContent(content); err != nil {
		return "", fmt.Errorf("the rendered subagent does not parse back: %w", err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return path, nil
}

// Delete removes a subagent's definition file. It refuses a path that is
// not a file this package would have written (i.e. does not end in
// FileExt), so a caller cannot be tricked into deleting an unrelated file
// by way of a crafted name.
func Delete(dir, name string) error {
	path := Path(dir, name)
	if !strings.HasSuffix(path, FileExt) {
		return errors.New("refusing to delete a path that is not a subagent definition file")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("no subagent file at %s: %w", path, err)
	}
	return os.Remove(path)
}
