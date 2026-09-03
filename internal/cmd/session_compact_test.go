package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionCompactCommandRequiresExactlyOneArg(t *testing.T) {
	t.Parallel()

	require.Error(t, sessionCompactCmd.Args(sessionCompactCmd, nil))
	require.Error(t, sessionCompactCmd.Args(sessionCompactCmd, []string{"a", "b"}))
	require.NoError(t, sessionCompactCmd.Args(sessionCompactCmd, []string{"a"}))
}
