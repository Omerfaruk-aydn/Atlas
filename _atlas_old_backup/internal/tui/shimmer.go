package tui

import (
	"math"
	"strings"
	"sync"
	"time"
)

// shimmerPhase is the current sweep position of a shimmer animation. The
// shared ticker (see sharedShimmerClock) broadcasts phase advances to
// every mounted shimmer; individual shimmers read the latest phase when
// they render.
type shimmerPhase struct {
	value int
	now   time.Time
}

// sharedShimmerClock fans phase updates out to every mounted shimmer via
// a single ticker. Hermes's same pattern: instead of N independent
// tickers (one per loading section, which scales with content), one
// ticker drives all of them and the per-row offset gives each a
// diagonal-sweep illusion.
type sharedShimmerClock struct {
	mu      sync.Mutex
	phase   int
	tickMS  int
	stop    chan struct{}
	listen  []chan shimmerPhase
	running bool
}

const shimmerBand = 7
const shimmerAnimateMS = 30000

// newSharedShimmerClock returns a clock that ticks every tickMS; mount
// with Start, subscribe with Subscribe, render with View.
func newSharedShimmerClock(tickMS int) *sharedShimmerClock {
	if tickMS <= 0 {
		tickMS = 90
	}
	return &sharedShimmerClock{
		tickMS: tickMS,
		stop:   make(chan struct{}),
	}
}

// Subscribe adds a new listener channel; the caller reads phase updates
// off it and stops reading via Unsubscribe.
func (c *sharedShimmerClock) Subscribe() chan shimmerPhase {
	ch := make(chan shimmerPhase, 1)
	c.mu.Lock()
	c.listen = append(c.listen, ch)
	c.mu.Unlock()
	return ch
}

// Unsubscribe removes a listener (e.g. on component unmount).
func (c *sharedShimmerClock) Unsubscribe(ch chan shimmerPhase) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, l := range c.listen {
		if l == ch {
			c.listen = append(c.listen[:i], c.listen[i+1:]...)
			close(ch)
			return
		}
	}
}

// Start begins broadcasting phase updates on the shared tick. No-op if
// already running.
func (c *sharedShimmerClock) Start() {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.mu.Unlock()
	go func() {
		t := time.NewTicker(time.Duration(c.tickMS) * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case now := <-t.C:
				c.mu.Lock()
				c.phase++
				phase := c.phase
				listen := append([]chan shimmerPhase(nil), c.listen...)
				c.mu.Unlock()
				update := shimmerPhase{value: phase, now: now}
				for _, ch := range listen {
					select {
					case ch <- update:
					default:
						// Listener slow / not reading — skip this tick
						// rather than block the broadcast.
					}
				}
			case <-c.stop:
				return
			}
		}
	}()
}

// Shimmer is one skeleton's render state. Construct with NewShimmer and
// pass through View(width) to render a single row.
type Shimmer struct {
	clock *sharedShimmerClock
	sub   chan shimmerPhase
	row   int // row index, used to offset the sweep so the loaders look diagonal
}

// NewShimmer constructs one loader row that subscribes to the shared
// ticker. row is the row index (0-based) used to phase-offset the band
// sweep.
func NewShimmer(clock *sharedShimmerClock, row int) *Shimmer {
	return &Shimmer{
		clock: clock,
		sub:   clock.Subscribe(),
		row:   row,
	}
}

// Close unsubscribes from the shared clock. Call on unmount.
func (s *Shimmer) Close() {
	if s.sub != nil {
		s.clock.Unsubscribe(s.sub)
		s.sub = nil
	}
}

// View returns a string of width w with the shimmer band sweeping
// across it. The band is a run of cells at the muted/accent boundary;
// outside the band the placeholder reads as a plain muted run.
func (s *Shimmer) View(width int, muted, accent func(string) string) string {
	if width <= 0 {
		return ""
	}
	if s.sub == nil {
		// No live clock — emit a static dimmed run so the row still
		// occupies the right number of cells.
		return muted(strings.Repeat("·", width))
	}
	var p shimmerPhase
	select {
	case p = <-s.sub:
	default:
		// No fresh tick yet — render a static frame rather than block.
	}
	// Sweep position in cells, derived from phase + per-row offset.
	// i*2 mirrors Hermes's diagonal-sweep effect.
	offset := (p.value + s.row*2)
	sweep := offset % (width + shimmerBand)
	// Build three segments: pre / band / post, with each "band" cell
	// at accent color, others at muted.
	pre := maxIntInt(sweep-shimmerBand, 0)
	midStart := sweep - shimmerBand
	if midStart < 0 {
		midStart = 0
	}
	midEnd := minIntInt(sweep, width)
	postStart := sweep
	if postStart > width {
		postStart = width
	}
	var b strings.Builder
	if pre > 0 {
		b.WriteString(muted(strings.Repeat("·", pre)))
	}
	if midEnd > midStart {
		b.WriteString(accent(strings.Repeat("·", midEnd-midStart)))
	}
	if postStart < width {
		b.WriteString(muted(strings.Repeat("·", width-postStart)))
	}
	return b.String()
}

func maxIntInt(a, b int) int { return int(math.Max(float64(a), float64(b))) }
func minIntInt(a, b int) int { return int(math.Min(float64(a), float64(b))) }
