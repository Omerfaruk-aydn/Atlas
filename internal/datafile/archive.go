// Package datafile reads the handful of binary and structured formats
// that a plain text view can't meaningfully show: archives, SQLite
// databases, and Jupyter notebooks. Each format uses what's already a
// dependency of this module -- the standard library's archive/zip and
// archive/tar for archives, and internal/db's existing SQLite
// connection for databases -- rather than adding new ones.
package datafile

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"
)

// ArchiveEntry is one file or directory inside an archive.
type ArchiveEntry struct {
	Name  string
	Size  int64
	IsDir bool
}

// ArchiveKind reports which archive format a path names, or "" when it
// isn't a recognised one.
func ArchiveKind(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar"):
		return "tar"
	default:
		return ""
	}
}

// ListArchive returns every entry in a .zip, .tar, or .tar.gz/.tgz file.
func ListArchive(path string) ([]ArchiveEntry, error) {
	switch ArchiveKind(path) {
	case "zip":
		return listZip(path)
	case "tar", "tar.gz":
		return listTar(path)
	default:
		return nil, fmt.Errorf("%s is not a recognised archive format (.zip, .tar, .tar.gz, .tgz)", path)
	}
}

// maxArchiveEntryBytes bounds how much of one entry's content is read
// back -- an entry that happens to be a multi-gigabyte log file would
// otherwise be read into memory whole just to answer "what's in this
// archive."
const maxArchiveEntryBytes = 1 << 20 // 1 MiB

// ReadArchiveEntry returns one entry's content as text, truncated at
// maxArchiveEntryBytes. The truncated bool reports whether that
// happened.
func ReadArchiveEntry(path, entryName string) (content string, truncated bool, err error) {
	switch ArchiveKind(path) {
	case "zip":
		return readZipEntry(path, entryName)
	case "tar", "tar.gz":
		return readTarEntry(path, entryName)
	default:
		return "", false, fmt.Errorf("%s is not a recognised archive format (.zip, .tar, .tar.gz, .tgz)", path)
	}
}

func listZip(path string) ([]ArchiveEntry, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	entries := make([]ArchiveEntry, 0, len(r.File))
	for _, f := range r.File {
		entries = append(entries, ArchiveEntry{
			Name: f.Name, Size: int64(f.UncompressedSize64), IsDir: f.FileInfo().IsDir(),
		})
	}
	return entries, nil
}

func readZipEntry(path, entryName string) (string, bool, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", false, err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name != entryName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", false, err
		}
		defer rc.Close()
		return readCapped(rc, maxArchiveEntryBytes)
	}
	return "", false, fmt.Errorf("no entry named %q in %s", entryName, path)
}

func listTar(path string) ([]ArchiveEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tr, err := tarReaderFor(path, f)
	if err != nil {
		return nil, err
	}

	var entries []ArchiveEntry
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, ArchiveEntry{
			Name: hdr.Name, Size: hdr.Size, IsDir: hdr.Typeflag == tar.TypeDir,
		})
	}
	return entries, nil
}

func readTarEntry(path, entryName string) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()

	tr, err := tarReaderFor(path, f)
	if err != nil {
		return "", false, err
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false, err
		}
		if hdr.Name == entryName {
			return readCapped(tr, maxArchiveEntryBytes)
		}
	}
	return "", false, fmt.Errorf("no entry named %q in %s", entryName, path)
}

func tarReaderFor(path string, f *os.File) (*tar.Reader, error) {
	if ArchiveKind(path) == "tar.gz" {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		return tar.NewReader(gz), nil
	}
	return tar.NewReader(f), nil
}

func readCapped(r io.Reader, max int64) (string, bool, error) {
	limited := io.LimitReader(r, max+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", false, err
	}
	if int64(len(data)) > max {
		return string(data[:max]), true, nil
	}
	return string(data), false, nil
}
