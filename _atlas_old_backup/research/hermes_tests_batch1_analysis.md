# Hermes-Agent ui-tui `__tests__` — Batch 1 Analysis (60 files)

Source: `NousResearch/hermes-agent`, `ui-tui/src/__tests__/`. All 60 assigned files were
read in full (see manifest at end). This is behavior/design intelligence for Atlas
(Go/Bubbletea TUI), not code to copy — patterns and constants only.

## Terminal-compat: emoji, truecolor, platform (HIGHEST PRIORITY for Atlas)

**`emoji.test.ts` — `ensureEmojiPresentation`**
- Core rule: certain codepoints (⚠ ℹ ❤ ✔ etc.) default to **text presentation** in
  Unicode and render as thin monochrome glyphs unless followed by **VS16** (`U+FE0F`,
  the emoji variation selector). Hermes injects VS16 right after every "text-default"
  emoji codepoint that lacks one.
- Idempotent: does nothing if VS16 already present (checked by exact match, not just
  presence-anywhere).
- ZWJ sequences: VS16 must be inserted **before** a following ZWJ, e.g. `❤ + ZWJ + 🔥`
  (heart-on-fire) becomes `❤️‍🔥` — insert VS16 between the base char and the ZWJ, not
  after the whole sequence, or the ligature fails to form on many terminals.
- Explicit text selector (VS15, `U+FE0E`) is left alone — never double-inject.
- Keycap sequences (`1⃣` = digit + `U+20E3`) are untouched since the base digit isn't a
  text-default emoji.
- Returns the **same object reference** when no change is needed (perf/memoization
  discipline) — a design detail worth mirroring if Atlas memoizes rendered strings.
- **Implication for Atlas**: Atlas stripped emoji wholesale due to cmd.exe bugs. Hermes'
  actual approach is the opposite direction of the same problem (ensuring correct
  presentation) — confirms that the deeper issue is codepoint-presentation ambiguity,
  which is exactly the class of bug that also causes width-miscalculation crashes on
  Windows. If Atlas ever re-enables limited emoji, the VS16-injection table is the
  concrete fix, and it must ship paired with correct width math (an emoji+VS16 is still
  width 2, but naive `len()` on the flag-emoji/ZWJ forms will disagree with terminal
  rendering — a likely source of Atlas's original crashes).

**`forceTruecolor.test.ts`**
- Env keys involved: `COLORTERM`, `FORCE_COLOR`, `HERMES_TUI_TRUECOLOR`, `NO_COLOR`,
  `TERM`, `TERM_PROGRAM`.
- Default: does **not** force truecolor (no env mutation) unless something opts in.
- **Apple Terminal special case**: on `TERM_PROGRAM=Apple_Terminal`, truecolor is
  *downgraded* even if the env already advertises `COLORTERM=truecolor` /
  `FORCE_COLOR=3` — because pre-Tahoe Apple Terminal lies about truecolor support.
  `shouldDowngradeAppleTerminalTruecolor()` is a named, independently-testable
  predicate — a good pattern (don't bury platform quirks inside a giant if-chain).
- `HERMES_TUI_TRUECOLOR=1` explicit opt-in sets `COLORTERM=truecolor` +
  `FORCE_COLOR=3` and overrides the Apple downgrade AND any existing `FORCE_COLOR`
  value (even `FORCE_COLOR=0`).
- `HERMES_TUI_TRUECOLOR=0` opt-out prevents forcing even when other signals are
  present.
- `NO_COLOR` always wins over `HERMES_TUI_TRUECOLOR=1` (respects the NO_COLOR spec).
- Non-Apple terminals that already advertise truecolor are left untouched (no
  unnecessary env rewrite).
- **Implication for Atlas**: Atlas's truecolor-detection uncertainty on Windows should
  follow the same layered-override order: `NO_COLOR` > explicit user toggle > terminal
  self-report, with a **specific named exception list** (Windows equivalents: legacy
  `cmd.exe`/`conhost.exe` claim ANSI support via `ENABLE_VIRTUAL_TERMINAL_PROCESSING`
  but historically mis-render truecolor in some legacy consoles — worth an explicit
  downgrade predicate analogous to `shouldDowngradeAppleTerminalTruecolor`, keyed off
  `WT_SESSION` presence/absence — WT_SESSION set means real Windows Terminal, absent
  usually means legacy conhost).

**`platform.test.ts`** — extremely detailed modifier-key semantics, useful even though
Atlas is a different framework:
- `isActionMod`: on macOS, both `key.super` (Cmd, kitty-protocol) and `key.meta`
  (legacy-terminal Cmd signal) count as the action modifier; elsewhere only `ctrl`.
- **Critical distinction Hermes learned the hard way (many "round-N Copilot review"
  regressions)**: on macOS, `key.meta` is heavily overloaded — it fires for literal
  Alt/Option, for Cmd on legacy (non-kitty) terminals, AND for bare Esc on some
  terminals. Because of that ambiguity, user-*configured* custom bindings (e.g. a
  custom voice-record key `ctrl+o`) must **never** accept `key.meta` as a Cmd
  fallback — only the hardcoded default binding (`ctrl+b`) gets the Cmd-via-meta
  legacy fallback, and even that fallback requires literal `key.super`, not `key.meta`.
  Every custom binding is checked for **extra modifier bits** too (Ctrl+Alt+O must NOT
  fire a `ctrl+o` binding; Shift-held variants must not fire either) — chords are
  exact-match, not "at least these bits."
- Reserved chords: `ctrl+c`/`ctrl+d`/`ctrl+l` are intercepted by the global input
  handler before any user-configurable binding gets a chance, so a config parser
  should refuse to accept them and fall back to default rather than silently
  advertising a dead shortcut. `ctrl+x` is conditionally reserved (only during
  queue-edit mode).
- `super+{c,d,l,v}` and `alt+{c,d,l}` are reserved on macOS (copy/exit/clear/paste
  action-mod chords) but are safe on Linux/Windows because those platforms' action
  modifier is Ctrl, not Super/Alt — so a *cross-platform config reservation list must
  itself be platform-conditional*.
- Named-key parsing table: `space`, `enter`/`return`, `tab`, `escape`/`esc`,
  `backspace`, `delete`/`del` are all accepted with common aliases.
- Multi-modifier configs (`ctrl+alt+r`, `cmd+ctrl+b`) are rejected outright — no
  attempt to guess which modifier "wins"; fall back to documented default rather than
  silently picking one.
- Bare-char configs without an explicit modifier (`o`, `space`) are rejected, mirroring
  the reference CLI's contract that raw single keys require an explicit modifier.
- Ambiguous modifier spellings (`meta`, `cmd`, `command` in config) are rejected as
  unparseable on the grounds that `meta` is ambiguous on the wire — users should spell
  `super`/`win` for the platform action modifier.
- Formatting: `super`/`win` render as **"Cmd"** on darwin and **"Super"** elsewhere —
  the same internal representation, platform-dependent display string.
- `isMacActionFallback`: macOS-only fallback that routes bare `Ctrl+K`/`Ctrl+W` to
  readline kill-to-end / delete-word — a no-op on other platforms where
  `isActionMod` handles it directly.
- **Implication for Atlas**: if Atlas supports configurable keybindings across
  platforms, this test file is a direct blueprint for the edge cases to guard:
  exact-chord matching (no accidental supersets), platform-conditional reserved-chord
  lists, explicit-modifier-required parsing, and rejecting ambiguous modifier names
  instead of guessing.

## IME / international input

**`imeVietnameseTelex.test.tsx`** — end-to-end regression tests built from *real
captured byte streams* from OpenKey and EVKey (Vietnamese Telex IMEs) on macOS.
- Telex IMEs commit a finished syllable as a **burst**: some number of backspace bytes
  (`\x7f`) optionally preceded by a **U+202F (NARROW NO-BREAK SPACE)** marker byte,
  followed by the recomposed character(s), often all in a single fused stdin chunk
  (e.g. `'\x7f\x7fạnh'` — two backspaces then "ạnh" in one read).
- Two real captured root causes fixed in Hermes: (1) their keypress parser was
  splitting a fused control-byte+text chunk and dropping the text half; (2) their text
  input widget deferred multi-char (IME/paste) inserts through a **16ms key-burst
  commit window** (`FRAME_BATCH_MS`) that raced against re-renders, dropping tail
  characters. Fix: commit multi-character inserts **synchronously**, not via the
  deferred burst path.
- **Fast-echo suppression window**: after any full Ink repaint (which an IME
  recompose burst forces via the NNBSP marker), the very next backspace must be
  "fast-echo suppressed" for **60ms** — i.e. it must NOT go through the optimistic
  fast terminal-echo path (writing `\b \b` directly) because that would strand the
  NNBSP as a visible stray space. After 60ms, fast-echo resumes normally.
- Tests assert against the *immediate* value right after the last stdin read (no
  trailing wait), specifically to prove commits are synchronous and not dependent on
  a timer firing.
- **Implication for Atlas**: if Atlas's Bubbletea input model ever sees garbled or
  dropped characters from CJK/Vietnamese IME users, look for: (a) a keypress parser
  that splits fused control+text bytes, (b) any deferred/batched commit path for
  multi-byte inserts that races a repaint, (c) no "just repainted, suppress optimistic
  echo for N ms" guard. This is a narrow but real correctness class most terminal UIs
  never test for.

## Composer / input mechanics

**`cursorDriftRegression.test.ts`** — pinned regression for multi-line cursor drift.
Root cause: a hand-rolled word-wrap algorithm for cursor-position math disagreed with
the actual rendering library's (`wrap-ansi`) wrap points, so the displayed cursor drifted
from the real text end, worse on narrow terminals. Fix: cursor-position layout must
**source its line-break decisions from the exact same wrapping function used for
rendering** — never maintain a second independent wrap implementation. Concretely
verified: exact-fill text (text length == cols) does NOT wrap to a phantom next line;
"branch investigate" at cols=20 stays on one line; "hello world" at cols=8 wraps to
`["hello ", "world"]` with cursor landing on line 1, col 5, not a phantom line 2.
**Directly relevant to Atlas** if Bubbletea's textarea does its own cursor math instead
of relying on lipgloss's wrap function.

**`composerHighlights.test.ts`** — token-highlighting rules for the composer input,
character-exact:
- Highlighted token kinds: `/command` at any word start, `@file:`/`@diff`/`@staged`
  references (including backtick-quoted values with spaces, e.g. `@file:\`my notes.md\``),
  and paste/attachment placeholders `[[ Image 1 ]]` / `[[ log.. [3 lines] ]]`.
- A **bare trailing slash** (`/`) or a **half-typed token** (`/wor`, `@fi`) is
  highlighted live so the accent color tracks the caret while typing, not only once
  the token closes.
- Explicitly NOT highlighted: absolute paths (`/usr/local/bin`), relative paths
  (`src/foo/bar`), bare math-looking slashes (`3 /4`), standalone `/`, email addresses.
- Round-trip guarantee: concatenating all returned segments' text always reconstructs
  the original string exactly (critical invariant for any highlighter).
- `highlightsStable(prev, next)`: a **fast-echo bypass gate** — highlighting can skip
  a full re-render pass only if every character already on screen keeps the same
  color (i.e., a token just *grew*, like `/wor` → `/work`). If a keystroke would
  recolor an *already-rendered* cell (e.g. `[[ a ]` → `[[ a ]]` completing the token,
  or `/usr` → `/usr/` making a highlighted slash into a plain path), the bypass is
  blocked and a full repaint is required. This is a concrete, testable optimization
  strategy for incremental terminal repaints that Atlas's composer could adopt if it
  does similar per-character coloring.

**`completionApply.test.ts`** — slash/argument completion-application semantics:
- Replacement is driven by an explicit `replace_from` index from the completion
  source, not a naive "does input start with /" check — this matters for mid-message
  completions where the row itself may or may not carry a leading slash independent
  of the input's leading slash.
- **Enter-key swallow rule**: `completionToApplyOnSubmit` must return `null` (don't
  eat Enter) when applying the completion row would be a no-op OR would only append a
  **trailing space** to an already-complete command — otherwise Enter appears to do
  nothing the first time because the popover consumed it to "complete" a command that
  was already fully typed. This exact bug class (Enter swallowed by a completion popup
  when text is already correct) is common and worth testing explicitly in Atlas.

**`attachments.test.ts`** — token expansion for pastes/images:
- Collapsed paste labels (`[[ hello.. [3 lines] .. world ]]`) expand back to full
  content at submit time; repeated identical labels expand in **submission order**
  (first occurrence gets first paste, etc.) — order-sensitive, not content-matched.
- Image tokens resolve to **empty string** (not a placeholder) when expanded — "the
  gateway already holds the file" — and any double space left by removing an
  image-only token mid-sentence is cleaned up.
- Image indices are **never reused** after deletion — deleting `[[ Image 1 ]]` and
  attaching a new image gives it index 3 (next-after-max), not 1, to avoid
  `expandTokens` resolving two different files to one stale label.

**`inputSelectionClipboard.test.ts` / `inlineSlashSkill.test.ts`**:
- Mid-message `/` completion only triggers after whitespace/newline, never at
  position 0 of a non-empty line where a command is already anchored, and a
  mid-message command reference (e.g. `/personality alic`) is NOT completed with
  further args — inline references are name-only, argument parsing is reserved for
  position-0 invocations.
- Selection-aware clipboard shortcuts (copy/cut) only fire when there's an active,
  non-collapsed composer text selection (`start !== end`); otherwise the shortcut key
  is left free for other bindings.

## Status bar / chrome (statusRule, statusBarTicker, appChromeStatusRule*)

**`statusRule.test.ts`** (cross-check target explicitly called out):
- `statusRuleWidths(cols, cwdLabel, reservedWidth?)` returns `{leftWidth,
  separatorWidth, rightWidth}` that must always sum to `<= cols`.
- The cwd/branch segment is the **lowest priority** and is truncated/dropped first as
  the terminal narrows; at cols=2 it's fully `{leftWidth: 2, rightWidth: 0,
  separatorWidth: 0}`.
- Width budgeting for CJK cwd/branch text uses **display width** (`stringWidth`, i.e.
  wcwidth-style double-width awareness), not UTF-16 code-unit length — the exact class
  of bug Atlas needs on Windows too since Go's naive `len(string)` / `utf8.RuneCountInString`
  both disagree with terminal display columns for CJK/emoji.
- Priority order for shedding status-bar segments as the terminal narrows (documented
  via `statusBarSegments`): perf read-outs (cache-hit %, latency, tokens/sec) go
  first at fixed breakpoints (**108, 100, 94 cols** disable tps/latency/cacheHit
  progressively), then `bar/duration/compressions/voice/bg/subagents` shed in that
  declared array order as cols shrink through **95, 87, 83, 79, 75, 71**, and the
  context-usage bar collapses to a compact token count below **~60 cols**.
- `busyIndicatorWidth(style, timed)`: the `unicode` spinner style reserves exactly
  **1 column** (bare braille spinner, no verb text) vs. wider styles like `kaomoji`
  that carry a glyph+verb; an elapsed-time tail only adds width when the turn is
  actually timed.
- Session-title display: pinned at the far-right edge, replacing (not appending to)
  the cwd label when a title exists. A past bug used a raw full-saturation accent hue
  as a **background** fill behind near-white text — contrast ratio ~1.5-2:1
  (unreadable); fixed by putting the accent color on the **text** instead of as a
  background fill, following the theme convention that raw accent hues are never
  solid fills (only softened tints are).
- MCP/session-count segments are click targets (`onClick` handlers wired directly
  into the status text elements) — a UI affordance Atlas's Bubbletea status bar
  likely can't replicate directly (no mouse click targets in most terminals without
  explicit mouse-tracking mode), but worth knowing the reference design assumes
  clickable status segments when mouse mode is on.
- Notices (credits warnings etc.) **replace** the idle status verb slot but never
  render while `busy` (a running turn always wins the display slot); the notice's
  color is driven by its `level` (`error`→theme error, `success`→statusGood, etc.),
  and long notices truncate with `wrap: 'truncate-end'` on a `flexShrink: 1` container
  so the model/context segments never get pushed off-screen by an overlong message.
- Battery indicator: `🔋` glyph on AC-off, `⚡` while charging; colored by a
  `category` field (`critical`→statusCritical, etc.); omitted entirely when
  `battery: null` or `available: false` (desktop/server has no battery — self-hides
  rather than showing "0%").

**`appChromeStatusRuleDevCredits.test.ts`**: a dev-only credits banner
(`HERMES_TUI_DEV_CREDITS`-gated) coexists alongside a regular notice rather than
replacing it, showing a `Δ` delta-spend segment converting micros→cents for display.

**`appChromeBlockedTimers.test.tsx`** (very detailed timer-lifecycle contract):
- Two live 1-second clocks back the idle status rule (SessionDuration + IdleSince);
  busy mode instead arms a FaceTicker glyph/verb rotation at a **2500ms cadence** plus
  its own elapsed clock.
- **Occlusion-based timer pausing**: when a full-screen/modal overlay genuinely covers
  the status rule (model picker, pager, pet picker, plugins hub, sessions list, skills
  hub, or the generic "widget" slot), the rule's timers are torn down entirely — saves
  needless work and avoids invisible-but-ticking intervals.
  - Overlays that render in **normal document flow** and merely push the rule down
    (approval prompt, sudo prompt, billing, clarify, confirm, secret, journey, agents,
    subscription) do **NOT** pause the clocks — the rule is still visible, just moved.
  - An "ambient dock" widget slot also does not pause anything (in-flow, doesn't
    cover the rule).
  - A **bottom**-positioned status bar is never covered by a floating overlay that
    grows upward from the composer (`position: absolute; bottom: 100%`), even when
    the same overlay *would* cover a **top**-positioned bar — occlusion depends on
    both overlay geometry and status-bar placement config.
- **Re-sync-on-reveal, not resume-from-stale**: when an overlay closes and the paused
  clocks resume, the elapsed-time read-outs are recomputed from the current wall
  clock, not resumed from the value frozen when the overlay opened — a naive
  "pause/resume the interval" implementation would freeze the displayed elapsed time
  for the entire duration the overlay was open, which is the exact regression this
  guards against.
- **Implication for Atlas**: if Atlas's Bubbletea status bar has any `tea.Tick`-driven
  countdown/elapsed timers, and any full-screen overlay/modal, verify: (1) timers stop
  entirely when truly occluded (save CPU/render churn), (2) elapsed-time display
  recomputes from wall-clock on reveal rather than resuming a frozen counter, (3) an
  overlay that merely repositions content (not occludes it) must NOT stop the timer.

**`statusBarTicker.test.ts`**: verb text for the busy indicator is padded to a fixed
width (`VERB_PAD_LEN`) with a **trailing ellipsis attached directly to the verb**
(`"reading…"` padded, not `"reading" + padding + "…"`), so all busy-verb strings
render at identical width without the ellipsis visually drifting.

## Gateway / RPC / WebSocket transport

**`gatewayClient.test.ts`** (very detailed — most transferable to Atlas's own
gateway/backend transport layer if any):
- **Heartbeat**: `WS_HEARTBEAT_INTERVAL_MS` ping / `WS_HEARTBEAT_DEAD_MS` dead-timeout
  constants; only pings when the backend advertises the heartbeat capability in its
  `gateway.ready` payload (`payload.heartbeat: true`) — an older backend that omits
  this is never pinged, avoiding protocol errors against legacy servers.
- **Auto-reconnect**: exponential backoff bounded by `RECONNECT_BASE_MS` /
  `RECONNECT_MAX_MS`; reconnects on missed heartbeat ack or socket close, but
  explicitly does **not** reconnect after an intentional `kill()` (disposed flag)
  and does not double-reconnect if an `exit` event handler itself calls `start()`
  again (a subtle self-reentrancy guard).
- **Buffered-event replay ordering (issue #36658)**: in "attach" mode (joining an
  already-running gateway), the server may replay buffered events (e.g.
  `gateway.ready`) **before** the client's subscribe effect runs. The fix: `drain()`
  must defer flushing buffered events to a **microtask**, never emit them
  synchronously inside `drain()` itself — synchronous emission inside a React
  effect's own execution can trigger "too many re-renders" by cascading setState
  calls into the same commit. Additionally, **FIFO order must be preserved** even
  when a live event arrives in the gap between `drain()` returning and the deferred
  microtask running — the live event must queue *behind* the earlier-buffered one,
  not jump the queue.
- **URL secret redaction**: connection URLs carrying `?token=...&channel=...` are
  redacted in logs (`ws://host/api/ws?***`) even when the URL is malformed enough
  that `new URL()` itself throws (e.g. port > 65535) — the redaction fallback must
  not rely on being able to parse the URL structurally; it degrades to blunt secret
  stripping. Also verified redaction survives constructor-throw paths for both the
  primary and "sidecar" (pub/sub mirror) socket.
- Sidecar/mirror websocket: event frames are mirrored verbatim (same JSON string) to
  a secondary "sidecar" socket if configured, for pub/sub fan-out to other
  observers (e.g. a dashboard).
- RPC rejection wording differs by cause: `"gateway websocket closed (<code>)"` vs.
  `"gateway closed"` (explicit kill) vs. `"gateway attach url changed"` (URL rotated
  mid-flight, in-flight requests against the stale URL are rejected rather than
  silently orphaned).

**`gatewayRecovery.test.ts`**: crash-loop recovery budget — `planGatewayRecovery`
tracks a rolling list of recovery-attempt timestamps; `GATEWAY_RECOVERY_LIMIT`
attempts within `GATEWAY_RECOVERY_WINDOW_MS` are allowed before recovery gives up
(`recover: false`) rather than spawn-storming; attempts older than the window are
pruned so recovery **re-arms** after a quiet period. The recovery *target* session id
is remembered across repeated crash-loop attempts even while the "live" session id is
transiently null between crashes.

**`createGatewayEventHandler.test.ts`** (huge file, ~1400 lines) — selected
load-bearing behaviors:
- **Notice ("credits" banner) state machine**: notices arriving mid-turn (`busy`)
  are **held** and only flush at turn-end (message.complete/interrupt/error), not
  shown immediately — avoids visually interrupting an in-progress response. A
  `ttl`-kind notice's self-expire timer starts counting from when it becomes
  **visible**, not from when it arrived (so an 8-second TTL notice held for a
  30-second turn still gets its full 8 seconds once shown). Latest-arriving notice
  wins if two arrive mid-turn; the earlier one's stale TTL timer must not later wipe
  the newer notice (separate/cancelled timers per notice, not a shared timer slot). A
  matching `notification.clear` for the currently-pending (not-yet-shown) notice
  drops it before it ever surfaces.
- **Raw-text-over-rendered-ANSI precedence (#16391)**: the gateway can send both a
  raw `text` and a Rich-rendered `rendered` (with embedded ANSI) field; the TUI must
  always prefer raw `text`, only falling back to `rendered` when `text` is absent —
  otherwise visible escape codes leak into the transcript. Delta accumulation
  similarly always appends raw `text`, ignoring `rendered` deltas entirely, to avoid
  replacing the whole accumulated buffer with just the latest fragment.
- **Inline diff anchoring**: a diff block produced mid-turn is spliced into the
  transcript as its own segment **exactly where the tool executed** (between the
  narration before and after), not appended to the end of the final response — and
  is **de-duplicated** if the assistant's own final text narrates/embeds the same
  diff verbatim (checked via matching fenced-code-block count), to avoid showing the
  same patch twice.
- **Skin/theme polarity switching**: on `skin.changed`, dark-authored `colors` block
  is used unless `HERMES_TUI_BACKGROUND` env signals a light terminal, in which case a
  separate hand-tuned `light_colors` block is preferred over automatic light/dark
  adaptation of the dark palette. When a skin owns the background it emits **both**
  `OSC 11` (background) and `OSC 10` (foreground) so the terminal's *default*
  foreground (used by borders/plain text) matches the skin — omitting the paired
  OSC-10 leaves default-fg elements at the host terminal's own near-black on a
  now-black skin background, i.e. invisible. Dropping the skin resets **both** via
  `OSC 111`/`OSC 110` (reset-to-default), not just clearing one side.
- Session auto-resume precedence at startup: explicit env `STARTUP_RESUME_ID` beats
  config's `tui_auto_resume_recent`; a crash-recovery sid (`recoverSidRef`) is
  consumed **once** then cleared so a later ordinary restart doesn't keep resuming
  the same crashed session; any RPC failure during the resume-decision chain falls
  back to forging a brand-new session rather than blocking startup.
- **Post-interrupt event suppression**: after `interruptTurn()`, late-arriving
  reasoning/tool/todo events from the still-unwinding backend turn are dropped until
  the next `message.start` — this is the fix for a reported bug where Ctrl-C
  appeared to be ignored because stale events kept populating the UI for ~1s after
  the interrupt. A `keepBusy` interrupt mode holds `busy: true` through the drain so
  a race between the interrupt settling and the cancelled turn's own
  `message.complete` doesn't duplicate/leak the cancelled turn's "Operation
  interrupted…" text into the transcript.
- Voice: a stop-phrase transcript ends voice mode without ever submitting a turn
  (explicit user intent to stop, not a message). `voice.submit_mode: 'draft'` leaves
  the transcript in the composer for editing instead of auto-submitting; an invalid
  submit_mode value falls back to direct-submit rather than silently doing nothing.

## MoA (mixture-of-agents) / subagents

**`moaProgressActivity.test.ts`**: progress lines like `"MoA: refs 2/3"` **replace in
place** as each reference completes (no stacking of stale progress lines), and swap
entirely to `"MoA: aggregating…"` once the aggregator phase starts.

**`createGatewayEventHandler.test.ts`** subagent handling: terminal statuses
(`timeout`, `error`) are sticky — late `start`/`spawn_requested` events for the same
subagent id must not clobber a terminal status back to running. Unknown/unexpected
status strings from `subagent.complete` normalize to `completed` rather than being
passed through raw. A one-time-per-turn "nudge toward /agents" hint appears on the
first delegation event of a turn, resets on the next `message.start`, is suppressed
while the `/agents` overlay is already open, but is NOT consumed (still fires later
in the same turn) if the overlay was open only briefly and then closed mid-turn — the
nudge "credit" isn't burned just because the user glanced at the dashboard early.

**`spawnHistoryStore.test.ts`**: same terminal-status normalization
(`timeout`/`error` preserved, unknown → `completed`) applies to on-disk snapshot
loading, mirroring the live event-handler logic — two independent code paths that
must agree on the same normalization rule.

## Markdown / math / text rendering

**`markdown.test.ts`**:
- Emphasis regex avoids matching intraword underscores (`snake_case_var`,
  `__init__`, `__name__`) but still allows intraword **asterisk** emphasis
  (`a*b*c` → italic "b") — underscore emphasis requires word boundaries, asterisk
  does not (matches CommonMark's actual disambiguation rule).
- Kaomoji-style `~!`/`~?` decorators are excluded from subscript/strikethrough
  matching (`~2~` still matches subscript, but `~! ... ~!` conversational tildes
  don't).
- Inline math (`$...$`, `\(...\)`) is matched as a **single opaque token** so
  markdown/subscript/superscript syntax characters *inside* the math span
  (`$P=a_n x^n$`) are never separately reinterpreted.
- Bare URLs get **readable slug-derived labels** synthesized from the URL path
  (e.g. `/things-to-do/puerto-rico-el-yunque-rainforest-adventure` →
  "Puerto Rico El Yunque Rainforest Adventure") instead of showing the raw URL, but
  an **authored** markdown link label always wins over both the slug fallback and any
  fetched page `<title>` — title-fetching is best-effort enrichment only for bare
  URLs or URL-as-label links, never overriding explicit authored text.
- CJK table column alignment bug: padding computed via naive character-length
  instead of true display-width drifted CJK header/body columns out of alignment by
  2 cells per CJK character — the general "CJK is double-width, string length lies"
  class of bug that's also relevant to Atlas's own Windows-console width handling.
- Prose foreground color must consistently be the **theme's** ink color, never the
  raw terminal-default foreground, even mid-line after an inline token switches
  color and back — a "one line, one consistent palette" invariant.

**`mathUnicode.test.ts`** (`texToUnicode` — LaTeX→Unicode for terminal rendering):
concrete substitution table worth knowing exists: Greek letters, set/logic operators,
`\mathbb`/`\mathcal`/`\mathfrak` blackboard-bold-style capitals, sub/superscripts
(falls back to parenthesized form `x^(iπ)` when a script has no Unicode glyph, and
strips to bare `^∞` when the body collapses to one already-substituted character),
`\frac{a}{b}` → `a/b` with parens added only when numerator/denominator are
multi-token, `\boxed{...}` wrapped in private-use sentinel chars for later highlight
styling, combining marks for `\overline`/`\hat`/`\vec`, and a "longest match first +
word-boundary lookahead" strategy so `\leqq` (unmapped) isn't corrupted by a
`\le`-prefix match, and unknown commands are always preserved verbatim rather than
partially mangled.

**`emoji.test.ts`** already covered above; **`text.test.ts`**/`stripInlineMarkup` not
separately grouped here (folded into markdown.test.ts summary since it's the same
regex family under test).

## External links

**`externalLink.test.ts`**: strict SSRF-style guard before ever fetching a page title
for a bare URL — blocks localhost, private ranges (`10.0.0.0/8`, `172.16.0.0/12`,
`192.168.0.0/16`), link-local (`169.254.0.0/16`, `fe80::`), unique-local IPv6
(`fd00::/8`), and unqualified/`.local` hostnames, while allowing public IPs
(`8.8.8.8`) — relevant if Atlas ever fetches remote content based on model-supplied
URLs. Title fetches are deduplicated (in-flight requests for the same URL share one
network call) and cache across protocol/`www.` URL variants of the same canonical
URL. Error-page titles ("Just a moment...", a Cloudflare challenge page marker) are
explicitly filtered out rather than displayed as if legitimate.

## Clipboard (clipboard.test.ts / osc52.test.ts) — directly reusable for Atlas on Windows

- Windows clipboard read/write goes through **PowerShell**, base64-encoding the text
  through `-Command` args rather than piping through stdin, specifically to preserve
  CJK/emoji correctly (`[Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes(...))`
  for read, `[Convert]::FromBase64String(...)` + `UTF8.GetString` reversed for write).
  Raw stdin/stdout piping to `clip.exe`/`Get-Clipboard` mangles non-ASCII; base64
  round-trip is the fix. **This is a concrete, directly portable pattern for Atlas's
  Windows clipboard support.**
- WSL detection precedence: `WSL_INTEROP` env var → use `powershell.exe` (not
  `powershell`) for both read/write; `WSL_DISTRO_NAME` also routes to
  `powershell.exe`; **WSLg** (Wayland forwarding under WSL2, `WAYLAND_DISPLAY` set)
  still prefers the Windows-clipboard PowerShell path over `wl-copy`, i.e. Windows
  interop takes priority over the Linux desktop clipboard tooling when both signals
  are present.
- Linux clipboard fallback chain: `wl-copy`/`wl-paste` (Wayland) → `xclip` → `xsel`,
  tried in that order, each only attempted if the prior one fails/is absent.
- macOS uses `pbcopy`/`pbpaste` directly (no fallback chain needed).
- `isUsableClipboardText`: rejects empty/whitespace-only content AND rejects
  binary-looking payloads (PNG magic bytes, TIFF metadata garbage with U+FFFD replacement
  characters) — a clipboard-content sanity filter Atlas's paste-handling should
  probably also have (avoid pasting a screenshot's raw bytes as "text").
- **OSC52** (clipboard-over-SSH escape sequence): wrapped in `\x1bPtmux;...\x1b\\`
  DCS passthrough when running inside `TMUX`, otherwise sent raw as `\x1b]52;c;<base64>\x07`.
  A slash command explicitly **prefers OSC52 for remote/SSH sessions** and native
  clipboard tools for local sessions (including local tmux) — `isRemoteShellSession()`
  is the gate. OSC52 *read* (query) support is separately gated behind a terminal
  capability probe (`readOsc52Clipboard` returns null if the querier is unsupported)
  since read-back requires the terminal to answer the query, which far fewer
  terminals support than write. **Directly relevant**: if Atlas runs over SSH,
  OSC52 write is the standard mechanism and this file has the exact escape-sequence
  format plus the tmux-wrapping quirk.

## Misc smaller findings

- **`bundleNoAsyncEsmDeadlock.test.ts`**: a very specific esbuild circular-module
  deadlock (async `__esm` init wrappers in a circular graph never resolve because the
  bundler's lightweight init helper doesn't await nested inits) — caused by
  re-exporting a transitively-async dependency graph. Not directly applicable to Go,
  but the general lesson (avoid re-exporting unused heavy dependencies "just because
  it's convenient," verify build output doesn't accidentally bundle a duplicate
  competing implementation) transfers.
- **`precisionWheel.test.ts`**: mouse-wheel scroll has a "precision mode" entered on
  the first modifier-held wheel tick; same-animation-frame wheel events (within an
  ~8ms window based on the test's `1000`/`1008` timestamps) are coalesced to avoid
  throttling legitimate line-by-line scroll, but immediate direction reversals are
  never coalesced (always processed individually). After modifier release, momentum
  is preserved briefly (~50ms window in the test) before precision mode exits.
- **`memoryMonitor.test.ts`**: heap-usage thresholds are **relative to a configured
  ceiling**, not a hardcoded absolute (an old hardcoded 2.5GB critical threshold was
  wrong on machines with an 8GB heap ceiling — 2.5GB is only 31% there). Warn-level
  fires on **fast/steep** growth (a big jump between two polling ticks while above a
  floor) rather than slow accumulation, and re-arms once heap usage drops back below
  the floor.
- **`prompt.test.ts`**: Termux gets an ASCII-only prompt marker (`>` instead of `❯`)
  and suppresses the profile-name prefix on narrow Termux widths, re-enabling it only
  on very wide panes — a concrete Android/Termux terminal accommodation.
- **`paths.test.ts`**: cwd/branch/project label truncation always prioritizes keeping
  the **branch name and/or project name** intact, truncating the path portion first
  from the left with a leading ellipsis; branch names that are themselves very long
  truncate from the right instead.
- **`details.test.ts`**: per-section detail-visibility overrides can escape even a
  global "hidden" mode — i.e. a section-specific override always wins over the global
  default, at any level of the hierarchy (global mode < persisted per-section value <
  in-session `/details` command override applies globally unless a more specific
  section override exists).
- **`stateIsolation.test.ts`**: high-frequency turn-state updates (e.g. every
  streaming token) must not trigger re-renders in UI-store subscribers that only
  care about coarse fields like `busy`/`sid` — verified via a shallow-equal selector
  gate. Direct blueprint for keeping Bubbletea's Update/View cycle cheap during
  streaming if Atlas separates "streaming text" state from "chrome" state.
- **`mergeUsageStable.test.ts`**: usage-stat merges must return the **same object
  reference** when nothing actually changed (not just an equivalent new object) to
  avoid spurious re-renders of every usage subscriber on each streaming delta —
  reference-stability as a performance contract, generalizable to any Go struct
  passed through a pub/sub or Bubbletea message that's compared by identity/dirty-flag.

## Top recommendations for Atlas (ranked by impact/effort)

1. **Windows clipboard via PowerShell + base64 round-trip** (from `clipboard.test.ts`).
   High impact (Atlas already has clipboard pain), low effort — it's a fully
   specified command + encoding scheme, directly implementable in Go via
   `os/exec` + `encoding/base64`. Also adds WSL/WSLg detection precedence for
   users running Atlas inside WSL.

2. **Truecolor detection with an explicit override chain**: `NO_COLOR` > explicit
   user env toggle > terminal self-report, with a **named, isolated predicate**
   for known-lying terminals (Apple Terminal pre-Tahoe in Hermes; the Windows
   analog is likely legacy conhost without `WT_SESSION`). Testable in isolation from
   the rest of env-detection logic, exactly like `shouldForceTruecolor` /
   `shouldDowngradeAppleTerminalTruecolor`. Medium effort, addresses a stated known
   uncertainty.

3. **Display-width-aware truncation/padding everywhere text is truncated to fit a
   column budget** (status bar, cwd labels, table rendering) — confirmed as a
   recurring real bug class (CJK table misalignment, cwd truncation) independent of
   the ASCII-only path. Go's `github.com/mattn/go-runewidth` or similar is the
   direct analog to the `stringWidth` helper used throughout these tests. High
   impact if Atlas has any CJK-using users; also underlies emoji-width correctness.

4. **Occlusion-aware timer lifecycle for the status bar** (from
   `appChromeBlockedTimers.test.tsx`): stop `tea.Tick`s entirely when a modal
   overlay truly covers the status bar, resume by **recomputing from wall-clock**
   rather than resuming a frozen counter, and don't pause for overlays that merely
   reposition (don't occlude) the bar. Medium effort, meaningful CPU/correctness win
   if Atlas has persistent timers and any full-screen modal.

5. **Post-interrupt event suppression window** (from `createGatewayEventHandler.test.ts`):
   after a Ctrl-C/interrupt, drop late-arriving streaming events from the
   still-unwinding backend turn until the next turn actually starts — directly
   addresses the "interrupt looks ignored because stale output keeps appearing"
   symptom, which is a very plausible bug class for any TUI agent with an async
   backend and a cancel button.

6. **Reference-stable state merges to avoid re-render storms during streaming**
   (`mergeUsageStable`, `stateIsolation`): only mint a new state object/struct when a
   field actually changed; gate high-frequency streaming updates so only the
   directly-affected UI region re-renders. Low effort, meaningful perf win under
   heavy token streaming.

7. **Enter-key-swallow guard for completion popups** (`completionApply.test.ts`):
   explicit rule that Enter must NOT be intercepted by an open completion/autocomplete
   list if applying the top suggestion would be a no-op or only add trailing
   whitespace to an already-complete command. Low effort, fixes a concrete and
   easy-to-reproduce UX bug class.

8. **IME-safe input commit path** (`imeVietnameseTelex.test.tsx`): if Atlas supports
   or plans to support CJK/Vietnamese input, ensure multi-character IME recompose
   bursts (backspace-burst + replacement chars, sometimes fused in one read) commit
   synchronously rather than through any debounced/batched key-processing path, and
   add a short (~60ms) "just repainted, suppress optimistic echo" window after a full
   repaint. Higher effort (requires reproducing exact IME byte patterns to test
   against), but this exact bug class is otherwise nearly impossible to catch without
   dedicated tests.

9. **Cursor-position math must derive from the same wrap function used for
   rendering** (`cursorDriftRegression.test.ts`): if Atlas's composer computes cursor
   row/col independently from how lipgloss/Bubbles actually wraps text, unify them —
   any divergence produces visible cursor drift on narrow terminals, exactly the bug
   this test suite exists to pin down. Medium effort depending on current
   architecture, but a real, user-visible correctness issue if present.

10. **OSC52 write support with tmux DCS passthrough wrapping**, gated to remote/SSH
    sessions with native OS clipboard preferred locally (`osc52.test.ts`,
    `createSlashHandler.test.ts`'s `/copy` routing test). Relevant if Atlas is ever
    used over SSH — currently likely has no remote-clipboard story at all. Medium
    effort, well-specified escape-sequence format provided above.

---

## File manifest — confirmation (60/60 read, 0 failures)

activeSessionSwitcher.test.ts, appChromeBlockedTimers.test.tsx,
appChromeStatusRule.test.tsx, appChromeStatusRuleDevCredits.test.tsx,
approvalAction.test.ts, asCommandDispatch.test.ts, attachments.test.ts,
billingStepUp.test.tsx, blockLayout.test.ts, brandingMcpCount.test.ts,
bundleNoAsyncEsmDeadlock.test.ts, charts.test.ts, clipboard.test.ts,
completionApply.test.ts, composerHighlights.test.ts, constants.test.ts,
createGatewayEventHandler.test.ts, createSlashHandler.test.ts,
cursorDriftRegression.test.ts, details.test.ts, emoji.test.ts,
externalLink.test.ts, forceTruecolor.test.ts, gatewayClient.test.ts,
gatewayRecovery.test.ts, gracefulExit.test.ts, imeVietnameseTelex.test.tsx,
inlineSlashSkill.test.ts, inputSelectionClipboard.test.ts, journeyCommand.test.ts,
loaders.test.ts, markdown.test.ts, mathUnicode.test.ts, memoryMonitor.test.ts,
mergeUsageStable.test.ts, messageLine.test.ts, messages.test.ts,
moaProgressActivity.test.ts, modelPicker.test.ts, orchestratorPromptSession.test.ts,
osc52.test.ts, overlayPrimitives.test.ts, parentLog.test.ts, paths.test.ts,
petPane.test.tsx, petPolling.test.ts, platform.test.ts, precisionWheel.test.ts,
prompt.test.ts, providers.test.ts, queueSubmission.test.ts, reasoning.test.ts,
rpc.test.ts, scroll.test.ts, scrollBoxRendererBounds.test.ts, slashParity.test.ts,
spawnHistoryStore.test.ts, stateIsolation.test.ts, statusBarTicker.test.ts,
statusRule.test.ts

All 60 files were fetched via a sparse-checkout clone of
`github.com/NousResearch/hermes-agent` (`ui-tui/src/__tests__/`) and read in full.
No file failed to fetch or read.
