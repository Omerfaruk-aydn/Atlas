package tui

import (
	"regexp"
	"strings"
	"time"
)

// CompletionRequest is the input the pipeline decides what kind of
// completion to fetch. Three flavors, mirroring Hermes's
// useCompletion.completionRequestForInput:
//
//  1. complete.path: a file-path completion (broad regex matches
//     quoted/unquoted paths, `~/`, `../`, `@`-prefixed, drive letters).
//  2. complete.slash at position 0: full slash command.
//  3. complete.slash mid-prose with skillsOnly:true (handled
//     separately because built-ins like /model don't make sense
//     embedded in prose).
type CompletionKind int

const (
	CompletionPath CompletionKind = iota
	CompletionSlashPos0
	CompletionSlashInline
)

// CompletionRequest holds the decision.
type CompletionRequest struct {
	Kind CompletionKind
	// Filter is the substring to match against. For a path request
	// it's the typed path prefix; for a slash request it's the
	// command name fragment after the leading "/".
	Filter  string
	// ReplaceFrom is the buffer index from which a completion would
	// replace (Hermes calls this `replace_from`). For path/slash
	// completions it's the start of the path/token being completed.
	ReplaceFrom int
	// SkillsOnly is true for inline mid-prose slash completions.
	SkillsOnly bool
}

var (
	pathQuoteRe  = regexp.MustCompile(`(?://|^|[\s"'()\[\]{},;])((?:"[^"]+"|'[^']+'|\x60[^\x60]+\x60|~?/[^\s"'()\[\]{},;]*|~?\\[^\\]*|\.\.?/?[^\s"'()\[\]{},;]*|[A-Za-z]:\\[^\s"'()\[\]{},;]*))`)
	atRefRe       = regexp.MustCompile(`@(file|diff|staged)(?::[^\s]+|:\x60[^\x60]+\x60)?`)
	inlineSlashRe = regexp.MustCompile(`(?:^|[\s\n])/([A-Za-z][\w-]*)?$`)
	pos0SlashRe   = regexp.MustCompile(`^/([A-Za-z][\w-]*)?$`)
)

// completionRequestForInput decides which completion shape the current
// input warrants. Returns (CompletionRequest{}, false) when no
// completion applies.
func completionRequestForInput(input string, caret int) (CompletionRequest, bool) {
	if caret > len(input) {
		caret = len(input)
	}
	head := input[:caret]
	// 1. Inline mid-prose slash?
	if m := inlineSlashRe.FindStringIndex(head); m != nil {
		return CompletionRequest{
			Kind:        CompletionSlashInline,
			Filter:      head[m[1]-1:],
			ReplaceFrom: m[0],
			SkillsOnly:  true,
		}, true
	}
	// 2. Position-0 slash?
	if m := pos0SlashRe.FindStringIndex(head); m != nil {
		return CompletionRequest{
			Kind:        CompletionSlashPos0,
			Filter:      head[m[0]+1:],
			ReplaceFrom: 0,
			SkillsOnly:  false,
		}, true
	}
	// 3. File path?
	if m := pathQuoteRe.FindStringIndex(head); m != nil {
		return CompletionRequest{
			Kind:        CompletionPath,
			Filter:      head[m[0]:],
			ReplaceFrom: m[0],
		}, true
	}
	// 4. @-ref completion (without typing the colon).
	if at := strings.LastIndex(head, "@"); at >= 0 {
		tail := head[at:]
		if atRefRe.MatchString(tail) {
			return CompletionRequest{
				Kind:        CompletionPath,
				Filter:      tail,
				ReplaceFrom: at,
			}, true
		}
	}
	return CompletionRequest{}, false
}

// CompletionFlight is the staleness guard. Each in-flight completion
// RPC gets a monotonically increasing seq; the App.Update case compares
// the seq on the returned message to the current one and silently
// no-ops if a newer request has been issued.
//
// The Hermes pattern closes a real race: without the guard, a slow
// 60ms-debounced completion arriving after the user has typed more
// characters would clobber the up-to-date list with stale results.
type CompletionFlight struct {
	current int64
}

func (f *CompletionFlight) Next() int64 {
	f.current++
	return f.current
}

func (f *CompletionFlight) IsStale(seq int64) bool {
	return seq < f.current
}

// completionDebounce is the 60ms debounce window before firing the
// async request. The pipeline schedules a tea.Tick; the App.Update
// case's tick handler calls RunCompletion and produces a
// completionFilterMsg.
const completionDebounceMS = 60

// pendingCompletion is the in-flight state, kept on the App so a new
// keystroke can cancel the previous request cleanly.
type pendingCompletion struct {
	firedAt  time.Time
	seq      int64
	filter   string
	kind     CompletionKind
}

// CompletionPipeline owns the debounce + flight-counter state. The
// App calls Schedule on every keystroke; the pipeline returns either
// nil (no completion needed) or a tea.Cmd that, when fired, runs the
// async completion against the registry.
type CompletionPipeline struct {
	flight   CompletionFlight
	pending  *pendingCompletion
	debounce time.Duration
	clock    func() time.Time
}

func newCompletionPipeline() *CompletionPipeline {
	return &CompletionPipeline{
		debounce: completionDebounceMS * time.Millisecond,
		clock:    time.Now,
	}
}

// Schedule records a new completion request. Returns a tea.Cmd that
// fires after the debounce window; the App's Update case can decide
// whether to actually issue the RPC or discard if a newer keystroke
// has already overwritten pending.
func (p *CompletionPipeline) Schedule(input string, caret int) (cmd completionDebounceCmd, request CompletionRequest, ok bool) {
	req, ok := completionRequestForInput(input, caret)
	if !ok {
		p.pending = nil
		return completionDebounceCmd{}, CompletionRequest{}, false
	}
	seq := p.flight.Next()
	p.pending = &pendingCompletion{
		firedAt: p.clock(),
		seq:     seq,
		filter:  req.Filter,
		kind:    req.Kind,
	}
	return completionDebounceCmd{seq: seq, at: p.clock().Add(p.debounce)}, req, true
}

// IsStale returns true if a pending completion's seq is older than
// the pipeline's current one. The App.Update handler should silently
// drop the result when this is true.
func (p *CompletionPipeline) IsStale(seq int64) bool {
	return p.flight.IsStale(seq)
}

// completionDebounceCmd is the tea.Tick-style message the pipeline
// emits after the debounce window. The App routes it back into
// RunCompletion.
type completionDebounceCmd struct {
	seq int64
	at  time.Time
}

// RunCompletion is the App's entry point: when the debounce cmd
// fires, the App looks at p.pending and the seq, and if they're still
// current, runs the actual completion against the registry. Returns
// nil if the pending has been superseded.
func (p *CompletionPipeline) RunCompletion(seq int64) []FuzzyScoreItem {
	if p.pending == nil || p.pending.seq != seq {
		return nil
	}
	// For now, only slash completions have a backing store. The
	// other kinds would call into the file system / network — left
	// as future work.
	if p.pending.kind != CompletionSlashPos0 && p.pending.kind != CompletionSlashInline {
		return nil
	}
	return nil // the App does the actual fuzzy against its slash registry
}
