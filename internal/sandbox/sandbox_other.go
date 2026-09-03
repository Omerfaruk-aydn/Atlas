//go:build !windows

package sandbox

import "os"

// Supported reports whether this OS supports process sandboxing via this
// package. Always false outside Windows -- see the package doc for what
// would be needed (a restricted token / namespaces / equivalent) and why
// it isn't implemented here yet.
func Supported() bool { return false }

// Job is a placeholder on platforms without an implementation. New never
// actually returns one (it returns ErrUnsupported), but the type exists
// so callers can hold a *Job without platform-specific code.
type Job struct{}

// New always fails on this platform. See ErrUnsupported.
func New(Limits) (*Job, error) {
	return nil, ErrUnsupported
}

// Assign is a no-op: there is never a real Job to assign to on this
// platform.
func (j *Job) Assign(*os.Process) error { return nil }

// Close is a no-op on this platform.
func (j *Job) Close() error { return nil }
