package gitx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// blameRepo builds a file whose lines come from two different commits by
// two different authors, so attribution can actually be checked.
func blameRepo(t *testing.T) string {
	t.Helper()
	dir := gitRepo(t)

	writeFile(t, dir, "f.txt", "line one\nline two\n")
	commit(t, dir, "first commit")

	writeFile(t, dir, "f.txt", "line one\nline two\nline three\nline four\n")
	git(t, dir, "add", "-A")
	git(t, dir, "-c", "user.name=Second", "-c", "user.email=second@example.com",
		"commit", "-m", "second commit")

	return dir
}

func TestBlameAttributesEachLineToItsCommit(t *testing.T) {
	dir := blameRepo(t)

	got, err := Blame(context.Background(), dir, "f.txt", BlameOptions{})
	require.NoError(t, err)
	require.Len(t, got, 4)

	require.Equal(t, "Test", got[0].Author)
	require.Equal(t, "first commit", got[0].Summary)
	require.Equal(t, "Second", got[2].Author)
	require.Equal(t, "second commit", got[2].Summary)
}

func TestBlameRecordsLineNumbersAndContent(t *testing.T) {
	dir := blameRepo(t)

	got, err := Blame(context.Background(), dir, "f.txt", BlameOptions{})
	require.NoError(t, err)
	for i, l := range got {
		require.Equal(t, i+1, l.Line)
	}
	require.Equal(t, "line one", got[0].Content)
	require.Equal(t, "line four", got[3].Content)
}

func TestBlameRecordsCommitIdentity(t *testing.T) {
	dir := blameRepo(t)

	got, err := Blame(context.Background(), dir, "f.txt", BlameOptions{})
	require.NoError(t, err)
	require.Len(t, got[0].Hash, 40)
	require.Len(t, got[0].Short, 8)
	require.False(t, got[0].Date.IsZero())
	// The two commits must not be conflated.
	require.NotEqual(t, got[0].Hash, got[2].Hash)
}

func TestBlameHonoursALineRange(t *testing.T) {
	dir := blameRepo(t)

	got, err := Blame(context.Background(), dir, "f.txt", BlameOptions{StartLine: 3, EndLine: 4})
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, 3, got[0].Line)
	require.Equal(t, "Second", got[0].Author)
}

func TestBlameAcceptsASingleLine(t *testing.T) {
	dir := blameRepo(t)

	got, err := Blame(context.Background(), dir, "f.txt", BlameOptions{StartLine: 1})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, 1, got[0].Line)
}

func TestBlameCanReadAnOlderRevision(t *testing.T) {
	dir := blameRepo(t)

	got, err := Blame(context.Background(), dir, "f.txt", BlameOptions{Rev: "HEAD~1"})
	require.NoError(t, err)
	// The older revision had only two lines.
	require.Len(t, got, 2)
}

// A reformatting commit should not be able to claim authorship of every
// line it merely reindented.
func TestBlameCanIgnoreWhitespaceChanges(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "f.txt", "original line\n")
	commit(t, dir, "wrote the line")

	writeFile(t, dir, "f.txt", "    original line\n")
	git(t, dir, "add", "-A")
	git(t, dir, "-c", "user.name=Reformatter", "-c", "user.email=r@example.com",
		"commit", "-m", "reindent")

	withWhitespace, err := Blame(context.Background(), dir, "f.txt", BlameOptions{})
	require.NoError(t, err)
	require.Equal(t, "Reformatter", withWhitespace[0].Author)

	ignoring, err := Blame(context.Background(), dir, "f.txt", BlameOptions{IgnoreWhitespace: true})
	require.NoError(t, err)
	require.Equal(t, "Test", ignoring[0].Author)
}

// Blame is read as blocks, not as a per-line list.
func TestSpansCollapseConsecutiveLinesFromOneCommit(t *testing.T) {
	dir := blameRepo(t)

	lines, err := Blame(context.Background(), dir, "f.txt", BlameOptions{})
	require.NoError(t, err)

	spans := Spans(lines)
	require.Len(t, spans, 2)
	require.Equal(t, 1, spans[0].Start)
	require.Equal(t, 2, spans[0].End)
	require.Equal(t, 2, spans[0].Lines())
	require.Equal(t, 3, spans[1].Start)
	require.Equal(t, 4, spans[1].End)
}

// The same commit reappearing after a gap is a separate block, not an
// extension of the earlier one.
func TestSpansDoNotBridgeAGap(t *testing.T) {
	lines := []BlameLine{
		{Line: 1, Hash: "a"},
		{Line: 2, Hash: "b"},
		{Line: 3, Hash: "a"},
	}
	spans := Spans(lines)
	require.Len(t, spans, 3)
}

func TestAuthorsRanksByLineCount(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "f.txt", "a\nb\nc\n")
	commit(t, dir, "three lines")
	writeFile(t, dir, "f.txt", "a\nb\nc\nd\n")
	git(t, dir, "add", "-A")
	git(t, dir, "-c", "user.name=Second", "-c", "user.email=second@example.com",
		"commit", "-m", "one more")

	lines, err := Blame(context.Background(), dir, "f.txt", BlameOptions{})
	require.NoError(t, err)

	authors := Authors(lines)
	require.Len(t, authors, 2)
	require.Equal(t, "Test", authors[0].Author)
	require.Equal(t, 3, authors[0].Lines)
	require.Equal(t, "Second", authors[1].Author)
	require.Equal(t, 1, authors[1].Lines)
	require.False(t, authors[0].Latest.IsZero())
}

func TestBlameFailsOnAMissingFile(t *testing.T) {
	dir := blameRepo(t)

	_, err := Blame(context.Background(), dir, "no-such-file.txt", BlameOptions{})
	require.Error(t, err)
}

func TestBlameOnAnEmptyFileReturnsNothing(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "empty.txt", "")
	commit(t, dir, "add empty")

	got, err := Blame(context.Background(), dir, "empty.txt", BlameOptions{})
	require.NoError(t, err)
	require.Empty(t, got)
}

// A line whose content looks like a porcelain header must not be
// mistaken for one -- that is exactly how a stateful parser drifts.
func TestBlameSurvivesContentThatLooksLikeAHeader(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "f.txt",
		"0123456789abcdef0123456789abcdef01234567 1 1\nreal line\n")
	commit(t, dir, "tricky content")

	got, err := Blame(context.Background(), dir, "f.txt", BlameOptions{})
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "0123456789abcdef0123456789abcdef01234567 1 1", got[0].Content)
	require.Equal(t, "real line", got[1].Content)
	require.Equal(t, 2, got[1].Line)
}
