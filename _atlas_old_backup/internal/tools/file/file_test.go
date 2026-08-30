package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("merhaba dünya"), 0o644); err != nil {
		t.Fatal(err)
	}

	input, _ := json.Marshal(map[string]string{"path": path})
	res, err := ReadTool{}.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}
	if res.Content != "merhaba dünya" {
		t.Errorf("expected file content, got %q", res.Content)
	}
}

func TestReadToolMissingFile(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"path": filepath.Join(t.TempDir(), "yok.txt")})
	res, err := ReadTool{}.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError for missing file")
	}
}

func TestWriteTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "out.txt")

	input, _ := json.Marshal(map[string]string{"path": path, "content": "test içerik"})
	res, err := WriteTool{}.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("write_file did not create the file: %v", err)
	}
	if string(data) != "test içerik" {
		t.Errorf("expected written content, got %q", string(data))
	}
}

func TestEditToolSingleMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("foo bar baz"), 0o644)

	input, _ := json.Marshal(map[string]string{"path": path, "old_string": "bar", "new_string": "QUX"})
	res, err := EditTool{}.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "foo QUX baz" {
		t.Errorf("expected replaced content, got %q", string(data))
	}
}

func TestEditToolAmbiguousMatchRequiresReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("a a a"), 0o644)

	input, _ := json.Marshal(map[string]string{"path": path, "old_string": "a", "new_string": "b"})
	res, err := EditTool{}.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected an error for an ambiguous (multi-match) old_string without replace_all")
	}

	data, _ := os.ReadFile(path)
	if string(data) != "a a a" {
		t.Errorf("file should be unchanged after a rejected ambiguous edit, got %q", string(data))
	}
}

func TestEditToolReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("a a a"), 0o644)

	input, _ := json.Marshal(map[string]any{"path": path, "old_string": "a", "new_string": "b", "replace_all": true})
	res, err := EditTool{}.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "b b b" {
		t.Errorf("expected all occurrences replaced, got %q", string(data))
	}
}

func TestWriteToolPreviewExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("eski içerik"), 0o644)

	input, _ := json.Marshal(map[string]string{"path": path, "content": "yeni içerik"})
	preview, err := WriteTool{}.Preview(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.Old != "eski içerik" || preview.New != "yeni içerik" || preview.Path != path {
		t.Errorf("unexpected preview: %+v", preview)
	}

	// Preview must not have modified the file.
	data, _ := os.ReadFile(path)
	if string(data) != "eski içerik" {
		t.Errorf("Preview must not write to disk, file now contains %q", string(data))
	}
}

func TestWriteToolPreviewNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "yeni.txt")
	input, _ := json.Marshal(map[string]string{"path": path, "content": "merhaba"})
	preview, err := WriteTool{}.Preview(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.Old != "" {
		t.Errorf("expected empty Old for a new file, got %q", preview.Old)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("Preview must not create the file")
	}
}

func TestEditToolPreviewMatchesExecute(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("foo bar baz"), 0o644)

	input, _ := json.Marshal(map[string]string{"path": path, "old_string": "bar", "new_string": "QUX"})

	preview, err := EditTool{}.Preview(input)
	if err != nil {
		t.Fatalf("unexpected preview error: %v", err)
	}
	if preview.Old != "foo bar baz" || preview.New != "foo QUX baz" {
		t.Errorf("unexpected preview: %+v", preview)
	}

	// Preview must not have modified the file.
	data, _ := os.ReadFile(path)
	if string(data) != "foo bar baz" {
		t.Errorf("Preview must not write to disk, file now contains %q", string(data))
	}

	if _, err := (EditTool{}).Execute(context.Background(), input); err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != preview.New {
		t.Errorf("Execute's result %q should match Preview's New %q", string(data), preview.New)
	}
}

func TestEditToolPreviewAmbiguousMatchErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("a a a"), 0o644)

	input, _ := json.Marshal(map[string]string{"path": path, "old_string": "a", "new_string": "b"})
	if _, err := (EditTool{}).Preview(input); err == nil {
		t.Error("expected Preview to error on an ambiguous match, same as Execute")
	}
}

func TestEditToolNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("hello"), 0o644)

	input, _ := json.Marshal(map[string]string{"path": path, "old_string": "not-there", "new_string": "x"})
	res, err := EditTool{}.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected an error when old_string is not found")
	}
}
