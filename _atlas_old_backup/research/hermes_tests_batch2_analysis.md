# Hermes Agent TUI — Test Batch 2 Analysis (57 files)

Source: `ui-tui/src/__tests__/` (second half, alphabetically `streamingMarkdown` → `widgetSdk`), from `github.com/NousResearch/hermes-agent` (MIT). Read in full, including edge-case assertions and inline comments explaining *why* each behavior exists. Goal: extract concrete, portable behavior for Atlas (Go/Bubbletea).

---

## 1. Composer / text input (textInput* cluster — 14 files)

This is the deepest-value cluster. Hermes's composer is NOT a naive textarea — it has a dual-path render architecture (Ink virtual-DOM render + a raw-stdout "fast-echo" bypass for the common case), transactional clipboard ops, readline-style line-kill semantics, and a hand-verified word-wrap/cursor model that is unit-tested against `wrap-ansi` byte-for-byte.

### 1.1 Fast-echo bypass (`textInputFastEcho.test.ts`, `textInputCursorSourceOfTruth.test.ts`)
- For the common case (cursor at end of a single-line, non-empty buffer, appending plain ASCII that won't hit the wrap column), Hermes writes bytes **directly to stdout** instead of going through a full Ink re-render — this is what makes typing feel instant even under React reconciliation overhead.
- Guard conditions for the bypass (`canFastAppendShape`) — ALL must hold: cursor at end of line; buffer non-empty; no `\n` in buffer; appending would not reach the column width (`col + insertedLen < cols`, strictly less than, not `<=`).
- The bypass explicitly **rejects**: Vietnamese precomposed letters and combining marks (IME composition risk even though grapheme is width-1), CJK/wide characters, emoji, NBSP, Latin-1 accented letters (é, ñ) — anything that isn't plain 7-bit ASCII text, because it might be a partial IME composition state.
- Backspace bypass (`canFastBackspaceShape`) additionally rejects deleting at a **soft-wrap boundary** — i.e. when the cursor's computed visual column is exactly 0 (wrapped to next visual row) — because `"\b \b"` can't move the terminal cursor across a wrap boundary; falls through to full Ink re-render in that case. This is checked with `cursorLayout` at 1 column past a multiple of the terminal width.
- Backspace/append effects return a structured object pairing the exact write bytes with the cursor-advance delta (`{ newValue, newCursor, removed, write, advanceDelta }`) so a caller can never apply one without the other — prevents cursor/text desync bugs.
- **Terminal allowlist for fast-echo** (`supportsFastEchoTerminal`): disabled in Apple Terminal (256-color escape parsing bug), disabled inside tmux (`TMUX` env set, or `TERM` starts with `tmux`/`tmux-256color` even without `TMUX` forwarded — covers SSH-from-tmux), disabled by default in Termux (opt-in via `HERMES_TUI_TERMUX_FAST_ECHO=1`), but explicitly NOT disabled for GNU screen (`TERM=screen*`) since no drift was reported there. tmux wins over Termux opt-in if both are set.
- Colors used by the bypass MUST route through the same `colorize()` helper Ink uses (never hand-rolled ANSI) — a hand-rolled `38;2;r;g;b` truecolor escape broke on Apple Terminal (fell back to gray) and, worse, a hand-rolled placeholder escape left the **dim SGR flag stuck ON** for all subsequent unstyled cells on legacy Terminal.app (it walks compound SGR params one-by-one and treated the literal `2` in `38;2;...` as SGR 2 = dim, with no closing `22`).
- `resolveCursorLayout` always reads a fresh **ref** (`curRef.current`), never the React `cur` state directly — a documented regression class ("cursor-drift") where TextInput re-renders (from an unrelated parent state change like a spinner tick) between the fast-echo write and the deferred 16ms `setCur` flush, and the layout effect would republish a declaration computed from stale `cur`, clobbering the correct Ink-level cursor bump.

### 1.2 Cursor layout / word-wrap (`textInputWrap.test.ts`)
- `cursorLayout(text, cursor, cols)` is unit-tested for byte-identical agreement with `wrap-ansi(text, cols, {hard:true, trim:false})`'s end-of-text position, across incremental typing at 6 different column widths (10/14/20/30/50/80) — every single character step must match, not just the final state.
- Exact-fill text (length === cols) does NOT wrap to a phantom next line — old hand-rolled wrap algorithm forced cursor to `(line+1, 0)` on exact fill; correct behavior keeps it at `(line, cols)`.
- Words wrap as whole units at the space, never split mid-word, when a word doesn't fit remaining row width.
- `composerPromptWidth('>') === 2`, `composerPromptWidth('❯') === 2`, `composerPromptWidth('Ψ >') === 4` — prompt width is glyph-cells-plus-one-gap-cell, not naive `.length`.
- `stableComposerColumns(paneWidth, promptWidth)`: reserves a scrollbar gutter but never starves the composer — e.g. `stableComposerColumns(6, 3) === 1` (never goes to 0/negative).
- `offsetFromPosition` is the exact inverse of `cursorLayout` — maps a (visual line, visual column) click back to a buffer offset, including past-end clamping and clicking past a hard `\n`.

### 1.3 Line-kill / readline motions (`textInputKillLine.test.ts`, `textInputLineKill.test.ts`, `textInputLineNav.test.ts`)
- **Ctrl+U / Ctrl+K are scoped to the current logical line**, not the whole buffer — this matters for multiline drafts. `killToLineStart('one\ntwo', 7)` → `{cursor: 4, value: 'one\n'}` (only kills `'two'`, keeps `'one\n'`).
- Critically: pressing kill-to-start again **when already at a line boundary consumes the newline**, so repeated presses make progress toward emptying a multiline draft one line at a time (`killToLineStart('one\n', 4) → {cursor: 3, value: 'one'}`). Symmetric for kill-to-end joining the next line.
- No-op at true buffer start/end (doesn't error, doesn't loop).
- **Cmd+Backspace (line-kill) modifier detection is deliberately narrow**: only the `super` bit (kitty CSI-u / VS Code `modifyOtherKeys` protocol) counts as "Cmd" — NOT `meta` (Option key reports as `meta` on macOS in hermes-ink, and Option+Backspace must stay delete-*word*) and NOT `ctrl` (Ctrl+Backspace is delete-word on Linux/Windows in readline/VS Code/browsers). This is a 3-way keybinding disambiguation Atlas will need if it wants Cmd+Backspace = kill-line on macOS.
- `lineNav` (Up/Down within a multiline composer): preserves visual column across lines, clamps to the shorter line's end when moving to a shorter line, returns `null` (fall through to global handler / history nav) when input is single-line or already at buffer's first/last line.

### 1.4 Cut/paste, right-click, delete-word (`textInputCut.test.ts`, `textInputRightClick.test.ts`, `textInputWordDelete.test.ts`)
- **Transactional cut**: `cutSelection` awaits the clipboard `write()` promise BEFORE calling `removeSelection()`. If the write fails (e.g. headless/SSH, no clipboard), the text is explicitly **NOT removed** — prevents data loss where a cut with no clipboard support would silently destroy the selection. Also verified no fire-and-forget: removal doesn't happen until the write promise resolves.
- Right-click: paste if no selection or a collapsed (empty) range; copy the selected slice if non-empty; unicode-safe (CJK/emoji slices copy correctly); falls back to paste on out-of-range indices instead of crashing.
- `deleteWordForward` (Ctrl+Delete): removes the word AND its trailing space; cursor stays fixed while text shrinks around it; no-op at buffer end. Notably documents a fragile cross-repo contract: the desktop app maps Ctrl+Delete to `ESC d` (meta+"d"), decoded via hermes-ink's `META_KEY_CODE_RE` — if that decode path changes, Ctrl+Delete silently regresses to typing a literal "d".

### 1.5 Return/submit semantics (`textInputReturnAction.test.ts`, `textInputReturnBurst.test.ts`, `textInputBurstInput.test.ts`, `textInputPassThrough.test.ts`, `textInputSubmitClear.test.tsx`)
- **Platform-specific Enter behavior**: on macOS, plain Enter (`\r`) submits; a bare **LF** (`\n`, no CR) is treated as a "multiline fallback" and inserts a newline instead of submitting (covers terminals/IMEs that send LF for Shift+Enter). Ctrl/Shift/Meta/Super+Enter always insert a newline on macOS.
- On non-macOS: Shift+Enter and Ctrl+Enter insert newlines; plain Enter/LF submits UNLESS the session is over **SSH** (`SSH_CONNECTION` set), in which case even bare LF is treated as the multiline fallback (remote terminals often mangle Shift+Enter into plain LF).
- **Burst input containing both text and Return in one chunk** (common with fast typing or IME auto-submit): `valueForReturnSubmit` correctly incorporates printable text that arrived bundled with the `\r`/`\n` into the submitted value, including for CJK/IME commit text arriving in the same burst.
- Regression test for a Korean IME commit immediately followed by Enter: rendered through a full mocked stdin/stdout Ink instance — sends prefix text, then final syllable + `\r` together, then a subsequent keystroke. Asserts the submitted text is exactly the full string (no dropped final syllable) AND that the composer correctly clears to just the next keystroke (not `full + nextChar`) — this is exactly the class of bug a naive composer could hit with async input handling.
- **Voice record key pass-through**: `shouldPassThroughToGlobalHandler` lets a user-configured voice shortcut (e.g. `ctrl+o`, `alt+r`, `ctrl+space`, `ctrl+enter`) skip the composer and reach the global handler, while NOT swallowing ordinary typing of that same character without the modifier. Global control keys (Ctrl+C, Ctrl+X, Escape, Tab, PageUp/Down) always pass through regardless of voice config.
- `shouldPreserveCtrlJNewline`: Ghostty terminal needs Ctrl+J (LF) preserved as a literal newline (not treated as submit) even when it's running inside tmux and TERM/TERM_PROGRAM get masked to tmux's values — detected via `GHOSTTY_RESOURCES_DIR` env var surviving through tmux.
- Non-bracketed multi-character input bursts (from fast typing, not a real terminal paste) apply immediately as a batch equal to applying each character individually — `shouldRouteMultiCharInputAsPaste` distinguishes a real paste (has embedded newlines) from a fast keystroke burst (no newlines) so a burst of typed chars doesn't get mis-routed through paste-handling logic.
- `applyPrintableInsert` also replaces an active selection range in one step, and rejects control/escape-bearing input (`\x1b[200~...`, tab) outright — returns `null` rather than inserting garbage.

---

## 2. Turn / streaming state, queueing (turnController/turnStore/useQueue/useSubmission — Atlas has none of this)

### 2.1 Message queue while busy (`useQueue.test.ts`) — directly maps to a feature Atlas lacks
- Queue items pair full submission `text` with a collapsed `display` label (e.g. a long paste becomes `[[ first.. [3 lines] .. last ]]` for compact rendering while the full text is what gets sent).
- **Editing a queued item preserves the pairing**: if a user edits the *displayed* (collapsed) text around a paste token, the edit is applied to both the collapsed display and the underlying full text — text inserted around the label lands around the actual expanded content too (`before [[..]] after` → text becomes `before <full paste> after`).
- If a user *replaces* the collapsed label entirely (not just adds text around it), the paste linkage is dropped — the replacement becomes literal plain text (no attempt to re-derive the paste).
- `removeAtInPlace`/`takeQueueItem`/`prependQueueItem` mutate an array in place and are no-ops on out-of-bounds indices (never throw).

### 2.2 Submission race (`submissionCore.test.ts`) — a concrete queue-mode race Atlas should replicate the fix for
- `submitPrompt` flips a global `busy=true` flag **synchronously**, before the async `input.detect_drop` RPC (which checks if the pasted content is actually a dropped file path) resolves. This closes a real regression: without the synchronous flag, a second rapid Enter press during the async gap would race a second `prompt.submit` onto the backend instead of taking the local-enqueue path.
- No session (`sid` null) → refuses to submit, shows `"session not ready yet"`, never marks busy, never calls `detect_drop`.
- A literal-submission path (startup `-q` query) can `skipDetectDrop` entirely and goes straight to `prompt.submit`, verbatim (including shell-metacharacter-looking text like `/model $(rm -rf ~)`) — no injection/escaping performed client-side, treated as opaque text.
- `isSessionBusyError` matches specific legacy error message strings only (`'session busy'`, `'waiting for model response'`), not arbitrary errors — string-based error classification, fragile but explicit.

### 2.3 Notice lifecycle across turns (`turnControllerNotice.test.ts`)
- Two notice categories with different lifetimes: **"flash and yield"** notices (`credits.usage`, `credits.grant_spent`) auto-clear the instant a new turn starts (`turnController.startMessage()`), vs **sticky** notices (`credits.depleted`) that persist across turns until explicitly cleared by policy. A billing/credits-adjacent concept, but the flash-vs-sticky notice pattern is a reusable idea for Atlas's own status-line messaging (e.g. transient "rate limited, retrying" vs. persistent "auth expired").

### 2.4 Todo/plan tracking (`turnControllerTodos.test.ts`, `turnStore.test.ts`)
- Todos support a `parent` field for nested subtasks (hierarchical plan display) but **self-referential parents are dropped** (a todo listing itself as its own parent is sanitized away rather than creating an infinite loop in tree rendering).
- At turn end, todos are "archived" into a transcript trail entry: completed-only todos archive with `todoCollapsedByDefault: true`; if any todo is still pending/in-progress, the archived entry gets `todoIncomplete: true` so the UI can render a distinct "left unfinished" hint. Archiving clears the live todo list back to empty and is idempotent (returns `[]` if there's nothing to archive).

### 2.5 Subagent tree aggregation (`subagentTree.test.ts`) — relevant if Atlas ever supports subagents/parallel tool calls
- Builds a tree from a flat list keyed by `parentId`; **orphaned children (parent id not present in the list) are promoted to top-level** rather than dropped — defensive against out-of-order/partial event streams.
- Children are sorted by `(depth, index)`, not insertion order — stable regardless of event arrival order.
- `hotness = totalTools / totalDuration`, explicitly `0` when duration is `0` (guards div-by-zero) rather than `Infinity`.
- Formatting helpers: `fmtCost` (`0` → `''`, `<$0.01` for sub-cent, else `$X.XX`), `fmtTokens` (542 → `'542'`, 1234 → `'1.2k'`, 45678 → `'46k'`), `fmtDuration` (`0s`/`42s`/`1m`/`2m 14s` — omits the `0s` in whole-minute cases but keeps seconds otherwise), `sparkline` renders `0` values as literal spaces (not the bottom-most block glyph) so an all-zero series reads as empty rather than a flat baseline.

### 2.6 Streaming markdown incremental scanner (`streamingMarkdown.test.ts`) — very relevant for Atlas's Glamour-based streaming render
- `advanceScan` incrementally splits a growing markdown stream into "settled" (immutable, already-rendered) blocks plus a live "tail," splitting only at a **blank-line boundary that is not inside an open fenced code block or open `$$`/`\[` display-math block**. This lets the renderer commit chunks progressively without re-rendering the whole document on every token, while never splitting mid-construct.
- Verified idempotent, append-only (`state.blocks` only grows, never mutates a committed entry), and produces byte-identical output whether the same final text arrives in one shot or fed in arbitrary small random chunks (fuzzed with a seeded PRNG at multiple cut-point strategies).
- A blank line consisting of 3+ consecutive newlines does NOT produce a whitespace-only block (`'alpha\n\n\n\nbeta\n\n'` splits into `['alpha\n\n', '\n\nbeta\n\n']`, not three pieces).
- A setext heading (`Title\n====\n`) is kept **contiguous with its paragraph** — the underline can never be torn from the title text it decorates, by construction (only blank-line boundaries are legal splits, and setext underlines have no blank line before them).
- Splitting the same text into blocks-plus-tail and rendering each block through the markdown renderer separately produces **byte-identical terminal output** to rendering the whole text as one call — this is the actual regression contract: incremental rendering must be visually indistinguishable from a single full re-render, verified at every simulated streaming step, not just the final frame.

---

## 3. Terminal compatibility (terminalModes/terminalParity/terminalSetup/termux — 5 files)

### 3.1 Exit-safety mode reset (`terminalModes.test.ts`) — Atlas should have an equivalent
- A single hardcoded escape-sequence blob (`TERMINAL_MODE_RESET`) disables every "sticky" terminal mode the app might have turned on: DEC mouse tracking (`?1000l` through `?1006l`, `?9l`), bracketed paste (`?2004l`), alt-screen (`?1049l`), focus reporting (`?1004l`), Kitty keyboard protocol (`?2029l`, `<u`), and a couple of vendor-specific `'z`/`'{` modes.
- This reset must be callable **synchronously from a `process.on('exit', ...)` handler** — exit handlers cannot await, so the entire reset is one single synchronous `write()` call. Verified explicitly with a mock "exit-style" invocation.
- Skips non-TTY streams (no-op, doesn't throw).
- **A concrete leaked-mouse-tracking bug this exists to prevent**: without this, killing the TUI (Ctrl+C, `/quit`, or any `process.exit()` path) can leave DEC mouse tracking enabled in the parent shell, which then reads raw mouse escape sequences as garbage keystrokes.

### 3.2 Terminal default fg/bg painting (`terminalModes.test.ts`, cont.)
- OSC 10 (foreground) / OSC 11 (background) are painted with a hex color and the reset sequence only appends an OSC 10/11 restore-to-default (`\x1b]1{10,11}\x07`) if a color was actually painted this session — never emits a spurious restore if nothing was ever painted. Symmetric setter contract tested via `describe.each` over both slots.
- `isPaintableHex` requires exactly `#RRGGBB` (6 hex digits, case-insensitive) — rejects 3-digit shorthand, wrong lengths, and non-hex.

### 3.3 IDE keybinding auto-configuration (`terminalSetup.test.ts`) — largest file in this cluster, VS Code/Cursor/Windsurf specific
- Detects VS Code-family terminals via distinguishing env vars: `CURSOR_TRACE_ID` → cursor, `VSCODE_GIT_ASKPASS_MAIN` containing "windsurf" → windsurf, `TERM_PROGRAM=vscode` → vscode.
- Computes the OS-correct `keybindings.json` path per platform (macOS: `~/Library/Application Support/<App>/User`, Linux: `~/.config/<App>/User`, Windows: `%APPDATA%/<App>/User`).
- Custom JSON-with-comments parser (`stripJsonComments`) strips `//` line comments and `/* */` block comments and trailing commas before `]`/`}`, while **preserving `//`-looking text inside actual JSON string values** (doesn't false-positive strip a literal `"// not a comment"` string) and gracefully handles an unterminated block comment (consumes to EOF rather than erroring).
- Injects specific CSI-u-encoded keybindings: Cmd+C (`[99;13u`, macOS only, forwards copy when text is selected via `terminalFocus && terminalTextSelected`), Shift+Enter/Ctrl+Enter/Cmd+Enter (`[13;2u`/`;5u`/`;9u`), Cmd+Z/Shift+Cmd+Z for undo (macOS only).
- **Conflict detection is `when`-clause-aware**, not just key-string matching: a same-key binding with no `when` (global) is flagged as a conflict against ANY scoped binding; an overlapping-but-not-identical `when` (e.g. plain `terminalFocus` vs. `terminalFocus && terminalTextSelected`) is flagged as a conflict (the broader one would shadow); a **negated** clause (`terminalFocus && !terminalTextSelected`) is correctly recognized as logically disjoint and NOT a conflict; a binding scoped to an unrelated context (`editorFocus`) is not a conflict even on the same key.
- Existing `keybindings.json` is backed up (`copyFile`) only when a write is actually about to happen — never touches the file just to check for conflicts.
- **Refuses to run over SSH** entirely (checks `SSH_CONNECTION`/`SSH_TTY`/`SSH_CLIENT`) since it can't reach the local IDE's config directory.
- Detects and **migrates legacy `\\\r\n`-style Enter-forwarding sequences** to the modern CSI-u encoding, reporting exactly how many bindings were migrated in the result message.

### 3.4 Cross-terminal parity hints (`terminalParity.test.ts`)
- Surfaces distinct warning "hints" keyed by cause: `apple-terminal`, `remote` (SSH), `tmux` — all three can fire simultaneously for a session that's SSH'd into a machine running Apple Terminal inside tmux.
- The "you should configure IDE keybindings" hint is suppressed once the keybindings file already has all the modern CSI-u bindings, but stays active if the file has any of the same keys bound to the legacy `\\\r\n` sequence — reuses the migration-detection logic from 3.3.

### 3.5 Termux-specific composer layout (`termux.test.ts`, `termuxComposerLayout.test.ts`)
- Termux mode is auto-detected via `TERMUX_VERSION` or a `PREFIX=/data/data/com.termux/...` path, defaults ON when detected, and has an explicit env-var opt-out (`HERMES_TUI_TERMUX_MODE=0`) that only takes effect if actually inside Termux (setting the flag outside Termux is a no-op).
- Termux narrows the prompt to a single-cell ASCII character (`>` instead of `❯`) and drops profile-name prefixes on narrow panes, but restores both above a specific width threshold (tested at exactly 72 vs 120 columns).
- Reserves a *smaller* gutter than desktop mode on narrow widths, freeing 2 extra columns for the composer (Termux's on-screen keyboard already eats vertical space, so horizontal composer room is prioritized) — concrete numbers: at 40 total columns with an 8-col reserve, desktop leaves 28 usable columns, Termux leaves 30.

---

## 4. Text utilities, syntax highlighting, viewport/scrolling, virtualization (misc — largest remaining group)

### 4.1 `lib/text.ts` (`text.test.ts`)
- `buildVerboseToolTrailLine` caps embedded tool-result previews to keep the persisted (always-expanded) trail line small — a 40KB result is compressed to well under 2KB with an `"omitted"` marker, closing a real OOM bug where a giant browser-snapshot result blew up the Ink render tree.
- `estimateTokensRough`: 4 chars/token, rounding **up** (`'a'` → 1 token, `'abcd'` → 1, `'abcde'` → 2) — a cheap heuristic worth copying verbatim for any token-budget UI hint.
- `stripAnsi` vs `sanitizeAnsiForRender`: the former strips ALL escape sequences including SGR color codes (for plain-text extraction); the latter keeps valid SGR color spans but strips cursor-control sequences (`\x1b[2J`, `\x1b[?25l`, OSC titles) and dangling/incomplete CSI prefixes — useful distinction for Atlas if it ever needs to sanitize untrusted tool output before feeding it through Glamour/Lipgloss.
- `thinkingPreview` inserts paragraph breaks before markdown-bold headings inside a reasoning stream (`**Heading**\ntext**NextHeading**` → properly spaced), and has a hard length ceiling (~24k) that still **guarantees the live tail (most recent text) survives** even when the total reasoning exceeds the bound — never truncates from the end, always keeps the freshest content visible.
- `boundedLiveRenderText`: caps a live-updating block by both `maxChars` and `maxLines`, always keeping the **tail** (most recent lines/chars), with an `"omitted N lines"` marker.
- `estimateRows` (row-count estimator for markdown, feeding virtualization): correctly counts `~~~` tilde-fenced code blocks and `- [ ]`/`- [x]` checklist items as list rows; treats `snake_case` identifiers as a single visual unit for wrap-width purposes (doesn't wrap mid-identifier at the underscore) — same row count as if underscores were spaces.

### 4.2 Syntax highlighting (`syntax.test.ts`)
- Whole-line `//` comments are painted a single dim color for the entire line (not tokenized further). Python's `#` is recognized as a comment starter, explicitly distinguished from a CSS selector context. Unsupported languages fall through completely unstyled (single token, original text, empty color).

### 4.3 Theme system (`theme.test.ts`, `themeBoot.test.ts`) — cross-check against Atlas's theme
- **Light-mode detection precedence** (`detectLightMode`), most to least authoritative: explicit `HERMES_TUI_LIGHT`/`HERMES_TUI_THEME` env override > `COLORFGBG` (queried from terminal, bg slot 7 or 15 = light) > `HERMES_TUI_BACKGROUND` hex luminance > a small `TERM_PROGRAM` allowlist (Apple Terminal defaults light) > default dark. Malformed `COLORFGBG` (non-numeric trailing field) and malformed hex backgrounds fall through cleanly to the next tier rather than misfiring.
- A **skin-authored background color "owns the polarity"** of everything else — if a skin declares `background: '#000000'`, all contrast-adaptation math runs against that black, not the host terminal's actual (possibly light) background, since the skin visually painted over it via OSC 11.
- Foreground colors on a light background get a **gentle multiplicative "rescue" lift** (contrast floor ~1.18:1, hue-preserving — a warm near-white color stays warm, just brightened) rather than a harsh WCAG-style contrast-lock — deliberately keeps the "vivid, low-contrast" aesthetic of a dark-authored skin recognizable when rendered on light, rather than darkening it to pass accessibility contrast ratios.
- Colors resolve through `themeToneHex`, supporting an `ansi256(N)` pseudo-format resolved via the standard 6×6×6 cube / grayscale-ramp formulas, always yielding a literal `#rrggbb` — an unresolvable tone (`ansi256(999)`, `'inherit'`, `''`) resolves to `''`, meaning "release to terminal default" rather than an error.
- Boot-time theme caching (`themeBoot.test.ts`): a cached background from a previous session is only "seeded" into the current env if there's no stronger *current* signal (never overrides an explicit user env var) and never seeds an untrusted pure-black `#000000` fingerprint (a common false-negative OSC-11 answer). A cached theme-pin (e.g. user ran `/theme light` last session) replays BOTH the pin and its associated background together to avoid a light→dark→light flash on boot.

### 4.4 Viewport / scrolling (`viewport.test.ts`, `viewportStore.test.ts`, `wheelAccel.test.ts`)
- `stickyPromptFromViewport`: shows a "sticky" floating prompt banner (like a sticky header) only when the newest user prompt has scrolled entirely above the visible viewport AND its answer isn't fully visible either — hides it once any part of a newer user message is on-screen, and always hides it when pinned to the bottom (`atBottom=true`).
- `viewportSnapshotKey`/`scrollbarSnapshotKey` build cheap string keys from viewport state (`top:bottom:height:scrollHeight:pending`) explicitly including in-flight "pending scroll delta" for the *content* viewport but the scrollbar-thumb position is computed from the **committed** scrollTop only (deliberately excludes pending delta) — scrollbar thumb tracks actual position, not an animation-in-flight target.
- `computeWheelStep`/`initWheelAccel` implements two distinct acceleration curves for native-terminal wheel events vs xterm.js-style events: same-direction fast repeats ramp a multiplier up (never below base=1); a gap beyond a timing window resets to base; a direction **flip is deferred one event** for "bounce" detection (a mouse's return-to-neutral micro-bounce shouldn't register as a real reversal) — flip-back within the bounce window engages `wheelMode`; flip-back outside it is treated as a genuine reversal. `wheelMode` also auto-disengages after 5 consecutive sub-5ms events (trackpad signature, distinct from a physical wheel) or 1.5s of idle.

### 4.5 Virtualized history rendering (`virtualHeights.test.ts`, `virtualHistoryClamp.test.ts`, `virtualHistoryOffsetCache.test.ts`, `useVirtualHistoryHeights.test.ts`) — most relevant if Atlas's transcript pane ever needs virtualization for long sessions
- Height estimation is **capped at ~800 rows** even for a pathological 1MB single-line message, specifically so an offset-rebuild triggered on every uncached message doesn't block the UI on cold mount; verified to execute in well under 50ms even at that size. Real height converges post-mount via actual layout measurement.
- Invalid measured heights (`NaN`, `±Infinity`, negative, absurdly large like 1 billion) are explicitly "quarantined" — neither cached nor used to adjust scroll position — and the estimator's own return value is separately validated the same way, both falling back to a safe default.
- Height caches are pruned to only the currently-active item keys on each pass (stale/removed items' cached heights are dropped, not accumulated forever).
- Scroll-position compensation ("keep visual anchor in place when content above the viewport resizes") only fires for rows **above or overlapping** the current viewport — a resize below or fully inside the visible viewport does not adjust scroll, and while pinned to the live tail (sticky), no compensation happens at all even for a large resize (the content just grows below the fold).
- A generation/layout-version counter guards against a stale unmount-time height measurement (from the *previous* item set, mid width-transition) polluting the new item set's cache.
- `shouldSetVirtualClamp`: never clamps while sticky (following live tail) or while a live tail is actively growing beneath the virtualized region, even if not sticky — clamping only kicks in once the user has manually broken away from the bottom.

---

## Files touched only lightly above but still fully read (accounted for)
`subscriptionCommand.test.ts`, `subscriptionOverlay.test.tsx`, `topupCommand.test.ts`, `usageCommand.test.ts`, `wakeCommand.test.ts` — all billing/subscription/wake slash-command tests; Atlas-irrelevant business logic (Nous Portal billing flows, credit balances, wake-word listener ownership) but the **pattern** worth noting: every RPC-backed slash command test builds a fake `ctx` with a `rpc` mock that routes by method name to a results map, and asserts on `sys()`-printed transcript lines — a clean, reusable test-harness shape if Atlas adopts a similar RPC/gateway pattern.
`useBatteryPoll.test.ts`, `useCompletion.test.ts`, `useComposerState.test.ts`, `useConfigSync.test.ts`, `useInputHandlers.test.ts`, `useSessionLifecycle.test.ts`, `useSubmission.test.ts`, `userWidgets.test.ts`, `widgetGrid.test.ts`, `widgetGridComponent.test.tsx`, `widgetSdk.test.ts`, `weatherApp.test.ts`, `thinkingLiveCollapse.test.tsx`, `thinkingMoaReferenceVisibility.test.tsx` — read in full; notable smaller findings:
- `looksLikeDroppedPath`: heuristically detects a dragged-and-dropped file path pasted into the composer (macOS screenshot temp paths, `file://` URIs, image extensions, escaped-space paths) vs. ordinary multiline text paste, so the composer can offer "attach as file" instead of inline text.
- `resolveCtrlCComposerAction`: Ctrl+C priority order is **clear draft > interrupt running turn > exit** — a non-empty composer always eats Ctrl+C to clear itself first, even mid-stream; only an empty composer While busy interrupts the turn; only idle+empty exits.
- ToolTrail "thinking" panel auto-expands while reasoning is actively streaming and auto-collapses the instant streaming ends, but ONLY when the user's preference is `collapsed` (the default) — an explicit `expanded` preference is never auto-collapsed regardless of streaming state. A related regression test (#64701) specifically checks that a *post-mount* re-sync effect doesn't clobber an initial `reasoningAlwaysVisible` mount value.
- Widget SDK (`widgetSdk.test.ts`, `widgetGrid.test.ts`, `widgetGridComponent.test.tsx`) is a fairly elaborate ambient-app-panel system (modal vs ambient, docks vs corner rails, CSS-grid-like `layoutGridAreas`/`resolveGridTracks` with fr-units, min-sizes, row/col spans, dense auto-flow) — likely out of scope for Atlas but the grid-layout algorithm (`resolveGridTracks`) is a nicely self-contained, reusable pure function if Atlas ever wants a flexible pane-splitting layout.

---

## Top recommendations for Atlas

Ranked by (behavioral value the user actually feels) / (implementation effort), given Atlas's current input is a bare bubbles/textarea with no history, no queueing, no advanced cursor ops.

1. **Line-scoped Ctrl+U/Ctrl+K with newline-consuming repeat** (§1.3). Cheap to implement (pure string-slicing logic, no async), immediately makes multiline composing feel like a real terminal editor. The "second press consumes the boundary newline so repeats make progress" detail is the non-obvious part worth copying exactly.

2. **Message queue while busy** (§2.1, `useQueue.test.ts`). Atlas has no equivalent at all — currently a user presumably can't type ahead while the agent is streaming. The collapsed-display/full-text pairing pattern and in-place edit semantics are small, well-specified, and high perceived value (matches Claude Code / Codex CLI UX).

3. **Synchronous busy-flag flip on submit, before any async round-trip** (§2.2). Directly prevents a double-submit race on rapid Enter — cheap one-line fix pattern (`markSubmitting()` before the async call, not inside its `.then`), likely already a latent bug class in any bubbletea app that awaits before updating state.

4. **Ctrl+C priority: clear draft > interrupt > exit** (§4, `resolveCtrlCComposerAction`). A one-function decision table; meaningfully changes what feels like "my agent nukes my in-progress question when I hit Ctrl+C" vs. matching user expectations from other CLI agents.

5. **Terminal-mode reset on exit, single synchronous write** (§3.1). If Atlas enables any mouse tracking or bracketed paste, it needs an equivalent `process.on...`-analogous safety net (Go: `signal.Notify` + deferred write, or an atexit-style hook) that disables every sticky mode in one shot — otherwise killing Atlas can leave the parent shell reading mouse garbage as keystrokes.

6. **Incremental streaming-markdown boundary scanner** (§2.6). If Atlas's Glamour rendering re-renders the whole response on every token today, this pattern (split only at blank lines outside open fences/math blocks, append-only committed blocks) is the standard fix for streaming-render flicker/cost, and it's a pure, well-tested function — good fit for direct porting.

7. **Fast-echo bypass for the ASCII common case** (§1.1). Higher effort (needs careful terminal-capability detection and a raw-write path alongside Bubbletea's render loop) but is the single biggest "why does this feel snappier than my textarea" lever if Atlas's input ever feels laggy under load. Can be deferred; the guard-condition list (end-of-line, no wrap, ASCII-only, wrap-boundary-aware backspace) is the hard-won part to copy.

8. **Cursor-layout parity with the actual wrap algorithm** (§1.2). If Atlas computes cursor position independently from how Lipgloss/the render pipeline actually wraps text, character-by-character incremental testing against the real wrap function (not just spot-checks) is the right regression-prevention pattern — this is exactly the kind of subtle off-by-one that "looks fine in the demo, breaks on real typing."

9. **Transactional clipboard cut** (§1.4). Small, prevents real data loss (headless/SSH environments frequently lack clipboard access) — never remove selected text until the clipboard write is confirmed.

10. **Flash-vs-sticky notice lifecycle** (§2.3). A tiny, generically useful state-machine idea (some status messages should auto-clear on the next action, others persist until resolved) worth adopting for Atlas's own status/error bar independent of the (Atlas-irrelevant) billing context it appears in here.
