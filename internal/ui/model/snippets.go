package model

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ui/dialog"
)

// snippetsPath returns the path to the user's saved-prompt-snippets file,
// stored alongside the global data config rather than inside it: snippets
// are personal scratch text, not settings, and keeping them in a separate
// file means they don't get pulled into config validation/schema.
func snippetsPath() string {
	return filepath.Join(filepath.Dir(config.GlobalConfigData()), "snippets.json")
}

// loadSnippets reads the saved snippets, returning an empty slice (not an
// error) if the file doesn't exist yet.
func loadSnippets() ([]dialog.Snippet, error) {
	data, err := os.ReadFile(snippetsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var snippets []dialog.Snippet
	if err := json.Unmarshal(data, &snippets); err != nil {
		return nil, err
	}
	return snippets, nil
}

func saveSnippetsFile(snippets []dialog.Snippet) error {
	data, err := json.MarshalIndent(snippets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(snippetsPath(), data, 0o644)
}
