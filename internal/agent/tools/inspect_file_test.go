package tools

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runInspectFile(t *testing.T, workingDir string, params InspectFileParams) string {
	t.Helper()
	tool := NewInspectFileTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)

	resp, err := tool.Run(context.Background(), fantasy.ToolCall{ID: "1", Name: InspectFileToolName, Input: string(input)})
	require.NoError(t, err)
	return resp.Content
}

func TestInspectFileArchiveListAndRead(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "sample.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	w, err := zw.Create("hello.txt")
	require.NoError(t, err)
	_, err = w.Write([]byte("hello world"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())

	list := runInspectFile(t, dir, InspectFileParams{Path: "sample.zip"})
	require.Contains(t, list, "hello.txt")

	read := runInspectFile(t, dir, InspectFileParams{Path: "sample.zip", Action: "read", Entry: "hello.txt"})
	require.Contains(t, read, "hello world")
}

func TestInspectFileArchiveReadRequiresEntry(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "sample.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())

	out := runInspectFile(t, dir, InspectFileParams{Path: "sample.zip", Action: "read"})
	require.Contains(t, out, "entry is required")
}

func TestInspectFileSQLiteSchemaAndQuery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sample.db")

	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	ctx := context.Background()
	_, err = conn.ExecContext(ctx, `CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)`)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, `INSERT INTO items (id, name) VALUES (1, 'widget'), (2, 'gadget')`)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	schema := runInspectFile(t, dir, InspectFileParams{Path: "sample.db"})
	require.Contains(t, schema, "items")
	require.Contains(t, schema, "name")

	query := runInspectFile(t, dir, InspectFileParams{Path: "sample.db", Action: "query", Query: "SELECT name FROM items ORDER BY id"})
	require.Contains(t, query, "widget")
	require.Contains(t, query, "gadget")

	readErr := runInspectFile(t, dir, InspectFileParams{Path: "sample.db", Action: "read"})
	require.Contains(t, readErr, "not meaningful")
}

func TestInspectFileNotebookListAndRead(t *testing.T) {
	dir := t.TempDir()
	nbPath := filepath.Join(dir, "sample.ipynb")
	content := `{
		"cells": [
			{"cell_type": "markdown", "source": ["# Title"]},
			{"cell_type": "code", "source": ["print('hi')"], "outputs": [{"output_type": "stream", "text": ["hi\n"]}]}
		]
	}`
	require.NoError(t, os.WriteFile(nbPath, []byte(content), 0o644))

	list := runInspectFile(t, dir, InspectFileParams{Path: "sample.ipynb"})
	require.Contains(t, list, "2 cell(s)")
	require.Contains(t, list, "markdown")

	read := runInspectFile(t, dir, InspectFileParams{Path: "sample.ipynb", Action: "read", Cell: 1})
	require.Contains(t, read, "print('hi')")
	require.Contains(t, read, "hi")
}

func TestInspectFileNotebookReadOutOfRange(t *testing.T) {
	dir := t.TempDir()
	nbPath := filepath.Join(dir, "sample.ipynb")
	require.NoError(t, os.WriteFile(nbPath, []byte(`{"cells": []}`), 0o644))

	out := runInspectFile(t, dir, InspectFileParams{Path: "sample.ipynb", Action: "read", Cell: 0})
	require.Contains(t, out, "out of range")
}

func TestInspectFileRejectsUnknownFormat(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "plain.txt")
	require.NoError(t, os.WriteFile(txtPath, []byte("just text"), 0o644))

	out := runInspectFile(t, dir, InspectFileParams{Path: "plain.txt"})
	require.Contains(t, out, "not a recognised")
}

func TestInspectFileRequiresPath(t *testing.T) {
	dir := t.TempDir()
	out := runInspectFile(t, dir, InspectFileParams{})
	require.Contains(t, out, "path is required")
}
