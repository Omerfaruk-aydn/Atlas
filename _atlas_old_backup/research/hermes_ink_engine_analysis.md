# Hermes-ink engine internals — analysis for Atlas (Go/Bubbletea)

Scope: the "remaining hermes-ink" slice — the custom Ink reconciler/layout/event-system
internals that a prior scoping pass tried to skip as "not portable." Read in full per
explicit instruction. Source: `NousResearch/hermes-agent`,
`ui-tui/packages/hermes-ink/src/{ink,native-ts,bootstrap,hooks}`, plus root `.d.ts` files
and `entry-exports.ts`. ~94 files, MIT license, design patterns only (no code copied).

**Headline finding**: this slice is overwhelmingly a from-scratch reimplementation of a
browser-style rendering stack for the terminal — a Yoga flexbox port, a React reconciler
host config, a DOM-like event system (capture/bubble, focus manager), and a screen-buffer
diff/paint engine. Bubbletea's own `Update`/`View`/`Cmd` loop replaces essentially the
entire reconciler+renderer+DOM stack; Lipgloss replaces the flexbox+border+color layer.
Most files below get a one-line verdict. A handful contain genuinely portable ideas —
those get real depth, called out explicitly.

---

## ink/layout/ (4 files)

- **node.ts** — `LayoutNode` interface: the full Yoga API surface (flex props, edges,
  measure funcs) abstracted behind an interface so the underlying implementation is
  swappable. Pure plumbing.
- **geometry.ts** — `Point`/`Size`/`Rectangle`/`Edges` value types + `unionRect`,
  `clampRect`, `withinBounds`. Trivial, portable in spirit but not worth translating —
  Atlas doesn't do a two-pass flex layout so it doesn't need a rectangle-union damage
  model at this layer (Lipgloss handles box sizing directly).
- **yoga.ts** — one-line factory (`createLayoutNode` → `createYogaLayoutNode`).
- **engine.ts** — `YogaLayoutNode`: adapter class mapping the `LayoutNode` interface onto
  the actual Yoga TS port (native-ts/yoga-layout). Pure adapter, no algorithm.

**Verdict**: full flexbox layout engine, no Bubbletea equivalent needed — Lipgloss's
box model (fixed width/height + Join functions) already covers Atlas's actual layout
needs without a two-pass constraint solver.

## ink/events/ (14 files) — DOM-style event system

This is the one subsystem worth reading as a *catalog of event types Atlas might be
missing*, even though the dispatch mechanism itself (capture/bubble tree walk, React
priority lanes) has no Bubbletea equivalent — `tea.Msg` + `Update` already is Atlas's
event system.

- **event.ts** — base `Event` class: just `stopImmediatePropagation()`.
- **terminal-event.ts** — `TerminalEvent extends Event`: adds `target`/`currentTarget`/
  `eventPhase`/`bubbles`/`cancelable`/`preventDefault()`/`stopPropagation()` — a full
  DOM `Event` reimplementation, plus an `EventTarget` type (`parentNode` + handler map)
  so plain DOM nodes can act as dispatch targets.
- **dispatcher.ts** — `Dispatcher.dispatch()`: walks target→root collecting capture
  handlers (unshift, root-first) and bubble handlers (push, target-first), then runs
  them in order with per-node `stopPropagation`/`stopImmediatePropagation` checks. Also
  maps event type → React scheduler priority (discrete for keyboard/click/focus/paste,
  continuous for resize/scroll/mousemove) — this priority-lane concept is React-specific
  and has no Bubbletea analogue (Bubbletea has no update-priority concept; every `Msg`
  is handled synchronously in `Update`).
- **event-handlers.ts** — static maps: event type → prop name (`onKeyDown`/
  `onKeyDownCapture` etc.) and the full set of recognized handler prop names, used by
  the reconciler to route JSX props into `_eventHandlers` instead of DOM attributes.
- **emitter.ts** — a `NodeEventEmitter` subclass whose `emit()` respects
  `stopImmediatePropagation()` on `Event`-typed payloads, and disables the 10-listener
  warning (many components legitimately share one input stream).
- **click-event.ts** / **mouse-event.ts** — `col`/`row` (absolute), `localCol`/`localRow`
  (recomputed per-handler, relative to *that* handler's box — not the leaf that was
  actually clicked), `cellIsBlank` (lets a handler ignore clicks on unwritten cells to
  the right of text), and for mouse events a raw `button` code.
- **keyboard-event.ts** — normalizes to browser semantics: `key` is the literal char for
  printable keys, a name string for special keys (`'down'`, `'return'`); the idiomatic
  check is `key.length === 1`. Derives `key` from ctrl-byte vs printable-char vs named
  special key.
- **input-event.ts** — the composer-facing shape (`Key` flags: arrows, `pageUp/Down`,
  `wheelUp/Down`, `home/end`, `ctrl/shift/fn/meta/super`, plus a resolved `input` string).
  Contains the gnarliest logic in the events dir: unwinds CSI-u (Kitty), xterm
  modifyOtherKeys, and application-keypad escape sequences back into a clean
  `{key, input}` pair, with explicit handling for Vietnamese-IME-style fused control+text
  bytes and unmapped Kitty functional keycodes (swallowed rather than leaked as garbage
  text). This logic actually lives in `parse-keypress.ts`; `input-event.ts` is the
  event-object wrapper around it.
- **focus-event.ts** — `FocusEvent extends TerminalEvent`, `'focus'|'blur'`, carries
  `relatedTarget` (previous/next focused node), always bubbles, never cancelable.
  **Gap for Atlas**: Bubbletea has no focus-event concept at all today (single global
  Update loop, "focus" is whatever bookkeeping the app does itself). If Atlas ever grows
  multiple focusable widgets (multi-pane layout, tab-through form fields), a lightweight
  `tea.Msg`-based `FocusMsg{Target, Related}` / `BlurMsg{...}` pair modeled on this would
  give consistent focus-change notification instead of ad hoc bools threaded through
  every component.
- **paste-event.ts** — `PasteEvent extends TerminalEvent`, carries the full pasted
  `text` as one event (not per-character). **Gap for Atlas**: this is the most concrete,
  actionable gap in the whole slice. Atlas currently has "no bracketed-paste-specific
  handling, relies on whatever bubbletea/x/term provides by default" — meaning a paste
  of N characters likely arrives as N separate `KeyMsg`s (or worse, gets partially
  misinterpreted if the pasted text contains control bytes that look like escape
  sequences). Ink's model — recognize the bracketed-paste start/end markers (CSI 200~ /
  201~) at the tokenizer layer, buffer everything between them, and emit ONE event with
  the literal text — is directly portable and meaningfully improves both correctness
  (no accidental keybinding triggers from pasted text containing e.g. arrow-key-looking
  bytes) and performance (one state update instead of N). See `parse-keypress.ts` notes
  below for the exact buffering algorithm.
- **resize-event.ts** — `ResizeEvent extends TerminalEvent` carrying `columns`/`rows`.
  Bubbletea already has `tea.WindowSizeMsg` — no gap.
- **terminal-focus-event.ts** — `TerminalFocusEvent`, `'terminalfocus'|'terminalblur'`,
  driven by DECSET 1004 terminal-focus-reporting (CSI I / CSI O — the *terminal window*
  gaining/losing OS-level focus, distinct from in-app focus above). **Gap for Atlas**:
  genuinely missing capability. Enabling DECSET 1004 and parsing CSI I/O lets Atlas pause
  expensive animations/spinners when the terminal window itself is unfocused (backgrounded
  tab/window), and is a prerequisite for the tab-status/title features some agent CLIs
  use to signal "done" only when the user isn't looking. Cheap to add: one DEC private
  mode enable/disable pair + two new escape patterns in the input parser.
- **cmd-shortcuts.test.ts** (skim) — locks in modifier-parsing behavior: Shift/Ctrl+Enter
  via CSI-u and modifyOtherKeys, Kitty "super" (Cmd) preserved as a distinct modifier
  from "meta" (Alt/Option), and the specific VS Code/Cursor forwarded-Cmd+C sequence.

**Verdict for events/ as a whole**: the capture/bubble dispatch mechanism itself is
React-DOM plumbing with no reason to exist in Atlas. But the **event taxonomy** — focus,
paste, terminal-focus, resize, mouse (click/down/up/drag/enter/leave), keyboard — is a
useful checklist. Atlas has resize and (already, via bubbletea) keyboard; it is missing
paste-as-one-event, in-app focus/blur, terminal-window-focus, and any mouse events.

## ink/hooks/ (14 files)

All are React hooks wrapping context lookups into the `Ink` instance — no algorithmic
content, purely "React idiom for reaching into the renderer singleton." One exception:

- **use-terminal-viewport.ts** — computes whether an element is currently scrolled into
  view by walking the DOM parent chain (not the yoga parent chain — deliberately, to
  pick up `scrollTop` on intermediate `ScrollBox` containers) and comparing against a
  viewport window whose top additionally accounts for a "cursor-restore scroll" fudge
  factor (+1 row) that has to exactly match the same fudge factor in `log-update.ts`'s
  diff engine, or animations get incorrectly paused/resumed at the viewport boundary.
  This is a real technique (visibility-based animation throttling tied precisely to the
  paint engine's own scroll math) but it's only meaningful because Ink's renderer tracks
  scroll position itself; Atlas's `bubbles/viewport` component already knows its own
  visible range natively, so there's nothing to port — just confirms Atlas's viewport
  component is the right abstraction to hang "pause offscreen animations" logic off of.
- **use-declared-cursor.ts** — lets a component *declare* where the physical terminal
  cursor should park after each frame (so IME preedit and screen-reader/magnifier focus
  track the real caret instead of Ink's own virtual cursor). Real technique, not
  applicable to Atlas today (Bubbletea doesn't offer this level of cursor control, and
  Atlas likely doesn't need IME composition support), but worth remembering if CJK input
  or accessibility ever becomes a requirement.
- Rest (use-animation-frame, use-app, use-cursor-advance, use-external-process,
  use-input, use-interval, use-search-highlight, use-selection, use-stdin,
  use-tab-status, use-terminal-focus, use-terminal-title): thin context-read wrappers.
  No Bubbletea equivalent needed — Bubbletea's model/update already gives every
  component full access to whatever state it needs without a context-lookup indirection.

## ink/ top-level (53 files)

### High-value, read in full

**log-update.ts** (753 lines) — the core screen-diff/repaint algorithm. This is the
piece to study if Atlas ever wants tighter terminal-repaint control than
`bubbletea`'s own renderer gives:
- Diffs previous vs. next `Frame` (a `Screen` cell buffer + cursor + viewport) and emits
  a `Diff` (list of typed `Patch`es: `stdout`, `clear`, `clearTerminal`, `cursorMove`,
  `cursorTo`, `styleStr`, `hyperlink`, `carriageReturn`, cursor show/hide).
- **Never absolute-positions the cursor except CSI H on alt-screen.** All movement is
  *relative* (`cursorMove(dx, dy)`), because the terminal's scrollback offset is
  unknowable in main-screen mode. It tracks a virtual cursor (`VirtualScreen`) purely to
  compute the deltas.
  variable-height content in a fixed-size window.
- Full-reset detection: it proactively falls back to a full repaint
  (`fullResetSequence_CAUSES_FLICKER`) whenever a change would require touching
  scrollback-only rows it can't seek back to, whenever the terminal resizes, or whenever
  shrinking content would need to "un-clear" now-visible former-scrollback rows —
  eraseLines/clear can't do that, only a full redraw can.
- **DECSTBM scroll-region optimization** (alt-screen only): when only a `ScrollBox`'s
  `scrollTop` changed and the region doesn't touch the terminal's last row, it issues a
  hardware scroll (`CSI top;bot r` + `CSI n S/T`) instead of rewriting every visible row.
  This is a genuinely portable, high-value technique: **a scrolled list rendered via
  hardware scroll-region + SU/SD costs O(new rows) bytes instead of O(viewport) bytes**,
  and is instant instead of causing full-frame flicker. Atlas's `bubbles/viewport`
  currently just redraws the whole content string on every scroll step; wiring DECSTBM
  underneath a Bubbletea custom renderer (or writing raw ANSI around Bubbletea's output
  for a scroll-heavy view) would be the single most impactful perf win in this file for
  Atlas, if scroll performance on long conversation transcripts ever becomes a problem.
- Space-skipping optimizations: unstyled trailing spaces are never written (cursor
  advance suffices); a run of foreground-only-styled spaces matching the *previous*
  cell's style is skipped too (`visibleCellAtIndex`'s `lastRenderedStyleId` param) —
  avoids writing an SGR sequence purely to draw invisible whitespace.
- Wide-character/emoji edge cases: never writes a 2-cell-wide glyph that would cross the
  viewport edge (checked per-grapheme-length since flag/ZWJ emoji need a stricter
  boundary than plain CJK); and a `needsWidthCompensation()` heuristic detects
  emoji/symbols whose *terminal* wcwidth table may disagree with Unicode (Unicode
  12.0+ pictographs, and text-default emoji + VS16) and defensively pre-paints a blank
  cell at the second column via CHA so old terminals with stale wcwidth tables don't
  visually collide the next glyph into the emoji's second cell.

**parse-keypress.ts** (918 lines) — keyboard/mouse/response parsing, read in full. Key
edge cases handled, several directly relevant to Atlas regardless of renderer:
- **Kitty keyboard protocol (CSI u)** and **xterm `modifyOtherKeys` (CSI 27;mod;code~)**
  both decoded via the same `decodeModifier()` bitmask (`shift=1, meta=2, ctrl=4,
  super=8`), letting the parser distinguish **super (Cmd/Win) from meta (Alt/Option)** —
  something legacy VT100-style escape sequences fundamentally cannot express. If Atlas
  wants Cmd-based shortcuts (Cmd+C, Cmd+Backspace-as-word-delete) to work distinctly
  from Option/Alt-based ones, it needs at minimum to opt into one of these two protocols
  and decode this bitmask — legacy ESC-sequence parsing alone cannot see `super` at all.
- **Bracketed paste**: tokenizer recognizes `CSI 200~`/`CSI 201~` start/end markers;
  everything between is buffered as literal text (even embedded escape sequences are
  NOT reinterpreted inside a paste) and emitted as one `isPasted: true` key event on the
  END marker — even for an *empty* paste (so e.g. macOS clipboard-image paste handling
  can detect it). A watchdog flush (`isFlush`) recovers if the terminal ever drops the
  end marker, so the whole session doesn't get stuck treating all future input as
  paste content. **This is the single most concretely portable piece in the entire
  file set** — Atlas's own docstring already flags "no bracketed-paste-specific
  handling"; this is the exact state machine to replicate on top of whatever raw byte
  stream `bubbletea`/`x/term` hands over.
- **Mixed control+text token splitting** (`isControlChar`/`parseTextKeypresses`): some
  IMEs (Vietnamese Telex via OpenKey/Unikey/EVKey) emit a stray erase byte fused with
  the finished composed character in a single read, e.g. `"\x7fô"`. The parser splits
  each embedded control byte (except CR/LF, deliberately preserved for
  paste/Enter semantics) into its own keypress so the printable runs around it survive
  instead of the whole token being discarded. A narrow but real correctness bug class
  Atlas should be aware exists if it ever gets IME-input bug reports.
- SGR mouse (`CSI < btn;col;row M/m`) and legacy X10 mouse (`CSI M` + 3 raw bytes) both
  parsed; wheel events stay in the keyboard-event lane (`wheelup`/`wheeldown` "keys") so
  existing keybinding infrastructure can bind scroll without a separate mouse-event
  path — a nice practical pattern (treat wheel as a synthetic key, not a mouse event)
  if Atlas ever adds any mouse support at all and wants to reuse its existing keymap
  system for scroll.
- Terminal **response** parsing (DECRPM, DA1/DA2, Kitty-flags query reply, cursor
  position report, generic OSC reply, XTVERSION) is syntactically disambiguated from
  keypresses inline in the same tokenizer — see `terminal-querier.ts` below for why
  this matters.

**hit-test.ts** — mouse-to-terminal-cell hit-testing. `hitTest()` walks the DOM tree
using a `nodeCache` populated by the paint pass (post-layout screen-space rects,
including scroll offset) and returns the deepest node containing `(col, row)`,
traversing children in *reverse* order so later-painted siblings (which visually sit on
top) win — the exact z-order semantics a mouse hit-test needs and a naive first-match
tree walk would get backwards. `dispatchClick` also handles click-to-focus (walks up
from the hit node to the nearest `tabIndex`-bearing ancestor) and per-handler coordinate
translation (`localCol`/`localRow` recomputed relative to whichever ancestor's handler
is currently firing, not the original leaf). `dispatchHover` implements proper
mouseenter/mouseleave semantics (non-bubbling, diffed against the previous hovered set
so moving between children of the same hover-region doesn't refire). **This is the
concrete algorithm Atlas would need if it ever adds mouse support** — the technique
(cache each rendered node's screen rect during paint, then do an O(depth) tree descent
per click rather than a linear scan) is the standard, correct approach and is directly
transferable regardless of language.

**ink.tsx** (2840 lines, read in full) — the top-level `Ink` class: owns the render
loop, alt-screen lifecycle, selection state, search highlight, cursor-declaration
resolution, and process-exit cleanup. Almost all of it is deeply coupled to the
React-reconciler-driven double-buffered-frame model and has no Bubbletea translation
(Bubbletea's own `Update`/`View`/tea.Cmd loop *is* this file, architecturally). Two
techniques worth lifting out on their own:
- **Backpressure coalescing** (`onRender`, `pendingWriteStart`/`coalescedBackpressureFrames`):
  when the previous `stdout.write()` hasn't drained yet (`write()` returned false and
  the drain callback hasn't fired), skip rendering entirely and retry on a short timer,
  rather than queuing more writes behind an already-backed-up pipe. Capped at
  `MAX_COALESCED_BACKPRESSURE_FRAMES` (10) so a terminal whose drain callback never
  fires (stuck pipe) can't wedge the app forever — it force-writes through after the
  ceiling. **Directly portable**: if Atlas ever renders fast/large diffs to a slow
  terminal (SSH, tmux, wide CJK-heavy output), the same technique (check the last
  write's completion before starting a new one, drop/coalesce frames rather than queue)
  avoids exactly the failure mode Go's `os.Stdout.Write` can also hit under a full pipe.
- **Physical-cursor drift self-healing** (`ALT_SCREEN_ANCHOR_CURSOR`, the CSI H
  preamble): rather than trusting that the terminal's physical cursor is where the app
  last left it, every alt-screen frame starts with an absolute `CSI H` and *recomputes*
  from a known-zero baseline — defends against any out-of-band cursor movement (tmux
  status-bar redraw, a stray write from a misbehaving library) silently accumulating
  drift frame over frame. Worth remembering as a general "don't trust cumulative
  relative state across renders you don't fully control" pattern, though it's really
  just restating a defensive-programming principle in this specific domain.

### Moderate-value / confirm-and-skip

- **screen.ts** (1591 lines, read ~700) — the cell buffer. Central technique: cells are
  packed as 2×Int32 per cell in one contiguous `Int32Array`/`BigInt64Array` (not
  objects), avoiding GC pressure (a 200×120 screen would otherwise allocate 24,000
  objects/frame); strings (chars, hyperlinks) and ANSI style runs are interned into
  shared pools (`CharPool`/`HyperlinkPool`/`StylePool`) so cells store small integer IDs
  and equality/diffing is integer comparison, not string comparison; `StylePool.intern`
  further steals one bit of the ID itself to flag "has a visible effect on space
  characters" so the render loop can skip invisible styled-space cells with a single
  bitmask check instead of a lookup. **This is a legitimate high-performance-terminal-
  renderer pattern** (pack cells, intern strings/styles, bit-pack metadata into the ID)
  that would matter for Atlas only if profiling ever showed cell-buffer allocation or
  string-diffing as a bottleneck — Go's GC and string interning story are different
  enough (Go strings are already interned-ish via the runtime, and Bubbletea doesn't
  expose a raw cell buffer at all) that this isn't a drop-in port, but the *shape* of
  the optimization (avoid per-cell object allocation, avoid per-cell string comparison)
  is the right idea if Atlas ever writes a custom low-level renderer.
- **selection.ts** (1143 lines, read ~200 + skimmed test) — mouse-drag text-selection
  state machine for alt-screen mode: anchor/focus model (not start/end — normalized at
  render time), word/line/char selection modes via an iTerm2-compatible word-character
  class (`[\p{L}\p{N}_/.\-+~\\]`) so double-click on a path selects the whole path, and
  an accumulator (`scrolledOffAbove`/`Below`) that captures text from rows about to
  scroll out of the viewport *before* the scroll happens, so a selection dragged past
  the visible edge during auto-scroll doesn't lose the part that's already scrolled off
  screen. No current relevance to Atlas (no mouse support), but is the reference
  implementation to copy from if Atlas ever adds click-drag text selection.
- **render-node-to-output.ts** (1888 lines, read ~180 + structural skim) — the
  recursive DOM→Output tree walk (Atlas's rough equivalent of "build the View() string
  from the model", except operating on a pre-computed Yoga layout tree with clip
  regions, blit-from-previous-frame optimization, and absolute-position overlay
  handling). Also owns the **scroll-drain easing curves**: a proportional
  catch-up curve for native-terminal scroll bursts (drains ~3/4 of pending distance per
  frame, log-decaying to a smooth stop) and a separate curve for xterm.js-hosted
  terminals (VS Code) that drains instantly for small pending amounts (≤5 rows) but
  steps at a fixed small increment for larger bursts to avoid visible jumps. **The
  easing-curve idea itself is portable** if Atlas's viewport ever gets a "smooth scroll"
  feature and needs to decide how many rows to reveal per animation tick — proportional-
  with-a-floor easing is a reasonable off-the-shelf curve.
- **terminal.ts** — a pile of environment/version-based terminal-quirk detection.
  Concretely useful even for Atlas's existing termenv-based approach: **DEC 2026
  (synchronized output / BSU-ESU) detection is per-terminal-emulator, not just
  per-multiplexer** — `isSynchronizedOutputSupported()` explicitly distrusts tmux AND
  Zellij (both re-chunk the stream and break BSU/ESU atomicity even if the *underlying*
  terminal supports it) while allowlisting iTerm2/WezTerm/Ghostty/kitty/foot/Alacritty/
  Windows Terminal/VTE≥0.68 by `TERM_PROGRAM`/`TERM`/env heuristics. **If Atlas ever
  wants flicker-free diff-based redraws, wrapping frame writes in
  `\x1b[?2026h...\x1b[?2026l` gated by this same detection logic is a direct,
  self-contained, high-value port** — it's a small, well-isolated piece of domain
  knowledge (which terminals lie about DEC 2026 support through a multiplexer) that
  would otherwise take real trial-and-error to rediscover.
- **terminal-querier.ts** — timeout-free terminal-capability query batching: every
  terminal has answered Primary Device Attributes (`CSI c`) since VT100, and terminals
  answer queries **in the order sent**, so this sends a DA1 as a sentinel after a batch
  of real queries (DECRQM, OSC color, Kitty-flags, XTVERSION, etc.) and resolves any
  query whose response hasn't arrived by the time DA1's reply shows up as
  "unsupported," with **zero setTimeout-based timeouts and zero risk of falsely
  reporting a slow-but-real reply as unsupported**. **This is a clean, portable, fully
  self-contained technique** for any TUI that needs to probe terminal capabilities
  (color depth, sync-output support, focus-reporting support, etc.) without guessing at
  a timeout value — directly applicable if Atlas ever wants runtime capability
  detection beyond static env-var sniffing.
- **stringWidth.ts** — Unicode display-width measurement with a real, currently-broken-
  in-common-libraries edge case fixed: ambiguous-width glyphs (e.g. ⚠ U+26A0, which the
  popular `string-width` npm package reports as width 2) are treated as narrow per the
  Unicode-recommended default for non-East-Asian contexts; zero-width character
  detection (combining marks across Latin/Devanagari/Thai/Lao/Arabic ranges, ZWJ/ZWNJ,
  variation selectors, BOM) is hand-rolled rather than relying on a generic library;
  emoji width uses grapheme-cluster segmentation with special-cased regional-indicator
  flag pairs (1 vs 2 code-cell width depending on solo vs. paired) and "incomplete
  keycap" sequences (digit + VS16 without the trailing U+20E3 combining enclosing
  keycap renders as width 1, not the usual keycap-emoji width 2). Wrapped in an
  ASCII-fast-path (skip all of the above for short pure-ASCII strings) and an LRU cache
  bounded at 8192 entries, because a CPU profile showed this function at 21% of total
  runtime during fast scroll in the real app. **This is exactly the kind of "specific
  ANSI/Unicode edge case handled" the task asked to flag** — if Atlas (via
  `go-runewidth` or similar) ever mis-renders `⚠` as double-width, or mis-measures
  flag emoji / incomplete keycaps, this file is the reference for the correct behavior
  and the exact Unicode ranges to special-case.
- **ansi-transition.ts** — a narrow, real ANSI-emission correctness bug and its fix:
  bold (SGR 1) and dim (SGR 2) share a single reset code (SGR 22) despite being
  independent terminal attributes; a naive diff-based style-transition emitter (the
  underlying `diffAnsiCodes` library used here) that treats "same endCode" as "same
  slot, new code overwrites" gets this wrong — transitioning bold→dim without emitting
  the shared reset first leaves the terminal in bold+dim rather than plain dim, and
  because subsequent transitions are computed from the *tracked* (wrong) state, the
  corruption compounds across a whole render session, showing up as unpredictable
  weight/brightness on spans of text. The fix (detect when a transition would need to
  *remove* a member of the shared-reset family, emit SGR 22 first, then re-apply
  everything in the target style that still carries weight) is a genuinely specific,
  transferable ANSI edge case — **any Go ANSI-diffing/transition code (if Atlas or a
  future custom renderer ever does incremental SGR diffing rather than emitting a full
  style reset per cell) should special-case bold/dim the same way**, or avoid the whole
  class of bug by never doing incremental SGR diffing at all (emit full styles per
  styled run, which is what Lipgloss does today — so this is a non-issue for Atlas as
  currently built, but a landmine to know about if that ever changes).

### Pure plumbing / confirmed non-portable (one line each)

- **Ansi.tsx** — parses pre-formatted ANSI strings (e.g. from `cli-highlight`) back
  into styled `<Text>` spans, for embedding externally-colored tool output. No
  Bubbletea equivalent needed — Atlas would just print the raw ANSI through directly
  or use termenv's own parsing if it ever needs to re-style foreign ANSI output.
- **bidi.ts** — Unicode Bidi Algorithm reordering for RTL text (Hebrew/Arabic/etc.),
  gated on Windows Terminal / conhost / VS Code terminal (which don't implement bidi
  natively; macOS terminals do). Real, narrow feature; portable only if Atlas ever
  needs RTL text support on Windows, at which point Go has bidi libraries
  (`golang.org/x/text/unicode/bidi`) that would need similar OS/terminal gating logic
  copied from here.
- **cache-eviction.ts** — unifies eviction (`clear()` or keep-half) across four
  content-keyed LRU caches under memory pressure. Plumbing tied to those specific
  caches; the *idea* of a memory-pressure eviction hook is generic but there's nothing
  concrete to port until Atlas has equivalent hot-path caches worth bounding.
- **clearTerminal.ts** — per-platform clear-with-scrollback sequence selection
  (modern Windows Terminal/mintty support `CSI 3J`, legacy conhost doesn't). Small,
  useful reference if Atlas's clear-screen command doesn't already handle legacy
  Windows conhost specially.
- **colorize.ts** — chalk-level workarounds: xterm.js (VS Code terminal) under-reports
  truecolor support so is force-upgraded; tmux is force-downgraded to 256-color because
  it silently drops truecolor bg sequences unless the user has explicitly configured
  `Tc` passthrough; legacy Apple Terminal gets a perceptually-tuned RGB→256 downgrade
  (HSL-based, not naive nearest-cube) instead of chalk's default. These are real,
  narrow terminal-compatibility facts — worth knowing if Atlas's termenv-based color
  handling ever produces washed-out colors specifically inside tmux or VS Code's
  terminal, but not an algorithm to port, just environment facts to replicate via
  termenv's own profile-detection if not already covered.
- **constants.ts** — two numeric constants (16ms frame interval, backpressure ceiling).
- **cursor.ts** — a 3-field type.
- **devtools.ts** — empty stub for optional react-devtools-core import.
- **dom.ts** — the DOM node model (`DOMElement`/`TextNode`), attribute/style diffing
  with dirty-marking, and a small per-node text-measurement cache keyed by
  `${width}|${widthMode}` (capped at 16 entries, FIFO eviction) to avoid re-wrapping
  text when Yoga's two-pass measurement probes the same node at multiple candidate
  widths in one layout pass. Pure React-reconciler-target plumbing — Bubbletea's model
  IS the tree, no separate DOM layer needed.
- **focus.ts** — `FocusManager`: single active-element + bounded focus-restoration
  stack (max 32, deduped) so Tab-cycling through many elements doesn't leak memory, and
  on node removal, walks the stack to restore focus to the most recent still-mounted
  element. The *pattern* (bounded undo-stack for focus restoration on removal) is
  reasonable if Atlas ever builds multi-pane focus management, but there's no
  Bubbletea hook to attach it to today.
- **frame.ts** — `Frame`/`Patch`/`Diff` type definitions and `shouldClearScreen()`
  (resize, or content taller than viewport in either the old or new frame). Pure types.
- **get-max-width.ts** — one Yoga-node arithmetic helper (width minus padding/border).
- **global.d.ts** — empty (`export {}`).
- **hyperlinkHover.ts** — inverts all cells sharing a hovered OSC-8 hyperlink URL as
  the closest terminal-native equivalent of a cursor-hover affordance (terminals can't
  change the system mouse cursor). No relevance without mouse support.
- **instances.ts** — a `Map<stdout, Ink>` singleton registry so repeated `render()`
  calls reuse one instance. Node/Ink-specific bookkeeping.
- **line-width-cache.ts** / **lru.ts** — generic LRU width-memoization + a shared
  `lruEvict()` helper (drop-oldest by insertion order, either fully or to a keep-ratio).
  Trivial, would port 1:1 into Go with a map + doubly-linked-list or `container/list`
  if Atlas ever profiles string-width computation as hot (unlikely at Atlas's likely
  content volume vs. a long-running chat TUI rendering 8k+ messages).
- **measure-element.ts** / **measure-text.ts** — thin Yoga-node / text measurement
  wrappers (single-pass width+height via `indexOf('\n')` instead of `split` to avoid
  array allocation — a reasonable micro-optimization, not a functional idea).
- **node-cache.ts** — `WeakMap<DOMElement, CachedLayout>` for blit/clear bookkeeping.
  Renderer plumbing.
- **optimizer.ts** — post-diff patch-list optimizer (merge consecutive cursor moves,
  concatenate adjacent style strings, dedupe repeated hyperlinks, cancel
  cursor-show/hide pairs). A reasonable "coalesce your write patches before flushing"
  pattern in general, but tied to this exact `Patch` type.
- **output.ts** — the paint-time operation queue (write/blit/clip/clear/shift/noSelect)
  that gets flushed into a `Screen`. Includes the grapheme-clustering + per-line
  character cache (`charCache`, capped at 16384 entries) that turns "tokenize + cluster
  graphemes" into a cache hit for unchanged lines, and hand-rolled ANSI-escape-sequence
  skipping in the character loop (recognizes and skips CSI/OSC/DCS/charset-designation
  sequences that the upstream tokenizer didn't already strip, so stray escape bytes in
  tool output don't desync cursor-position bookkeeping). Deep renderer internals with
  no Bubbletea equivalent needed.
- **reconciler.ts** — React-reconciler host config (createInstance, commitUpdate,
  appendChild, etc.), wiring styles/event-handlers/attributes onto DOM nodes and
  Yoga nodes. Pure React internals — Bubbletea's own Update/View loop replaces this
  entirely.
- **render-border.ts** — border-drawing with viewport-clipping and embedded
  border-text (title bars). Compares conceptually to Lipgloss's built-in
  `.Border()`: Lipgloss already handles border style/rendering natively without
  needing this level of manual clip-math, so nothing to port — worth confirming
  Atlas is actually leaning on Lipgloss's own border+clip handling rather than
  hand-rolling any of this.
- **renderer.ts** — orchestrates one frame: Yoga layout validity checks, alt-screen
  height clamping (defends against a sibling accidentally rendering outside
  `<AlternateScreen>` by clamping instead of corrupting the whole terminal), and the
  alt-screen `viewport.height = rows+1` / cursor-clamp hack that prevents the
  cursor-restore step from ever emitting a scroll-triggering line-feed. Tightly coupled
  to the rest of this custom renderer.
- **root.ts** — public `render()`/`createRoot()` API surface (React-DOM-createRoot-
  style). API-shape plumbing.
- **render-to-screen.ts** — renders a React element to an isolated off-screen buffer
  purely to scan it for search-highlight matches (row/col positions), reusing one
  root/container/pools across calls for speed. Renderer-internal search-support
  utility.
- **searchHighlight.ts** — scans a screen buffer for case-insensitive query matches,
  building a per-row code-unit→cell-index map to correctly handle wide characters and
  multi-code-unit lowercasing (e.g. Turkish İ). The *mapping technique* (don't assume
  1 haystack-char == 1 terminal cell) is the transferable idea if Atlas ever implements
  in-app search/highlight over rendered content with wide characters present.
- **squash-text-nodes.ts** — flattens a nested `<Text>` tree into either styled
  segments (preserving per-run styles/hyperlinks) or a plain string (for layout
  measurement). Tree-flattening plumbing specific to this component model.
- **supports-hyperlinks.ts** — extends the `supports-hyperlinks` npm package's
  detection with extra known-good terminals (ghostty, kitty, alacritty, iTerm) via
  `TERM_PROGRAM`/`LC_TERMINAL`/`TERM` — small, useful list if Atlas emits OSC-8
  hyperlinks and wants to gate on real support rather than assuming.
- **tabstops.ts** — 8-column tab expansion tokenizing around existing ANSI sequences
  so a tab inside colored text doesn't get corrupted. Small, correct, portable
  algorithm if Atlas ever needs to expand raw tabs in externally-sourced text (agent
  tool output) before measuring/wrapping it — worth checking whether Atlas already
  handles this or assumes tabs never appear in rendered content.
- **terminal-focus-state.ts** — a tiny non-React pub/sub for terminal-window-focus
  state, feeding `useSyncExternalStore`. Paired with the terminal-focus-event gap
  noted above.
- **useTerminalNotification.ts** — OSC-sequence builders for iTerm2/Kitty/Ghostty
  desktop notifications, terminal bell, and OSC 9;4 progress-bar reporting (with a
  version-gated support check: ConEmu all versions, Ghostty 1.2.0+, iTerm2 3.6.6+,
  explicitly NOT Windows Terminal which repurposes OSC 9;4 for notifications instead).
  If Atlas ever wants a taskbar/dock progress indicator or a native OS notification
  when a long agent run finishes, this is a ready-made compatibility matrix — worth
  flagging as a "nice to have" rather than a gap, since it's optional polish, not
  something users would perceive as "the TUI looks bad" without it.
- **warn.ts** — one integer-validation debug-log helper.
- **widest-line.ts** — trivial max-line-width helper built on the shared
  `lineWidth` cache.
- **wrap-text.ts** / **wrapAnsi.ts** — memoized text-wrapping (wrap/char-wrap/
  trim-soft-wrap-boundaries/truncate-start/middle/end variants), keyed by
  `(maxWidth, wrapType, text)`, because a CPU profile showed wrap-ansi → string-width
  at 30% of runtime during fast scroll. The memoization *pattern* (cache pure
  text-layout functions keyed on their full input) is generic and would help Atlas if
  Lipgloss's own wrapping ever profiles as hot on a long transcript — but Lipgloss
  likely already has its own internal caching/is fast enough at Atlas's scale; only
  worth revisiting if a real profile shows a problem.

## native-ts/yoga-layout/ (2 files, skimmed per instructions)

`enums.ts` (112 lines) + `index.ts` (2326 lines): a complete, dependency-free pure-TS
port of Facebook's Yoga flexbox algorithm (previously WASM-based; now synchronous JS
with no async load/instantiate step). Confirmed low value for Atlas — full flexbox
constraint solving has no reason to exist in a Bubbletea app; Lipgloss's simpler
fixed-box model is the correct fit for a terminal chat UI's actual layout needs
(vertical stacks, fixed-width panels, joined boxes), not a general flex/wrap/grow/
shrink solver.

## bootstrap/state.ts

Four-function stub module (`flushInteractionTime`, `updateLastInteractionTime`,
`markScrollActivity` are no-ops in this build; `getIsInteractive()` checks both
stdin/stdout TTY-ness). Essentially dead/vestigial in this slice — the real
implementation presumably lives in a package this sparse-checkout excluded. Nothing
to port.

## hooks/ (package-level: use-stderr.ts, use-stdout.ts)

Trivial `useMemo`-wrapped `{stream, write}` handles over `process.stderr`/
`process.stdout`. No algorithmic content.

## entry-exports.ts / index.d.ts / ambient.d.ts / text-input.d.ts

Public API surface + TypeScript ambient module shims for untyped/partially-typed deps
(react-reconciler, bidi-js, lodash-es subpaths, semver, Bun global). One documented
architectural note worth flagging as a *pattern*, not code: `entry-exports.ts`
deliberately does NOT re-export `ink-text-input` because that package depends on the
upstream `ink` npm package, and re-exporting it from a source-level `@hermes/ink`
package would drag a second, circularly-async-initializing copy of `ink` into any
bundler consuming `@hermes/ink` from source — causing a startup deadlock (issue
#31227) with only a partial ANSI reset ever written to the terminal. The generalizable
lesson: **when a package internally re-implements or forks a widely-used library,
avoid accidentally re-exporting a third-party package that itself depends on the
*original* (non-forked) library**, or bundlers can end up loading two incompatible
copies with circular init dependencies. Not directly applicable to Atlas (single Go
binary, no bundler-level module duplication risk), but worth remembering if Atlas ever
vendors or forks a Go TUI dependency and re-exposes a public API surface around it.

---

## Files in the manifest I could not fully deep-read line-by-line

Given the volume (~94 files, several exceeding 1500-2800 lines), the following were
read thoroughly enough to state their purpose and extract any portable technique
accurately, but not transcribed/verified line-by-line in full:
- `screen.ts` (read first ~700 of 1591 lines — the packed-cell/pool-interning core;
  remainder is more `setCellAt`/`shiftRows`/`blitRegion` mechanical cell-buffer
  operations building on the same technique already captured).
- `selection.ts` (read first ~200 of 1143 lines — full data model and word-boundary
  algorithm; remainder is more selection-state transition functions — drag/extend/
  shift/capture-scrolled-rows — following the same anchor/focus pattern).
- `render-node-to-output.ts` (read ~180 of 1888 lines plus scroll-hint/damage-tracking
  module-level state; remainder is the recursive per-node-type paint dispatch —
  Box/Text/Link/RawAnsi/ScrollBox-specific painting logic built on the already-
  captured clip/blit/scroll-hint/damage machinery).
- `styles.ts` (read first ~120 of 750 lines — confirmed it's the Box-style-props →
  Yoga-node-setter mapping layer; remainder is more of the same per-property mapping).
- `ink.tsx` was read in FULL (2840 of 2840 non-sourcemap lines).
- `parse-keypress.ts`, `hit-test.ts`, `log-update.ts`, and the entire `ink/events/`
  and `ink/hooks/` directories were read in full as instructed.
- All `.test.ts`/`.test.tsx` files were skimmed for locked-in behavior (test titles +
  representative assertions), not deep-read line by line, per the task's own
  instruction for test files.

No file in the manifest was skipped entirely — every file was opened and its purpose
confirmed at minimum.

---

## Top recommendations for Atlas

Kept short and honest — most of this slice is legitimately non-portable engine
plumbing (a from-scratch React-for-terminals renderer that Bubbletea's own loop
already replaces). These are the items with real, concrete substance:

1. **Bracketed-paste handling is a real, currently-missing gap.** Atlas's own scoping
   notes confirm "no bracketed-paste-specific handling currently." `parse-keypress.ts`'s
   model — recognize `CSI 200~`/`CSI 201~`, buffer everything between as literal text
   (never reinterpreted as escape sequences), emit one paste event even for empty
   pastes, and watchdog-flush if the terminal ever drops the end marker — is a small,
   self-contained state machine directly portable to Go regardless of renderer. This
   fixes both a correctness class of bug (pasted text containing control-sequence-
   looking bytes triggering keybindings) and reduces N keystroke updates to 1.

2. **DEC 2026 synchronized-output detection + BSU/ESU wrapping**, if Atlas ever sees
   visible flicker during redraws. `terminal.ts`'s `isSynchronizedOutputSupported()`
   is a compact, battle-tested capability matrix (explicitly distrusts tmux and Zellij
   even when the underlying terminal supports it, allowlists everything else by
   `TERM_PROGRAM`/`TERM`/env) that would otherwise take real trial-and-error to
   rediscover from scratch.

3. **Timeout-free terminal capability probing via a DA1 sentinel** (`terminal-querier.ts`).
   If Atlas ever wants to query terminal capabilities (color depth beyond env-var
   sniffing, focus-reporting support, sync-output support) at runtime rather than
   guessing from `TERM`/`TERM_PROGRAM`, the "queue queries, terminate the batch with a
   universal DA1 sentinel, resolve unanswered queries as unsupported when DA1's reply
   arrives" pattern eliminates the usual guess-a-timeout problem entirely.

4. **stringWidth.ts's Unicode edge cases are worth a spot-check against whatever width
   library Atlas currently uses** (likely `go-runewidth` or similar) — specifically:
   ambiguous-width glyphs like `⚠` (U+26A0) reported as double-width by some libraries
   when it should be narrow, regional-indicator flag emoji (1 cell alone, 2 cells
   paired), and "incomplete keycap" sequences (digit+VS16 without the keycap-enclosing
   combiner rendering as width 1, not the usual width 2). These are concrete,
   reproducible rendering bugs, not theoretical.

5. **DECSTBM hardware-scroll optimization** — lower priority (only matters if Atlas's
   `bubbles/viewport` scroll performance is ever actually a user-visible problem on
   long transcripts), but if it is, `log-update.ts`'s technique — detect "only
   scrollTop changed, nothing else in the visible region moved" and emit a scroll-
   region + SU/SD hardware scroll instead of rewriting every visible row — is the
   correct, proven fix, not a novel idea to prototype from scratch.

Everything else of substance in this slice (mouse hit-testing algorithm, focus-event
taxonomy, terminal-window-focus reporting, packed-cell buffer design, ANSI bold/dim
transition bug) is real and correctly engineered, but conditional on Atlas eventually
adding a feature it doesn't have today (mouse support, in-app focus management,
terminal-focus-aware animation throttling, or a hand-rolled cell-buffer renderer) —
worth keeping this document as a reference for *when* those features are considered,
not acting on now.
