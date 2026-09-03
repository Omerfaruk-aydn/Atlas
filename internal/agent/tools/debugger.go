package tools

import (
	"context"
	_ "embed"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/debugger"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/permission"
	"github.com/google/go-dap"
)

const DebuggerToolName = "debugger"

//go:embed debugger.md
var debuggerDescription string

// waitStoppedTimeout bounds continue/next/step_in/step_out waiting for the
// program to stop again. Unlike the debugger package's per-request
// ActionTimeout (setBreakpoints, evaluate, a quick round trip), this waits
// on the debuggee actually doing something, which is legitimately slow --
// but still must not park a turn forever on a program that never hits
// another breakpoint and never exits.
const waitStoppedTimeout = 5 * time.Minute

var debuggerActions = []string{
	"start", "breakpoint", "continue", "next", "step_in", "step_out", "stack", "variables", "eval", "output", "stop",
}

// debuggerReadOnlyActions never change debugger or program state, only
// observe it -- Safe like bash's read-only allowlist and browser's
// text/html/screenshot/url.
var debuggerReadOnlyActions = map[string]bool{
	"stack":     true,
	"variables": true,
	"output":    true,
	"stop":      true,
}

type DebuggerParams struct {
	Action string `json:"action" description:"One of: start, breakpoint, continue, next, step_in, step_out, stack, variables, eval, output, stop. See the tool description for what each needs and returns."`
	// Program and Args are for start.
	Program string   `json:"program,omitempty" description:"Path to the Go program to debug, for the start action: a package directory (most common) or a single .go file."`
	Args    []string `json:"args,omitempty" description:"Command-line arguments to pass to the program, for the start action."`
	// File and Lines are for breakpoint.
	File  string `json:"file,omitempty" description:"Source file to set breakpoints in, for the breakpoint action."`
	Lines []int  `json:"lines,omitempty" description:"Line numbers to set breakpoints at, for the breakpoint action. Replaces any breakpoints already set in file; an empty list clears them."`
	// FrameIndex is for stack, variables, and eval.
	FrameIndex int `json:"frame_index,omitempty" description:"Which stack frame to use, for the stack, variables, and eval actions: 0 (the default) is the innermost frame where execution is currently stopped."`
	// VariablesReference is for variables.
	VariablesReference int `json:"variables_reference,omitempty" description:"Expand the children of one structured variable, for the variables action, using the ref number from a previous variables result -- instead of listing the current frame's top-level scopes."`
	// Expression is for eval.
	Expression string `json:"expression,omitempty" description:"Expression to evaluate in the program's current, paused context, for the eval action."`
}

type DebuggerPermissionsParams struct {
	Action             string   `json:"action"`
	Program            string   `json:"program,omitempty"`
	Args               []string `json:"args,omitempty"`
	File               string   `json:"file,omitempty"`
	Lines              []int    `json:"lines,omitempty"`
	FrameIndex         int      `json:"frame_index,omitempty"`
	VariablesReference int      `json:"variables_reference,omitempty"`
	Expression         string   `json:"expression,omitempty"`
}

type DebuggerResponseMetadata struct {
	Action string `json:"action"`
}

// debugSession is the subset of *debugger.Session the tool drives,
// structurally implemented by it, so tests can supply a fake instead of
// launching a real `dlv dap` process.
type debugSession interface {
	WaitStopped(ctx context.Context) (*debugger.StoppedInfo, *int, error)
	SetBreakpoints(ctx context.Context, file string, lines []int) ([]dap.Breakpoint, error)
	Continue(ctx context.Context) error
	Next(ctx context.Context) error
	StepIn(ctx context.Context) error
	StepOut(ctx context.Context) error
	StackTrace(ctx context.Context, threadID, levels int) ([]dap.StackFrame, error)
	Scopes(ctx context.Context, frameID int) ([]dap.Scope, error)
	Variables(ctx context.Context, variablesReference int) ([]dap.Variable, error)
	Evaluate(ctx context.Context, expression string, frameID int) (dap.EvaluateResponseBody, error)
	DrainOutput() string
	Close()
}

// debugSessions is the seam NewDebuggerTool depends on instead of
// *debugger.Manager[*debugger.Session] directly, so tests can supply a
// fake session without launching a real debugger.
type debugSessions interface {
	Start(ctx context.Context, id, program string, args []string) (debugSession, error)
	Get(id string) (debugSession, bool)
	Close(id string)
}

// liveDebugSessions adapts *debugger.Manager[*debugger.Session] (whose
// methods return the concrete *debugger.Session) to debugSessions (which
// deals in the debugSession interface) -- Go does not let a generic
// instantiation satisfy an interface with a covariant return type on its
// own.
type liveDebugSessions struct {
	m *debugger.Manager[*debugger.Session]
}

func (l liveDebugSessions) Start(ctx context.Context, id, program string, args []string) (debugSession, error) {
	return l.m.Start(ctx, id, program, args)
}

func (l liveDebugSessions) Get(id string) (debugSession, bool) {
	s, ok := l.m.Get(id)
	if !ok {
		return nil, false
	}
	return s, true
}

func (l liveDebugSessions) Close(id string) {
	l.m.Close(id)
}

func NewDebuggerTool(permissions permission.Service, cfg config.ToolDebugger) fantasy.AgentTool {
	manager := debugger.GetManager(debugger.Options{
		DlvPath:       cfg.DlvPath,
		ActionTimeout: cfg.GetActionTimeout(),
	})
	return newDebuggerTool(permissions, liveDebugSessions{m: manager})
}

func newDebuggerTool(permissions permission.Service, sessions debugSessions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		DebuggerToolName,
		debuggerDescription,
		func(ctx context.Context, params DebuggerParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			action := strings.ToLower(strings.TrimSpace(params.Action))
			if !slices.Contains(debuggerActions, action) {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("unknown action %q, must be one of: %s", params.Action, strings.Join(debuggerActions, ", "))), nil
			}

			chatSessionID := GetSessionFromContext(ctx)
			if chatSessionID == "" {
				return fantasy.ToolResponse{}, fmt.Errorf("session ID is required for using the debugger")
			}

			p, err := permissions.Request(
				ctx,
				permission.CreatePermissionRequest{
					SessionID:   chatSessionID,
					Path:        params.Program,
					ToolCallID:  call.ID,
					ToolName:    DebuggerToolName,
					Action:      action,
					Description: debuggerActionDescription(action, params),
					Params:      DebuggerPermissionsParams(params),
					Safe:        debuggerReadOnlyActions[action],
				},
			)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !p {
				return NewPermissionDeniedResponse(permissions), nil
			}

			if action == "stop" {
				sessions.Close(chatSessionID)
				return fantasy.NewTextResponse("Debug session closed."), nil
			}

			metadata := DebuggerResponseMetadata{Action: action}

			if action == "start" {
				if params.Program == "" {
					return fantasy.NewTextErrorResponse("program is required for the start action"), nil
				}
				sess, err := sessions.Start(ctx, chatSessionID, params.Program, params.Args)
				if err != nil {
					return fantasy.NewTextErrorResponse("failed to start debugger: " + err.Error()), nil
				}
				return waitAndDescribe(ctx, sess, metadata)
			}

			sess, ok := sessions.Get(chatSessionID)
			if !ok {
				return fantasy.NewTextErrorResponse("no debug session is running; use action \"start\" first"), nil
			}

			return runDebuggerAction(ctx, sess, action, params, metadata)
		},
	)
}

func debuggerActionDescription(action string, params DebuggerParams) string {
	switch action {
	case "start":
		return fmt.Sprintf("Start debugging: %s %s", params.Program, strings.Join(params.Args, " "))
	case "breakpoint":
		return fmt.Sprintf("Set breakpoints in %s: %v", params.File, params.Lines)
	case "eval":
		return "Evaluate in debugger: " + params.Expression
	case "stack", "variables", "output", "stop":
		return "Debugger " + action
	default:
		return "Debugger action: " + action
	}
}

func runDebuggerAction(ctx context.Context, sess debugSession, action string, params DebuggerParams, metadata DebuggerResponseMetadata) (fantasy.ToolResponse, error) {
	switch action {
	case "breakpoint":
		if params.File == "" {
			return fantasy.NewTextErrorResponse("file is required for the breakpoint action"), nil
		}
		bps, err := sess.SetBreakpoints(ctx, params.File, params.Lines)
		if err != nil {
			return fantasy.NewTextErrorResponse("setting breakpoints failed: " + err.Error()), nil
		}
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse(formatBreakpoints(bps)), metadata), nil

	case "continue":
		if err := sess.Continue(ctx); err != nil {
			return fantasy.NewTextErrorResponse("continue failed: " + err.Error()), nil
		}
		return waitAndDescribe(ctx, sess, metadata)

	case "next":
		if err := sess.Next(ctx); err != nil {
			return fantasy.NewTextErrorResponse("next failed: " + err.Error()), nil
		}
		return waitAndDescribe(ctx, sess, metadata)

	case "step_in":
		if err := sess.StepIn(ctx); err != nil {
			return fantasy.NewTextErrorResponse("step_in failed: " + err.Error()), nil
		}
		return waitAndDescribe(ctx, sess, metadata)

	case "step_out":
		if err := sess.StepOut(ctx); err != nil {
			return fantasy.NewTextErrorResponse("step_out failed: " + err.Error()), nil
		}
		return waitAndDescribe(ctx, sess, metadata)

	case "stack":
		frames, err := sess.StackTrace(ctx, 0, 0)
		if err != nil {
			return fantasy.NewTextErrorResponse("stack failed: " + err.Error()), nil
		}
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse(formatStack(frames)), metadata), nil

	case "variables":
		text, err := describeVariables(ctx, sess, params)
		if err != nil {
			return fantasy.NewTextErrorResponse("variables failed: " + err.Error()), nil
		}
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse(text), metadata), nil

	case "eval":
		if params.Expression == "" {
			return fantasy.NewTextErrorResponse("expression is required for the eval action"), nil
		}
		frameID, err := frameIDAt(ctx, sess, params.FrameIndex)
		if err != nil {
			return fantasy.NewTextErrorResponse("eval failed: " + err.Error()), nil
		}
		result, err := sess.Evaluate(ctx, params.Expression, frameID)
		if err != nil {
			return fantasy.NewTextErrorResponse("eval failed: " + err.Error()), nil
		}
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse(formatDebugValue(result.Result, result.Type, result.VariablesReference)), metadata), nil

	case "output":
		out := sess.DrainOutput()
		if out == "" {
			out = BashNoOutput
		}
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse(TruncateOutput(out)), metadata), nil

	default:
		return fantasy.NewTextErrorResponse("unknown action: " + action), nil
	}
}

// waitAndDescribe blocks for the debuggee's next stop/exit and formats it,
// bounding the wait so a program that never stops again cannot park the
// turn forever.
func waitAndDescribe(ctx context.Context, sess debugSession, metadata DebuggerResponseMetadata) (fantasy.ToolResponse, error) {
	waitCtx, cancel := context.WithTimeout(ctx, waitStoppedTimeout)
	defer cancel()

	stop, exitCode, err := sess.WaitStopped(waitCtx)
	if err != nil {
		return fantasy.NewTextErrorResponse("waiting for the debugger failed: " + err.Error()), nil
	}
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(formatStop(stop, exitCode)), metadata), nil
}

func formatStop(stop *debugger.StoppedInfo, exitCode *int) string {
	if exitCode != nil {
		return fmt.Sprintf("Program exited with code %d.", *exitCode)
	}
	if stop == nil {
		return "Debug session terminated."
	}
	loc := ""
	if stop.File != "" {
		loc = fmt.Sprintf(" at %s:%d (%s)", stop.File, stop.Line, stop.FuncName)
	}
	return fmt.Sprintf("Stopped (%s), thread %d%s.", stop.Reason, stop.ThreadID, loc)
}

func formatBreakpoints(bps []dap.Breakpoint) string {
	if len(bps) == 0 {
		return "No breakpoints set."
	}
	var b strings.Builder
	for _, bp := range bps {
		status := "verified"
		if !bp.Verified {
			status = "not verified"
			if bp.Message != "" {
				status += ": " + bp.Message
			}
		}
		fmt.Fprintf(&b, "line %d: %s\n", bp.Line, status)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatStack(frames []dap.StackFrame) string {
	if len(frames) == 0 {
		return "No stack frames (is the program stopped?)."
	}
	var b strings.Builder
	for i, f := range frames {
		loc := ""
		if f.Source != nil {
			loc = fmt.Sprintf(" at %s:%d", f.Source.Path, f.Line)
		}
		fmt.Fprintf(&b, "%d: %s%s\n", i, f.Name, loc)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatDebugValue(value, typ string, variablesReference int) string {
	s := value
	if typ != "" {
		s += " (" + typ + ")"
	}
	if variablesReference != 0 {
		s += " [ref=" + strconv.Itoa(variablesReference) + "]"
	}
	return s
}

// frameIDAt resolves a stack frame index (0 == innermost) to the frame ID
// the DAP scopes/evaluate requests need.
func frameIDAt(ctx context.Context, sess debugSession, index int) (int, error) {
	frames, err := sess.StackTrace(ctx, 0, index+1)
	if err != nil {
		return 0, err
	}
	if index >= len(frames) {
		return 0, fmt.Errorf("frame_index %d is out of range (%d frame(s) available)", index, len(frames))
	}
	return frames[index].Id, nil
}

// describeVariables lists either the children of one variable
// (variables_reference set) or every scope's top-level variables for a
// stack frame (the default).
func describeVariables(ctx context.Context, sess debugSession, params DebuggerParams) (string, error) {
	if params.VariablesReference != 0 {
		vars, err := sess.Variables(ctx, params.VariablesReference)
		if err != nil {
			return "", err
		}
		return formatVariableList(vars), nil
	}

	frameID, err := frameIDAt(ctx, sess, params.FrameIndex)
	if err != nil {
		return "", err
	}
	scopes, err := sess.Scopes(ctx, frameID)
	if err != nil {
		return "", err
	}
	if len(scopes) == 0 {
		return "No variable scopes for this frame.", nil
	}

	var b strings.Builder
	for _, scope := range scopes {
		fmt.Fprintf(&b, "%s:\n", scope.Name)
		vars, err := sess.Variables(ctx, scope.VariablesReference)
		if err != nil {
			return "", fmt.Errorf("scope %q: %w", scope.Name, err)
		}
		if len(vars) == 0 {
			b.WriteString("  (none)\n")
			continue
		}
		for _, v := range vars {
			fmt.Fprintf(&b, "  %s = %s\n", v.Name, formatDebugValue(v.Value, v.Type, v.VariablesReference))
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func formatVariableList(vars []dap.Variable) string {
	if len(vars) == 0 {
		return "(no children)"
	}
	var b strings.Builder
	for _, v := range vars {
		fmt.Fprintf(&b, "%s = %s\n", v.Name, formatDebugValue(v.Value, v.Type, v.VariablesReference))
	}
	return strings.TrimRight(b.String(), "\n")
}
