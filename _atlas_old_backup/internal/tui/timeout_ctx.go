package tui

import (
	"context"
	"time"
)

// newTimeoutContext is a tiny stdlib-free context creator. Used by
// the git-branch and other subprocess hooks so they don't have to
// import context themselves. The returned cancel function MUST be
// called by the caller (typically via defer) to release resources.
func newTimeoutContext(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
