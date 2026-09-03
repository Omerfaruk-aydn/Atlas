// Package sandbox contains a spawned process (and anything it spawns) in
// an OS-level container: on Windows, a Job Object with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, optionally capped on process count
// and per-process memory.
//
// This is process lifetime and resource containment, not a security
// sandbox. A contained process can still read and write any file it has
// permission to and reach the network normally -- nothing here restricts
// that. What it guarantees is that the process cannot outlive being cut
// off (no orphaned child trees survive their parent closing the job), and
// optionally that it cannot fork past a process-count ceiling or exceed a
// memory ceiling. Building an actual filesystem/network security boundary
// (a restricted token with a low integrity level, or Windows Defender
// Application Guard-style containment) is real, separate work this
// package does not attempt.
//
// Currently implemented for Windows only (see sandbox_windows.go); on
// every other OS, New returns ErrUnsupported (see sandbox_other.go).
package sandbox

import "errors"

// ErrUnsupported is returned by New on a platform without an
// implementation (anything but Windows, for now).
var ErrUnsupported = errors.New("process sandboxing is not supported on this OS")

// Limits configures a container's constraints. The zero value for a field
// means "no limit" for that dimension.
type Limits struct {
	// MaxProcesses caps how many processes may be active in the
	// container at once -- a basic fork-bomb mitigation. 0 means
	// unlimited.
	MaxProcesses uint32
	// MaxMemoryBytes caps the committed memory of any single process in
	// the container. 0 means unlimited.
	MaxMemoryBytes uint64
}
