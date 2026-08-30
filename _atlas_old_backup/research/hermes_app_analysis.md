# Hermes Agent TUI — `ui-tui/src/app/` deep analysis

Slice: state management / turn lifecycle / hooks layer. All 39 files in the manifest were read (2 test files skimmed per instructions, noted below). Timing constants pulled from `ui-tui/src/config/timing.ts` (not in the manifest but imported by `turnController.ts` and others, so read for completeness).

---

## timing.ts (referenced constants, for context)

```
STREAM_BATCH_MS        = 16    // reasoning/status/tool-progress coalescing tick
STREAM_IDLE_BATCH_MS   = 16    // streaming text batch delay, idle baseline
STREAM_SCROLL_BATCH_MS = 96    // batch delay while user is scrolling (throttle streaming repaint)
STREAM_TYPING_BATCH_MS = 80    // batch delay while user is typing mid-turn
TYPING_IDLE_MS         = 250   // "stopped typing/scrolling" cooldown before relaxing back to idle batch
REASONING_PULSE_MS     = 700   // reasoning-active pulse indicator duration
RESIZE_COALESCE_MS     = 32    // ~30fps resize-event coalescing (leading+trailing)
DOUBLE_ESC_MS          = 500   // double-Esc-to-discard-draft window
```

Atlas comparison: Atlas already has a ~40ms render-throttle tick for streaming text — close to Hermes's 16ms idle baseline but Hermes **dynamically widens** the batch delay under load (80ms typing, 96ms scrolling) and narrows back after 250ms of quiet. That adaptive widening is the interesting new idea, not the base number.

---

## turnController.ts (1105 lines, read in full — the core file)

**Purpose**: A single mutable class instance (`export const turnController = new TurnController()`) that owns *all* per-turn ephemeral state: streaming buffer, reasoning buffer, tool trail, subagent tree, notices, activity feed. It is not a React hook — it's called directly from the gateway event switch (`createGatewayEventHandler.ts`) and from UI callbacks, and it pushes results into two nanostores (`turnStore.ts` for turn-scoped state, `uiStore.ts` for session-scoped UI state).

**Key state-machine transitions** (method → nanostore patch):
- `startMessage()` → turn begins: resets reasoning/tools/trail, sets `busy: true`, also has special-cased "notice yielding" logic (a `credits.usage`/`credits.grant_spent` notice clears itself at next-turn-start so it doesn't camp the status bar).
- `recordMessageDelta(text)` → accumulates `bufRef`, calls `scheduleStreaming()` (debounced via `streamDelay`, itself dynamic — see `boostStreamingForTyping/Scroll/relaxStreaming`).
- `recordReasoningDelta`/`recordReasoningAvailable` → separate reasoning buffer + `pulseReasoningStreaming()` (700ms pulse timer).
- `recordToolStart` / `recordToolProgress` / `recordToolComplete` → tool trail entries (max 8 = `TRAIL_LIMIT`, activity feed capped at 8 = `ACTIVITY_LIMIT`), each also flushes the streaming segment first (tool calls interrupt narration into discrete "segments").
- `flushStreamingSegment()` → seals the current streaming buffer into a committed `Msg` segment (splits off a `<reasoning>` tag if present via `hasReasoningTag`/`splitReasoning`).
- `pushInlineDiffSegment()` → diff blocks are their own segment type, anchored between narration before/after the edit — explicitly NOT glued onto the final message. De-duped against the final text at `recordMessageComplete` so an agent narrating "here's the diff" doesn't double-render it.
- `recordMessageComplete(payload)` → the turn-end reconciliation: dedupes segments against final text (`finalTail`), archives todos, persists the subagent spawn-tree snapshot (fire-and-forget RPC + in-memory `spawnHistoryStore`), then calls `idle()` + `clearReasoning()` + `flushPendingNotice()`. Returns `{finalMessages, finalText, wasInterrupted}` for the caller to append to the transcript.
- `interruptTurn()` → drains segments to the transcript, appends a `[interrupted]` marker (folding in any partial buffer/pending tools), sets `status: 'interrupted'` with an `INTERRUPT_COOLDOWN_MS = 1500` timer that then flips back to `'ready'`. Has a `keepBusy` option so a queued follow-up message can ride out the "real" settle edge (`message.complete`) instead of racing the still-unwinding interrupted turn — this fixed a duplicate-user-bubble race bug (documented in a comment) that Atlas should watch for if it queues messages during interrupt.

**Notice system ("Strategy B")** — the most intricate piece, worth studying closely for Atlas's own status-bar/toast system:
- A `notification.show` event arrives mid-turn → held as `pendingNotice` (latest-wins) rather than shown immediately, because a busy turn's status slot is owned by the "FaceTicker" (Atlas equivalent: the spinner/status text).
- `flushPendingNotice()` is called ONLY at the three real turn-end sites (`recordMessageComplete`, `interruptTurn`, `recordError`) — never from `idle()`/`reset()`, because those also run on session switches and would leak session A's notice into session B.
- TTL clock starts at `applyNotice()` (when the notice becomes visible), not at arrival — so an "8s restored" notice always gets its full 8s of visible time regardless of how long it waited.
- `clearNotice(key)` only clears if keys match, guarding against a stale/late clear wiping a newer notice.

**Go/Bubbletea translation note**: This confirms Atlas's instinct to keep turn state on the root `App` struct rather than a separate controller object — Hermes needs a separate `TurnController` class only because React effects can't cleanly own mutable per-turn timers themselves (hooks re-run, closures go stale) and nanostores decouple state changes from React's render cycle. In Bubbletea, `Update()` already receives all events centrally and the `App` struct's fields ARE the single mutable source of truth updated synchronously on each message — there's no closure-staleness problem to route around. So: **keep turn state as plain fields on `App`**, but Atlas should still port the *specific mechanisms*, translated as:
  - Streaming batch timer → a `tea.Tick` cmd re-armed only when not already pending (Hermes's `if (this.streamTimer) return` pattern → check a `bool timerArmed` field before issuing a new `tea.Tick`).
  - Dynamic batch delay (16/80/96ms) → a `streamDelay time.Duration` field on `App`, bumped by keystroke/scroll messages, decayed by a `TYPING_IDLE_MS`-style timeout message.
  - "Pending notice held until turn end, flushed only at named turn-end sites" → an Atlas `pendingNotice *Notice` field, with the flush call made explicit at the same 3 sites (message complete, interrupt, error) — NOT inside a generic `resetTurn()`, to avoid the session-leak bug class Hermes explicitly guarded against.
  - Diff-segment anchoring (edits render inline between narration, not appended) → if Atlas ever streams file-diff tool results, apply the same "flush current streaming text as its own segment, then insert the diff, then resume" ordering rather than accumulating everything into one blob.

---

## turnStore.ts / uiStore.ts / interfaces.ts

`turnStore.ts` is a thin nanostore wrapper (`$turnState` atom + `patchTurnState`/`getTurnState`/`useTurnSelector`) holding only per-turn fields (streaming buffer, tools, reasoning, subagents, todos, activity, turnTrail). `uiStore.ts` is the session-scoped counterpart (busy, status, theme, indicator style, notice, usage, statusBar mode, etc.) — the split exists so a turn ending (`resetTurnState`) never touches session-level UI prefs, and so React components can subscribe narrowly via `useSyncExternalStore`-based selectors instead of re-rendering on every field change.

`interfaces.ts` (654 lines) is pure TypeScript type surface — no logic — defining every prop-drilled interface (`GatewayEventHandlerContext`, `SlashHandlerContext`, `InputHandlerContext`, `ComposerActions/State/Refs`, the Billing/Subscription overlay state machines, `AppLayoutProps`, etc.). Notable: `INDICATOR_STYLES = ['ascii','emoji','kaomoji','unicode']` — a busy-indicator style enum Atlas doesn't currently have (worth stealing as a config option, trivial effort). Also `ComposerToken` — a discriminated union for `[[ Image N ]]` / pasted-block tokens embedded inline in composer text (see useComposerState below).

**Go translation**: In Go this whole file collapses to plain struct definitions; the value here is only "what fields exist" reference material, not pattern-porting.

---

## createGatewayEventHandler.ts (1507 lines, full read)

The central `switch (ev.type)` dispatcher mapping ~40 gateway WS event types to store patches / turnController calls / sys() transcript lines. Structurally this is Hermes's equivalent of Atlas's `Update()` gateway-message branch. Notable patterns:

- **Theme/background detection cascade**: OSC-11 (terminal background) → OSC-10 (foreground, as a polarity tiebreaker for terminals that lie about background, e.g. xterm.js reporting pure black) → macOS `AppleInterfaceStyle` shell-out as last resort, gated behind a 1500ms timeout so it only fires if nothing else answered. Distrust of exact `#000000` as a probe answer (treated as "unset default", not a real reading) is a specific, non-obvious gotcha worth remembering if Atlas ever does background-color auto-detection.
- **Theme commit anti-tear**: after a theme swap, schedules `forceRedraw` ~40ms later (`setTimeout(..., 40)`) "deferred ~2 frames so React+Ink flush the recolored tree first" — a full repaint after incremental diffing can leave stale cells. Bubbletea equivalent: after a full-palette swap, Atlas should issue a `tea.Sequence` that first re-renders normally, then forces a clear+repaint one tick later if the terminal doesn't already guarantee full redraw semantics (most Bubbletea backends redraw whole-frame anyway, but if Atlas does partial/diffed rendering this is a real risk).
- **Crash-recovery / auto-resume boot sequence** (`handleReady`): checks `recoverSidRef` (mid-session gateway crash) first, then `STARTUP_RESUME_ID` (explicit resume), then optionally `display.tui_auto_resume_recent` (auto-resume most recent session), else forges a new session. All three paths call `scheduleStartupPrompt()` to flush any `-q` launcher-provided startup query once a session id exists (polls up to 40×100ms=4s for `sid` to appear).
- **Delegation/subagent event handling**: `subagent.spawn_requested` / `.start` / `.thinking` / `.tool` / `.progress` / `.complete` all funnel through `turnController.upsertSubagent()`, using `createIfMissing: false` for the "update" events so a late event arriving after `message.complete` already fired `idle()` can't resurrect a finished subagent into the live state (a "ghost subagent" class of bug). Terminal-status guard (`isTerminalStatus`) prevents late "running" events from overwriting a completed/errored/timed-out state.
- **First-delegation nudge** (`maybeNudgeAgents`): once per turn, if delegation starts and the user hasn't opened `/agents`, push a one-time discoverability hint. Lazily fetches `display.tui_agents_nudge` config once via a memoized promise (`fullConfigPromise`) so multiple concerns needing full config only cost one RPC round-trip.
- **Usage/status shallow-compare guard** (`usageChanged`/`mergeUsageStable`): iterates the union of keys generically (not a fixed list) so a future `Usage` field is never silently excluded from the change-detection — and returns the *same object reference* when nothing changed, specifically to avoid a per-streaming-delta re-render storm that caused observed status-bar flicker on iTerm2. **Directly applicable to Atlas**: if Atlas's Bubbletea model does `model.usage = newUsage` unconditionally on every delta tick, it forces a full View() re-render every tick even when values are unchanged — do the same "compare, keep old reference if equal" trick before triggering a re-render Cmd.

**Go/Bubbletea translation**: this whole file maps to Atlas's central `Update(msg tea.Msg)` type-switch on gateway-originated messages. The pattern to copy: keep each `case` body SHORT and delegate real logic to methods on a `turnController`-equivalent (in Atlas's case, methods on `App` itself, per the "keep it flat" decision above) — Hermes's switch stays readable at 1500 lines because each case is 2-15 lines of orchestration, not inline logic.

---

## Slash command system — registry.ts / types.ts / fuzzyScore.ts (+ test) / createSlashHandler.ts

This is the most directly reusable subsystem for Atlas, per the brief.

**types.ts**: `SlashCommand = { name, aliases?, help?, usage?, run: (arg, ctx, cmd) => void }`. Dead simple — a flat table, not a tree/trie.

**registry.ts**: `SLASH_COMMANDS` is just `[...coreCommands, ...topupCommands, ...sessionCommands, ...]` — each command group is a plain array exported from its own file under `slash/commands/`. A `Map<string, SlashCommand>` (`byName`) is built once at module load by flat-mapping `[cmd.name, ...cmd.aliases]` → command, giving O(1) exact lookup via `findSlashCommand(name)`. **This is the pattern to copy for Atlas's slash registration**: one file per logical command group, a flat array export, concatenated into one master list, aliases folded into the same lookup map at startup — no dynamic registration machinery needed unless you want runtime-loaded plugin commands (Hermes does support this separately via `sdk/registry.ts` widget apps, which register additional commands dynamically post-boot; the fallback path in `createSlashHandler.ts` checks `getWidgetApp(parsed.name)` after the static table misses).

**fuzzyScore.ts** (full detail, since explicitly requested):
- `SlashScoreItem = { id, label?, aliases?, description? }`.
- `tokenizeSearchText(value)`: lowercases, returns `[fullNormalizedString, ...alphanumericWordTokens]` — e.g. `"Commit & Push"` → `['commit & push', 'commit', 'push']`. Non-alphanumeric characters are the token boundary (`/[^a-z0-9]+/`).
- `normalizeSlashSearchQuery(query)`: trim, strip leading `/`+, lowercase.
- Scoring tiers, **lower score wins, `Infinity` = no match**:
  - Tier 0: exact match on id/label/alias (`field === query` or `/field === query`)
  - Tier 1: prefix match (`field.startsWith(query)`)
  - Tier 2: substring match (`field.includes(query)`)
  - Tier 3/4/5: same three tiers but against the **description** text tokens, at a `+3` offset — so a description match never outranks any name-tier match, but CAN surface a command whose name doesn't match at all (e.g. typing `/timer` finds a command named `clock` whose help text says "Start a countdown timer").
  - `scoreSlashMenuItem` takes `Math.min(nameFieldsScore, descriptionFieldsScore)`.
- `rankSlashItems(items, query, toScoreItem)`: empty query returns the list **untouched** (preserves caller/browse order); otherwise maps to `{index, item, score}`, filters out `Infinity`, sorts by `(score, then original index)` — the stable-sort-by-original-index is what keeps ties in registration order rather than shuffling.
- Test file (`fuzzyScore.test.ts`, skimmed) confirms exact tier boundaries: `'recaps'`→0 (exact), `'rec'`→1 (prefix), `'caps'`→2 (substring), description tiers verified at exactly 3/4/5, and a same-tier tie-break-by-original-order test.

**createSlashHandler.ts**: the dispatch entry point (`(cmd: string) => boolean`). Flow:
1. Parse into `{name, arg}`.
2. Look up in the static registry (`findSlashCommand`) — if found, call `.run()` directly, done.
3. Else check the dynamic widget-app registry (`getWidgetApp`) — handles user-defined `/mywidget` commands registered after boot.
4. Else check the server-driven `catalog.canon` map (skill/plugin commands the *backend* knows about) — first for an exact alias match, then falls back to `scoreSlashMenuItem` fuzzy matching restricted to `score < 3` (name-tier only, never description-tier, since auto-executing on a description match would be surprising) to resolve typo'd/abbreviated commands (e.g. `/hea` → `/heartbeat`). If multiple canon names tie at the best score, it reports "ambiguous command: X, Y, Z" instead of guessing.
5. Final fallback: RPC to the backend (`slash.exec`, then `command.dispatch` on failure) for server-implemented commands, with response types `exec | plugin | alias | skill | send | prefill` each handled differently (alias recurses through the handler; skill/send post a transcript message with a possibly-different "display" vs. "message" text — the display is what the user sees, the message is what the model receives, letting a `/mycommand` expand into a large hidden system prompt).
6. `stale()`/`guarded()` helpers gate all async RPC callbacks against a monotonic `slashFlightRef` counter AND a session-id check — if a newer slash command started, or the session switched, before the RPC resolves, the callback silently no-ops. **This exact "flight counter to invalidate stale async callbacks" pattern is worth porting** — Atlas likely already has something similar for tool-approval races, but if not, it's a cheap, high-value guard against race conditions when RPCs are in flight during rapid user input.

**Go/Bubbletea translation for the slash system**:
- `SlashCommand` interface/struct → identical shape in Go: `{Name string; Aliases []string; Help string; Run func(arg string, ctx *SlashCtx, cmd string)}`.
- Registry → a package-level `[]SlashCommand` built from `append`-ing per-file slices, then a `map[string]*SlashCommand` built once in an `init()` or lazily on first use.
- Fuzzy scoring → a direct, trivial port: it's pure string logic with zero framework dependency (no React/hooks involved at all) — this is the single easiest piece to lift near-verbatim into a `internal/slash/fuzzy.go`, changing only syntax. Recommend porting the exact tier scheme (0/1/2 name, 3/4/5 description) rather than inventing a new one, since the tests already validate the intended UX.
- Flight-counter staleness guard → an `int64` counter field on `App`, incremented each time a slash command dispatches an async `tea.Cmd`; the `tea.Cmd`'s returned message carries the flight value it was issued with, and `Update()` compares against the current counter before applying the RPC result.

---

## Slash command groups — core.ts / debug.ts / ops.ts / session.ts / setup.ts / wake.ts / subscription.ts / topup.ts

All read in full. These are ~30 individual `/command` implementations. Most are thin RPC-call-then-render-transcript-line wrappers with no reusable pattern beyond what's already covered above. Specific things worth flagging:

- **core.ts**: `/details` is a two-level config model — a global `detailsMode` (hidden/collapsed/expanded/cycle) plus optional PER-SECTION overrides (`thinking`, `tools`, `subagents`, `activity`), each independently settable and reset-able. Atlas's details/verbosity toggle (if any) could adopt this two-level shape cheaply if it ever needs finer control than a single global switch. `/copy` shows a clean fallback chain: native clipboard → OSC52 escape sequence → explicit failure message, with the OSC52 path force-selected for remote/SSH sessions (`isRemoteShellSession`) since native clipboard access is impossible there.
- **debug.ts**: `/heapdump`, `/mem`, `/theme-info` — diagnostic commands reading `process.memoryUsage()` directly. Trivial to replicate in Go via `runtime.MemStats`.
- **ops.ts**: `/rollback` (checkpoint list/diff/restore) and `/replay` (in-memory + disk-backed spawn-tree history, capped at `HISTORY_LIMIT = 10` via `spawnHistoryStore.ts`) are the most complex, session/subagent-specific — not directly relevant to Atlas unless it grows a multi-agent/checkpoint feature.
- **session.ts** (759 lines): `/model`, `/sessions`, `/compress`, `/branch`, `/voice`, `/pet`, `/theme`, `/reasoning`, `/busy` (interrupt/queue/steer mode), `/usage`. The `/busy` command is notable — it's a user-facing control for the exact "what happens when you hit Enter mid-turn" policy (see `submissionCore.ts`/`useSubmission.ts` below) with three modes: `queue` (append, drain after), `steer` (inject after next tool call without interrupting), `interrupt` (immediately redirect the live turn). Atlas currently doesn't have a steer mode — only interrupt. This three-way policy, with `queue` as the TUI's own overridden default (see useConfigSync notes below), is a meaningfully richer input-during-busy UX than "just interrupt" and worth considering if Atlas ever gets complaints about interrupt eating in-progress typing.
- **setup.ts**: trivial — shells out to `hermes setup` via `withInkSuspended` (suspends the Ink render loop while a child TTY process runs). Bubbletea equivalent: `tea.ExecProcess`.
- **wake.ts**: voice wake-word toggle, session-local override flag (`wakeState.ts`) so a `/wake off` survives gateway reconnects without needing persistence — not relevant to Atlas (no voice), skip.
- **subscription.ts / topup.ts**: billing-heavy as expected, skimmed per instructions. The only structurally interesting bit both share: `buildOverlayCtx`/`buildSubscriptionCtx` construct a closure-bundle of async functions once (capturing the live `SlashRunCtx`) and stash it into the overlay's nanostore state — "the overlay only renders + routes keys, all RPC/error-mapping logic lives in the slash command file" is a clean separation of concerns (dumb view, smart controller-closure) that generalizes past billing.

---

## delegationStore.ts / overlayStore.ts / inputSelectionStore.ts / petFlashStore.ts / spawnHistoryStore.ts / wakeState.ts

All small nanostore modules, same shape: `atom<T>` + `get`/`patch`/`reset` free functions, no classes. `overlayStore.ts` is worth noting for its **soft-reset pattern** (`resetFlowOverlays`): on every turn end, FLOW-scoped overlays (approval/clarify/confirm/sudo/secret/pager) are cleared, but user-toggled ones (agents dashboard, model picker, sessions, skills hub) are explicitly preserved by copying their current value forward — a full `resetOverlayState()` would silently close `/agents` every time a turn finished, which was an actual regression they fixed. **Directly applicable to Atlas**: if Atlas has any "close everything" reset call on turn completion, audit it for the same bug class — anything the user explicitly opened (a picker, a settings panel) should not be casualties of an unrelated turn-lifecycle reset.

`petFlashStore.ts`/`spawnHistoryStore.ts`/`wakeState.ts`/`inputSelectionStore.ts` are cosmetic/feature-specific (animated pet, subagent replay, voice wake, text-selection clipboard state) — no reusable pattern beyond "small atom + free functions," not relevant to Atlas's current scope.

---

## gatewayContext.tsx / gatewayRecovery.ts

`gatewayContext.tsx`: a 20-line React Context provider wrapping `{gw, rpc}` — pure DI boilerplate, no logic. Skip; N/A in Go (Atlas passes deps as struct fields/params).

`gatewayRecovery.ts` (36 lines, but important): pure, unit-testable crash-recovery budgeting logic, decoupled from React/refs on purpose. `planGatewayRecovery(liveSid, recoverSid, attempts, now)`:
```
GATEWAY_RECOVERY_LIMIT = 3          // max respawn attempts
GATEWAY_RECOVERY_WINDOW_MS = 60_000 // sliding window
```
Filters `attempts` to only those within the last 60s, recovers only if `recent.length < 3`, and returns which sid to resume (prefers the live sid, falls back to a previously-pending recovery target so a crash-loop-on-startup keeps retrying the *same* session instead of stranding it after one attempt). This is exactly the kind of logic Atlas should have if it manages a subprocess (e.g., an MCP server or agent backend process) — a sliding-window crash budget prevents a spawn-storm on a persistently broken child while still auto-recovering from a one-off crash. **Recommend porting near-verbatim** as a small pure Go function with the same signature shape — cheap, high-value, directly testable.

---

## petFlashStore / scroll.ts / sessionResumeView.ts (+test) / setupHandoff.ts

`scroll.ts`: `scrollWithSelectionBy(delta, {scrollRef, selection})` — mouse-wheel/keyboard scroll logic that also shifts an active text selection's anchor/focus rows when scrolling with a selection active (drag-selecting past the viewport edge). Handles a stale-cache correction: `getScrollHeight()` is render-time-cached, but after a streaming tail commits into virtual history the real (Yoga-computed) height can be fresher, so a wheel-down at the (stale) bottom re-checks `getFreshScrollHeight()` before concluding there's nothing more to reveal. Relevant to Atlas only if it implements text selection during scroll; otherwise skip.

`sessionResumeView.ts` (+ `.test.ts`, skimmed as instructed): `scheduleResumeScrollToBottom(scrollRef, delays=[0,80,240])` — on session resume, re-snaps scroll-to-bottom at three delays (immediate, +80ms, +240ms) rather than once, because the virtual-history layout settles asynchronously as the terminal remeasures wrapped rows; each timer checks `isSticky()` (except the first, unconditional) and `getLastManualScrollAt() > startedAt` so a manual scroll during that window cancels future snaps. `refreshSessionView()` = `evictInkCaches('all') + forceRedraw()`. **Multi-delay re-snap pattern is worth porting** if Atlas's transcript view has any async/deferred height measurement on session load or window resize — a single immediate scroll-to-bottom can land short if content reflows a frame later.

`setupHandoff.ts`: `runExternalSetup()` — suspends the Ink render loop, shells out to `hermes setup`, checks the result code, re-probes `setup.status`, and starts a fresh session on success. Straightforward `tea.ExecProcess`-equivalent sequencing; no novel pattern.

---

## submissionCore.ts

Small (133 lines) but load-bearing: `submitPrompt()` is the actual prompt-submission RPC pipeline, factored out of the `useSubmission` hook specifically so **the synchronous-busy invariant is unit-testable without a React runtime**. The key fix documented in the comments: `markSubmitting()` flips `busy: true` **synchronously, before any await** — because the previous code awaited an `input.detect_drop` RPC first and only set busy inside the `.then()`, leaving a race window where a second Enter-press within that round-trip read `busy === false` and submitted a duplicate prompt instead of queueing. **This is a genuinely important, non-obvious bug-class lesson for Atlas**: any "detect intent, then submit" async pre-check before marking a session busy creates exactly this race. Atlas should audit its own submit path for the same pattern — if there's any `await` between "user pressed Enter" and "mark busy", that's a duplicate-submission window.

Secondary detail: `isSessionBusyError` regex-matches gateway error messages (`/session busy|waiting for model response/i`) as a defensive fallback re-queue path, kept even though the primary race was fixed elsewhere — belt-and-suspenders against a future/legacy backend that still rejects instead of queueing.

---

## useLongRunToolCharms.ts (69 lines — already ported into Atlas, verified)

Confirmed exact match to the numbers Atlas already ported:
```
DELAY_MS = 8_000        // tool must run 8s+ before first ambient message
INTERVAL_MS = 10_000     // minimum gap between successive charms for the same tool
MAX_CHARMS_PER_TOOL = 2  // cap per tool invocation
```
Implementation detail confirmed: runs a 1-second `setInterval` tick (not per-tool timers) that iterates all currently-running tools, checks `now - startedAt >= DELAY_MS` and per-tool slot cooldown/count, and clears the whole slot map when `!busy || !tools.length`. Slots for tools that finished (not in the live `tools` list anymore) are pruned every tick. Nothing to change here — Atlas's port already matches. One nuance to double check in Atlas's port: the interval clears itself entirely (not just skips) when busy goes false, and is re-created (not just left running) each time the `tools` dependency array changes — confirm Atlas's Bubbletea `tea.Tick` re-arming does the equivalent (don't leave a stale ticker running into the next turn).

---

## useBatteryPoll.ts

Trivial: 30s (`BATTERY_POLL_MS = 30_000`) polling of a `system.battery` RPC, gated entirely on `ui.battery` (the display toggle) — no session-id gate, since battery state is host-level not session-level. Clears the cached reading immediately when disabled. Direct Bubbletea translation: a `tea.Tick(30*time.Second, ...)` loop, only issued while the toggle is on.

---

## usePet.ts (352 lines)

Feature-specific (animated terminal pet via Kitty graphics protocol or Unicode half-block cells) — not relevant to Atlas's UI goals, but the **state-derivation pattern** is worth noting as a general design: `derivePetState({busy, toolRunning, reasoning, awaitingInput})` is a pure function computing one of 6 discrete states with explicit priority order (`awaitingInput` > `toolRunning` > `reasoning` > `busy` > `idle`), shared verbatim in concept with the backend's Python `derive_pet_state` and the desktop app — i.e., "derive UI state from turn/ui signals via one pure, priority-ordered function, call it from every store-change listener" is a clean pattern for any status-indicator Atlas might animate (e.g., a spinner glyph that should show different frames when awaiting approval vs. streaming vs. idle).

---

## useConfigSync.ts (405 lines)

Polls `config.get {key: mtime}` every `MTIME_POLL_MS = 5000` and re-hydrates full display config (`config.get {key: full}`) only when the mtime actually changed — avoiding a full config re-fetch on every poll tick. Notable sub-patterns:

- **MCP reload handshake**: a separate `mcp_rev` counter is compared each poll; only bumps `accepted` when the server *confirms* (`status: 'reloaded'`) it actually loaded that revision — a failed/rejected reload leaves `accepted` unchanged so the *next* poll retries the same revision rather than silently giving up. Explicitly decoupled from the mtime-changed check so retries happen every poll tick, not just on the next unrelated config edit.
- **Busy-input-mode TUI override**: `TUI_BUSY_DEFAULT = 'queue'`, overriding the CLI-wide default of `'interrupt'` — documented rationale: "in a full-screen TUI you're typically authoring the next prompt while the agent is still streaming, and an unintended interrupt loses work." This is a considered, UX-driven default divergence Atlas should weigh: **if Atlas defaults to interrupt-on-Enter-while-busy, consider defaulting to queue instead**, matching this exact reasoning.
- **Fail-safe config application**: on an RPC failure (`cfg === null`), most fields silently keep their last-good value rather than reverting to a hardcoded default — e.g. `destructiveSlashConfirm` is only touched `...(cfg ? {...} : {})`, so a transient RPC hiccup can't accidentally disable a safety confirmation.
- Multiple small `normalizeX(raw)` pure functions (`normalizeStatusBar`, `normalizeBusyInputMode`, `normalizeIndicatorStyle`, `normalizeMouseTracking`) each handle a legacy-boolean/string-alias/malformed-value fallback chain — a reusable idiom: every config value gets a pure "coerce anything into a valid enum, with an explicit default" function, unit-testable independent of the polling loop.

---

## useComposerState.ts (498 lines)

The composer/input-box logic — likely the richest input-handling material for the "what is Atlas missing entirely" question.

- **Refs as source of truth, not state**: `inputRef`/`tokensRef` mirror `input`/`tokens` state because "keystroke handlers... run several times before React re-renders" — i.e. synchronous reads during rapid key events need the ref, not the (batched) state. Bubbletea sidesteps this naturally since `Update()` is synchronous per-message with no render-batching gap, but it's a reminder that **any Atlas code path reading composer/input state outside of `Update()` (e.g. inside a `tea.Cmd` closure) must capture the value at Cmd-creation time**, not read a stale outer variable.
- **`[[ Token N ]]` inline attachment system** (`ComposerToken`): pasted images and large text pastes become an inline placeholder token (`[[ Image 2 ]]`, or a paste-collapse label) directly in the composer text — deleting the bracketed text IS how you detach the attachment (`syncTokens()` diffs tokens against current text on every edit and fires an `image.detach` RPC for any token whose bracket text vanished). Token budget: `TOKEN_MAX_COUNT = 32`, `TOKEN_MAX_TOTAL_BYTES = 4 * 1024 * 1024`, trimmed oldest-first. **This is a meaningfully richer paste/attachment model than a typical single "attached file" indicator** — if Atlas ever supports image/large-paste attachments, the inline-bracketed-token-as-the-UI approach (no separate attachment list to keep in sync) is worth adopting.
- **Paste collapsing thresholds**: `pasteCollapseLines` (default 5) / `pasteCollapseChars` (default 2000), both configurable via `paste_collapse_threshold`/`paste_collapse_char_threshold` config keys — a paste exceeding either threshold collapses to a one-line token instead of dumping raw text into the composer. **Atlas is very likely missing this entirely** — pasting a large diff/log into a text-only composer either floods the input box or (if Atlas doesn't handle multi-line paste specially at all) may just work character-by-character with visible reflow lag. Concrete, easy win: detect paste-length/line-count, collapse to a placeholder + defer full text until submit.
- **Dropped-file-path detection heuristic** (`looksLikeDroppedPath`): a fast client-side regex/prefix check (matches `file://`, `~/`, `./`, `../`, quoted absolute paths, Windows drive letters) run BEFORE bothering the server, gating whether to even attempt server-side `image.attach`/`input.detect_drop` RPCs — avoids a round-trip for obviously-not-a-path text like "/help".
- **Clipboard image paste** (`pasteClipboardImage`): distinguishes an explicit `/paste` (reports failure) from a "quiet" probe fired on an empty bracketed-paste event (a terminal delivering an image paste as zero text) — silently no-ops if there's nothing there, since a failed speculative probe shouldn't spam the user.
- **Editor handoff** (`openEditor`): writes composer content (`inputBuf` lines + current `input`) to a tempfile, suspends the Ink render loop, spawns `$EDITOR` synchronously (`spawnSync ... stdio: 'inherit'`), reads back on exit code 0, submits directly. Straightforward `tea.ExecProcess` equivalent for Atlas if it wants a "compose in $EDITOR" feature (Ctrl+G in Hermes).
- **Multiline input via trailing backslash**: not in this file but referenced — `useSubmission.ts`'s `submit()` checks `value.endsWith('\\')` and if so pushes to `inputBuf` (an array of already-committed lines) and clears the current line, joining with `\n` only at actual submit. **This is Hermes's whole multiline-input mechanism** — a trailing backslash continues to a new line instead of submitting, joined at the end. If Atlas's composer is currently single-line-only or uses a different multiline convention (e.g. Shift+Enter), this is worth comparing — trailing-backslash is a plain-text-terminal-friendly convention that works even over dumb SSH sessions without special key sequences.

---

## useInputHandlers.ts (738 lines, full read)

The global `useInput` keybinding dispatcher — the single largest source of "things Atlas's input handling might be missing." Full keymap captured below with priority order (checks run top-to-bottom, first match wins):

1. **Double-Esc discard draft** (`DOUBLE_ESC_MS = 500`): two Esc presses within 500ms with non-empty composer → push draft to history (so Up-arrow can recall it), then clear. Checked BEFORE the overlay-blocked early-return, so it works even mid-prompt-overlay.
2. **Overlay-blocked routing**: when a blocking overlay (`$isBlocked`) is up, most keys are swallowed except: Ctrl+C (cancel/deny the overlay), pager-specific j/k/g/G/PageUp/Space navigation, widget-app key dispatch, and — critically — **scroll keys always fall through** even while blocked (`shouldFallThroughForScroll`: wheel, PageUp/Down, Shift+Up/Down) so the user can still read transcript context above a pending approval/clarify prompt instead of the UI feeling fully locked.
3. **Completion menu Up/Down** (when `completions.length && input && historyIdx === null`): cycles the fuzzy-match dropdown instead of history.
4. **Mouse wheel**: has a whole acceleration subsystem (`computeWheelStep`/`initWheelAccelForHost`) plus a separate **precision mode** (`computePrecisionWheelStep`) entered when a modifier (Meta/Ctrl — chosen specifically because Cmd is intercepted by the terminal on macOS) is held during wheel scroll: one row per frame, no acceleration, for fine control. Entering precision mode explicitly resets the acceleration state so the next normal wheel event doesn't inherit stale momentum.
5. **Shift+Up/Down**: single-line scroll (distinct from PageUp/Down).
6. **PageUp/PageDown**: half-viewport step (`Math.max(4, floor(viewport/2))`), deliberately kept under Ink's `delta < innerHeight` DECSTBM fast-path threshold — a Bubbletea-specific-irrelevant optimization note, but the "half viewport, not full" sizing choice generalizes.
7. **Voice toggle chord** (Esc-based combo) — before generic Esc handling, so it can't be swallowed by queue-edit-cancel or selection-clear.
8. **Esc**: queue-edit cancel > selection-clear > (nothing, if neither active).
9. **Up/Down arrow with empty `inputBuf`** (single-line input) and cursor testing (no line above/below within a multi-line draft): cycles the **queue first, then falls back to history** (`cycleQueue(1) || cycleHistory(-1)`) — i.e. queued-but-not-yet-sent messages take priority over prior *sent* history when navigating up. Tracks a `historyDraftRef` so navigating away from an in-progress unsent draft and back restores it (like a shell's readline behavior).
10. **Copy shortcut** (Cmd/Ctrl+C-as-copy on some terminals): selection copy > input-selection clipboard copy > (macOS: no-op, since real Ctrl+C should follow) > (non-mac: falls through to interrupt/clear/exit).
11. **Ctrl+X**: cut input selection > remove-from-queue-if-editing > open sessions overlay (three-way overload on one chord based on context).
12. **Ctrl+O**: open model picker without disturbing the composer draft (distinct from `/model` which requires clearing to type the command).
13. **Ctrl+C** — the big one, `resolveCtrlCComposerAction({busy, hasDraft, hasSession})`:
    - non-empty composer draft → **always** `clear` (even mid-stream) — this was an explicit UX fix; previously Ctrl+C during streaming interrupted the turn even when the user was just trying to clear a typo'd draft.
    - empty draft + busy + live session → `interrupt`
    - otherwise → `exit` (idle hotkey exit, which in dashboard-embedded mode is redirected to "start fresh chat" instead of actually killing the process).
14. **Ctrl+D**: same three-way idle-exit routing as Ctrl+C's exit branch.
15. **Ctrl+L**: clear selection + force full redraw.
16. **Voice record toggle key** (configurable binding, default Ctrl+B): optimistic UI flip before RPC confirms.
17. **Cmd/Ctrl+G** (+ Alt+G fallback for VSCode/Cursor, which intercept the primary binding for "Find Next"): open $EDITOR.
18. **Shift+Tab**: toggle yolo-mode (skip-approvals) without spending a turn — explicitly guarded to not fire while a completion dropdown is open (Tab already means "accept completion" there).
19. **Tab** (with completions open): apply the highlighted completion.
20. **Ctrl+K** (mac) / `isAction(key,ch,'k')`: pop the front of the queue and dispatch it immediately, if a session exists.

**Directly relevant gaps for Atlas** (per the brief's ask about composer detail Atlas may be missing):
- **Queue-vs-history dual up/down-arrow stack**: Atlas likely only has plain history recall; Hermes layers a *queue* (messages typed-and-queued-but-not-sent, e.g. while busy) on top, checked first. If Atlas has no "type ahead while busy" queue feature at all, this whole 2-tier navigation is moot — but if Atlas does have any queuing, replicate the "queue cycling takes priority over history cycling" order and the up/down-only-when-cursor-has-no-line-above/below logic (so multi-line drafts still let arrow keys move within the draft before falling through to queue/history).
- **Ctrl+C draft-clear-first semantics**: this is a real, documented bug-fix (interrupt used to eat drafts) — Atlas should verify its own Ctrl+C priority order matches "non-empty draft wins over interrupt" if it has both features.
- **Modifier-held wheel = precision scroll**: a nice-to-have accessibility/precision feature, low effort if Atlas's Bubbletea mouse-wheel handling already distinguishes modifier state.
- **Scroll-keys-fall-through-during-blocking-overlay**: if Atlas has any modal/blocking prompt (tool approval y/n), verify the transcript can still scroll while it's up — this was explicitly called out in Hermes as a past UX complaint ("felt like the prompt had locked the entire UI").

**Go/Bubbletea translation**: `useInput`'s single big callback maps directly to Atlas's `Update()` key-message branch; the priority-ordered if/else-early-return chain is exactly how a Bubbletea `case tea.KeyMsg:` switch should also be structured — check overlay-blocking first, then scroll, then draft-state-dependent chords, then generic single-key bindings last. The one non-obvious translation point: Hermes's key handler reads `getUiState()` (a snapshot getter) at the top of every invocation to avoid stale-closure reads of nanostore state inside the `useInput` callback — Bubbletea's `Update(msg, model)` receives the current model by value/pointer each call, so this concern doesn't apply, but it confirms Atlas's pattern of reading `m.someField` directly in `Update()` (rather than caching it in a local at hook-mount time, which isn't a thing in Bubbletea anyway) is already correct.

---

## useMainApp.ts (1283 lines, full read — the root orchestration hook)

This is Hermes's rough equivalent of Atlas's top-level `App` struct + `NewApp()` constructor + the non-Update() glue code. It:
- Owns all `useState` for transcript (`historyItems`), voice, session timers (`turnStartedAt`/`lastTurnEndedAt`), catalog, etc.
- Wires every other hook together (`useComposerState`, `useConfigSync`, `useBatteryPoll`, `useSessionLifecycle`, `useSubmission`, `useInputHandlers`, `useLongRunToolCharms`) and builds the `createGatewayEventHandler`/`createSlashHandler` closures, injecting all their dependencies.
- Contains **`startPromptLiveSession()`** — a standalone exported async function (not a hook) implementing `hermes --tui -p "prompt" --model X`-style one-shot new-session-with-prompt flow: create a live session, optionally switch model first (session-scoped `config.set`), then dispatch the prompt. Cleanly separated from React so it's independently testable.
- **Resize handling is two-tiered**: a `RESIZE_COALESCE_MS = 32` coalescer for the `cols` state itself (throttles remounting virtual-history rows during a drag-resize), PLUS a separate 100ms-debounced `terminal.resize` RPC to the backend after the resize settles, which also re-checks `scrollRef.current?.isSticky()` before force-snapping to bottom (a manual scroll during the 100ms window should not be overridden).
- **Height-cache eviction with an LRU-ish cap**: `heightCachesRef` is a `Map<cacheKey, Map<rowKey, height>>` keyed by `${sid}:${cols}:${promptWidth}:${compact}:${detailsLayoutKey}` — i.e., a full remeasurement cache invalidated whenever any of those five things change, capped at `MAX_HEIGHT_CACHE_BUCKETS = 12` buckets (oldest evicted via `.keys().next().value`). This exists because wrapped-line heights are expensive to recompute and depend on several orthogonal inputs.
- **Terminal title composition**: `marker` (⚠ blocked / ⏳ busy / ✓ idle) + session title + model + cwd, updated via `useTerminalTitle`. Cheap, portable idea for Atlas if it ever sets a terminal tab title.
- **Queue-drain-on-settle effect**: whenever `busy` flips to false (turn ended, OR an interrupt, OR a shell.exec finished, OR an error recovered) AND the queue is non-empty AND nothing is mid-edit, dequeues and resubmits one item. Comment explicitly notes this covers non-`message.complete` settle paths (shell exec, errors) that would otherwise strand a queued message forever if the drain logic only listened for `message.complete`.
- **Terminal-parity hints**: on mount, probes for terminal capability quirks (`terminalParityHints()`) and surfaces each as a one-time activity note (deduped via a `Set` ref so hints don't repeat across re-renders).

**Go/Bubbletea translation**: this file is the strongest evidence that Atlas's "flat App struct" choice is correct at Atlas's current scale — Hermes needs ~10 separate hooks plus this 1283-line orchestrator specifically because React forces state to live in `useState`/`useRef` slots that must be threaded through prop objects to child hooks/components, and closures must be kept fresh via dependency arrays. None of that ceremony exists in Bubbletea: an `App struct { ... }` with all these concerns as plain fields, and a single `Update()` method, is not just adequate but is *functionally what this file's runtime behavior already reduces to* once you strip the React scaffolding — it's one big mutable-state coordinator either way. The two real, portable ideas independent of the React/Go framework difference are: (1) the coalesced-resize + settle-then-RPC pattern, and (2) the layout-height-cache keyed by a composite of everything that could invalidate it, capped and LRU-evicted — both are just "debounce plus bounded cache," equally applicable in Go.

---

## Files not deep-dived (per instructions — billing/subscription business logic)

`slash/commands/topup.ts` and `slash/commands/subscription.ts` were read in full but their charge-settlement/error-code-mapping bodies are Nous-portal-specific and out of scope for Atlas; only the shared "closure-bundle context object, dumb overlay view" pattern was extracted above.

## Test files (skimmed per instructions)

- `sessionResumeView.test.ts`: confirms the 3-delay re-snap behavior and manual-scroll cancellation described above via fake timers; no additional behavior beyond what's documented in `sessionResumeView.ts`.
- `slash/fuzzyScore.test.ts`: confirms exact score tier values (0/1/2 name, 3/4/5 description) and stable-sort-by-original-index tie-breaking; used to corroborate the fuzzyScore.ts write-up above.

All 39 manifest files were read; none were unreachable.

---

## Top recommendations for Atlas (ranked by impact/effort)

1. **Port the slash-command fuzzy-scoring scheme (fuzzyScore.ts) near-verbatim.** Pure string logic, zero framework coupling, ~60 lines, fully covered by existing tests to copy the intent from. Immediately upgrades Atlas's prefix-only filter to also match on description text and rank tiers sensibly. Highest impact/effort ratio in this whole slice.

2. **Fix the async-submit race class demonstrated in submissionCore.ts: mark "busy" synchronously before any `await` in the submit path.** If Atlas has any pre-submit async check (file-drop detection, validation RPC, etc.) before flipping busy state, audit it now — this exact bug (double-submit racing a detect-drop RPC) was significant enough that Hermes extracted the fix into its own testable module with an extensive comment trail.

3. **Add a paste-collapse threshold for large/multi-line pastes** (`pasteCollapseLines`≈5, `pasteCollapseChars`≈2000): collapse to a placeholder token, expand only at submit. Directly addresses the brief's question about composer gaps — if Atlas's composer currently has no special handling for large pastes, this is a visible, common-case UX problem (pasting a stack trace or diff) with a simple, well-specified fix.

4. **Adopt the notice/status "hold-until-turn-end, flush only at named turn-end sites" pattern from turnController's Notice system** if Atlas has (or plans) any toast/banner notification system that competes with an active status/spinner display. The critical rule — flush at explicit named sites (message-complete/interrupt/error), never inside a generic reset — avoids a documented session-leak bug class.

5. **Port `planGatewayRecovery`'s sliding-window crash-recovery budget** (3 attempts / 60s window) verbatim if Atlas manages any subprocess (backend agent process, MCP server, etc.) that could crash-loop. It's a small, pure, already-tested function.

6. **Consider Hermes's three-way busy-input policy (queue/steer/interrupt)**, defaulting the TUI specifically to `queue` (not the CLI-wide `interrupt` default) with the documented rationale "don't lose a draft to an accidental interrupt." If Atlas currently only supports interrupt-on-Enter-while-busy, at minimum evaluate whether `queue` should be the TUI default.

7. **Reuse-object-if-unchanged before triggering re-render** (the `usageChanged`/`mergeUsageStable` shallow-compare pattern) — cheap to add anywhere Atlas patches a struct field from streaming deltas and immediately triggers a `View()` refresh; prevents flicker/thrash on high-frequency updates without needing a deeper render-diffing overhaul.

8. **Multi-delay scroll-to-bottom re-snap on session resume/load** (0/80/240ms, cancel-on-manual-scroll) — if Atlas's transcript view can reflow asynchronously after initial render (e.g., width-dependent Markdown wrapping), a single immediate scroll-to-bottom can land short; the layered re-snap is a small, mechanical fix.

9. **Ctrl+C-clears-draft-before-interrupt priority** — verify Atlas's own interrupt keybinding doesn't eat an in-progress draft; Hermes had exactly this regression and fixed it with an explicit priority function (`resolveCtrlCComposerAction`) worth mirroring even just as a design checklist item.

10. **Queue-vs-history layered up/down-arrow navigation** — lower priority since it depends on Atlas having (or wanting) a message queue at all; if it does, copy the "queue cycles first, history second, only when the cursor has no adjacent line in a multi-line draft" logic wholesale rather than reinventing it.
