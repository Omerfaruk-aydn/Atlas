package debugger

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/go-dap"
)

// Options configures how a Session launches Delve and bounds its actions.
type Options struct {
	// DlvPath is the `dlv` binary to run. Empty means "dlv" on PATH.
	DlvPath string
	// ActionTimeout bounds a single request/response round trip
	// (setBreakpoints, evaluate, ...). It does not bound continue/step,
	// which wait for the debuggee to actually stop -- that can take
	// arbitrarily long and is bounded by the caller's context instead.
	ActionTimeout time.Duration
}

// StoppedInfo describes where execution is paused.
type StoppedInfo struct {
	Reason   string
	ThreadID int
	File     string
	Line     int
	FuncName string
}

// Session owns one `dlv dap` process debugging one program: launch, set
// breakpoints, step, inspect. It is not safe for concurrent use by more
// than one caller at a time (mirrors a real debugger's single line of
// control), which the debugger tool enforces per chat session.
type Session struct {
	cmd    *exec.Cmd
	conn   net.Conn
	client *client
	opts   Options

	mu              sync.Mutex
	currentThreadID int
	closed          bool

	events    chan Event
	eventsWG  sync.WaitGroup
	lastStop  *StoppedInfo
	lastExit  *int
	lastTerm  bool
	outputBuf strings.Builder
	outMu     sync.Mutex
}

// Start launches `dlv dap`, connects to it, and runs the program under the
// debugger with an initial breakpoint at entry so the caller can set real
// breakpoints before anything actually executes.
func Start(ctx context.Context, opts Options, program string, args []string) (*Session, error) {
	dlvPath := opts.DlvPath
	if dlvPath == "" {
		dlvPath = "dlv"
	}
	if opts.ActionTimeout <= 0 {
		opts.ActionTimeout = 30 * time.Second
	}

	addr, ln, err := reserveLoopbackAddr(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to reserve a port for dlv dap: %w", err)
	}
	ln.Close() // dlv needs the port free to bind it itself

	// context.Background(), not ctx: the dlv process must outlive this
	// call (it runs for the whole debug session, well past Start
	// returning), the same reason shell.BackgroundShellManager starts
	// commands detached from the calling request's context.
	cmd := exec.CommandContext(context.Background(), dlvPath, "dap", "--listen", addr)
	// dlv builds the target with `go build` from its own process cwd. Left
	// at whatever directory atlas-agent happens to be running in --
	// almost always a different Go module, its own -- that build fails
	// with "directory ... outside main module or its selected
	// dependencies". Running from the program's own directory (or its
	// parent, if program is a single file) puts the build in the right
	// module context regardless of where atlas-agent itself is rooted.
	if dir := programDir(program); dir != "" {
		cmd.Dir = dir
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start %s: %w", dlvPath, err)
	}

	conn, err := dialWithRetry(ctx, addr, 5*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("failed to connect to dlv dap: %w", err)
	}

	s := &Session{
		cmd:    cmd,
		conn:   conn,
		client: newClient(conn),
		opts:   opts,
		events: make(chan Event, 64),
	}
	s.eventsWG.Add(1)
	go s.pumpEvents()

	if err := s.launch(ctx, program, args); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// programDir returns the directory dlv's `go build` should run from for
// program: program itself if it is already a directory, otherwise its
// parent. Empty (and thus "leave cmd.Dir alone") if program can't be
// stat'd -- an unresolvable program is reported clearly enough by the
// launch request itself.
func programDir(program string) string {
	info, err := os.Stat(program)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return program
	}
	return filepath.Dir(program)
}

// reserveLoopbackAddr picks a free loopback port by binding to :0. The
// listener is returned still open so the caller controls exactly when the
// port is released, narrowing (not eliminating) the window in which
// something else could grab it before dlv does.
func reserveLoopbackAddr(ctx context.Context) (string, net.Listener, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	return ln.Addr().String(), ln, nil
}

func dialWithRetry(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(timeout)
	var dialer net.Dialer
	var lastErr error
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		attemptCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		conn, err := dialer.DialContext(attemptCtx, "tcp", addr)
		cancel()
		if err == nil {
			return conn, nil
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	return nil, fmt.Errorf("timed out waiting for dlv dap to listen on %s: %w", addr, lastErr)
}

// pumpEvents keeps the latest stop/exit/terminate state and appends
// program output, so a request handler (which runs on whatever goroutine
// the tool calls it from) can read consistent state without racing the
// client's read loop.
func (s *Session) pumpEvents() {
	defer s.eventsWG.Done()
	for ev := range s.client.Events() {
		switch ev.Kind {
		case "stopped":
			s.mu.Lock()
			s.currentThreadID = ev.ThreadID
			s.mu.Unlock()
			// The stopped event alone doesn't carry a file/line -- that
			// needs a stackTrace call, done lazily by whoever is waiting
			// on this stop (see WaitStopped), not here, so pumpEvents
			// never blocks on a request of its own.
			select {
			case s.events <- ev:
			default:
			}
		case "terminated", "exited":
			select {
			case s.events <- ev:
			default:
			}
		case "output":
			s.outMu.Lock()
			s.outputBuf.WriteString(ev.Output)
			s.outMu.Unlock()
		}
	}
}

func (s *Session) launch(ctx context.Context, program string, args []string) error {
	initReq := &dap.InitializeRequest{
		Request: dap.Request{Command: "initialize"},
		Arguments: dap.InitializeRequestArguments{
			AdapterID:       "atlas-agent",
			LinesStartAt1:   true,
			ColumnsStartAt1: true,
			PathFormat:      "path",
		},
	}
	var initResp dap.InitializeResponse
	if err := s.client.send(ctx, initReq, &initResp); err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}

	launchArgs, err := json.Marshal(struct {
		Mode        string   `json:"mode"`
		Program     string   `json:"program"`
		Args        []string `json:"args,omitempty"`
		StopOnEntry bool     `json:"stopOnEntry"`
		OutputMode  string   `json:"outputMode"`
	}{
		Mode:        "debug",
		Program:     program,
		Args:        args,
		StopOnEntry: true,
		// "local" (Delve's default) connects the debuggee's stdout/stderr
		// straight to dlv's own, which atlas-agent never reads. "remote"
		// relays it as DAP output events instead, which DrainOutput
		// surfaces to the tool -- otherwise the debuggee could deadlock
		// waiting for a doomed write to a closed pipe, and its output
		// would be invisible either way.
		OutputMode: "remote",
	})
	if err != nil {
		return err
	}

	// launch's response arrives before Delve is actually ready to accept
	// setBreakpoints, so this doesn't wait for the "initialized" event or
	// send configurationDone before returning: the tool only needs the
	// process running and paused at entry, which the caller observes via
	// WaitStopped.
	launchReq := &dap.LaunchRequest{Request: dap.Request{Command: "launch"}, Arguments: launchArgs}
	if err := s.client.send(ctx, launchReq, nil); err != nil {
		// A launch failure is almost always a build error, and the actual
		// compiler output only ever arrives as "output" events, never in
		// the response itself -- give the caller a moment to receive them
		// and fold whatever came in into the error, or this just reports
		// "Build error, check the debug console" with no console to check.
		time.Sleep(200 * time.Millisecond)
		if out := s.DrainOutput(); out != "" {
			return fmt.Errorf("launch failed: %w\n%s", err, out)
		}
		return fmt.Errorf("launch failed: %w", err)
	}

	confReq := &dap.ConfigurationDoneRequest{Request: dap.Request{Command: "configurationDone"}}
	if err := s.client.send(ctx, confReq, nil); err != nil {
		return fmt.Errorf("configurationDone failed: %w", err)
	}

	return nil
}

// WaitStopped blocks until the debuggee stops, exits, or the session
// terminates, returning which of those happened. Call it after Start,
// Continue, Next, StepIn, or StepOut, all of which resume or begin
// execution without waiting for it to pause again themselves.
func (s *Session) WaitStopped(ctx context.Context) (*StoppedInfo, *int, error) {
	select {
	case ev, ok := <-s.events:
		if !ok {
			return nil, nil, fmt.Errorf("debugger connection closed")
		}
		switch ev.Kind {
		case "stopped":
			info, err := s.describeStop(ctx, ev.ThreadID, ev.Reason)
			if err != nil {
				return nil, nil, err
			}
			s.mu.Lock()
			s.lastStop = info
			s.mu.Unlock()
			return info, nil, nil
		case "exited":
			code := ev.ExitCode
			s.mu.Lock()
			s.lastExit = &code
			s.mu.Unlock()
			return nil, &code, nil
		case "terminated":
			s.mu.Lock()
			s.lastTerm = true
			s.mu.Unlock()
			return nil, nil, nil
		default:
			return nil, nil, fmt.Errorf("unexpected event %q while waiting for a stop", ev.Kind)
		}
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
}

func (s *Session) describeStop(ctx context.Context, threadID int, reason string) (*StoppedInfo, error) {
	frames, err := s.StackTrace(ctx, threadID, 1)
	if err != nil || len(frames) == 0 {
		// The location is best-effort context, not the point of the
		// stop -- report the stop itself even if the frame lookup fails.
		return &StoppedInfo{Reason: reason, ThreadID: threadID}, nil
	}
	top := frames[0]
	info := &StoppedInfo{Reason: reason, ThreadID: threadID, FuncName: top.Name}
	if top.Source != nil {
		info.File = top.Source.Path
	}
	info.Line = top.Line
	return info, nil
}

func (s *Session) currentThread() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentThreadID
}

func (s *Session) actionCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, s.opts.ActionTimeout)
}

// SetBreakpoints replaces all breakpoints in file with ones at lines.
// Passing an empty lines clears every breakpoint in that file.
func (s *Session) SetBreakpoints(ctx context.Context, file string, lines []int) ([]dap.Breakpoint, error) {
	ctx, cancel := s.actionCtx(ctx)
	defer cancel()

	req := &dap.SetBreakpointsRequest{
		Request: dap.Request{Command: "setBreakpoints"},
		Arguments: dap.SetBreakpointsArguments{
			Source: dap.Source{Path: file},
			Lines:  lines,
		},
	}
	for _, l := range lines {
		req.Arguments.Breakpoints = append(req.Arguments.Breakpoints, dap.SourceBreakpoint{Line: l})
	}

	var resp dap.SetBreakpointsResponse
	if err := s.client.send(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp.Body.Breakpoints, nil
}

// Continue resumes the current thread. Call WaitStopped afterward to learn
// what happens next.
func (s *Session) Continue(ctx context.Context) error {
	ctx, cancel := s.actionCtx(ctx)
	defer cancel()
	req := &dap.ContinueRequest{
		Request:   dap.Request{Command: "continue"},
		Arguments: dap.ContinueArguments{ThreadId: s.currentThread()},
	}
	return s.client.send(ctx, req, nil)
}

// Next steps over the current line.
func (s *Session) Next(ctx context.Context) error {
	ctx, cancel := s.actionCtx(ctx)
	defer cancel()
	req := &dap.NextRequest{
		Request:   dap.Request{Command: "next"},
		Arguments: dap.NextArguments{ThreadId: s.currentThread()},
	}
	return s.client.send(ctx, req, nil)
}

// StepIn steps into the call on the current line.
func (s *Session) StepIn(ctx context.Context) error {
	ctx, cancel := s.actionCtx(ctx)
	defer cancel()
	req := &dap.StepInRequest{
		Request:   dap.Request{Command: "stepIn"},
		Arguments: dap.StepInArguments{ThreadId: s.currentThread()},
	}
	return s.client.send(ctx, req, nil)
}

// StepOut runs until the current function returns.
func (s *Session) StepOut(ctx context.Context) error {
	ctx, cancel := s.actionCtx(ctx)
	defer cancel()
	req := &dap.StepOutRequest{
		Request:   dap.Request{Command: "stepOut"},
		Arguments: dap.StepOutArguments{ThreadId: s.currentThread()},
	}
	return s.client.send(ctx, req, nil)
}

// StackTrace returns up to levels frames (0 for "all") of threadID, or the
// currently stopped thread if threadID is 0.
func (s *Session) StackTrace(ctx context.Context, threadID, levels int) ([]dap.StackFrame, error) {
	ctx, cancel := s.actionCtx(ctx)
	defer cancel()
	if threadID == 0 {
		threadID = s.currentThread()
	}
	req := &dap.StackTraceRequest{
		Request:   dap.Request{Command: "stackTrace"},
		Arguments: dap.StackTraceArguments{ThreadId: threadID, Levels: levels},
	}
	var resp dap.StackTraceResponse
	if err := s.client.send(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp.Body.StackFrames, nil
}

// Scopes returns the variable scopes (locals, arguments, ...) for a frame.
func (s *Session) Scopes(ctx context.Context, frameID int) ([]dap.Scope, error) {
	ctx, cancel := s.actionCtx(ctx)
	defer cancel()
	req := &dap.ScopesRequest{
		Request:   dap.Request{Command: "scopes"},
		Arguments: dap.ScopesArguments{FrameId: frameID},
	}
	var resp dap.ScopesResponse
	if err := s.client.send(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp.Body.Scopes, nil
}

// Variables returns the children of a variables reference, as returned by
// Scopes or by a structured Variable's own VariablesReference.
func (s *Session) Variables(ctx context.Context, variablesReference int) ([]dap.Variable, error) {
	ctx, cancel := s.actionCtx(ctx)
	defer cancel()
	req := &dap.VariablesRequest{
		Request:   dap.Request{Command: "variables"},
		Arguments: dap.VariablesArguments{VariablesReference: variablesReference},
	}
	var resp dap.VariablesResponse
	if err := s.client.send(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp.Body.Variables, nil
}

// Evaluate runs expression in the context of frameID (0 for the global/
// default context).
func (s *Session) Evaluate(ctx context.Context, expression string, frameID int) (dap.EvaluateResponseBody, error) {
	ctx, cancel := s.actionCtx(ctx)
	defer cancel()
	req := &dap.EvaluateRequest{
		Request:   dap.Request{Command: "evaluate"},
		Arguments: dap.EvaluateArguments{Expression: expression, FrameId: frameID, Context: "repl"},
	}
	var resp dap.EvaluateResponse
	if err := s.client.send(ctx, req, &resp); err != nil {
		return dap.EvaluateResponseBody{}, err
	}
	return resp.Body, nil
}

// DrainOutput returns everything the debuggee has printed since the last
// call and clears the buffer, the same pattern bash/job_output use for
// captured process output.
func (s *Session) DrainOutput() string {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	out := s.outputBuf.String()
	s.outputBuf.Reset()
	return out
}

// Close disconnects from Delve and kills the dlv process. Safe to call
// more than once.
func (s *Session) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := &dap.DisconnectRequest{
		Request:   dap.Request{Command: "disconnect"},
		Arguments: &dap.DisconnectArguments{TerminateDebuggee: true},
	}
	_ = s.client.send(disconnectCtx, req, nil)

	_ = s.client.Close()
	_ = s.conn.Close()
	s.eventsWG.Wait()

	if s.cmd != nil && s.cmd.Process != nil {
		done := make(chan error, 1)
		go func() { done <- s.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = s.cmd.Process.Kill()
			<-done
		}
	}
}

// ExeSuffix is the extension `dlv` needs appended to its bare name to be
// found via exec.LookPath / os.Stat on this platform ("" everywhere but
// Windows). Exported so callers resolving a dlv path (config defaults,
// doctor, tests) share one answer instead of each guessing separately.
func ExeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
