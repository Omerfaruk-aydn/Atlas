package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/util"
	"github.com/stretchr/testify/require"
)

// A notification is one line in a fixed-width chrome. Clipboard text is
// arbitrary, so the excerpt has to survive a copied paragraph without
// becoming one.
func TestPasteExcerpt(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", pasteExcerptWidth*3)

	for _, tc := range []struct {
		name, in, want string
	}{
		{"short path is kept whole", `D:\Atlas\shot.png`, `D:\Atlas\shot.png`},
		{"empty stays empty", "", ""},
		{"stops at the first newline", "first line\nsecond line", "first line"},
		{"stops at a carriage return", "first line\r\nsecond", "first line"},
		{"long text is cut", long, strings.Repeat("x", pasteExcerptWidth) + "…"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, pasteExcerpt(tc.in))
		})
	}
}

// Multi-byte text must be cut by rune, not by byte, or the excerpt ends in a
// broken code point — and Turkish paths hit this on the first character.
func TestPasteExcerptCutsRunesNotBytes(t *testing.T) {
	t.Parallel()

	got := pasteExcerpt(strings.Repeat("ö", pasteExcerptWidth+10))
	require.Equal(t, strings.Repeat("ö", pasteExcerptWidth)+"…", got)
	require.True(t, len(got) > pasteExcerptWidth, "a byte-wise cut would have been shorter")
}

// Both clipboard routes end in attachImageFile, so its refusals are what the
// user actually reads when a paste does not produce an attachment. Each one
// has to name the real reason — the bug this replaced returned nothing at all.
func TestAttachImageFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	png := filepath.Join(dir, "shot.png")
	require.NoError(t, os.WriteFile(png, []byte("\x89PNG\r\n\x1a\n"), 0o600))

	notImage := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(notImage, []byte("hello"), 0o600))

	tooBig := filepath.Join(dir, "huge.png")
	require.NoError(t, os.WriteFile(tooBig, make([]byte, common.MaxAttachmentSize+1), 0o600))

	t.Run("attaches a supported image", func(t *testing.T) {
		t.Parallel()

		msg := attachImageFile(png)
		att, ok := msg.(message.Attachment)
		require.True(t, ok, "expected an attachment, got %T: %v", msg, msg)
		require.Equal(t, "shot.png", att.FileName)
		require.Equal(t, png, att.FilePath)
		require.NotEmpty(t, att.Content)
	})

	t.Run("uppercase extensions are still images", func(t *testing.T) {
		t.Parallel()

		upper := filepath.Join(dir, "SHOT.PNG")
		require.NoError(t, os.WriteFile(upper, []byte("\x89PNG\r\n\x1a\n"), 0o600))

		_, ok := attachImageFile(upper).(message.Attachment)
		require.True(t, ok, "Explorer preserves the case the file was named with")
	})

	for _, tc := range []struct {
		name, path, wantSubstring string
	}{
		{"unsupported type", notImage, "not a supported image format"},
		{"missing file", filepath.Join(dir, "gone.png"), "Unable to read file"},
		{"a folder", dir, "not a supported image format"},
		{"oversized file", tooBig, "max 5MB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := attachImageFile(tc.path)
			require.NotNil(t, msg, "a refusal must never be silent")
			info, ok := msg.(util.InfoMsg)
			require.True(t, ok, "expected an InfoMsg, got %T", msg)
			require.Contains(t, info.Msg, tc.wantSubstring)
		})
	}
}

// A folder named like an image gets past the extension check, so the directory
// case needs its own guard further down.
func TestAttachImageFileRejectsImageNamedFolder(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "album.png")
	require.NoError(t, os.Mkdir(dir, 0o750))

	info, ok := attachImageFile(dir).(util.InfoMsg)
	require.True(t, ok, "a folder must not become an attachment")
	require.Contains(t, info.Msg, "is a folder")
}
