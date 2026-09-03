//go:build !windows

package sandbox

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSupportedIsFalseOutsideWindows(t *testing.T) {
	t.Parallel()
	require.False(t, Supported())
}

func TestNewReturnsErrUnsupported(t *testing.T) {
	t.Parallel()
	job, err := New(Limits{})
	require.Nil(t, job)
	require.True(t, errors.Is(err, ErrUnsupported))
}

func TestJobMethodsAreNoopsOnANilJob(t *testing.T) {
	t.Parallel()
	var job *Job
	require.NoError(t, job.Assign(nil))
	require.NoError(t, job.Close())
}
