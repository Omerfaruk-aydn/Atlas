package debugger

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/go-dap"
	"github.com/stretchr/testify/require"
)

// fakeAdapter is a minimal DAP peer for testing client without a real `dlv
// dap` process: it reads whatever request the client sends and lets the
// test decide how (or whether) to respond, and can push events
// unsolicited, exactly like a real adapter would.
type fakeAdapter struct {
	conn net.Conn
	br   *bufio.Reader
}

func newFakeAdapter(t *testing.T) (*client, *fakeAdapter) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})
	return newClient(clientConn), &fakeAdapter{conn: serverConn, br: bufio.NewReader(serverConn)}
}

func (f *fakeAdapter) recv(t *testing.T) dap.RequestMessage {
	t.Helper()
	msg, err := dap.ReadProtocolMessage(f.br)
	require.NoError(t, err)
	req, ok := msg.(dap.RequestMessage)
	require.True(t, ok, "expected a request message, got %T", msg)
	return req
}

func (f *fakeAdapter) respondSuccess(t *testing.T, req dap.RequestMessage, body any) {
	t.Helper()
	base := req.GetRequest()
	resp := &rawResponse{
		ProtocolMessage: dap.ProtocolMessage{Type: "response"},
		RequestSeq:      base.Seq,
		Success:         true,
		Command:         base.Command,
		Body:            body,
	}
	require.NoError(t, dap.WriteProtocolMessage(f.conn, resp))
}

func (f *fakeAdapter) respondFailure(t *testing.T, req dap.RequestMessage, message string) {
	t.Helper()
	base := req.GetRequest()
	resp := &rawResponse{
		ProtocolMessage: dap.ProtocolMessage{Type: "response"},
		RequestSeq:      base.Seq,
		Success:         false,
		Command:         base.Command,
		Message:         message,
	}
	require.NoError(t, dap.WriteProtocolMessage(f.conn, resp))
}

func (f *fakeAdapter) sendEvent(t *testing.T, ev dap.Message) {
	t.Helper()
	require.NoError(t, dap.WriteProtocolMessage(f.conn, ev))
}

// rawResponse implements dap.ResponseMessage with an arbitrary body, since
// go-dap's per-command response types (InitializeResponse, ...) each hard
// -code their own body type -- this lets the fake adapter answer any
// command generically.
type rawResponse struct {
	dap.ProtocolMessage
	RequestSeq int    `json:"request_seq"`
	Success    bool   `json:"success"`
	Command    string `json:"command"`
	Message    string `json:"message,omitempty"`
	Body       any    `json:"body,omitempty"`
}

func (r *rawResponse) GetResponse() *dap.Response {
	return &dap.Response{
		ProtocolMessage: r.ProtocolMessage,
		RequestSeq:      r.RequestSeq,
		Success:         r.Success,
		Command:         r.Command,
		Message:         r.Message,
	}
}

// newDAPEvent builds an Event base with Type set, since dap.DecodeMessage
// routes purely on the wire "type" field -- an Event left at its zero value
// (Type == "") fails to decode and the read loop treats that exactly like a
// closed connection.
func newDAPEvent(name string) dap.Event {
	return dap.Event{
		ProtocolMessage: dap.ProtocolMessage{Type: "event"},
		Event:           name,
	}
}

func newInitializeRequest() *dap.InitializeRequest {
	return &dap.InitializeRequest{
		Request:   dap.Request{Command: "initialize"},
		Arguments: dap.InitializeRequestArguments{AdapterID: "test"},
	}
}

func TestClientSendReceivesMatchingResponse(t *testing.T) {
	t.Parallel()
	c, adapter := newFakeAdapter(t)

	done := make(chan error, 1)
	var resp dap.InitializeResponse
	go func() {
		done <- c.send(t.Context(), newInitializeRequest(), &resp)
	}()

	req := adapter.recv(t)
	require.Equal(t, "initialize", req.GetRequest().Command)
	adapter.respondSuccess(t, req, dap.Capabilities{SupportsConfigurationDoneRequest: true})

	require.NoError(t, <-done)
	require.True(t, resp.Body.SupportsConfigurationDoneRequest)
}

func TestClientSendReturnsErrorOnUnsuccessfulResponse(t *testing.T) {
	t.Parallel()
	c, adapter := newFakeAdapter(t)

	done := make(chan error, 1)
	go func() {
		done <- c.send(t.Context(), newInitializeRequest(), nil)
	}()

	req := adapter.recv(t)
	adapter.respondFailure(t, req, "boom")

	err := <-done
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}

func TestClientSendPropagatesContextCancellation(t *testing.T) {
	t.Parallel()
	c, adapter := newFakeAdapter(t)

	// Drain the request so the write side of the (unbuffered) pipe doesn't
	// block forever -- the point of this test is a response that never
	// arrives, not a request that's never even read. Errors are ignored
	// (rather than asserted, which is unsafe from a non-test goroutine):
	// the pipe closing during t.Cleanup is an expected way for this to end.
	go func() { _, _ = dap.ReadProtocolMessage(adapter.br) }()

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()

	err := c.send(ctx, newInitializeRequest(), nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestClientFailsPendingRequestsWhenConnectionCloses(t *testing.T) {
	t.Parallel()
	c, adapter := newFakeAdapter(t)

	done := make(chan error, 1)
	go func() {
		done <- c.send(t.Context(), newInitializeRequest(), nil)
	}()

	adapter.recv(t) // make sure the request actually landed before closing
	adapter.conn.Close()

	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("send did not return after the connection closed")
	}
}

func TestClientDeliversStoppedEvent(t *testing.T) {
	t.Parallel()
	c, adapter := newFakeAdapter(t)

	adapter.sendEvent(t, &dap.StoppedEvent{
		Event: newDAPEvent("stopped"),
		Body:  dap.StoppedEventBody{Reason: "breakpoint", ThreadId: 1},
	})

	select {
	case ev := <-c.Events():
		require.Equal(t, "stopped", ev.Kind)
		require.Equal(t, "breakpoint", ev.Reason)
		require.Equal(t, 1, ev.ThreadID)
	case <-time.After(2 * time.Second):
		t.Fatal("stopped event was not delivered")
	}
}

func TestClientDeliversExitedEvent(t *testing.T) {
	t.Parallel()
	c, adapter := newFakeAdapter(t)

	adapter.sendEvent(t, &dap.ExitedEvent{
		Event: newDAPEvent("exited"),
		Body:  dap.ExitedEventBody{ExitCode: 3},
	})

	select {
	case ev := <-c.Events():
		require.Equal(t, "exited", ev.Kind)
		require.Equal(t, 3, ev.ExitCode)
	case <-time.After(2 * time.Second):
		t.Fatal("exited event was not delivered")
	}
}

func TestClientDeliversOutputEvent(t *testing.T) {
	t.Parallel()
	c, adapter := newFakeAdapter(t)

	adapter.sendEvent(t, &dap.OutputEvent{
		Event: newDAPEvent("output"),
		Body:  dap.OutputEventBody{Output: "hello from the debuggee\n"},
	})

	select {
	case ev := <-c.Events():
		require.Equal(t, "output", ev.Kind)
		require.Equal(t, "hello from the debuggee\n", ev.Output)
	case <-time.After(2 * time.Second):
		t.Fatal("output event was not delivered")
	}
}

func TestClientEventsChannelClosesWhenConnectionCloses(t *testing.T) {
	t.Parallel()
	c, adapter := newFakeAdapter(t)
	adapter.conn.Close()

	select {
	case _, ok := <-c.Events():
		require.False(t, ok, "Events() channel should close, not deliver a value")
	case <-time.After(2 * time.Second):
		t.Fatal("Events() channel did not close after the connection closed")
	}
}
