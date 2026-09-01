package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The whole point of the fallback is that an installation made before the
// rename keeps working without anything being moved. These cover the three
// states a path can be in.
func TestPreferExisting(t *testing.T) {
	dir := t.TempDir()

	t.Run("uses the current name when it exists", func(t *testing.T) {
		current := filepath.Join(dir, "a", appName+".json")
		require.NoError(t, os.MkdirAll(filepath.Dir(current), 0o750))
		require.NoError(t, os.WriteFile(current, []byte("{}"), 0o600))
		require.NoError(t, os.WriteFile(legacyName(current), []byte("{}"), 0o600))

		require.Equal(t, current, preferExisting(current),
			"with both present the current name must win")
	})

	t.Run("falls back when only the legacy name exists", func(t *testing.T) {
		current := filepath.Join(dir, "b", appName+".json")
		require.NoError(t, os.MkdirAll(filepath.Dir(current), 0o750))
		require.NoError(t, os.WriteFile(legacyName(current), []byte("{}"), 0o600))

		require.Equal(t, legacyName(current), preferExisting(current),
			"an installation predating the rename must still be found")
	})

	t.Run("returns the current name when neither exists", func(t *testing.T) {
		current := filepath.Join(dir, "c", appName+".json")
		require.Equal(t, current, preferExisting(current),
			"anything created from scratch must land on the current name")
	})
}

// legacyName rewrites only the program's own name, and only once, so a path
// that happens to repeat it is not mangled.
func TestLegacyName(t *testing.T) {
	t.Parallel()

	require.Equal(t, "crush.json", legacyName("atlas.json"))
	require.Equal(t, ".crushrc", legacyName(".atlasrc"))
	require.Equal(t, ".crush", legacyName(".atlas"))
	require.Equal(t, "AGENTS.md", legacyName("AGENTS.md"), "unrelated names pass through")

	// A path names the program twice — the config directory and the file
	// inside it — so both elements have to be rewritten. Rewriting only the
	// first produced ~/.config/crush/atlas.json, a path that exists nowhere.
	require.Equal(t, filepath.Join("home", ".config", "crush", "crush.json"),
		legacyName(filepath.Join("home", ".config", "atlas", "atlas.json")))

	// Only elements that *are* one of this program's names are rewritten. A
	// directory that merely begins with the word belongs to the user.
	for _, unchanged := range []string{
		filepath.Join("srv", "atlas-projects", "notes.md"),
		filepath.Join("home", "atlassian", "config"),
		"atlasctl",
	} {
		require.Equal(t, unchanged, legacyName(unchanged),
			"%q is not one of this program's names", unchanged)
	}

	// The suffixes the program actually produces.
	require.Equal(t, "crush.db", legacyName("atlas.db"))
	require.Equal(t, "crush.log", legacyName("atlas.log"))
	require.Equal(t, filepath.Join(".crush", "logs", "crush.log"),
		legacyName(filepath.Join(".atlas", "logs", "atlas.log")))
}

func TestWithLegacyNames(t *testing.T) {
	t.Parallel()

	got := withLegacyNames("."+appName+"rc", appName+"rc")
	require.Equal(t, []string{".atlasrc", "atlasrc", ".crushrc", "crushrc"}, got,
		"current spellings must be searched before legacy ones")

	require.Equal(t, []string{"AGENTS.md"}, withLegacyNames("AGENTS.md"),
		"a name with nothing to rewrite must not be duplicated")
}

// Environment variables were prefixed CRUSH_ before the rename, and shell
// profiles that set them must keep working.
func TestLookupEnvPrefersCurrentPrefix(t *testing.T) {
	t.Setenv(envPrefix+"GLOBAL_CONFIG", "new")
	t.Setenv(legacyEnvPrefix+"GLOBAL_CONFIG", "old")
	v, ok := lookupEnv("GLOBAL_CONFIG")
	require.True(t, ok)
	require.Equal(t, "new", v)
}

func TestLookupEnvFallsBackToLegacyPrefix(t *testing.T) {
	os.Unsetenv(envPrefix + "CACHE_DIR")
	t.Setenv(legacyEnvPrefix+"CACHE_DIR", "old")
	v, ok := lookupEnv("CACHE_DIR")
	require.True(t, ok)
	require.Equal(t, "old", v, "a profile setting only the old prefix must still be honoured")
}

func TestLookupEnvUnset(t *testing.T) {
	os.Unsetenv(envPrefix + "NOT_SET_ANYWHERE")
	os.Unsetenv(legacyEnvPrefix + "NOT_SET_ANYWHERE")
	_, ok := lookupEnv("NOT_SET_ANYWHERE")
	require.False(t, ok)
}

// A project holding only the pre-rebrand data directory must go on using it,
// or it loses its sessions and its database.
func TestLookupDataDirectoryFallsBackToLegacy(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, legacyName(defaultDataDirectory))
	require.NoError(t, os.MkdirAll(legacy, 0o750))

	got, ok := lookupDataDirectory(dir)
	require.True(t, ok)
	require.Equal(t, legacy, got)
}

func TestLookupDataDirectoryPrefersCurrent(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, defaultDataDirectory)
	require.NoError(t, os.MkdirAll(current, 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, legacyName(defaultDataDirectory)), 0o750))

	got, ok := lookupDataDirectory(dir)
	require.True(t, ok)
	require.Equal(t, current, got)
}

// Both spellings of the shell config have to be recognised as shell configs;
// treating a legacy one as JSON would fail to parse it.
func TestIsShellConfigAcceptsBothSpellings(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"atlasrc", ".atlasrc", "crushrc", ".crushrc"} {
		require.True(t, isShellConfig(filepath.Join("/tmp", name)), name)
	}
	for _, name := range []string{"atlas.json", "crush.json", "AGENTS.md"} {
		require.False(t, isShellConfig(filepath.Join("/tmp", name)), name)
	}
}
