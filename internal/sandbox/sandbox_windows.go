//go:build windows

package sandbox

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Supported reports whether this OS supports process sandboxing via this
// package. Always true on Windows.
func Supported() bool { return true }

// Job wraps a Windows Job Object. See the package doc for exactly what
// containment does and does not mean.
type Job struct {
	handle windows.Handle
}

// New creates a Job enforcing limits. JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
// is always set, so every process ever assigned to it is terminated the
// moment the job is closed, regardless of what other limits are asked
// for.
func New(limits Limits) (*Job, error) {
	handle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateJobObject: %w", err)
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if limits.MaxProcesses > 0 {
		info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
		info.BasicLimitInformation.ActiveProcessLimit = limits.MaxProcesses
	}
	if limits.MaxMemoryBytes > 0 {
		info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY
		info.ProcessMemoryLimit = uintptr(limits.MaxMemoryBytes)
	}

	if _, err := windows.SetInformationJobObject(
		handle,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("SetInformationJobObject: %w", err)
	}

	return &Job{handle: handle}, nil
}

// Assign puts process under this job's limits.
//
// There is an inherent, brief race between a process starting and this
// call returning: anything the process spawns in that window is not yet
// contained. Eliminating it entirely means launching the process
// suspended (CREATE_SUSPENDED), assigning it to the job, then resuming
// its main thread -- which requires driving CreateProcess directly
// instead of through os/exec, since os/exec does not expose the thread
// handle a resume needs. Callers here go through os/exec (see
// internal/shell), so the window stays open; it is narrow, and no
// containment at all is strictly worse.
func (j *Job) Assign(process *os.Process) error {
	if j == nil || process == nil {
		return nil
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(process.Pid))
	if err != nil {
		return fmt.Errorf("OpenProcess: %w", err)
	}
	defer windows.CloseHandle(handle)
	return windows.AssignProcessToJobObject(j.handle, handle)
}

// Close releases the job. Every process still assigned to it is
// terminated immediately, per JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE. Safe to
// call on a nil *Job.
func (j *Job) Close() error {
	if j == nil {
		return nil
	}
	return windows.CloseHandle(j.handle)
}
