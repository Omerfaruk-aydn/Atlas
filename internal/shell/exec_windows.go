//go:build windows

package shell

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/sandbox"
	"mvdan.cc/sh/v3/interp"
)

// defaultKillTimeout matches mvdan's DefaultExecHandler default.
const defaultKillTimeout = 2 * time.Second

// isolateProcess is a no-op on Windows. Session isolation via Setsid is a
// Unix-only concept; Windows uses CREATE_NEW_PROCESS_GROUP which mvdan's
// default handler already handles adequately.
func isolateProcess(_ *exec.Cmd) {}

// processGroupExecHandler returns interp.DefaultExecHandler on Windows,
// unless sandboxing has been turned on via SetSandboxLimits, in which
// case every process it spawns is additionally assigned to a
// sandbox.Job -- see sandboxedExecHandler.
func processGroupExecHandler(killTimeout time.Duration) interp.ExecHandlerFunc {
	base := interp.DefaultExecHandler(killTimeout)
	return func(ctx context.Context, args []string) error {
		limits, enabled := currentSandboxLimits()
		if !enabled {
			return base(ctx, args)
		}
		return sandboxedExec(ctx, args, limits)
	}
}

// sandboxedExec mirrors interp.DefaultExecHandler's Windows behavior
// exactly (same lookup, same immediate-kill-on-cancel semantics, same
// exit-status mapping), with one addition: the spawned process is
// assigned to a fresh sandbox.Job right after it starts, so it (and
// anything it spawns) cannot outlive the job closing, and is optionally
// capped on process count / memory. See internal/sandbox's package doc
// for exactly what that does and does not protect against.
//
// A job that fails to create or assign falls back to running the command
// unsandboxed rather than refusing to run it at all -- logged once so the
// gap is visible, since silently degrading a safety feature is worse
// than a loud one.
func sandboxedExec(ctx context.Context, args []string, limits sandbox.Limits) error {
	hc := interp.HandlerCtx(ctx)
	path, err := interp.LookPathDir(hc.Dir, hc.Env, args[0])
	if err != nil {
		fmt.Fprintln(hc.Stderr, err)
		return interp.ExitStatus(127)
	}

	job, jobErr := sandbox.New(limits)
	if jobErr != nil {
		slog.Warn("Failed to create sandbox job; running command unsandboxed", "command", args[0], "error", jobErr)
	} else {
		defer job.Close()
	}

	cmd := exec.Cmd{
		Path:   path,
		Args:   args,
		Env:    execEnvList(hc.Env),
		Dir:    hc.Dir,
		Stdin:  hc.Stdin,
		Stdout: hc.Stdout,
		Stderr: hc.Stderr,
	}

	err = cmd.Start()
	if err == nil {
		if job != nil {
			if assignErr := job.Assign(cmd.Process); assignErr != nil {
				slog.Warn("Failed to sandbox process; it will run unconstrained", "command", args[0], "error", assignErr)
			}
		}

		stopf := context.AfterFunc(ctx, func() {
			// Matches interp.DefaultExecHandler: Go doesn't support
			// sending Interrupt on Windows, so cancellation always kills
			// immediately.
			_ = cmd.Process.Signal(os.Kill)
		})
		defer stopf()

		err = cmd.Wait()
	}

	switch err := err.(type) {
	case *exec.ExitError:
		if status, ok := err.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return interp.ExitStatus(128 + uint8(status.Signal()))
		}
		return interp.ExitStatus(uint8(err.ExitCode()))
	case *exec.Error:
		fmt.Fprintf(hc.Stderr, "%v\n", err)
		return interp.ExitStatus(127)
	default:
		return err
	}
}
