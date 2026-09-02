package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-models/pkg/catwalk"
	"github.com/stretchr/testify/require"
)

func resetProviderState() {
	providerOnce = sync.Once{}
	providerList = nil
	providerErr = nil
	catwalkSyncer = &catwalkSync{}
}

func TestProviders_Integration_AutoUpdateDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	// Use a test-specific instance to avoid global state interference.
	testCatwalkSyncer := &catwalkSync{}

	originalCatwalSyncer := catwalkSyncer
	defer func() {
		catwalkSyncer = originalCatwalSyncer
	}()

	catwalkSyncer = testCatwalkSyncer

	resetProviderState()
	defer resetProviderState()

	cfg := &Config{
		Options: &Options{
			DisableProviderAutoUpdate: true,
		},
	}

	providers, err := Providers(cfg)
	require.NoError(t, err)
	require.NotNil(t, providers)
	require.Greater(t, len(providers), 5, "Expected embedded providers")
}

func TestProviders_Integration_WithMockClients(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	// Create a fresh syncer for this test.
	testCatwalkSyncer := &catwalkSync{}

	// Initialize with a mock client.
	mockCatwalkClient := &mockCatwalkClient{
		providers: []catwalk.Provider{
			{Name: "Provider1", ID: "p1"},
			{Name: "Provider2", ID: "p2"},
		},
	}

	catwalkPath := tmpDir + "/Atlas-Agent/providers.json"
	testCatwalkSyncer.Init(mockCatwalkClient, catwalkPath, true)

	catwalkProviders, err := testCatwalkSyncer.Get(t.Context())
	require.NoError(t, err)
	require.Len(t, catwalkProviders, 2)
	require.Equal(t, "Provider1", catwalkProviders[0].Name)
}

func TestProviders_Integration_WithCachedData(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	// Create the cache file.
	catwalkPath := tmpDir + "/Atlas-Agent/providers.json"

	require.NoError(t, os.MkdirAll(tmpDir+"/Atlas-Agent", 0o755))

	// Write Catwalk cache.
	catwalkProviders := []catwalk.Provider{
		{Name: "Cached1", ID: "c1"},
		{Name: "Cached2", ID: "c2"},
	}
	data, err := json.Marshal(catwalkProviders)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(catwalkPath, data, 0o644))

	// Create a fresh syncer.
	testCatwalkSyncer := &catwalkSync{}

	// A client that reports the catalog is unchanged, so the cache is used.
	mockCatwalkClient := &mockCatwalkClient{
		err: catwalk.ErrNotModified,
	}

	testCatwalkSyncer.Init(mockCatwalkClient, catwalkPath, true)

	// Get providers - should use cached.
	catwalkResult, err := testCatwalkSyncer.Get(t.Context())
	require.NoError(t, err)
	require.Len(t, catwalkResult, 2)
	require.Equal(t, "Cached1", catwalkResult[0].Name)
}

// TestProviders_FallsBackToEmbeddedCatalog covers the case that keeps the
// program usable offline: nothing is cached and the fetch does not produce a
// catalog, so the copy bundled with this release is what callers get.
func TestProviders_FallsBackToEmbeddedCatalog(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	testCatwalkSyncer := &catwalkSync{}
	mockCatwalkClient := &mockCatwalkClient{
		err: catwalk.ErrNotModified,
	}

	catwalkPath := tmpDir + "/Atlas-Agent/providers.json"
	testCatwalkSyncer.Init(mockCatwalkClient, catwalkPath, true)

	catwalkResult, err := testCatwalkSyncer.Get(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, catwalkResult, "the embedded catalog stands in")
}

func TestCache_StoreAndGet(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cachePath := tmpDir + "/test.json"

	cache := newCache[[]catwalk.Provider](cachePath)

	providers := []catwalk.Provider{
		{Name: "Provider1", ID: "p1"},
		{Name: "Provider2", ID: "p2"},
	}

	// Store.
	err := cache.Store(providers)
	require.NoError(t, err)

	// Get.
	result, etag, err := cache.Get()
	require.NoError(t, err)
	require.Len(t, result, 2)
	require.Equal(t, "Provider1", result[0].Name)
	require.NotEmpty(t, etag)
}

func TestCache_GetNonExistent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cachePath := tmpDir + "/nonexistent.json"

	cache := newCache[[]catwalk.Provider](cachePath)

	_, _, err := cache.Get()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read provider cache file")
}

func TestCache_GetInvalidJSON(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cachePath := tmpDir + "/invalid.json"

	require.NoError(t, os.WriteFile(cachePath, []byte("invalid json"), 0o644))

	cache := newCache[[]catwalk.Provider](cachePath)

	_, _, err := cache.Get()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to unmarshal provider data from cache")
}

func TestCachePathFor(t *testing.T) {
	tests := []struct {
		name        string
		xdgDataHome string
		expected    string
	}{
		{
			name:        "with XDG_DATA_HOME",
			xdgDataHome: "/custom/data",
			expected:    "/custom/data/atlas/providers.json",
		},
		{
			name:        "without XDG_DATA_HOME",
			xdgDataHome: "",
			expected:    "", // Will use platform-specific default.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.xdgDataHome != "" {
				t.Setenv("XDG_DATA_HOME", tt.xdgDataHome)
			} else {
				t.Setenv("XDG_DATA_HOME", "")
			}

			result := cachePathFor("providers")
			if tt.expected != "" {
				require.Equal(t, tt.expected, filepath.ToSlash(result))
			} else {
				// This branch reads the machine's real data directory, so
				// which spelling comes back depends on what that machine
				// already has: a host carrying a pre-rebrand install keeps
				// using it. Both answers are correct here.
				require.True(t,
					strings.Contains(result, appName) || strings.Contains(result, legacyAppName),
					"expected the app's own directory in %q", result)
				require.Contains(t, result, "providers.json")
			}
		})
	}
}

// TestProviders_KeepsCatalogWhenCachingFails covers the case that used to
// sign Hyper users out: the provider list was fetched successfully but could
// not be written to the on-disk cache, and Providers discarded it. Hyper's
// endpoint and models live in the catalog rather than in the user's config,
// so losing it there removed the provider entirely and invalidated the
// user's saved model.
func TestProviders_KeepsCatalogWhenCachingFails(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	// A file where a directory needs to be, so every cache write fails.
	blocked := filepath.Join(tmpDir, "blocked")
	require.NoError(t, os.WriteFile(blocked, []byte("block"), 0o644))
	unwritable := filepath.Join(blocked, "subdir", "cache.json")

	resetProviderState()
	defer resetProviderState()

	// Prime the syncer with a mock client so Providers reuses the memoized
	// outcome instead of reaching the network.
	catwalkSyncer.Init(&mockCatwalkClient{
		providers: []catwalk.Provider{{Name: "Provider1", ID: "p1"}},
	}, unwritable, true)

	catwalkProviders, catwalkErr := catwalkSyncer.Get(t.Context())
	require.Error(t, catwalkErr, "cache write should fail")
	require.NotEmpty(t, catwalkProviders, "syncer still returns a usable catalog")

	providers, err := Providers(&Config{Options: &Options{}})

	// The failure is reported, but as a warning alongside a usable catalog.
	require.Error(t, err)
	require.Len(t, providers, 1)
	require.Equal(t, catwalk.InferenceProvider("p1"), providers[0].ID)
}

// TestProviders_HonorsDisableDefaultProviders makes sure the embedded
// catalog fallback does not smuggle a default provider back in.
func TestProviders_HonorsDisableDefaultProviders(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	resetProviderState()
	defer resetProviderState()

	providers, err := Providers(&Config{
		Options: &Options{DisableDefaultProviders: true},
	})
	require.NoError(t, err)
	require.Empty(t, providers)
}

// TestCacheStore_ReplacesFileInsteadOfRewritingIt guards the property that
// several Atlas-Agent instances depend on: the provider cache is swapped into place
// as a finished file, never truncated and refilled underneath a reader that is
// already reading it. A reader that loses that race cannot parse the catalog
// and silently falls back to the bundled copy.
func TestCacheStore_ReplacesFileInsteadOfRewritingIt(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "providers.json")
	c := newCache[[]catwalk.Provider](path)

	require.NoError(t, c.Store([]catwalk.Provider{{ID: "first", Name: "First"}}))
	before, err := os.Stat(path)
	require.NoError(t, err)

	require.NoError(t, c.Store([]catwalk.Provider{{ID: "second", Name: "Second"}}))
	after, err := os.Stat(path)
	require.NoError(t, err)

	// os.Stat on Windows resolves file identity lazily by reopening the path,
	// so both stats describe whichever file the path points at by the time
	// they are compared and SameFile cannot observe the replacement. The
	// write path is shared, so asserting this on the other platforms covers
	// it. The checks below still run everywhere.
	if runtime.GOOS != "windows" {
		require.False(t, os.SameFile(before, after),
			"the cache should be replaced by a rename, not rewritten in place")
	}

	// The new contents are complete and no temporary files are left behind.
	got, _, err := c.Get()
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, catwalk.InferenceProvider("second"), got[0].ID)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the cache file should remain")
	require.Equal(t, "providers.json", entries[0].Name())
}
