package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A config that never mentions retries must leave the provider library's
// default in place, not silently disable retrying.
func TestUnsetRetryBudgetLeavesTheDefaultAlone(t *testing.T) {
	a := &sessionAgent{}
	require.Nil(t, a.maxRetries())
}

func TestConfiguredRetryBudgetIsPassedThrough(t *testing.T) {
	n := 7
	a := &sessionAgent{maxProviderRetries: &n}
	got := a.maxRetries()
	require.NotNil(t, got)
	require.Equal(t, 7, *got)
}

// Zero is a real setting -- "do not retry" -- and must survive, unlike an
// absent one.
func TestZeroRetryBudgetDisablesRetries(t *testing.T) {
	n := 0
	a := &sessionAgent{maxProviderRetries: &n}
	got := a.maxRetries()
	require.NotNil(t, got)
	require.Equal(t, 0, *got)
}

func TestNegativeRetryBudgetIsClampedToNone(t *testing.T) {
	n := -3
	a := &sessionAgent{maxProviderRetries: &n}
	got := a.maxRetries()
	require.NotNil(t, got)
	require.Equal(t, 0, *got)
}

// The library must never be handed a pointer into the agent's own state.
func TestRetryBudgetIsCopied(t *testing.T) {
	n := 4
	a := &sessionAgent{maxProviderRetries: &n}
	got := a.maxRetries()
	*got = 99
	require.Equal(t, 4, n)
}
