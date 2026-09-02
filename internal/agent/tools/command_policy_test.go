package tools

import (
	"slices"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/stretchr/testify/require"
)

func blocked(t *testing.T, p CommandPolicy, command ...string) bool {
	t.Helper()
	for _, f := range p.blockFuncs() {
		if f(command) {
			return true
		}
	}
	return false
}

func TestEmptyPolicyKeepsTheBuiltInBlocks(t *testing.T) {
	var p CommandPolicy
	require.True(t, blocked(t, p, "curl", "https://example.com"))
	require.True(t, blocked(t, p, "sudo", "rm"))
	require.True(t, blocked(t, p, "npm", "install", "-g", "typescript"))
	require.False(t, blocked(t, p, "npm", "install"))
	require.False(t, blocked(t, p, "ls", "-la"))
}

func TestAllowLiftsACommandAndItsSubcommandBlocks(t *testing.T) {
	p := CommandPolicy{Allow: []string{"curl", "npm"}}
	require.False(t, blocked(t, p, "curl", "https://example.com"))
	require.False(t, blocked(t, p, "npm", "install", "-g", "typescript"))
	require.NotContains(t, p.banned(), "curl")

	// Everything else is untouched.
	require.True(t, blocked(t, p, "sudo", "rm"))
	require.True(t, blocked(t, p, "pnpm", "add", "-g", "x"))
}

func TestBlockAddsCommands(t *testing.T) {
	p := CommandPolicy{Block: []string{"kubectl", "terraform"}}
	require.True(t, blocked(t, p, "kubectl", "delete", "pod"))
	require.True(t, blocked(t, p, "terraform", "apply"))
	require.True(t, blocked(t, p, "curl", "https://example.com"))
}

// An allow is the more specific instruction, so it wins: blocking a command
// the user explicitly allowed would be the more surprising failure.
func TestAllowWinsOverBlockForTheSameCommand(t *testing.T) {
	p := CommandPolicy{Allow: []string{"kubectl"}, Block: []string{"kubectl"}}
	require.False(t, blocked(t, p, "kubectl", "get", "pods"))
}

func TestBannedListIsSortedDeduplicatedAndTrimmed(t *testing.T) {
	p := CommandPolicy{Block: []string{"curl", " kubectl ", "", "kubectl"}}
	got := p.banned()
	require.True(t, slices.IsSorted(got))
	require.Equal(t, 1, countOf(got, "kubectl"))
	require.Equal(t, 1, countOf(got, "curl"))
	require.NotContains(t, got, "")
	require.NotContains(t, got, " kubectl ")
}

func countOf(list []string, want string) int {
	var n int
	for _, s := range list {
		if s == want {
			n++
		}
	}
	return n
}

func TestNewCommandPolicyReadsOptions(t *testing.T) {
	require.Equal(t, CommandPolicy{}, NewCommandPolicy(nil))
	require.Equal(t, CommandPolicy{}, NewCommandPolicy(&config.Config{}))

	p := NewCommandPolicy(&config.Config{Options: &config.Options{
		AllowedCommands: []string{"curl"},
		BlockedCommands: []string{"kubectl"},
	}})
	require.Equal(t, CommandPolicy{Allow: []string{"curl"}, Block: []string{"kubectl"}}, p)
}
