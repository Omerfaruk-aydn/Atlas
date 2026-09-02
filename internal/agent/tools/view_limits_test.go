package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/stretchr/testify/require"
)

// The zero value has to be usable: it means "the built-in defaults", not
// "return nothing".
func TestZeroViewLimitsMeanTheDefaults(t *testing.T) {
	var l ViewLimits
	require.Equal(t, DefaultReadLimit, l.readLimit())
	require.Equal(t, MaxLineLength, l.lineLength())
}

func TestConfiguredViewLimits(t *testing.T) {
	l := ViewLimits{DefaultReadLimit: 42, MaxLineLength: 10}
	require.Equal(t, 42, l.readLimit())
	require.Equal(t, 10, l.lineLength())
}

func TestNewViewLimitsReadsTheToolsSection(t *testing.T) {
	require.Equal(t, ViewLimits{}, NewViewLimits(nil))
	require.Equal(t, ViewLimits{}, NewViewLimits(&config.Config{}))

	readLimit, lineLength := 500, 80
	got := NewViewLimits(&config.Config{Tools: config.Tools{View: config.ToolView{
		DefaultReadLimit: &readLimit,
		MaxLineLength:    &lineLength,
	}}})
	require.Equal(t, ViewLimits{DefaultReadLimit: 500, MaxLineLength: 80}, got)
}

func TestReadTextFileHonoursAConfiguredLineLength(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "long.txt")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", 200)+"\n"), 0o644))

	content, _, err := readTextFile(path, 0, 1, 0, 20)
	require.NoError(t, err)
	require.Contains(t, content, "...")
	require.Less(t, len(content), 200)

	// Zero still means the built-in length, which leaves this line alone.
	content, _, err = readTextFile(path, 0, 1, 0, 0)
	require.NoError(t, err)
	require.NotContains(t, content, "...")
}

// The description tells the model how many lines it gets by default, so a
// configured limit has to reach it.
func TestViewDescriptionReportsTheConfiguredReadLimit(t *testing.T) {
	require.Contains(t, viewDescription(ViewLimits{DefaultReadLimit: 777}), "777")
	require.Contains(t, viewDescription(ViewLimits{}), "200")
}
