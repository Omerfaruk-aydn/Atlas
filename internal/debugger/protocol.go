// Package debugger drives a debug adapter (currently Delve, Go's debugger,
// via `dlv dap`) over the Debug Adapter Protocol, so the agent's debugger
// tool can launch a Go program under a real debugger, set breakpoints,
// step through execution, and inspect variables -- the kind of interactive
// debugging a test suite or a log statement can't replace.
package debugger

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/google/go-dap"
)

// Event is a debug-adapter-initiated notification the caller did not ask
// for directly: the program stopped (breakpoint, step, entry...), it
// exited, the session terminated, or it printed something.
type Event struct {
	Kind string // "stopped", "terminated", "exited", "output"

	// Populated when Kind == "stopped".
	Reason   string
	ThreadID int

	// Populated when Kind == "exited".
	ExitCode int

	// Populated when Kind == "output".
	Output string
}

// client is the low-level DAP RPC layer: request/response correlation by
// sequence number, plus an event stream for adapter-initiated messages.
// It knows nothing about Delve or process management -- see session.go --
// so it can be tested against an in-memory fake adapter instead of a real
// `dlv dap` process.
type client struct {
	rw io.ReadWriteCloser
	br *bufio.Reader

	mu      sync.Mutex
	seq     int
	pending map[int]chan dap.Message
	closed  bool

	events chan Event
}

func newClient(rw io.ReadWriteCloser) *client {
	c := &client{
		rw:      rw,
		br:      bufio.NewReader(rw),
		pending: map[int]chan dap.Message{},
		events:  make(chan Event, 64),
	}
	go c.readLoop()
	return c
}

// Events streams adapter-initiated notifications. It closes when the
// underlying connection does.
func (c *client) Events() <-chan Event {
	return c.events
}

func (c *client) readLoop() {
	defer close(c.events)
	for {
		msg, err := dap.ReadProtocolMessage(c.br)
		if err != nil {
			c.failPending()
			return
		}

		switch m := msg.(type) {
		case dap.ResponseMessage:
			resp := m.GetResponse()
			c.mu.Lock()
			ch, ok := c.pending[resp.RequestSeq]
			if ok {
				delete(c.pending, resp.RequestSeq)
			}
			c.mu.Unlock()
			if ok {
				ch <- msg
				close(ch)
			}
		case dap.EventMessage:
			c.dispatchEvent(m)
		}
	}
}

// failPending unblocks every in-flight request when the connection dies,
// so a caller waiting on a response gets "connection closed" instead of
// hanging forever.
func (c *client) failPending() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	for seq, ch := range c.pending {
		close(ch)
		delete(c.pending, seq)
	}
}

func (c *client) dispatchEvent(m dap.EventMessage) {
	var ev Event
	switch e := m.(type) {
	case *dap.StoppedEvent:
		ev = Event{Kind: "stopped", Reason: e.Body.Reason, ThreadID: e.Body.ThreadId}
	case *dap.TerminatedEvent:
		ev = Event{Kind: "terminated"}
	case *dap.ExitedEvent:
		ev = Event{Kind: "exited", ExitCode: e.Body.ExitCode}
	case *dap.OutputEvent:
		ev = Event{Kind: "output", Output: e.Body.Output}
	default:
		return
	}

	select {
	case c.events <- ev:
	default:
		// A slow consumer must not block the read loop -- drop the event
		// rather than stall protocol traffic. Stopped/terminated/exited
		// are rare enough this should never actually trigger; output can
		// be lost under flooding without harm.
	}
}

// send issues req, waits for its matching response, and JSON-decodes the
// response body into out (nil to discard it). req must be a pointer to a
// concrete go-dap request type (e.g. *dap.LaunchRequest) so its embedded
// Seq field can be set before sending.
func (c *client) send(ctx context.Context, req dap.RequestMessage, out any) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("debugger connection is closed")
	}
	c.seq++
	seq := c.seq
	base := req.GetRequest()
	base.Seq = seq
	base.Type = "request"
	ch := make(chan dap.Message, 1)
	c.pending[seq] = ch
	c.mu.Unlock()

	if err := dap.WriteProtocolMessage(c.rw, req); err != nil {
		c.mu.Lock()
		delete(c.pending, seq)
		c.mu.Unlock()
		return fmt.Errorf("failed to send %s request: %w", base.Command, err)
	}

	select {
	case msg, ok := <-ch:
		if !ok {
			return fmt.Errorf("debugger connection closed while waiting for %s response", base.Command)
		}
		resp := msg.(dap.ResponseMessage).GetResponse()
		if !resp.Success {
			return fmt.Errorf("%s failed: %s", resp.Command, errorDetail(msg, resp.Message))
		}
		if out == nil {
			return nil
		}
		b, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, out)
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, seq)
		c.mu.Unlock()
		return ctx.Err()
	}
}

// errorDetail prefers the adapter's structured error (body.error.format),
// which carries the actual reason (e.g. why a launch failed), over the
// generic top-level response message ("Failed to launch") every adapter
// sends alongside it.
func errorDetail(msg dap.Message, fallback string) string {
	er, ok := msg.(*dap.ErrorResponse)
	if !ok || er.Body.Error == nil || er.Body.Error.Format == "" {
		return fallback
	}
	return er.Body.Error.Format
}

func (c *client) Close() error {
	return c.rw.Close()
}
