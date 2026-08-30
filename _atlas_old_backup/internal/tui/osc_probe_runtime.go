package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

// OscProbeRequest is the runtime that the App uses to actually issue
// the OSC 11/10 probe. The Hermes pattern: write the probe to stdout
// before entering the Bubbletea input loop, then read the reply off
// stdin in a raw pre-read goroutine matching against the expected
// reply grammar (the parser is in osc_probe.go).
//
// Atlas wires the probe at startup; the result lands as a
// backgroundDetectMsg tea.Msg the App's Update case consumes. The
// env-driven cascade (HERMES_TUI_BACKGROUND) still wins if no
// answer arrives within the timeout.
type OscProbeRequest struct {
	out     io.Writer
	in      io.Reader
	timeout time.Duration
	done    chan struct{}
	result  chan LightMode
}

// RunOscProbe issues the OSC 11 + 10 probes and returns the resolved
// LightMode. Safe to call once at startup; subsequent calls are
// no-ops (the first answer wins, mirroring Hermes's "first-writer-
// wins" reportedColorSlot).
func RunOscProbe(out io.Writer, in io.Reader, timeout time.Duration) LightMode {
	if out == nil {
		out = os.Stdout
	}
	if in == nil {
		in = os.Stdin
	}
	if timeout <= 0 {
		timeout = 200 * time.Millisecond
	}
	req := &OscProbeRequest{
		out:     out,
		in:      in,
		timeout: timeout,
		done:    make(chan struct{}),
		result:  make(chan LightMode, 1),
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// Write the probes. The terminal will answer in order; OSC 11
	// first (background), then OSC 10 (foreground).
	_, _ = fmt.Fprint(out, oscBgProbe)
	go drainAndReply(req, in, ctx)
	select {
	case mode := <-req.result:
		return mode
	case <-ctx.Done():
		return LightUnknown
	}
}

// drainAndReply reads from in until ctx is done or a parsed reply
// arrives, then sends the LightMode on req.result. Best-effort: any
// read error is swallowed (we're at startup, the user hasn't even
// typed yet).
func drainAndReply(req *OscProbeRequest, in io.Reader, ctx context.Context) {
	defer close(req.done)
	bgHex := ""
	reader := make(chan []byte, 8)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := in.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				select {
				case reader <- chunk:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	// Accumulate the reply across multiple reads. The terminal
	// emits the reply terminated by BEL or ST.
	acc := []byte{}
	for {
		select {
		case <-ctx.Done():
			if bgHex != "" {
				select {
				case req.result <- backgroundHexToLight(bgHex):
				default:
				}
			}
			return
		case chunk := <-reader:
			acc = append(acc, chunk...)
			// Look for an OSC 11 reply: "\x1b]11;...BEL or ST".
			line := string(acc)
			if r, g, b, ok := parseOscReply(line); ok {
				_ = r
				_ = g
				_ = b
				// Build the hex from the parsed RGB so the
				// existing backgroundHexToLight gets called.
				bgHex = fmt.Sprintf("#%02x%02x%02x", r, g, b)
				select {
				case req.result <- backgroundHexToLight(bgHex):
				default:
				}
				return
			}
		}
	}
}

// probeStartup is the App's Init helper. It returns a tea.Cmd that
// fires once with the resolved LightMode (or LightUnknown on
// timeout) so the App can update its theme.
func probeStartup() func() LightMode {
	mode := RunOscProbe(os.Stdout, os.Stdin, 200*time.Millisecond)
	return func() LightMode { return mode }
}
