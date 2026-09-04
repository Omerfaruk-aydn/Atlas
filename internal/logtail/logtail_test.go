package logtail

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeLogFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestTailReturnsLastNLines(t *testing.T) {
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	path := writeLogFile(t, strings.Join(lines, "\n")+"\n")

	got, err := Tail(path, Options{Lines: 3})
	require.NoError(t, err)
	require.Equal(t, []string{"line 8", "line 9", "line 10"}, got.Lines)
	require.Equal(t, 10, got.TotalLines)
}

func TestTailDefaultsTo100Lines(t *testing.T) {
	var lines []string
	for i := 1; i <= 150; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	path := writeLogFile(t, strings.Join(lines, "\n")+"\n")

	got, err := Tail(path, Options{})
	require.NoError(t, err)
	require.Len(t, got.Lines, 100)
	require.Equal(t, "line 150", got.Lines[len(got.Lines)-1])
}

func TestTailFiltersByGrep(t *testing.T) {
	content := "starting up\nconnected to db\nrequest failed: timeout\nrequest ok\n"
	path := writeLogFile(t, content)

	got, err := Tail(path, Options{Grep: "request"})
	require.NoError(t, err)
	require.Equal(t, []string{"request failed: timeout", "request ok"}, got.Lines)
}

func TestTailGrepIsCaseInsensitive(t *testing.T) {
	content := "Request Failed\nall good\n"
	path := writeLogFile(t, content)

	got, err := Tail(path, Options{Grep: "request failed"})
	require.NoError(t, err)
	require.Equal(t, []string{"Request Failed"}, got.Lines)
}

func TestTailFiltersByLevel(t *testing.T) {
	content := "INFO starting up\nERROR connection refused\nDEBUG retrying\nERROR timeout\n"
	path := writeLogFile(t, content)

	got, err := Tail(path, Options{Level: "error"})
	require.NoError(t, err)
	require.Equal(t, []string{"ERROR connection refused", "ERROR timeout"}, got.Lines)
}

func TestTailLevelMatchesStructuredFormats(t *testing.T) {
	content := "level=info msg=starting\nlevel=error msg=boom\n\"level\":\"warn\"\n"
	path := writeLogFile(t, content)

	got, err := Tail(path, Options{Level: "error"})
	require.NoError(t, err)
	require.Equal(t, []string{"level=error msg=boom"}, got.Lines)
}

func TestTailCombinesGrepAndLevel(t *testing.T) {
	content := "ERROR db connection lost\nERROR cache miss\nINFO db connected\n"
	path := writeLogFile(t, content)

	got, err := Tail(path, Options{Level: "error", Grep: "db"})
	require.NoError(t, err)
	require.Equal(t, []string{"ERROR db connection lost"}, got.Lines)
}

func TestTailReportsTruncated(t *testing.T) {
	var lines []string
	for i := 1; i <= 5; i++ {
		lines = append(lines, "ERROR boom")
		_ = i
	}
	path := writeLogFile(t, strings.Join(lines, "\n")+"\n")

	got, err := Tail(path, Options{Level: "error", Lines: 2})
	require.NoError(t, err)
	require.Len(t, got.Lines, 2)
	require.True(t, got.Truncated)
}

func TestTailReportsNotTruncatedWhenEverythingFits(t *testing.T) {
	path := writeLogFile(t, "line1\nline2\n")

	got, err := Tail(path, Options{Lines: 10})
	require.NoError(t, err)
	require.False(t, got.Truncated)
}

func TestTailReportsErrorForMissingFile(t *testing.T) {
	_, err := Tail(filepath.Join(t.TempDir(), "nope.log"), Options{})
	require.Error(t, err)
}

func TestTailHandlesEmptyFile(t *testing.T) {
	path := writeLogFile(t, "")

	got, err := Tail(path, Options{})
	require.NoError(t, err)
	require.Empty(t, got.Lines)
	require.Equal(t, 0, got.TotalLines)
}
