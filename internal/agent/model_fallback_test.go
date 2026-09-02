package agent

import (
	"net/http"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func testModel(provider string) Model {
	return Model{ModelCfg: config.SelectedModel{Provider: provider, Model: provider + "-model"}}
}

func TestModelChainStartsOnThePrimary(t *testing.T) {
	t.Parallel()
	primary := testModel("anthropic")
	chain := newModelChain(primary, []Model{testModel("openai")})

	require.Equal(t, primary, chain.Current())
	require.False(t, chain.Fellback())
}

func TestModelChainMovesOnRateLimit(t *testing.T) {
	t.Parallel()
	fallback := testModel("openai")
	chain := newModelChain(testModel("anthropic"), []Model{fallback})

	moved := chain.HandleRetry(&fantasy.ProviderError{StatusCode: http.StatusTooManyRequests})

	require.True(t, moved)
	require.True(t, chain.Fellback())
	require.Equal(t, fallback, chain.Current())
}

func TestModelChainIgnoresOtherErrors(t *testing.T) {
	t.Parallel()
	chain := newModelChain(testModel("anthropic"), []Model{testModel("openai")})

	for _, err := range []*fantasy.ProviderError{
		{StatusCode: http.StatusInternalServerError},
		{StatusCode: http.StatusUnauthorized},
		nil,
	} {
		moved := chain.HandleRetry(err)
		require.False(t, moved, "status %v must not trigger a failover", err)
	}
	require.False(t, chain.Fellback())
}

func TestModelChainWalksTheWholeList(t *testing.T) {
	t.Parallel()
	second := testModel("openai")
	third := testModel("groq")
	chain := newModelChain(testModel("anthropic"), []Model{second, third})

	rateLimited := &fantasy.ProviderError{StatusCode: http.StatusTooManyRequests}

	require.True(t, chain.HandleRetry(rateLimited))
	require.Equal(t, second, chain.Current())

	require.True(t, chain.HandleRetry(rateLimited))
	require.Equal(t, third, chain.Current())
}

func TestModelChainStopsAtTheEndOfTheList(t *testing.T) {
	t.Parallel()
	chain := newModelChain(testModel("anthropic"), nil)

	moved := chain.HandleRetry(&fantasy.ProviderError{StatusCode: http.StatusTooManyRequests})

	require.False(t, moved, "no fallback is configured, so a 429 has nowhere to go")
	require.False(t, chain.Fellback())
}

func TestModelChainNeverMovesBackward(t *testing.T) {
	t.Parallel()
	second := testModel("openai")
	chain := newModelChain(testModel("anthropic"), []Model{second})
	rateLimited := &fantasy.ProviderError{StatusCode: http.StatusTooManyRequests}

	require.True(t, chain.HandleRetry(rateLimited))
	require.Equal(t, second, chain.Current())

	// A further 429 -- now against the fallback, which has nowhere further
	// to go -- must not wrap back around to the primary.
	require.False(t, chain.HandleRetry(rateLimited))
	require.Equal(t, second, chain.Current())
}
