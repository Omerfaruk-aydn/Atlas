package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrBuiltin is returned when something tries to write over a skill that
// ships with the binary.
var ErrBuiltin = errors.New("that skill is built in and cannot be changed; give a new skill a different name and it will take precedence")

// frontmatter is the written form of a skill's header. It is a struct
// rather than a map so the field order is the file's order: yaml sorts a
// map's keys, which would put description above name and make a generated
// file look unlike a hand-written one. Everything optional is omitempty,
// because a skill file is read and edited by hand and a wall of empty keys
// is noise.
type frontmatter struct {
	Name                   string            `yaml:"name"`
	Description            string            `yaml:"description"`
	UserInvocable          bool              `yaml:"user-invocable,omitempty"`
	DisableModelInvocation bool              `yaml:"disable-model-invocation,omitempty"`
	License                string            `yaml:"license,omitempty"`
	Compatibility          string            `yaml:"compatibility,omitempty"`
	Metadata               map[string]string `yaml:"metadata,omitempty"`
}

// Render turns a skill back into the SKILL.md it came from.
func Render(s *Skill) ([]byte, error) {
	var b strings.Builder
	b.WriteString("---\n")

	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(frontmatter{
		Name:                   s.Name,
		Description:            s.Description,
		UserInvocable:          s.UserInvocable,
		DisableModelInvocation: s.DisableModelInvocation,
		License:                s.License,
		Compatibility:          s.Compatibility,
		Metadata:               s.Metadata,
	}); err != nil {
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

// SkillPath is where a skill of the given name lives under dir.
func SkillPath(dir, name string) string {
	return filepath.Join(dir, name, SkillFileName)
}

// Save validates a skill and writes it under dir as <dir>/<name>/SKILL.md.
//
// Validation happens before anything touches the disk, because a skill file
// that does not parse is not merely useless: it shows up in the discovery
// diagnostics of every later session until someone deletes it by hand.
func Save(dir string, s *Skill) (string, error) {
	path := SkillPath(dir, s.Name)

	// Validate against where the file will actually be, since the name has
	// to match its directory.
	check := *s
	check.Path = filepath.Dir(path)
	if err := check.Validate(); err != nil {
		return "", err
	}

	content, err := Render(s)
	if err != nil {
		return "", err
	}
	if _, err := ParseContent(content); err != nil {
		return "", fmt.Errorf("the rendered skill does not parse back: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("creating the skill directory: %w", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", fmt.Errorf("writing the skill: %w", err)
	}
	return path, nil
}

// Delete removes a skill directory, and only a skill directory: it refuses
// anything that does not hold a SKILL.md, so a wrong name cannot take a
// source tree with it.
func Delete(dir, name string) error {
	skillDir := filepath.Join(dir, name)
	if _, err := os.Stat(filepath.Join(skillDir, SkillFileName)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no skill named %q in %s", name, dir)
		}
		return err
	}
	return os.RemoveAll(skillDir)
}

// Find returns the skill with the given name, and whether one was found.
func Find(all []*Skill, name string) (*Skill, bool) {
	for _, s := range all {
		if strings.EqualFold(s.Name, name) {
			return s, true
		}
	}
	return nil, false
}
