# Hermes-Ink Analysis for Atlas (TUI/hermes-ink slice)

Scope: `ui-tui/packages/hermes-ink/` — Hermes's custom fork of Ink. This report
covers ONLY the manifest's ~54 files (components/termio/utils + the standalone
`src/ink/*.ts` files). The ~117-file yoga-flexbox/reconciler/hooks core was
deliberately NOT read — none of those files appear below.

All source lives in TypeScript + React + Ink; Atlas is Go + Bubbletea +
Lipgloss + Bubbles + Glamour. Nothing below is a code port — every note is a
"port the *idea*, not the code" translation.

---

## 1. Standalone files (`src/ink/*.ts`)

### terminal.ts / terminal-background.test.ts — **terminal background/foreground detection (HIGHEST VALUE)**

This is exactly the OSC-11/OSC-10 background-color probe Atlas is missing.

- `parseOscColor(data)`: parses X11 color replies. Handles `rgb:RRRR/GGGG/BBBB`
  (1-4 hex digits per channel, scaled via `round(value/(16^n-1)*255)`),
  `rgba:...` (alpha dropped), and plain `#hex`/`hex` (3/6/12-digit forms; the
  12-digit form takes the *high byte* of each 4-digit channel).
- Storage is a tiny "first-writer-wins" reactive slot (`reportedColorSlot()`):
  `set()` only applies once (defends against being re-probed / racing
  writes), `on(listener)` fires immediately if already resolved else queues.
  Two independent slots exist: background (OSC 11) and foreground (OSC 10).
- **Why both colors, not just background**: transparent-background terminal
  profiles report OSC 11 as the *unset default* (pure black) but still report
  the theme's true OSC 10 foreground — so foreground luminance is the
  fallback polarity signal when background is untrustworthy. Background is
  applied first; if it resolves, the foreground listener is never needed.
- Also in this file: `isSynchronizedOutputSupported()` (DEC 2026 BSU/ESU
  flicker-free redraw — allowlists WezTerm/iTerm/kitty/ghostty/foot/Windows
  Terminal, VTE≥0.68, excludes tmux/Zellij because those proxy/chunk the
  stream and break atomicity even though the outer terminal supports it),
  `isProgressReportingAvailable()` (OSC 9;4 progress bar — ConEmu all
  versions, Ghostty 1.2+, iTerm2 3.6.6+, explicitly excludes Windows Terminal
  which reinterprets OSC 9;4 as a notification), and
  `hasCursorUpViewportYankBug()` (Windows conhost's SetConsoleCursorPosition
  follows the cursor into scrollback on cursor-up sequences —
  `microsoft/terminal#14774` — gated on `win32` or `WT_SESSION`; this is a
  concrete, cited Windows rendering bug worth checking against Atlas's own
  redraw approach since the user is on Windows/cmd.exe).

**Go translation**: write `internal/tui/termprobe.go` (or extend `color.go`):
send `\x1b]11;?\x07` (+ `\x1b]10;?\x07`) to stdout, read the reply off stdin
before entering Bubbletea's input loop (or via a raw pre-read like
`terminal-querier.ts` below), parse with the same X11-color grammar, derive
light/dark from luminance, and feed the result into `lipgloss.AdaptiveColor`
resolution instead of trusting termenv's own guess. This is very likely the
single highest-leverage fix available for "may render wrong on Windows
cmd.exe" since it removes a heuristic guess and replaces it with ground truth
straight from the terminal.

### terminal-querier.ts — **timeout-free terminal query pattern**

A clean, reusable mechanism for asking the terminal a question without
guessing a timeout:

- Every query batch is closed with a DA1 sentinel (`CSI c`) — DA1 is answered
  by literally every terminal since VT100, and terminals answer in the order
  they were asked.
- `send(query)` pushes `{match, resolve}` onto a FIFO queue and writes the
  request.
- `flush()` pushes a sentinel and writes `CSI c`.
- `onResponse(r)`: first tries to match `r` against a pending query
  (FIFO first-match). If `r` is itself a DA1 and doesn't match anything,
  it resolves every query queued *before* the first pending sentinel with
  `undefined` (they didn't get answered → terminal doesn't support it) and
  resolves the sentinel's own promise.
- Net effect: `Promise.all([send(oscColor(11)), send(oscColor(10)),
  flush()])` resolves in one round trip, with unsupported queries cleanly
  resolving to `undefined` instead of hanging or requiring an arbitrary
  `setTimeout`.

**Go translation**: this pattern ports cleanly to Go with channels — write a
small `Querier` that writes a batch of escape sequences ending in `CSI c`,
reads stdin in a raw-mode goroutine matching against expected reply prefixes,
and closes out any still-pending requests when the DA1 reply (`CSI ? ... c`)
arrives. Directly reusable for the background-color probe above, or any
future capability probe (truecolor, XTVERSION, Kitty keyboard protocol).

### stringWidth.ts — **width calculation, NOT a "safe glyph" filter**

Important finding for Atlas's emoji problem: **this file does not contain
any "will this terminal render this glyph correctly" check.** It's purely
display-*width* math (how many columns should the cursor/layout budget for),
not a renderability/font-coverage check. Key points:

- Uses `Bun.stringWidth(str, {ambiguousIsNarrow: true})` when running under
  Bun; otherwise a JS fallback (`stringWidthJavaScript`) that:
  - Fast-paths pure-ASCII strings with a byte loop.
  - Otherwise uses `eastAsianWidth(cp, {ambiguousAsWide: false})` per
    grapheme cluster (via `Intl.Segmenter`), explicitly choosing "ambiguous
    width = narrow" as more correct for Western terminals (their comment:
    string-width mis-reports `⚠ U+26A0` as width 2; this implementation gets
    it right).
  - Emoji handling (`getEmojiWidth`): regional-indicator flag pairs = 2,
    single = 1; digit/`#`/`*` + VS16 without the `U+20E3` keycap combiner =
    1 (incomplete keycap sequence); everything else matched by
    `emoji-regex` = 2.
  - `isZeroWidth()` is a large explicit table: combining marks, ZWJ/ZWNJ,
    variation selectors, Indic/Thai/Lao vowel signs and viramas, Arabic
    formatting chars, surrogates, tag characters.
  - A comment explicitly documents a *deliberate* inaccuracy: complex Indic
    ligatures (e.g. Devanagari क्ष) render as ONE glyph but this width
    function returns the width of all base consonants summed, because that
    matches actual terminal cell allocation (which is what layout math needs)
    even though it overstates "visual" width.
- Two-level memoization: an inline ASCII fast path skips the cache entirely
  for short pure-ASCII strings (cache lookup would be slower than
  recomputation); everything else goes through an 8192-entry LRU `Map`.

**Implication for Atlas**: Atlas's blunt "strip all emoji" fix addresses a
different problem (glyph *rendering* — tofu boxes / wrong codepoint width in
the actual terminal font) than what this file solves (layout *width*
accounting). Porting this width logic (Go already has `mattn/go-runewidth`
and `rivo/uniseg` which lipgloss/bubbletea pull in, so this is **largely
redundant** — but the specific choices worth checking against Atlas's actual
dependency behavior are: (1) ambiguous-width-as-narrow (go-runewidth's
default may differ), (2) the keycap/regional-indicator special cases, (3)
whether Atlas's flexbox-equivalent (lipgloss.Width / bubbles layout) is even
using a East-Asian-width-aware function at all right now. The real fix for
"wrong glyph rendering" is a font/terminal *capability* allowlist (which
Hermes does NOT appear to implement either — see Top Recommendations).

### wrapAnsi.ts / wrap-text.ts — mostly redundant with Lipgloss

`wrapAnsi.ts` is a two-line wrapper picking `Bun.wrapAnsi` or npm's
`wrap-ansi` — no novel logic. `wrap-text.ts` builds on it with:
- An LRU cache keyed on `${maxWidth}|${wrapType}|${text}` (4096 entries) —
  profiling showed `wrap-ansi → string-width` at 30% of render time during
  fast scroll, so this is a pure performance patch.
- `truncate(text, columns, position)` supports `start`/`middle`/`end`
  ellipsis truncation with a "retry one column tighter if a wide char
  overshoots the boundary" correction (`sliceFit`).
- `trimSoftWrapBoundaries`: after hard-wrapping, trims the trailing space on
  a wrapped line or the leading space on the next line (but not both) so a
  soft-wrapped sentence doesn't show a doubled space at the seam.

Lipgloss already provides ANSI-aware wrapping (`lipgloss.NewStyle().Width()`
plus `muesli/reflow/wordwrap` and `ansi`) and truncation
(`lipgloss.Style.MaxWidth`/`truncate` helpers), so this is mostly **already
covered**. The one gap worth checking: does Atlas's chat-viewport re-wrap on
every render pass without a memoization cache? If Atlas ever profiles slow
scroll on a long transcript, an LRU wrap-result cache keyed on
`(width, content)` is a cheap, proven fix (they measured 30% of render time
here).

### tabstops.ts — small, portable, worth taking

8-column tab expansion (`DEFAULT_TAB_INTERVAL = 8`, POSIX default) that is
ANSI-escape-safe: it tokenizes the string first (via their own
`termio/tokenize.ts`) so tabs inside plain-text runs get expanded to the
next 8-column stop while escape sequences pass through untouched and don't
perturb the running column counter. If Atlas ever renders raw tool output
containing literal tabs (e.g. `cat -A`-style dumps, some CLI tool output),
this exact algorithm (interval - (column % interval)) is trivial to port to
Go using `lipgloss/ansi` for escape-aware tokenizing.

### widest-line.ts — trivial, redundant

Splits on `\n`, takes max width per line via a cached `lineWidth()`. Lipgloss
has no direct equivalent named function but `lipgloss.Width()` on a
multi-line string already returns the widest line. No action needed.

### supports-hyperlinks.ts — allowlist worth mirroring

Wraps the `supports-hyperlinks` npm package and extends it with a manual
allowlist (`ghostty, Hyper, kitty, alacritty, iTerm.app, iTerm2`) checked
against `TERM_PROGRAM` **and** `LC_TERMINAL` (the latter survives being
inside tmux, where `TERM_PROGRAM` gets overwritten to `tmux`). Also checks
`TERM` containing `kitty`. Small, portable, and directly useful if Atlas ever
wants OSC 8 hyperlinks (e.g. clickable file paths / URLs in tool output) —
Go has no widely-used hyperlink-support-detection package, so this env-var
allowlist is worth copying as-is into Go.

### terminal-focus-state.ts — DECSET 1004 focus tracking

A simple module-level pub/sub for terminal focus/blur (via DEC mode 1004
focus-reporting), exposed as `'focused' | 'blurred' | 'unknown'` (unknown =
terminal doesn't support focus events; treated identically to focused so
throttling never engages on terminals that can't tell you). Notifies
subscribers synchronously. Used (see ClockContext below) to halve the render
tick rate while the terminal is unfocused. Bubbletea has no built-in
focus-event support; this would require sending `CSI ?1004h` at startup and
watching for the `CSI I` / `CSI O` sequences in the input stream — feasible
but a build-it-yourself feature.

### selection.ts / searchHighlight.ts — NOT portable as-is

Both operate directly on Hermes's own cell-based `Screen` buffer (their
custom character-grid model with per-cell `styleId`, wide-char spacer cells,
per-row soft-wrap bitmap, "noSelect" gutter masking). This is genuinely deep,
well-thought-through mouse-selection logic (double-click word bounds via a
Unicode word-char class matching iTerm2's defaults, drag-to-scroll capture of
rows that scroll off-screen mid-drag, virtual-row tracking so
scroll-then-reverse-scroll restores the exact original selection, a plain
"detect a bare URL in the cell grid and open it on click" fallback for
terminals with no OSC-8 support). None of it is a two-line port: it assumes a
custom screen model Atlas doesn't have (Bubbletea has no character-grid
concept; it emits ANSI-styled strings). If Atlas ever wants mouse
text-selection in an alt-screen view, this file is a good *design reference*
for the state machine (anchor/focus/anchorSpan, scroll-accumulator arrays)
but would need a from-scratch reimplementation against whatever cell model
Atlas's own alt-screen renderer (if any) uses. Given effort/impact, this is
low priority.

### squash-text-nodes.ts / styles.ts — reconciler plumbing, low value

`squash-text-nodes.ts` flattens Ink's DOM tree of `<Text>`/`<Link>` nodes
into styled segments — pure React-reconciler-tree logic, not applicable to
Bubbletea/Lipgloss (no virtual DOM). `styles.ts` is Yoga-property mapping
boilerplate (margin/padding/flex/border → Yoga node setters); its only value
to Atlas is the **prop surface it exposes**, notably:
- `opaque` (fill interior with blank spaces before children render, without
  emitting SGR — useful for absolute-positioned overlays so nothing behind
  shows through padding/gaps),
- `noSelect: boolean | 'from-left-edge'` (exclude cells from text selection —
  ties to the selection.ts logic above),
- `borderText` (render text inline in the top/bottom border, e.g. a titled
  box). Lipgloss doesn't have a first-class "titled border" helper; if Atlas
  wants section-titled boxes this is worth a small custom Lipgloss helper.

### warn.ts — trivial, one-liner (`ifNotInteger` dev warning), no action.

### useTerminalNotification.ts — terminal notification/progress dispatch

A small React hook wrapping OSC-sequence emission: `notifyITerm2` (OSC 9
proprietary), `notifyKitty` (OSC 99, three-part title/body/focus), `notifyGhostty`
(OSC 777 `notify`), `notifyBell` (raw BEL — deliberately NOT wrapped in
tmux's DCS passthrough, because tmux's own bell-action flag only fires on a
raw, unwrapped BEL), and `progress()` (OSC 9;4 iTerm2/ConEmu/Ghostty progress
bar, gated on `isProgressReportingAvailable()` from terminal.ts above). If
Atlas ever wants "ding when a long tool call finishes" or a taskbar progress
indicator, this is a small, complete reference for which OSC variant to send
per terminal family.

---

## 2. components/ (21 files)

The manifest's own framing was right: the *props* these components expose are
the useful signal, not their Yoga-wiring internals (most files are
react-compiler-transformed and low-signal to read literally).

- **ScrollBox.tsx (read in full)** — Atlas's biggest architecture comparison
  point vs `bubbles/viewport`. Key differences:
  - **Sticky-to-bottom is a first-class, explicit boolean** (`stickyScroll`
    prop + `isSticky()` on the imperative handle), not inferred from
    `scrollTop == max`. It's cleared the instant the user calls
    `scrollTo`/`scrollBy` (any manual scroll breaks stickiness) and restored
    only via explicit `scrollToBottom()`. `bubbles/viewport` has no
    sticky-scroll concept at all — Atlas has to hand-roll "am I at the
    bottom" tracking today; this pattern (a boolean flag, not a
    position comparison) is more robust against off-by-one/height-changing
    races and worth adopting verbatim.
  - **Two write paths**: imperative (`scrollTo`/`scrollBy` mutate a DOM node
    field directly and schedule a render, bypassing React state entirely —
    "no reconciler overhead per wheel event") vs the sticky-bottom flag which
    still forces a full re-render. This split exists purely for perf on
    high-frequency wheel events; Atlas/Bubbletea's Update-loop model doesn't
    have an equivalent "bypass the framework" lever, so this is architecture-
    specific and not portable, but validates that Atlas doesn't need to fight
    performance here the way Hermes did.
  - **`scrollToElement(el, offset)`**: scroll so a specific *element* (not a
    raw row number) lands at the top, with the position resolved lazily at
    render time (reads the live layout, not a stale precomputed number) —
    useful pattern if Atlas ever needs "scroll to this specific chat
    message" (e.g. jump-to-search-result); `bubbles/viewport` only offers
    line-number scrolling.
  - **Viewport culling**: children are laid out at full height but only
    rows intersecting `[scrollTop, scrollTop+height]` are rendered —
    conceptually identical to what `bubbles/viewport` already does, so no
    gap here.

- **Box.tsx / Text.tsx** — prop surface signal: `Box` treats `onMouseEnter`/
  `onMouseLeave` as non-bubbling (unlike `onClick` which bubbles with a
  `stopImmediatePropagation()` escape hatch) — a distinction Atlas would only
  need if it grows real mouse-driven widgets. **Text.tsx's real find**:
  `shouldUseAnsiDim()` / `dimColorFallback()` — SGR 2 (dim) is not honored by
  every terminal; Hermes detects Apple_Terminal (and any terminal reporting
  no `VTE_VERSION`) and substitutes an actual muted color (`#6B7280`,
  theme-overridable via `setDimFallbackColor`) instead of emitting the SGR
  dim code that terminal would silently ignore. **This is directly relevant
  to Atlas's `internal/tui/color.go` dim/derived-color logic** — if Atlas
  emits `faint`/dim styling anywhere (lipgloss `Faint(true)`), it should
  verify this against Apple Terminal / low-fidelity terminals and consider
  falling back to an explicit muted color the same way, gated on a similar
  env probe. Also: bold and dim are made *mutually exclusive at the type
  level* via a TS discriminated union (`{bold?:never,dim?:never} |
  {bold:boolean,dim?:never} | {dim:boolean,bold?:never}`) — a nice API
  design idea if Atlas has a Go style-builder API surface, though Go's type
  system can't enforce this as cleanly.

- **AlternateScreen.tsx** — full lifecycle for entering/exiting the terminal
  alt-screen (DEC 1049) with a chosen mouse-tracking preset
  (`off`/`wheel`/`buttons`/`all`, i.e. 1000/1002/1003/1006 combinations).
  Notable robustness details: it unconditionally sends
  `DISABLE_MOUSE_TRACKING` (resets all 4 mouse DEC modes) before enabling
  the requested preset, specifically to defend against a previous
  crashed/lingering process having left a different mode asserted; and it
  sends the same disable-then-exit-alt-screen sequence on every teardown so
  a mid-mount crash can't leak DEC modes back to the host shell. Bubbletea's
  `tea.WithAltScreen()`/`tea.WithMouseAllMotion()` cover the enable side;
  this teardown-robustness pattern (always emit the full disable sequence on
  exit, never assume you know the current state) is worth checking Atlas
  actually does on all exit paths (panic recovery, signal handlers), not
  just the happy-path Quit.

- **Link.tsx** — always emits the OSC 8 escape sequence unconditionally
  (never gates on `supportsHyperlinks()`), because terminals that don't
  understand OSC 8 silently strip it with no visible cost, whereas gating it
  previously broke click-to-open in Apple Terminal (which isn't on the OSC-8
  allowlist but does forward the metadata to their in-process click
  dispatcher). **Direct lesson for Atlas**: if/when Atlas emits OSC 8 links,
  don't gate emission on a capability check — emit unconditionally, since
  the cost of a terminal ignoring it is zero and the cost of wrongly gating
  it off is a silently broken feature on an actually-capable terminal.

- **NoSelect.tsx** — a declarative wrapper (`fromLeftEdge` prop) for marking
  gutter regions (line numbers, diff +/- sigils) as excluded from
  click-drag text selection, so copying a diff yields clean pasteable code
  without the gutter noise. Ties to the selection.ts logic; only relevant if
  Atlas builds mouse-selection support.

- **ClockContext.tsx** — a shared render-tick clock with **focus-aware tick
  rate**: normal `FRAME_INTERVAL_MS` while the terminal has focus, doubled
  (`BLURRED_FRAME_INTERVAL_MS = FRAME_INTERVAL_MS * 2` per the sourcemap) while
  blurred, and the interval is torn down entirely when no subscriber has
  requested `keepAlive`. All subscribers in the same tick see an identical
  `tickTime` snapshot (kept animations synchronized). **Directly relevant to
  Atlas's existing render-throttle tick**: if Atlas doesn't already halve its
  tick rate on terminal blur, this is a cheap CPU-saving win, gated on
  DECSET 1004 focus events (see terminal-focus-state.ts above) — though it
  requires wiring up focus-event tracking first, which Atlas doesn't have.

- **ErrorOverview.tsx** — a crash screen that reads the throwing file off
  disk (via `stack-utils` + `code-excerpt`) and renders a few lines of
  source around the crash site with the offending line highlighted red — a
  nice-to-have polish idea for Atlas's panic/error display, low priority.

- **Button.tsx** — a render-prop pattern: `children` can be a function
  `(state: {focused, hovered, active}) => ReactNode`, letting the caller
  style based on interaction state while Button itself stays unstyled. A
  clean API idea if Atlas ever builds a reusable interactive-widget
  abstraction on top of Bubbles.

- **TerminalFocusContext.tsx / TerminalSizeContext.tsx / StdinContext.ts /
  AppContext.ts / CursorAdvanceContext.ts / CursorDeclarationContext.ts /
  Spacer.tsx / Newline.tsx / RawAnsi.tsx** — pure React-context/reconciler
  plumbing with no Bubbletea equivalent needed (Bubbletea's Model/Update/View
  already is the "context"). One exception: **RawAnsi.tsx** documents a
  perf-motivated escape hatch — when content is *already* ANSI-escaped and
  width-wrapped by an external renderer (their example: a native diff
  colorizer), it bypasses the normal parse→layout→re-serialize round trip
  entirely and hands the string straight to the output writer. If Atlas ever
  pipes large pre-rendered ANSI blocks (e.g. syntax-highlighted diffs from an
  external tool) through Glamour/Lipgloss and finds that a bottleneck, "skip
  re-parsing already-rendered ANSI, pass it straight to the terminal writer"
  is the applicable idea — though Bubbletea's View() model doesn't have the
  same reconciler round-trip cost this was fixing in Ink's React tree.

---

## 3. termio/ (12 files) — confirmed largely redundant with Atlas's Go deps

Read all 12 (ansi.ts, csi.ts, dec.ts, esc.ts, osc.ts in full; sgr.ts,
tokenize.ts, parser.ts, types.ts skimmed for structure; osc.test.ts,
parser.test.ts, tokenize.test.ts skimmed). This is a complete, from-scratch
ANSI/CSI/OSC/SGR parser+generator (semantic action types inspired by
Ghostty's `action.zig`, C0 control table, CSI param/intermediate/final byte
ranges, SGR parameter parsing with both `;` and `:` separator support,
DEC private-mode set/reset helpers). **Atlas already gets equivalent coverage
from its existing Go dependency chain** (bubbletea's own input driver,
lipgloss's `muesli/termenv`/`ansi` for SGR generation, and Go's terminal
raw-mode handling via `golang.org/x/term`) — there is no gap here worth
porting wholesale.

Two specific pieces stood out as worth extracting individually rather than
the parser as a whole:

- **osc.ts's clipboard logic (`setClipboard`, `shouldUseNativeClipboard`,
  `getClipboardPath`)**: a genuinely subtle, well-documented fallback chain
  for OSC 52 clipboard writes — native tool (pbcopy/wl-copy/xclip/xsel/
  clip.exe) fires in parallel as a safety net EXCEPT on terminals with known-
  good native OSC 52 support (Ghostty/kitty/WezTerm/Windows Terminal/VS Code)
  where racing a native tool against the terminal's own OSC 52 write actively
  corrupts the clipboard (their cited case: wl-copy on Wayland forking a
  daemon that races the terminal's write, ~30% empty-clipboard rate). Also:
  inside tmux, `tmux load-buffer -w -` is preferred over raw OSC 52 (works
  over SSH, survives detach/reattach), with `-w` dropped specifically for
  iTerm2 because tmux's own OSC-52 emission crashes iTerm2 over SSH. If Atlas
  ever adds "copy to clipboard" for code blocks/messages, this decision tree
  (SSH → OSC 52 only; tmux → load-buffer; native terminal → native tool
  unless it's on the race-prone list) is a complete, battle-tested reference.
- **DEC mode constants + mouse-tracking presets (dec.ts)**: the mapping from
  a named preset (`off`/`wheel`/`buttons`/`all`) to which of DEC 1000/1002/
  1003/1006 to enable, and WHY (1003's all-motion hover reporting is what
  spams tmux with false clipboard-probe noise) — useful if Atlas exposes
  mouse support as a user-facing toggle rather than an all-or-nothing switch.

---

## 4. utils/ (11 files)

- **env.ts** — `detectTerminal()`'s precedence chain is a clean, portable
  reference even though Atlas will do its own version in Go: check
  `CURSOR_TRACE_ID` → `TERM=xterm-ghostty` → `TERM` contains `kitty` →
  `TERM_PROGRAM` → `TMUX` → `STY` (GNU screen) → `KITTY_WINDOW_ID` →
  `WT_SESSION` → raw `TERM`. Also `supportsOsc52Clipboard()`'s terminal
  allowlist (ghostty/kitty/WezTerm/windows-terminal/vscode) ties directly
  into the clipboard race-avoidance logic above.
- **intl.ts** — wraps `Intl.Segmenter` for grapheme/word clustering,
  `Intl.RelativeTimeFormat` (cached per style/numeric combo), timezone and
  system-locale detection. Go's Unicode segmentation equivalent
  (`rivo/uniseg`) is already a transitive dependency via lipgloss/bubbles —
  **no gap**, but confirms that if Atlas ever needs relative-time formatting
  ("2 minutes ago") there's no Go stdlib equivalent to `Intl.RelativeTimeFormat`
  and it would need either a small hand-rolled formatter or a package like
  `dustin/go-humanize`.
- **earlyInput.ts** — captures raw stdin keystrokes typed *before* the app's
  own input loop is ready (buffers them in raw mode, handles backspace via
  grapheme-aware deletion, Ctrl+C/Ctrl+D, and swallows escape sequences),
  then hands the buffered text to the app once it's ready to seed the input
  box. Skips this entirely in `-p`/`--print` (piped) mode. **Worth
  considering for Atlas** if there's any perceptible gap between process
  start and the input box accepting keystrokes (e.g. slow provider/config
  init) — a real, if uncommon, source of "the app feels laggy" complaints.
- **envUtils.ts** — one-liner `isEnvTruthy()` (`1/true/yes/on` case/whitespace
  -insensitive). Trivial, but Atlas should standardize on one such helper if
  it has several env-var-boolean parsing call sites (Hermes clearly
  standardized this exact truthy-check across `HERMES_TUI_DIM`,
  `HERMES_TUI_FORCE_OSC52`, `HERMES_TUI_DISABLE_MOUSE_CLICKS`, etc.).
- **debug.ts / log.ts** — trivial stubs/env-gated `console.error` wrappers,
  no action.
- **semver.ts** — thin wrapper preferring `Bun.semver` over the npm `semver`
  package for TERM_PROGRAM_VERSION comparisons (Ghostty 1.2.0+, iTerm2
  3.6.6+ feature gates). Go has no direct need — version comparisons for
  terminal feature-gating in Go would use `golang.org/x/mod/semver` or a
  simple manual parse; low priority since Atlas likely won't need
  per-terminal-version feature gates as granular as Hermes's.
- **sliceAnsi.ts / execFileNoThrow.ts / fullscreen.ts** — sliceAnsi is
  redundant with what `lipgloss`/`muesli/ansi` already provide for
  ANSI-safe substring slicing (with an LRU cache for a profiled 18%-of-
  render-time hotspot — same "cache the wrap/slice result" lesson as
  wrap-text.ts). execFileNoThrow is generic subprocess plumbing, not
  TUI-specific. fullscreen.ts is a single one-line env-var check.

---

## Top recommendations for Atlas (ranked by impact/effort)

1. **Implement OSC 11/OSC 10 terminal background+foreground detection**
   (from `terminal.ts`). Highest confidence fix for "may render wrong on
   Windows cmd.exe" — Atlas currently trusts termenv's own guess with zero
   ground-truth check. Port `parseOscColor()`'s X11-color-spec parsing
   directly (it's ~40 lines, pure string math, no external deps) and use the
   timeout-free query pattern below to send/receive it.

2. **Port the DA1-sentinel query pattern** (from `terminal-querier.ts`) as
   the delivery mechanism for #1 (and any future terminal capability probe —
   truecolor support, Kitty keyboard protocol, XTVERSION). This is the
   piece that makes #1 reliable without a magic-number timeout: batch your
   query with a `CSI c` sentinel, and any terminal that doesn't answer your
   query answers DA1 instead, at which point you know to fall back.

3. **Audit dim/faint rendering against `shouldUseAnsiDim()` (Text.tsx)**.
   If Atlas emits `Faint(true)` anywhere, verify it isn't silently ignored on
   Apple Terminal / other low-VTE terminals, and consider a literal
   muted-color fallback the way Hermes does, gated on a similar env probe
   (`TERM_PROGRAM == Apple_Terminal` or missing `VTE_VERSION`).

4. **Investigate the real fix for Atlas's emoji problem separately from
   width math.** `stringWidth.ts` confirmed Hermes does NOT solve
   "will this glyph render as the wrong codepoint/tofu" — it only solves
   cursor/layout width accounting (which Atlas's Go deps, go-runewidth/
   uniseg, likely already handle adequately). The actual fix for "some
   symbols render wrong" is a font/terminal-capability allowlist that Hermes
   itself doesn't appear to have either — this needs separate research, not
   a port from this codebase. Re-enabling *some* symbols safely (rather than
   banning all emoji) would require Atlas to build its own allowlist (e.g.
   based on Unicode block + a small manually-curated "known safe on Windows
   Terminal / cmd.exe" set), which is genuinely new work, not something to
   copy from Hermes.

5. **Adopt Hermes's explicit sticky-scroll boolean pattern** (from
   `ScrollBox.tsx`) in whatever wraps Atlas's `bubbles/viewport` for the
   chat transcript: track "pinned to bottom" as a flag cleared by any manual
   scroll and set only by an explicit "scroll to bottom" action, rather than
   inferring it from a position comparison every frame. Also consider the
   `scrollToElement`-style "scroll to a specific message, resolved at render
   time" pattern if Atlas ever adds jump-to-message/jump-to-search-result.

6. **OSC 52 clipboard support**, if/when Atlas adds "copy to clipboard": the
   full decision tree in `osc.ts`'s `setClipboard`/`shouldUseNativeClipboard`
   (SSH → OSC-52-only; tmux → `load-buffer -w`, with `-w` dropped for
   iTerm2; native terminal → skip the racy native-tool safety net only on a
   small allowlist of terminals with known-good native OSC 52) is directly
   reusable as a spec, translated to Go's `os/exec` for the native-tool
   fallback (`pbcopy`/`wl-copy`/`xclip`/`xsel`/`clip.exe` — same list Windows
   needs `clip.exe`, relevant since the user is on Windows).

7. **Always emit OSC 8 hyperlinks unconditionally, never gate on capability
   detection** (from `Link.tsx`) — cheap correctness lesson if/when Atlas
   emits clickable links in tool output.

8. **Halve the render/animation tick rate on terminal blur** (`ClockContext.tsx`
   + `terminal-focus-state.ts`) — cheap CPU win, but requires first adding
   DECSET 1004 focus-event tracking (`CSI ?1004h`, watching for `CSI I`/`CSI O`
   in the input stream), which Atlas doesn't have today. Medium effort for a
   background-only CPU saving, not a visible fix — lower priority than 1-4.

9. **`supports-hyperlinks.ts`'s manual terminal allowlist** (checking both
   `TERM_PROGRAM` and `LC_TERMINAL`, the latter surviving inside tmux) is a
   small, complete reference if Atlas needs hyperlink-support detection
   before deciding whether recommendation #7 even needs a fallback path.

10. **`tabstops.ts`'s ANSI-safe 8-column tab expansion** — small and
    self-contained; worth a direct Go port only if Atlas discovers raw tool
    output containing literal tabs rendering misaligned today.

Explicitly deprioritized: the full `termio/` ANSI/CSI/OSC/SGR parser
(redundant with Atlas's existing Go deps), `selection.ts`/`searchHighlight.ts`
(deep but tied to a custom screen-buffer model Atlas doesn't have — high
effort, and mouse text-selection wasn't reported as a pain point), and
`wrapAnsi.ts`/`sliceAnsi.ts` (Lipgloss already covers ANSI-aware
wrap/truncate/slice; only the "add an LRU cache if scroll perf becomes a
problem" lesson is portable, and only if/when that becomes a measured
problem).
