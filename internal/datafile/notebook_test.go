package datafile

import (
	"os"
	"path/filepath"
	"testing"
)

func writeNotebook(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.ipynb")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadNotebookSourceAsArray(t *testing.T) {
	path := writeNotebook(t, `{
		"cells": [
			{
				"cell_type": "markdown",
				"source": ["# Title\n", "Some text."]
			},
			{
				"cell_type": "code",
				"source": ["print('hi')"],
				"outputs": [
					{"output_type": "stream", "name": "stdout", "text": ["hi\n"]}
				]
			}
		]
	}`)

	nb, err := ReadNotebook(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(nb.Cells) != 2 {
		t.Fatalf("expected 2 cells, got %d", len(nb.Cells))
	}
	if nb.Cells[0].Type != "markdown" || nb.Cells[0].Source != "# Title\nSome text." {
		t.Errorf("unexpected markdown cell: %+v", nb.Cells[0])
	}
	if nb.Cells[1].Source != "print('hi')" {
		t.Errorf("unexpected code source: %q", nb.Cells[1].Source)
	}
	if nb.Cells[1].OutputSummary != "hi\n" {
		t.Errorf("unexpected output summary: %q", nb.Cells[1].OutputSummary)
	}
}

func TestReadNotebookSourceAsString(t *testing.T) {
	path := writeNotebook(t, `{
		"cells": [
			{"cell_type": "raw", "source": "raw content here"}
		]
	}`)

	nb, err := ReadNotebook(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(nb.Cells) != 1 || nb.Cells[0].Source != "raw content here" {
		t.Fatalf("unexpected cells: %+v", nb.Cells)
	}
}

func TestReadNotebookErrorOutput(t *testing.T) {
	path := writeNotebook(t, `{
		"cells": [
			{
				"cell_type": "code",
				"source": "1/0",
				"outputs": [
					{"output_type": "error", "ename": "ZeroDivisionError", "evalue": "division by zero"}
				]
			}
		]
	}`)

	nb, err := ReadNotebook(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "error: ZeroDivisionError: division by zero"
	if nb.Cells[0].OutputSummary != want {
		t.Errorf("got %q, want %q", nb.Cells[0].OutputSummary, want)
	}
}

func TestReadNotebookDataOutput(t *testing.T) {
	path := writeNotebook(t, `{
		"cells": [
			{
				"cell_type": "code",
				"source": "df.head()",
				"outputs": [
					{"output_type": "execute_result", "data": {"text/plain": ["   a  b\n0  1  2"]}}
				]
			},
			{
				"cell_type": "code",
				"source": "plot()",
				"outputs": [
					{"output_type": "display_data", "data": {"image/png": "base64data"}}
				]
			}
		]
	}`)

	nb, err := ReadNotebook(path)
	if err != nil {
		t.Fatal(err)
	}
	if nb.Cells[0].OutputSummary != "   a  b\n0  1  2" {
		t.Errorf("unexpected text/plain summary: %q", nb.Cells[0].OutputSummary)
	}
	if nb.Cells[1].OutputSummary != "[display_data output, 1 format(s), not shown]" {
		t.Errorf("unexpected binary output summary: %q", nb.Cells[1].OutputSummary)
	}
}

func TestReadNotebookMissingFile(t *testing.T) {
	if _, err := ReadNotebook(filepath.Join(t.TempDir(), "missing.ipynb")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadNotebookInvalidJSON(t *testing.T) {
	path := writeNotebook(t, `not json at all`)
	if _, err := ReadNotebook(path); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
