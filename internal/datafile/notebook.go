package datafile

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// NotebookCell is one cell of a Jupyter notebook, reduced to what's
// worth showing in a terminal: its type, its source, and a short
// account of its outputs.
type NotebookCell struct {
	Type   string // "code", "markdown", or "raw".
	Source string
	// OutputSummary describes what the cell produced, when it's a code
	// cell with output -- text and error streams shown, images and other
	// binary output data named but not inlined.
	OutputSummary string
}

// Notebook is a .ipynb file's cells.
type Notebook struct {
	Cells []NotebookCell
}

// notebookJSON mirrors just the fields ReadNotebook uses out of the
// nbformat schema -- a notebook carries a great deal of metadata
// (kernel info, language version, widget state) that has no bearing on
// reading what the notebook says and did.
type notebookJSON struct {
	Cells []struct {
		CellType string          `json:"cell_type"`
		Source   json.RawMessage `json:"source"`
		Outputs  []struct {
			OutputType string          `json:"output_type"`
			Text       json.RawMessage `json:"text"`
			Data       map[string]any  `json:"data"`
			Name       string          `json:"name"`
			Ename      string          `json:"ename"`
			Evalue     string          `json:"evalue"`
		} `json:"outputs"`
	} `json:"cells"`
}

// ReadNotebook parses a .ipynb file's cells.
func ReadNotebook(path string) (Notebook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Notebook{}, err
	}

	var raw notebookJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return Notebook{}, fmt.Errorf("parsing notebook: %w", err)
	}

	nb := Notebook{Cells: make([]NotebookCell, 0, len(raw.Cells))}
	for _, c := range raw.Cells {
		cell := NotebookCell{
			Type:   c.CellType,
			Source: joinSource(c.Source),
		}

		var summaries []string
		for _, o := range c.Outputs {
			switch {
			case o.Ename != "":
				summaries = append(summaries, fmt.Sprintf("error: %s: %s", o.Ename, o.Evalue))
			case len(o.Text) > 0:
				summaries = append(summaries, joinSource(o.Text))
			case o.Data != nil:
				if text, ok := o.Data["text/plain"]; ok {
					summaries = append(summaries, joinSource(marshalBack(text)))
				} else {
					summaries = append(summaries, fmt.Sprintf("[%s output, %d format(s), not shown]", o.OutputType, len(o.Data)))
				}
			}
		}
		cell.OutputSummary = strings.Join(summaries, "\n")

		nb.Cells = append(nb.Cells, cell)
	}
	return nb, nil
}

// joinSource handles nbformat's two legal shapes for a text field: a
// single string, or an array of strings meant to be concatenated (each
// usually already ending in its own newline).
func joinSource(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}

	var asLines []string
	if err := json.Unmarshal(raw, &asLines); err == nil {
		return strings.Join(asLines, "")
	}
	return ""
}

func marshalBack(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}
