package datafile

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeZip(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	path := filepath.Join(dir, "sample.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTar(t *testing.T, dir, name string, gz bool, files map[string]string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var tw *tar.Writer
	var gzw *gzip.Writer
	if gz {
		gzw = gzip.NewWriter(f)
		tw = tar.NewWriter(gzw)
	} else {
		tw = tar.NewWriter(f)
	}
	for name, content := range files {
		hdr := &tar.Header{Name: name, Size: int64(len(content)), Mode: 0o644}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if gzw != nil {
		if err := gzw.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestArchiveKind(t *testing.T) {
	cases := map[string]string{
		"a.zip":     "zip",
		"a.tar":     "tar",
		"a.tar.gz":  "tar.gz",
		"a.tgz":     "tar.gz",
		"a.TAR.GZ":  "tar.gz",
		"a.txt":     "",
		"a.tar.bz2": "",
	}
	for path, want := range cases {
		if got := ArchiveKind(path); got != want {
			t.Errorf("ArchiveKind(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestListAndReadZip(t *testing.T) {
	dir := t.TempDir()
	path := writeZip(t, dir, map[string]string{
		"hello.txt": "hello world",
		"nested/a":  "nested content",
	})

	entries, err := ListArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	content, truncated, err := ReadArchiveEntry(path, "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Fatal("expected not truncated")
	}
	if content != "hello world" {
		t.Fatalf("got %q", content)
	}

	if _, _, err := ReadArchiveEntry(path, "missing.txt"); err == nil {
		t.Fatal("expected error for missing entry")
	}
}

func TestListAndReadTar(t *testing.T) {
	dir := t.TempDir()
	path := writeTar(t, dir, "sample.tar", false, map[string]string{
		"file.txt": "tar content",
	})

	entries, err := ListArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "file.txt" {
		t.Fatalf("unexpected entries: %+v", entries)
	}

	content, _, err := ReadArchiveEntry(path, "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "tar content" {
		t.Fatalf("got %q", content)
	}
}

func TestListAndReadTarGz(t *testing.T) {
	dir := t.TempDir()
	path := writeTar(t, dir, "sample.tar.gz", true, map[string]string{
		"file.txt": "gz content",
	})

	entries, err := ListArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("unexpected entries: %+v", entries)
	}

	content, _, err := ReadArchiveEntry(path, "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "gz content" {
		t.Fatalf("got %q", content)
	}
}

func TestReadArchiveEntryTruncates(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("x", int(maxArchiveEntryBytes)+100)
	path := writeZip(t, dir, map[string]string{"big.txt": big})

	content, truncated, err := ReadArchiveEntry(path, "big.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("expected truncated")
	}
	if int64(len(content)) != maxArchiveEntryBytes {
		t.Fatalf("expected %d bytes, got %d", maxArchiveEntryBytes, len(content))
	}
}

func TestListArchiveUnrecognisedFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.txt")
	if err := os.WriteFile(path, []byte("not an archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ListArchive(path); err == nil {
		t.Fatal("expected error for unrecognised format")
	}
	if _, _, err := ReadArchiveEntry(path, "x"); err == nil {
		t.Fatal("expected error for unrecognised format")
	}
}
