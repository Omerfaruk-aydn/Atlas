package agent

import (
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func stepsOf(n int) []fantasy.StepResult {
	steps := make([]fantasy.StepResult, n)
	for i := range steps {
		steps[i] = makeEmptyStep()
	}
	return steps
}

func TestMaxStepsReachedZeroIsUnbounded(t *testing.T) {
	t.Parallel()
	require.False(t, maxStepsReached(stepsOf(10_000), 0))
	require.False(t, maxStepsReached(nil, 0))
}

func TestMaxStepsReachedNegativeIsUnbounded(t *testing.T) {
	t.Parallel()
	require.False(t, maxStepsReached(stepsOf(10_000), -1))
}

func TestMaxStepsReachedUnderTheCap(t *testing.T) {
	t.Parallel()
	require.False(t, maxStepsReached(stepsOf(4), 5))
}

func TestMaxStepsReachedAtTheCap(t *testing.T) {
	t.Parallel()
	require.True(t, maxStepsReached(stepsOf(5), 5))
}

func TestMaxStepsReachedPastTheCap(t *testing.T) {
	t.Parallel()
	require.True(t, maxStepsReached(stepsOf(6), 5))
}

func TestMaxStepsReachedCapOfOneStopsImmediately(t *testing.T) {
	t.Parallel()
	require.True(t, maxStepsReached(stepsOf(1), 1))
	require.False(t, maxStepsReached(nil, 1))
}
