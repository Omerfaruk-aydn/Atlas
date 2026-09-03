package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/debugger"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/permission"
	"github.com/google/go-dap"
	"github.com/stretchr/testify/require"
)

// fakeDebugSession is a debugSession double so these tests exercise the
// tool's dispatch/validation/permission logic without launching a real
// dlv process.
type fakeDebugSession struct {
	stopped     *debugger.StoppedInfo
	exitCode    *int
	waitErr     error
	breakpoints []dap.Breakpoint
	bpErr       error
	frames      []dap.StackFrame
	scopes      []dap.Scope
	vars        map[int][]dap.Variable
	evalResult  dap.EvaluateResponseBody
	evalErr     error
	output      string
	closed      bool

	lastBreakpointFile  string
	lastBreakpointLines []int
	continued           bool
}

func (f *fakeDebugSession) WaitStopped(context.Context) (*debugger.StoppedInfo, *int, error) {
	if f.waitErr != nil {
		return nil, nil, f.waitErr
	}
	return f.stopped, f.exitCode, nil
}

func (f *fakeDebugSession) SetBreakpoints(_ context.Context, file string, lines []int) ([]dap.Breakpoint, error) {
	f.lastBreakpointFile, f.lastBreakpointLines = file, lines
	if f.bpErr != nil {
		return nil, f.bpErr
	}
	return f.breakpoints, nil
}

func (f *fakeDebugSession) Continue(context.Context) error { f.continued = true; return nil }
func (f *fakeDebugSession) Next(context.Context) error     { return nil }
func (f *fakeDebugSession) StepIn(context.Context) error   { return nil }
func (f *fakeDebugSession) StepOut(context.Context) error  { return nil }

func (f *fakeDebugSession) StackTrace(context.Context, int, int) ([]dap.StackFrame, error) {
	return f.frames, nil
}

func (f *fakeDebugSession) Scopes(context.Context, int) ([]dap.Scope, error) {
	return f.scopes, nil
}

func (f *fakeDebugSession) Variables(_ context.Context, ref int) ([]dap.Variable, error) {
	return f.vars[ref], nil
}

func (f *fakeDebugSession) Evaluate(context.Context, string, int) (dap.EvaluateResponseBody, error) {
	return f.evalResult, f.evalErr
}

func (f *fakeDebugSession) DrainOutput() string { out := f.output; f.output = ""; return out }
func (f *fakeDebugSession) Close()              { f.closed = true }

// fakeDebugSessions is the debugSessions double: one session per chat ID,
// created only via Start.
type fakeDebugSessions struct {
	sessions  map[string]*fakeDebugSession
	startErr  error
	nextStart *fakeDebugSession
}

func newFakeDebugSessions() *fakeDebugSessions {
	return &fakeDebugSessions{sessions: map[string]*fakeDebugSession{}}
}

func (f *fakeDebugSessions) Start(_ context.Context, id, _ string, _ []string) (debugSession, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	s := f.nextStart
	if s == nil {
		s = &fakeDebugSession{}
	}
	f.sessions[id] = s
	return s, nil
}

func (f *fakeDebugSessions) Get(id string) (debugSession, bool) {
	s, ok := f.sessions[id]
	if !ok {
		return nil, false
	}
	return s, true
}

func (f *fakeDebugSessions) Close(id string) {
	if s, ok := f.sessions[id]; ok {
		s.Close()
		delete(f.sessions, id)
	}
}

func runDebuggerTool(t *testing.T, sessions *fakeDebugSessions, perms permission.Service, params DebuggerParams) fantasy.ToolResponse {
	t.Helper()

	input, err := json.Marshal(params)
	require.NoError(t, err)

	ctx := context.WithValue(t.Context(), SessionIDContextKey, "test-session")
	resp, err := newDebuggerTool(perms, sessions).Run(ctx, fantasy.ToolCall{
		ID:    "test-call",
		Name:  DebuggerToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func TestDebuggerToolStartRequiresProgram(t *testing.T) {
	t.Parallel()
	resp := runDebuggerTool(t, newFakeDebugSessions(), &mockPermissionService{}, DebuggerParams{Action: "start"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "program is required")
}

func TestDebuggerToolStartReportsTheInitialStop(t *testing.T) {
	t.Parallel()
	sessions := newFakeDebugSessions()
	sessions.nextStart = &fakeDebugSession{stopped: &debugger.StoppedInfo{Reason: "entry", ThreadID: 1, File: "main.go", Line: 5, FuncName: "main.main"}}

	resp := runDebuggerTool(t, sessions, &mockPermissionService{}, DebuggerParams{Action: "start", Program: "./cmd/app"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "entry")
	require.Contains(t, resp.Content, "main.go:5")
}

func TestDebuggerToolStartReportsLaunchFailureAsAToolError(t *testing.T) {
	t.Parallel()
	sessions := newFakeDebugSessions()
	sessions.startErr = fmt.Errorf("dlv not found")

	resp := runDebuggerTool(t, sessions, &mockPermissionService{}, DebuggerParams{Action: "start", Program: "./cmd/app"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "failed to start debugger")
}

func TestDebuggerToolActionsBeforeStartFail(t *testing.T) {
	t.Parallel()
	resp := runDebuggerTool(t, newFakeDebugSessions(), &mockPermissionService{}, DebuggerParams{Action: "continue"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "no debug session is running")
}

func startedSession(t *testing.T) (*fakeDebugSessions, *fakeDebugSession) {
	t.Helper()
	sessions := newFakeDebugSessions()
	s := &fakeDebugSession{}
	sessions.sessions["test-session"] = s
	return sessions, s
}

func TestDebuggerToolBreakpointRequiresFile(t *testing.T) {
	t.Parallel()
	sessions, _ := startedSession(t)
	resp := runDebuggerTool(t, sessions, &mockPermissionService{}, DebuggerParams{Action: "breakpoint"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "file is required")
}

func TestDebuggerToolBreakpointSetsAndReportsVerification(t *testing.T) {
	t.Parallel()
	sessions, s := startedSession(t)
	s.breakpoints = []dap.Breakpoint{{Line: 8, Verified: true}, {Line: 20, Verified: false, Message: "unreachable"}}

	resp := runDebuggerTool(t, sessions, &mockPermissionService{}, DebuggerParams{Action: "breakpoint", File: "main.go", Lines: []int{8, 20}})
	require.False(t, resp.IsError)
	require.Equal(t, "main.go", s.lastBreakpointFile)
	require.Equal(t, []int{8, 20}, s.lastBreakpointLines)
	require.Contains(t, resp.Content, "line 8: verified")
	require.Contains(t, resp.Content, "line 20: not verified: unreachable")
}

func TestDebuggerToolContinueDrivesTheSessionAndReportsTheNextStop(t *testing.T) {
	t.Parallel()
	sessions, s := startedSession(t)
	s.stopped = &debugger.StoppedInfo{Reason: "breakpoint", ThreadID: 1, File: "main.go", Line: 8, FuncName: "main.main"}

	resp := runDebuggerTool(t, sessions, &mockPermissionService{}, DebuggerParams{Action: "continue"})
	require.False(t, resp.IsError)
	require.True(t, s.continued)
	require.Contains(t, resp.Content, "breakpoint")
	require.Contains(t, resp.Content, "main.go:8")
}

func TestDebuggerToolContinueReportsProgramExit(t *testing.T) {
	t.Parallel()
	sessions, s := startedSession(t)
	code := 0
	s.exitCode = &code

	resp := runDebuggerTool(t, sessions, &mockPermissionService{}, DebuggerParams{Action: "continue"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "exited with code 0")
}

func TestDebuggerToolStackReportsFrames(t *testing.T) {
	t.Parallel()
	sessions, s := startedSession(t)
	s.frames = []dap.StackFrame{
		{Name: "main.main", Line: 8, Source: &dap.Source{Path: "main.go"}},
		{Name: "runtime.main", Line: 200, Source: &dap.Source{Path: "runtime/proc.go"}},
	}

	resp := runDebuggerTool(t, sessions, &mockPermissionService{}, DebuggerParams{Action: "stack"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "0: main.main at main.go:8")
	require.Contains(t, resp.Content, "1: runtime.main at runtime/proc.go:200")
}

func TestDebuggerToolVariablesListsScopesByDefault(t *testing.T) {
	t.Parallel()
	sessions, s := startedSession(t)
	s.frames = []dap.StackFrame{{Id: 1}}
	s.scopes = []dap.Scope{{Name: "Locals", VariablesReference: 100}}
	s.vars = map[int][]dap.Variable{
		100: {{Name: "sum", Value: "0", Type: "int"}},
	}

	resp := runDebuggerTool(t, sessions, &mockPermissionService{}, DebuggerParams{Action: "variables"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Locals:")
	require.Contains(t, resp.Content, "sum = 0 (int)")
}

func TestDebuggerToolVariablesExpandsAReference(t *testing.T) {
	t.Parallel()
	sessions, s := startedSession(t)
	s.vars = map[int][]dap.Variable{
		42: {{Name: "Field", Value: "\"hi\"", Type: "string"}},
	}

	resp := runDebuggerTool(t, sessions, &mockPermissionService{}, DebuggerParams{Action: "variables", VariablesReference: 42})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Field = \"hi\" (string)")
	require.NotContains(t, resp.Content, "Locals:")
}

func TestDebuggerToolEvalRequiresExpression(t *testing.T) {
	t.Parallel()
	sessions, _ := startedSession(t)
	resp := runDebuggerTool(t, sessions, &mockPermissionService{}, DebuggerParams{Action: "eval"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "expression is required")
}

func TestDebuggerToolEvalReturnsTheResult(t *testing.T) {
	t.Parallel()
	sessions, s := startedSession(t)
	s.frames = []dap.StackFrame{{Id: 1}}
	s.evalResult = dap.EvaluateResponseBody{Result: "3", Type: "int"}

	resp := runDebuggerTool(t, sessions, &mockPermissionService{}, DebuggerParams{Action: "eval", Expression: "1+2"})
	require.False(t, resp.IsError)
	require.Equal(t, "3 (int)", resp.Content)
}

func TestDebuggerToolOutputReturnsDrainedOutput(t *testing.T) {
	t.Parallel()
	sessions, s := startedSession(t)
	s.output = "hello\n"

	resp := runDebuggerTool(t, sessions, &mockPermissionService{}, DebuggerParams{Action: "output"})
	require.False(t, resp.IsError)
	require.Equal(t, "hello\n", resp.Content)
}

func TestDebuggerToolStopClosesTheSession(t *testing.T) {
	t.Parallel()
	sessions, s := startedSession(t)

	resp := runDebuggerTool(t, sessions, &mockPermissionService{}, DebuggerParams{Action: "stop"})
	require.False(t, resp.IsError)
	require.True(t, s.closed)
	_, ok := sessions.Get("test-session")
	require.False(t, ok)
}

func TestDebuggerToolStopWithoutASessionIsANoop(t *testing.T) {
	t.Parallel()
	resp := runDebuggerTool(t, newFakeDebugSessions(), &mockPermissionService{}, DebuggerParams{Action: "stop"})
	require.False(t, resp.IsError)
}

func TestDebuggerToolRejectsUnknownAction(t *testing.T) {
	t.Parallel()
	resp := runDebuggerTool(t, newFakeDebugSessions(), &mockPermissionService{}, DebuggerParams{Action: "teleport"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "unknown action")
}

func TestDebuggerToolNeverStartsASessionWhenPermissionIsDenied(t *testing.T) {
	t.Parallel()
	sessions := newFakeDebugSessions()

	resp := runDebuggerTool(t, sessions, &denyingPermissionService{}, DebuggerParams{Action: "start", Program: "./cmd/app"})
	require.True(t, resp.IsError)
	require.Empty(t, sessions.sessions, "a denied call must not start a debug session")
}
