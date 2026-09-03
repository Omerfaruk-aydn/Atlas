package credentials

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCandidateAPIKeysCombinesPrimaryAndExtra(t *testing.T) {
	got := CandidateAPIKeys("$KEY1", []string{"$KEY2", "$KEY3"})
	require.Equal(t, []string{"$KEY1", "$KEY2", "$KEY3"}, got)
}

func TestCandidateAPIKeysSkipsBlanks(t *testing.T) {
	got := CandidateAPIKeys("", []string{"$KEY2", "", "$KEY3"})
	require.Equal(t, []string{"$KEY2", "$KEY3"}, got)
}

func TestRotatorPickSingleKeyNeverAdvances(t *testing.T) {
	r := New()
	for range 3 {
		require.Equal(t, "$KEY1", r.Pick("openai", []string{"$KEY1"}))
	}
}

func TestRotatorPickRoundRobins(t *testing.T) {
	r := New()
	keys := []string{"$KEY1", "$KEY2", "$KEY3"}

	require.Equal(t, "$KEY1", r.Pick("openai", keys))
	require.Equal(t, "$KEY2", r.Pick("openai", keys))
	require.Equal(t, "$KEY3", r.Pick("openai", keys))
	require.Equal(t, "$KEY1", r.Pick("openai", keys), "wraps back to the first")
}

func TestRotatorPicksAreIndependentPerProvider(t *testing.T) {
	r := New()
	keys := []string{"$KEY1", "$KEY2"}

	require.Equal(t, "$KEY1", r.Pick("openai", keys))
	require.Equal(t, "$KEY1", r.Pick("anthropic", keys), "independent from openai's rotation")
}

func TestRotatorAdvanceSkipsAKey(t *testing.T) {
	r := New()
	keys := []string{"$KEY1", "$KEY2", "$KEY3"}

	require.Equal(t, "$KEY1", r.Pick("openai", keys))
	r.Advance("openai")
	require.Equal(t, "$KEY3", r.Pick("openai", keys))
}

func TestRotatorNilReceiverIsSafe(t *testing.T) {
	var r *Rotator
	require.Equal(t, "$KEY1", r.Pick("openai", []string{"$KEY1"}))
	r.Advance("openai") // must not panic
	require.Equal(t, State{Next: map[string]int{}}, r.State())
}

func TestRotatorPersistsAcrossLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", StateFileName)
	keys := []string{"$KEY1", "$KEY2"}

	r1 := Load(path)
	require.Equal(t, "$KEY1", r1.Pick("openai", keys))
	require.FileExists(t, path)

	r2 := Load(path)
	require.Equal(t, "$KEY2", r2.Pick("openai", keys), "a fresh Rotator loaded from the same path must continue the rotation")
}

func TestLoadWithMissingFileStartsEmpty(t *testing.T) {
	r := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	require.Equal(t, State{Next: map[string]int{}}, r.State())
}

func TestLoadWithCorruptFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), StateFileName)
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o644))

	r := Load(path)
	require.Equal(t, State{Next: map[string]int{}}, r.State())
}

func TestReadStateReflectsWhatPickWrote(t *testing.T) {
	path := filepath.Join(t.TempDir(), StateFileName)
	r := Load(path)
	r.Advance("openai")
	r.Advance("openai")

	state := ReadState(path)
	require.Equal(t, 2, state.Next["openai"])
}

func TestReadStateWithMissingFileIsEmptyNotNil(t *testing.T) {
	state := ReadState(filepath.Join(t.TempDir(), "nope.json"))
	require.NotNil(t, state.Next)
	require.Empty(t, state.Next)
}
