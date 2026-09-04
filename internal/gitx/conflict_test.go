package gitx

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseConflictsFindsASimpleBlock(t *testing.T) {
	content := "line1\n<<<<<<< HEAD\nours line\n=======\ntheirs line\n>>>>>>> feature\nline2\n"

	blocks := ParseConflicts(content)
	require.Len(t, blocks, 1)
	b := blocks[0]
	require.Equal(t, 2, b.StartLine)
	require.Equal(t, 6, b.EndLine)
	require.Equal(t, "HEAD", b.OursLabel)
	require.Equal(t, "feature", b.TheirsLabel)
	require.Equal(t, []string{"ours line"}, b.OursLines)
	require.Equal(t, []string{"theirs line"}, b.TheirsLines)
}

func TestParseConflictsHandlesDiff3Style(t *testing.T) {
	content := "<<<<<<< HEAD\nours\n||||||| merged common ancestors\nbase\n=======\ntheirs\n>>>>>>> feature\n"

	blocks := ParseConflicts(content)
	require.Len(t, blocks, 1)
	require.Equal(t, []string{"base"}, blocks[0].BaseLines)
	require.Contains(t, blocks[0].BaseLabel, "merged common ancestors")
}

func TestParseConflictsFindsMultipleBlocks(t *testing.T) {
	content := "<<<<<<< HEAD\na\n=======\nb\n>>>>>>> f1\nmiddle\n<<<<<<< HEAD\nc\n=======\nd\n>>>>>>> f2\n"

	blocks := ParseConflicts(content)
	require.Len(t, blocks, 2)
	require.Equal(t, "f1", blocks[0].TheirsLabel)
	require.Equal(t, "f2", blocks[1].TheirsLabel)
}

func TestParseConflictsReturnsEmptyForCleanFile(t *testing.T) {
	blocks := ParseConflicts("just some\nordinary text\n")
	require.Empty(t, blocks)
}

func TestParseConflictsIgnoresAnUnterminatedBlock(t *testing.T) {
	content := "<<<<<<< HEAD\nours\n=======\ntheirs\n"

	blocks := ParseConflicts(content)
	require.Empty(t, blocks)
}

func TestParseConflictsHandlesMultilineSides(t *testing.T) {
	content := "<<<<<<< HEAD\nl1\nl2\nl3\n=======\nr1\nr2\n>>>>>>> feature\n"

	blocks := ParseConflicts(content)
	require.Len(t, blocks, 1)
	require.Len(t, blocks[0].OursLines, 3)
	require.Len(t, blocks[0].TheirsLines, 2)
}
