# Hermes Agent TUI — Deep Analysis for Atlas Redesign

Source: `github.com/NousResearch/hermes-agent` (MIT), shallow sparse-cloned into scratchpad,
sparse-checkout of `ui-tui/`, `tui_gateway/`, `docs/`. Note: `docs/user-guide/` does **not**
exist in this repo (no `tui.md`); the equivalent documentation lives in `ui-tui/README.md`,
which was read in full and is the primary doc source below. All code below is TypeScript +
React + a custom Ink fork (`packages/hermes-ink`, aliased `@hermes/ink`); the backend is
`tui_gateway/` (Python), talked to over newline-delimited JSON-RPC on stdio.

No code is copied into Atlas — every recommendation below is a translation note from
Ink/React/flexbox idioms to Go/Bubbletea/Lipgloss idioms.

---

## 1. Overview — architecture

- **Process model**: `ui-tui/src/entry.tsx` is a TTY gate that spawns `python -m
  tui_gateway.entry` as a child process (`gatewayClient.ts`) and speaks newline-delimited
  JSON-RPC over stdio. stdout = protocol; stderr = captured into an in-memory ring and
  surfaced as `gateway.stderr` events — **never written directly to the terminal** (avoids
  corrupting the alt-screen). Malformed stdout lines become `gateway.protocol_error` events
  rather than crashing.
- **Rendering model**: Ink (React reconciler targeting terminal cells) with a yoga-based
  flexbox layout engine (`packages/hermes-ink/src/ink/layout/*`, `native-ts/yoga-layout/*`).
  The app renders inside `AlternateScreen` (full alt-screen buffer) unless `INLINE_MODE` is
  set, in which case it renders as a plain flex column so native terminal scrollback works.
- **State**: nanostores (`$overlayState`, `$uiState`, `$delegationState`, `turnStore`, etc.) —
  small atomic global stores subscribed via `useStore`, not prop-drilled React context for
  everything. `useMainApp.ts` is the single top-level composition hook wiring every sub-hook.
- **Event flow**: gateway emits typed events (`message.delta`, `tool.start`, `tool.complete`,
  `reasoning.delta`, `approval.request`, `clarify.request`, `subagent.*`, `status.update`,
  `notification.show`, etc. — see README's full event table) → `createGatewayEventHandler.ts`
  maps them into `turnController.ts` (a stateful class, not a reducer) which buffers
  streaming deltas, debounces renders, and patches `turnStore`/`uiStore`.
- **Turn lifecycle** is explicitly a class-based controller (`TurnController` in
  `turnController.ts`, 1105 lines) rather than a plain reducer — it owns timers for
  streaming-batch flushing, reasoning-pulse timers, tool-progress debouncing, activity-trail
  limiting (`ACTIVITY_LIMIT=8`, `TRAIL_LIMIT=8`), and interrupt-cooldown (1500ms).
- **Theme model**: seeds → derived tokens → skin overrides → contrast/polarity adaptation, all
  computed at runtime in `theme.ts` (see §3). This is the single most sophisticated piece of
  the whole codebase and the highest-value transferable idea.

---

## 2. File-by-file catalog (every file actually read)

### Top-level / docs
- **`ui-tui/README.md`** (492 lines) — the closest thing to `docs/user-guide/tui.md`. Contains
  full hotkey tables, event table, slash-command registry, file map, theme model summary.
  Treat this as ground truth for interaction rules (queue semantics, `!cmd`/`{!cmd}`
  interpolation, editor handoff via `Cmd/Ctrl+G`, double-Esc-to-clear).

### `src/theme.ts` (969 lines) — **the color system**
- Defines `ThemeColors` (30+ named tokens: primary/accent/border/text/muted, completion*4,
  label/ok/error/warn, tool/thinking, syntax*4, prompt/sessionLabel/sessionBorder,
  status*5/selectionBg, diff*4, shellDollar) and `ThemeBrand` (name/icon/prompt/welcome/
  goodbye/tool/helpHeader).
- **Seeds → tokens architecture**: a theme/skin only supplies ~15 *seed* colors
  (`ThemeSeeds`: accent, bg, border?, error, ok, primary, prompt?, shellDollar, status*4,
  text, warn, plus optional identity-fill overrides `activeRow`/`muted`/`selection`/`surface`).
  Every other token (`muted`, `label`, `statusFg`, `surface`, `activeRow`, `selection`,
  `border` fallback) is **derived** via `deriveTones()` using a documented, reverse-engineered
  "mix ladder" (e.g. dark `muted ≈ desaturate(mix(accent, bg, .19), .16)`). This guarantees a
  skin can never ship an "incoherent dim" — every derived tone is mathematically tied to the
  skin's own identity colors.
- **Exact default palette (dark)**: accent `#FFBF00`, activeRow `#333355`, bg `#101014`,
  border `#CD7F32`, error `#ef5350`, ok `#4caf50`, primary `#FFD700`, prompt `#FFF8DC`,
  selection `#3a3a55`, shellDollar `#4dabf7`, statusBad `#FF8C00`, statusCritical `#FF6B6B`,
  statusGood `#8FBC8F`, statusWarn `#FFD700`, surface `#1a1a2e`, text `#FFF8DC`, warn `#ffa726`.
- **Exact default palette (light)**: accent `#956E00`, bg `#ffffff`, border `#A56628`, error
  `#C14240`, ok `#367E39`, primary `#867000`, prompt `#2B2014`, shellDollar `#377BB3`,
  statusBad `#A65A00`, statusCritical `#B94D4D`, statusGood `#5C7A5C`, statusWarn `#867000`,
  text `#3D2F13`, warn `#956115`. (Light seeds are explicitly `liftForContrast(dark, white,
  4.5)` of the dark seeds — i.e. the light theme is *computed from* the dark theme, not
  independently hand-picked.)
- **Diff colors**: dark `diffAdded rgb(220,255,220)` / `diffRemoved rgb(255,220,220)` /
  `diffAddedWord rgb(36,138,61)` / `diffRemovedWord rgb(207,34,46)`; light equivalents also
  defined (`DIFF_LIGHT`).
- **Contrast/polarity guarding** (`adaptColorsToBackground`): every foreground token is run
  through `liftForContrast` against the *real* resolved background with two floors —
  `DISPLAY_MIN_CONTRAST = 1.45` (dark) / `1.18` (light) for decorative/display text, and
  `SEMANTIC_MIN_CONTRAST = 2.2` (dark) / `1.6` (light) for meaning-bearing colors
  (ok/error/warn/status*). Background-role fills (completion menus, status bar, selection)
  are polarity-checked (`LIGHT_BG_MIN_LUMINANCE=0.4`, `DARK_BG_MAX_LUMINANCE=0.35`) and reset
  to the safe derived value if a skin's fill would land on the wrong pole (e.g. a navy menu
  fill accidentally shown on a light terminal).
- **Runtime background detection** (`detectLightMode`): explicit env var
  `HERMES_TUI_LIGHT`/`HERMES_TUI_THEME` wins, then a cached OSC-11-probed
  `HERMES_TUI_BACKGROUND` hex, then `COLORFGBG` (last field, slots 7/15 = light, 0-15 else =
  dark), then a `TERM_PROGRAM` allow-list (`Apple_Terminal` defaults light). Anything
  undecidable defaults dark.
- **ANSI-256 fallback path** (`normalizeThemeForAnsiLightTerminal`): for terminals without
  truecolor (specifically Apple Terminal without `COLORTERM=truecolor`), foreground colors are
  quantized to the nearest *readable* one of the 256-color cube via a hand-tuned scoring
  function (`bestReadableAnsiColor`) that penalizes candidates above a luminance ceiling and
  rewards hue/sat/lightness proximity — i.e. even the "worst case" 256-color terminal gets a
  deliberately computed palette, not naive truncation.
- `fromSkin()` is the entry point that turns arbitrary skin JSON (color keys like `ui_accent`,
  `banner_accent`, `status_bar_good`, `diff_added`, etc.) into a fully-derived, contrast-safe
  `Theme`. Skins may override individual derived tokens, but anything unset falls through the
  ladder.
- **Atlas translation**: Atlas's `internal/tui/styles.go` almost certainly hardcodes a fixed
  palette. The seed→derive→adapt architecture is the single highest-leverage idea to port:
  define ~12–15 seed colors, derive `muted`/`label`/`border`/`selection` via lipgloss color
  blending (Go has no native color-mix, but this is trivial: parse hex → lerp RGB → back to
  hex), and add a light/dark contrast floor pass using `github.com/lucasb-eyer/go-colorful` or
  hand-rolled WCAG luminance math (the formulas above are directly portable — same constants).

### `src/lib/color.ts` (325 lines) — the color primitive (theme.ts depends on it)
- `parseColor` (accepts `#rgb`, `#rrggbb`, `rgb()`), `mix` (sRGB lerp — **not** perceptual,
  deliberately, for parity with CSS `color-mix(in srgb, ...)`), `relativeLuminance` (WCAG),
  `contrastRatio`, `ensureContrast` (step-mix toward black/white pole until ratio clears a
  minimum, re-mixing from the *original* each step so hue decays linearly not exponentially),
  `liftForContrast` (xterm.js's own multiplicative 10%-step luminance algorithm — chosen
  specifically so a manually-lifted palette byte-matches what VS Code/Cursor's own
  `minimumContrastRatio` would have produced), `grayOf`/`desaturate`/`retone`/
  `boostSaturation`, and a chainable `color(x).mix().darken().ensureContrast().hex()` API.
- **Translation note**: this entire file is ~300 lines of pure math with zero Ink/React
  dependency — it is the most directly portable file in the repo. A Go port (`internal/tui/
  color.go`) implementing `Mix`, `RelativeLuminance`, `ContrastRatio`, `LiftForContrast`,
  `Desaturate` would let Atlas's `styles.go` adopt the exact same derivation ladder.

### `src/components/appLayout.tsx` (596 lines) — **top-level screen composition**
- `AppLayout` is the root: `Shell` = `AlternateScreen` (or `Fragment` in inline mode) wrapping
  a `flexDirection: column` root Box containing, top to bottom: a `flexDirection: row` band
  (left ambient rail | main content | right ambient rail), then (if no full-screen overlay)
  `PromptZone` (blocking prompts), `ComposerPane` (input), optional `FpsOverlay`, and a
  floating `PetPane` absolute-positioned bottom-right.
- Main content is one of three mutually exclusive full-height panes: `AgentsOverlayPane`
  (subagent delegation tree, full screen), `JourneyPane` (a full-screen view), or
  `TranscriptPane` (the normal chat transcript) — routed by `$overlayState`.
- **`TranscriptPane`**: a `ScrollBox` (custom scrollable Box, `stickyScroll` prop) containing a
  virtualized list (`virtualHistory.start/end`, top/bottom spacer Boxes for windowing) of
  message rows. Notable details:
  - A `───` divider renders above every user message *after* the first one (multi-turn visual
    segmentation), colored `theme.color.border`.
  - Intro/banner and generic "Panel" message kinds render specially (full Banner + SessionPanel,
    or a titled bordered Panel).
  - `LiveTodoPanel` is injected as a child of the *last user message's row* specifically so it
    scrolls with that turn (visually "belongs" to the active prompt).
  - A custom right-side `TranscriptScrollbar` (see overlayPrimitives) renders in its own
    1-column `NoSelect` gutter — draggable, using `┃`(thumb)/`│`(track) chars.
  - A floating "pet" mascot (`PetPane`) reserves either a right-side gutter or bottom row-band
    depending on terminal width (`MIN_GUTTER_BODY_COLS = 72`) so the mascot never overlaps
    transcript text — a nice example of responsive collision-avoidance for a decorative
    element. (Almost certainly out of scope for Atlas, but the *pattern* — a decorative overlay
    that publishes its footprint so layout code reserves space — is reusable for e.g. a
    "typing indicator" ghost.)
- **`ComposerPane`**: `NoSelect` column containing (top to bottom): `QueuedMessages` preview,
  a "N background tasks running" line, a "sticky prompt" (↳ breadcrumb of the scrolled-past
  user message) OR a 1-row spacer, `StatusRulePane` (if `statusBar==='top'`), `AmbientDock`
  (top), then the actual input row: `FloatingOverlays` (completions dropdown, absolutely
  positioned `bottom: 100%` above the input — i.e. dropdown grows *upward*), a `?`-triggered
  `HelpHint`, the multiline input buffer rows + live `TextInput` row with a `GoodVibesHeart`
  absolute-positioned top-right easter-egg, then `AmbientDock` (bottom) and
  `StatusRulePane` (if `statusBar==='bottom'`, the default).
- **Layout → Lipgloss mapping**: Ink's `flexDirection: row` + `flexGrow`/`flexShrink` on
  sibling Boxes maps directly to `lipgloss.JoinHorizontal` with fixed-width panes computed in
  Go (Lipgloss has no true flexbox, so widths must be pre-computed, exactly as Atlas's
  `picker.go`/`diff.go` likely already do). The **3-row-band root layout** (rail|content|rail
  → prompt-zone → composer) maps directly to `JoinVertical(Top, header, JoinHorizontal(...),
  promptZone, composer)`.

### `src/components/appChrome.tsx` (952 lines) — **status bar, busy indicator, scrollbar**
- **`FaceTicker`**: the busy/"thinking" indicator in the status bar. 4 selectable styles via
  `IndicatorStyle` (`/indicator` command): `kaomoji` (rotates through `FACES` pool every
  `FACE_TICK_MS=2500`), `emoji` (6-frame emoji cycle, `SPINNER_TICK_MS*6=600ms`), `ascii`
  (`|/-\\`, 100ms), `unicode` (braille spinner via `unicode-animations`, no verb text). Each
  style pairs the glyph with a rotating **verb** from `VERBS` (pondering, contemplating,
  musing, cogitating, ruminating, deliberating, mulling, reflecting, processing, reasoning,
  analyzing, computing, synthesizing, formulating, brainstorming — 15 total, padded to a fixed
  width `VERB_PAD_LEN` so the status bar doesn't jitter as the verb rotates) and a live
  elapsed-time clock (`fmtDuration(now - startedAt)`, ticking every 1s). Critically, all
  timers are **paused while an overlay occludes the status rule** (`$isStatusRuleOccluded`) —
  otherwise a hidden component would keep re-rendering for nothing.
- **Status bar composition** (`StatusRule`, the actual bottom bar): a single-height `Box`,
  strictly priority-ordered left-to-right with **progressive disclosure by terminal width**:
  1. leading `─ ` chrome, then optional battery (`⚡`/`🔋 NN%`), then the busy face OR idle
     status text OR a "notice" banner (credits/error banners share this slot).
  2. **Pinned, never-shrinking**: `model │ context`. Model text is cleaned via
     `shortModelLabel` (strips `claude-`/`anthropic-` prefixes, replaces `-`/`_` with spaces,
     collapses `3 5` → `3.5`). Context shows as `12k/128k` tokens or, below 72 cols, collapses
     to a bare `12k tok`.
  3. **Tail segments, added only if they fit** (`statusBarSegments(cols)` breakpoints):
     `bar` (visual `[████░░░░░░] 34%` context fill, ≥72 cols), `duration` (≥76), `compressions`
     count (≥80, colored red≥10/orange≥5), `voice` (≥84), `bg` task count (≥88), `subagents`
     count (≥92), `cacheHit %` (≥96), `latency` in seconds (≥104), `tps` tokens/sec (≥110).
     Each segment is tried via a `fits(width)` budget-consuming closure — **lowest priority
     drops first**, nothing ever truncates mid-segment.
  4. **Right-aligned**: cwd path (or session title if set, bold+accent-colored) — this is the
     *first* thing to yield space on narrow terminals (computed via `statusRuleWidths()`,
     which reserves `essentialWidth` for the pinned left segments before handing whatever's
     left to the right label).
  5. `SpawnHud`: a compact "d2/3 ⚡4/8" subagent depth/concurrency HUD appended at the very end,
     colored gray→warn(66%)→error(100%) as the delegation tree approaches its configured caps.
- **Exact context-bar coloring** (`ctxBarColor`): ≥95% → `statusCritical`, >80% → `statusBad`,
  ≥50% → `statusWarn`, else → `statusGood`. Bar rendered as `█`×filled + `░`×empty over 10
  cells (`ctxBar`, `w=10`).
  Battery coloring mirrors the same 4-tier scheme but is driven by a server-computed category.
- **`GoodVibesHeart`**: a random-colored `♥` glyph that flashes for 650ms on a trigger tick,
  color randomly chosen from `[error, warn, accent]` — purely decorative positive-feedback
  micro-animation, positioned absolute top-right of the input row.
- **`TranscriptScrollbar`**: draggable custom scrollbar — thumb size = `round(vp*vp/total)`,
  drag-to-scroll via `onMouseDown`/`onMouseDrag`, hover/grab both change thumb color
  (`t.color.accent` when hovered/grabbed, else `t.color.primary`); track color is `mix(muted-or-
  border, completionBg, 0.55-or-0.25)` — **never** SGR `dim` (explicitly commented: dim renders
  as an opaque black slab on transparent-background terminals, so real color blending is used
  instead).
- **Atlas translation**: Atlas's status bar (per user: "already has... a status bar with filled
  backgrounds") should adopt (a) the **priority-ordered progressive disclosure by width**
  pattern — compute a `fits()` budget and drop segments in a fixed priority order rather than
  truncating text; Lipgloss can measure rendered width with `lipgloss.Width()` for this. (b)
  The **pinned-vs-shrinkable** distinction: some segments (status/model/context) never drop;
  cwd/branch is the first thing sacrificed. (c) A **busy indicator with rotating verb +
  elapsed-time**, exactly like Claude Code's own "Simmering… (12s · esc to interrupt)" pattern
  — Atlas can reuse a verb list and a `time.Since(start)` ticker in a `tea.Tick`. (d) The
  **occlusion-aware timer pause** — Bubbletea equivalent: stop the ticker `tea.Cmd` chain (or
  gate the render) while an overlay covers the status line, to avoid wasted re-renders.

### `src/components/prompts.tsx` (501 lines) — **approval / clarify / confirm prompts**
- **`ApprovalPrompt`**: double-border (`borderStyle="double"`) box, `borderColor: warn`, header
  `⚠ approval required · {description}` in bold warn, then the command text **word-wrapped to
  panel width** (`wrapAnsi`, hard-wrap) and clipped to `CMD_PREVIEW_LINES=10` with an
  `… +N more lines` footer if longer — this avoids single-line truncation of long shell
  commands (a real UX bug class). Options list (`once/session/always/deny`, filtered
  dynamically — `always` is dropped if a "tirith warning" flag is present, or narrowed to just
  `once/deny` if `smartDenied`) rendered as numbered rows with a `▸`/`  ` cursor and a
  **selection chip** (bg fill + contrast-guaranteed ink via `chipRowProps`) on the active row.
  Footer hint: `↑/↓ select · Enter confirm · 1-{N} quick pick · Esc/Ctrl+C deny`.
  Keybinding logic is a **pure function** `approvalAction(ch, key, sel, opts)` returning a
  discriminated union (`{kind:'choose'|'move'|'noop'}`) — explicitly extracted so it's testable
  without mounting Ink; Atlas should keep its approval logic in a pure `func(msg, state)
  (action)` too for the same testability reason.
- **`ClarifyPrompt`**: supports both single-question and **batch mode** (a list of questions
  answered one at a time, `Tab`/`Shift+Tab` cycles the active question with wraparound). Each
  answered question shows a `✓` marker + its locked answer in `ok` color (or muted italic
  `(skipped)`); the active unanswered one shows `▸`; untouched ones show `·`. An "Other (type
  your answer)" row is always appended after the enumerated choices, switching into a
  `TextInput` sub-mode. Footer: `{answered}/{total} answered · ↑/↓ select · Enter lock answer ·
  Tab/Shift+Tab switch question · Esc/Ctrl+C cancel`.
- **`ConfirmPrompt`**: simple Y/N double-border dialog, `danger` prop swaps border+text color to
  `error` instead of `warn`, supports `y`/`n` letter quick-picks in addition to arrows+Enter.
- **Atlas translation**: Atlas already has a y/n approval overlay per the prompt — the two
  concrete upgrades worth stealing are (1) **word-wrap long commands to panel width with a
  capped-line + "+N more" footer** instead of single-line truncation, and (2) the
  **quick-pick number keys (1-4) alongside arrows**, which meaningfully speeds up repeat
  approvals for power users. The pure keybinding-dispatch-function pattern is good Go practice
  too (`func ApprovalKeyAction(msg tea.KeyMsg, sel int, opts []Choice) Action`).

### `src/components/todoPanel.tsx` (95 lines) — **todo/task panel**
- A collapsible section: header row `▸/▾ Todo (3/7)` (accent chevron, bold white "Todo" label,
  dim statusFg count) with `Box onClick` toggle (mouse-clickable in terminal!). When
  `incomplete` and some todos are still pending, appends a dim note `· incomplete · N still
  pending`. Body (when expanded) is `todoTree(todos)` — a `[todo, depth]` list rendered with
  `marginLeft: min(depth,4)*2` and a per-status glyph+color from `todoGlyph`/`todoTone`
  (active=text color, body=statusFg, else muted+dim). Supports controlled (external
  `collapsed`/`onToggle`, used by the "live" panel wired to turnStore) and uncontrolled
  (internal `useState`, used for archived-todo panels in scrollback) modes via the same
  component.
- **Atlas translation**: straightforward Bubbletea list component — collapsible header +
  indented tree rows, only needs (a) status glyph/color mapping and (b) a toggle key (or
  Lipgloss can't do mouse clicks the same way, so bind a keystroke instead, e.g. `t` to
  toggle). The **shared controlled/uncontrolled prop pattern** (component works standalone in
  history OR wired live) is a good Go pattern too: accept an optional `*bool` + callback,
  fall back to internal state when nil.

### `src/components/accordion.tsx` (58 lines) — **the single expand/collapse primitive**
- Explicitly documented as "THE expand/collapse primitive" reused for session-panel
  Tools/Skills/System-Prompt/MCP sections AND for widget-app accordions. `▸`/`▾` chevron in
  accent color, bold accent title, optional `(count)` and `suffix` (e.g. "in 3 categories").
  Controlled (`open` prop) or uncontrolled (`defaultOpen` + internal state) — same dual-mode
  pattern as TodoPanel.
- **Atlas translation**: this is exactly the primitive the user's prompt asked about
  ("collapsible banner sections with ▸/▾ chevrons for Tools/Skills/System Prompt/MCP
  Servers"). Build one Go `Accordion` type/helper (`chevron string, title string, count
  *int, suffix string, open bool`) and reuse it for every collapsible section in Atlas's
  session/help panels instead of writing bespoke toggle logic per panel.

### `src/components/loaders.tsx` (172 lines) — **shimmer/skeleton loading primitive**
- `Shimmer`: a single animated "loading" run — a highlight **band of 7 cells (`BAND=7`)
  sweeps left-to-right** across a run of `▁` block characters, entering from off-screen-left
  and exiting off-screen-right (`shimmerSegments` — pure function, unit-testable). Multiple
  rows offset their `phase` by `-i*2` to create a **diagonal shimmer** across a skeleton block.
  `ShimmerRows` composes label+value skeleton rows (mimicking a `label: value` layout) with
  this shimmer, used for the session-panel's lazy Tools/Skills sections while they're loading.
- **One shared animation clock**: all mounted shimmers subscribe to a single
  `setInterval(90ms)` (`subscribeShimmerClock`) instead of each mounting its own timer — this
  was a fix for a real perf bug (many independent 90ms intervals compounding into dozens of
  react re-renders/sec). The clock auto-stops when the last subscriber unmounts.
  Animation is capped at `SHIMMER_ANIMATE_MS=30_000` — after 30s a lazy skeleton freezes in
  place instead of animating forever (still reads as "loading" but stops burning CPU).
- **Atlas translation**: this exact idea — a shimmer/skeleton with a shared tick and a bounded
  animation budget — is the highest-value visual/animation borrow for "empty state" or
  "waiting for gateway" screens in Atlas. In Bubbletea: one `tea.Tick`-driven `phase int` in
  the root model, passed down to any number of skeleton-row renderers; cap total animated
  duration so a stuck loading state doesn't spin forever.

### `src/components/messageLine.tsx` (359 lines) — **transcript row renderer**
- Central per-message-kind switch: `trail` (todo panel or tool/reasoning trail),
  `tool` role (rounded-border preview box, ANSI passthrough or truncated text),
  `event` kind (dim `◈ {text}` marker, no gutter — model switches, delegation completions),
  `slash` kind (muted echo), long-system-message (collapsible, `▸/▾` + char count),
  assistant (streaming vs settled markdown via `StreamingMd`/`Md`), user (paste-backed long
  message gets a dim `[long message]` marker; `@ref`/skill tokens keep their composer accent
  color instead of flattening to body text).
- **Grouping/spacing rule** (`hasLeadGap`, in `domain/blockLayout.ts`, not read line-by-line
  but referenced extensively): a blank line is inserted above a block **iff it opens a new
  visual group relative to the stable predecessor** — computed per-block so a hidden block
  (e.g. tool trail hidden by `/details`) never leaves a floating empty gap. This is a subtly
  important detail: naive spacing (fixed margin per message type) produces uneven double-gaps
  when sections are toggled off; Hermes computes gaps from *what actually rendered*, not from
  message type alone.
- Every user/assistant row has a per-role **gutter** (`transcriptGutterWidth`) holding a glyph
  (from `ROLE[msg.role](t)` → `{glyph, prefix, body}` color triple) — this is the `❯`/`●`-style
  left-column marker pattern.
- `display.timestamps` support: optional dim `[HH:MM]` stamp rendered in its own row above the
  glyph row, gutter-aligned.
- A `└─ Response` separator row renders between a hidden tool-trail and the assistant's final
  text, only when there *was* a details section that's now collapsed — gives the reader a
  visual anchor for "here's where the actual answer starts" even with tools hidden.
- **Atlas translation**: the **content-driven spacing rule** (gap iff something actually
  rendered a new group, not gap-per-type) is worth implementing precisely — it's the fix for
  uneven blank-line spacing that most homegrown TUI chat renderers get wrong. The **gutter +
  glyph + colored prefix per role** is standard and Atlas likely has an equivalent already;
  what's probably missing is the `└─ Response` anchor row after a collapsed trail.

### `src/components/thinking.tsx` (partial read, first 450/1256 lines) — **spinners, tool trail tree, subagent tree**
- `Spinner`: picks a random braille-style spinner name from a curated pool —
  `THINK = [helix, breathe, orbit, dna, waverows, snake, pulse]` for reasoning/thinking state,
  `TOOL = [cascade, scan, diagswipe, fillsweep, rain, columns, sparkle]` for tool-execution
  state — via the `unicode-animations` package, giving visually distinct animated glyphs
  depending on *what kind* of work is happening (reasoning vs tool call), re-randomized per
  mount (`useMemo` keyed on `variant`).
- `StreamCursor`: a blinking `▍` cursor rendered at the end of in-flight streaming text,
  toggling every 420ms, only while actually streaming (frozen solid when not streaming/not
  visible).
- **Tree-drawing primitives** (`TreeRow`/`TreeTextRow`/`TreeNode`): build `├─ `/`└─ ` prefixed
  rows with `│ `/`  ` continuation rails (`treeLead`, `nextTreeRails`) — this is a full
  ASCII-tree renderer for nested tool calls / subagent hierarchies, dimmed by default
  (`stemDim=true`), with an optional `stemColor` override.
- `Chevron`: the accordion header used *inside* the tool/thinking trail specifically — supports
  Shift/Ctrl-click to "expand all" descendants at once (`onClick={e => onClick(!!e.shiftKey ||
  !!e.ctrlKey)}`), and a `tone` prop (`dim`/`error`/`warn`) that recolors both the label and
  chevron together.
- `SubagentAccordion`: a very elaborate per-subagent card — status-tinted (error/warn/dim),
  shows a compact "rollup" suffix line (`running · 4.2s · 3 tools · 812 tok · ⎘2 · 2↓ · +5t sub
  · ⚡1`) summarizing status/elapsed/tool-count/tokens/files-touched/descendant-count/nested-
  tool-count/active-count, all space-separated with `·`. Has independently-toggleable
  Thinking/Tool-calls/Notes/Children sub-sections, each its own `Chevron`.
- **`heatColor`**: colors a subagent tree branch by a computed "hotness" bucket, mapped onto a
  5-step palette `[border, accent, primary, warn, error]` — cool branches (bucket <2) get
  *no* override color (fade into dim chrome), only "hot" branches (high activity) draw the eye
  with escalating color. This is a genuinely nice attention-directing technique for a
  fan-out/subagent view.
- **Atlas translation**: if/when Atlas gets subagent or multi-step tool visualization, the
  **tree-rail rendering** (`├─`/`└─`/`│ ` continuation) is directly portable to Lipgloss (just
  string concatenation, no flexbox needed) and the **heat-colored branches** technique (color
  only what's "hot", let quiet branches recede to a single dim tone) is a strong pattern for
  keeping busy trees legible. Independently for the *near-term*, Atlas should adopt:
  variant-specific spinner pools (different frames for "thinking" vs "running a tool" vs
  "waiting on network") instead of one universal spinner, and a blinking-cursor treatment for
  in-flight streamed text.

### `src/components/branding.tsx` (563 lines) — **banner + session info panel**
- **Responsive `Banner`** with **4 discrete tiers** picked purely by terminal column count (no
  scaling, since terminal glyphs can't scale): full ASCII-art logo (`cols >= logoW+2`) → a
  3-row "compact banner" (`cols >= COMPACT_FROM=58`: a text rule bracketing the brand name,
  a centered tagline, another rule) → shortened name+icon two-liner (`cols>=46`) → hidden
  entirely below `HIDE_BELOW=34` cols. Exact thresholds: `HIDE_BELOW=34`, `COMPACT_FROM=58`,
  name shortens to first word below `cols<52`, tagline shortens progressively at `<64`/`<46`.
- **`SessionPanel`**: rounded-border panel (`borderStyle="round"`) with a **two-column grid on
  wide terminals** (`wide = cols>=90 && leftW+40<cols`): left column = hero ASCII art +
  model/cwd/session-id; right column = version header + collapsible Accordions for
  Tools/Skills/System-Prompt/MCP-Servers (exactly the 4 sections named in the task prompt).
  On narrow terminals it collapses to a single column with model/cwd/session inlined above the
  same accordions. Uses `ShimmerRows`/`InlineLoader` for lazy-loading skill/tool lists.
- Footer line: `{N} tools · {N} skills · {N} MCP · /help for commands`, plus an optional bold
  warn "! N commits behind — run `hermes update` to update" banner, and an install-warning row.
- **Atlas translation**: the **column-count-driven tier system for the banner** (not a single
  "does it fit" boolean, but 4 distinct layouts) is a strong, concrete pattern — Atlas's ASCII
  banner (if any) should pick from a small set of pre-authored layouts by `lipgloss.Width`
  breakpoints rather than trying to reflow one design. The **session info panel's 4 named
  accordion sections** map exactly to what the user described wanting; build one `Accordion`
  Go type (see accordion.tsx note above) and instantiate it 4 times.

### `src/components/streamingMarkdown.tsx` (166 lines) + `markdown.tsx` (not fully read, referenced) — **incremental markdown**
- Problem stated explicitly in the file's header comment: naively re-rendering
  `<Md text={fullTextSoFar}/>` on every streaming delta is O(total) per delta → O(total²)
  over a whole response. Solution: an **incremental line-by-line scanner**
  (`advanceScan`/`applyLine`) that tracks fence-open/math-open state across deltas in a
  `useRef`, and **freezes completed top-level blocks** (split at blank-line boundaries outside
  code fences) into an append-only array — each frozen block is rendered by its own memoized
  `<Md>` that never re-renders again once committed. Only the live in-flight tail re-parses
  per delta (O(tail) not O(total)). Explicit invariants documented: a partial trailing line
  never triggers a block boundary (it might still open a fence); an unmatched `$$`/`\[` display-
  math opener is treated as open forever within a streaming render (conservative — a committed
  block can't be un-decided once its closer streams in later).
- **Atlas translation**: if Atlas re-renders full markdown on every stream delta (likely, given
  Glamour's renderer isn't cheap), this is the single most concrete **performance-driving UX
  fix** available — split already-received text at the last blank-line-outside-fence boundary,
  cache the rendered (Glamour-rendered) output for every settled block, and only re-run Glamour
  on the small in-flight tail. This directly fixes any visible "jank" or slowdown during long
  streaming responses, which matters for perceived "polish."

### `src/components/overlayPrimitives.tsx` (247 lines) — **shared overlay/list-row visual language**
- `clampOverlayWidth(preferred, maxWidth, min=24)`: absolute-clamp helper — a picker "prefers"
  a width but must obey the parent's `maxWidth` even below its own comfort floor.
- `scrollbarColors(t, hover, grabbed)`: **the single scrollbar treatment reused everywhere**
  (transcript + every overlay) — thumb = `accent` if hover/grabbed else `primary`; track = a
  blend of (`border` if hover else `muted`) mixed 25%/55% toward `completionBg`. Comment
  explicitly warns against using SGR `dim` for this because it renders as a black slab on
  transparent-background terminals — **use real theme-blended colors for anything that must
  look "receded," never terminal dim attributes.**
- `listRowStyle(t, active)` / `chipRowProps(t, active)`: **the single selected-row treatment**
  used by completions dropdown, session switcher, and every picker — a background-color "chip"
  (`completionCurrentBg`) on the active row, with ink color re-lifted for contrast against that
  *specific* chip fill via `liftForContrast(text, chipBg, 4.5)` (so a cross-polarity theme,
  e.g. dark palette shown on a light terminal, never produces unreadable text-on-chip).
  Explicitly documented as a **replacement for `inverse`** (ANSI reverse-video), because
  `inverse` depends on the terminal's own unknowable default fg/bg pair.
- `MenuRow`/`ActionRow`: a numbered `▸ 1. Label` row and a plain `▸ Label` row, sharing the
  chip treatment.
- `UsageBars`/`usageBarsText`: a two-bar (`[██████░░░░]`) dollar-usage display — plan usage bar
  + top-up balance bar, both built from the same `barCells(ratio, cells=10)` helper (`█`×filled
  + `░`×empty).
- **Atlas translation**: three directly reusable primitives for Go: (1) a `ClampWidth(preferred,
  max, min)` helper, (2) **never use terminal reverse-video/dim for "selected" or "receded"
  state** — always blend real theme colors, since Windows Terminal / other terminals can have
  transparent or unusual backgrounds that make `dim`/`inverse` render badly (this is a subtle
  but important correctness note, especially since Atlas's user is on Windows/PowerShell); (3)
  the **contrast-relift-on-chip** pattern — whatever "selected row" background Atlas uses,
  re-derive the foreground's contrast against *that specific* background rather than assuming
  the normal text color will still be legible.

### `src/components/modelPicker.tsx` (partial, ~260/710 lines) — **model/provider picker**
- Multi-stage wizard (`Stage = 'provider' | 'key' | 'model' | 'disconnect'`): pick a provider →
  (if no API key) enter one inline → pick a model → done. Type-to-filter fuzzy search
  (`fuzzyRank`) active on both the provider list and model list, scoped per-stage and cleared
  on stage transition; `Esc` first clears an active filter before navigating back a stage
  (two-level back-button semantics). Width is `clampOverlayWidth(preferredWidth, maxWidth)`
  pinned so long names don't cause the picker to visibly resize while scrolling —
  `wrap="truncate-end"` on rows needs a stable width to truncate against.
- **Atlas translation**: Atlas's `picker.go` should confirm it (a) pins overlay width rather
  than auto-sizing to content (prevents jitter), and (b) supports type-to-filter, not just
  arrow navigation, for any list picker with more than ~8 items (model list, session list).

### `src/components/appOverlays.tsx` (389 lines) — **overlay routing / layering**
- Two independent overlay "slots": `PromptZone` (blocking, modal — approval/clarify/confirm/
  sudo/secret/billing/subscription — rendered inline in normal flex flow between transcript and
  composer, i.e. **pushes the composer down**, not truly floating) vs `FloatingOverlays`
  (sessions switcher, model picker, pet picker, skills/plugins hub, pager, completions dropdown
  — rendered `position: absolute, bottom: 100%` i.e. **floats above the composer without
  affecting its position**, growing upward from the input row).
  Both are single-column `WidgetGrid`s (a layout-engine grid, not bespoke flex) so every panel
  gets a defined "cell budget" (`maxWidth`) rather than assuming infinite width.
- All floating panels are wrapped in `<FloatBox>` (see appChrome.tsx: `borderStyle="double"`,
  `opaque`, `alignSelf="flex-start"`) — **`opaque` is explicitly used only on things that float
  over other content**, whereas normal in-flow content (banner, transcript) explicitly avoids
  `opaque` because it paints an unwanted black slab on transparent-background terminals when
  there's nothing behind it to opacify against.
- Completions dropdown: fixed-size viewport window (`COMPLETION_WINDOW=16`) centered on the
  current selection index, not a naive top-N slice — so the selection never runs off the
  visible window even deep in a long list. Two-column layout: name column auto-sized to the
  widest *visible* item + 2 padding, description column takes the rest, so descriptions align
  vertically as a table.
- **Atlas translation**: the **two distinct overlay categories (blocking-inline vs
  floating-above)** is a good mental model to adopt explicitly if Atlas doesn't already
  distinguish them — a confirmation prompt that *pushes* the composer down reads differently
  (and is architecturally simpler in Lipgloss, since it's just another row in
  `JoinVertical`) than a completions/picker dropdown that should float without disturbing the
  input's position (in Lipgloss this means rendering the dropdown as a separate block placed
  immediately above the composer in the vertical join, sized independently). The
  **windowed-centered-scroll for a long list** (not naive pagination) is worth adopting for any
  Atlas list with >12ish items.

### `src/app/turnController.ts` (partial, 140/1105 lines) — **turn/streaming state machine**
- A hand-written class (not React state/reducer) holding: `bufRef` (streaming text buffer),
  `activeTools: ActiveTool[]`, `activeReasoningText`, multiple named `Timer`s (`statusTimer`,
  `reasoningStreamingTimer`, `reasoningTimer`, `streamTimer`, `toolProgressTimer`), a dynamic
  `streamDelay` that starts at `STREAM_IDLE_BATCH_MS` and adapts, `turnTools`/
  `persistedToolLabels` sets to dedupe repeated tool announcements, and an
  `INTERRUPT_COOLDOWN_MS=1500` guard against double-interrupt races.
- **Timing constants** (`config/timing.ts`): `STREAM_BATCH_MS=16`, `STREAM_IDLE_BATCH_MS=16`
  (both ~1 frame at 60fps — deltas are batched into a render at most every 16ms, not rendered
  per-token), `STREAM_SCROLL_BATCH_MS=96` (scroll-follow batches slower than text render to
  avoid scroll-jitter), `STREAM_TYPING_BATCH_MS=80` (render batches slow down while the *user*
  is actively typing, to avoid competing for the render budget), `TYPING_IDLE_MS=250`,
  `REASONING_PULSE_MS=700` (a pulse animation cadence for the reasoning indicator),
  `RESIZE_COALESCE_MS=32` (~30fps resize-drag coalescing — SIGWINCH bursts fire once per pixel
  on some terminals so raw resize handling would stutter), `DOUBLE_ESC_MS=500` (double-Esc to
  discard draft, explicitly noted as "Claude Code / Gemini CLI parity").
- **Atlas translation**: the core reusable idea is **batched, adaptive-rate rendering of
  streaming text** — do not re-render on every token; coalesce into a `tea.Tick`-driven flush
  every ~16ms baseline, but slow that down to ~80-100ms while the user is actively typing (so
  the terminal doesn't fight the user for repaint bandwidth) and ~90-100ms for scroll-follow
  specifically. The **double-Esc-to-clear** and **resize-drag coalescing at ~30fps** are both
  cheap, concrete wins.

### `src/app/useLongRunToolCharms.ts` (69 lines) + `src/content/charms.ts` + `src/content/verbs.ts` — **"ambient activity" messages**
- Exactly the mechanism the task asked about. For any tool still running after
  `DELAY_MS=8_000` (8 seconds), the system injects an ambient status-trail message picked
  randomly from `LONG_RUN_CHARMS = ['still cooking…', 'polishing edges…', 'asking the void
  nicely…']`, formatted as `"{charm} ({toolLabel} · {elapsedSeconds}s)"`. Repeats at most
  `MAX_CHARMS_PER_TOOL=2` times per tool, gated to at least `INTERVAL_MS=10_000` (10s) apart,
  and stops immediately if the agent goes idle or the tool completes. A `Map<toolId, {count,
  lastAt}>` tracks this per-tool so multiple concurrently-running tools each get their own
  independent charm cadence, checked on a 1s tick.
- `content/verbs.ts` also defines `TOOL_VERBS` — a static map from tool name → present-
  participle verb (`browser→browsing`, `create_file→creating`, `delegate_task→delegating`,
  `patch→patching`, `run_command→running`, `search_code→searching`, `web_search→searching`,
  etc.) used to phrase "{verb}…" labels for the active-tool line, separate from the generic
  `VERBS` pool used by the idle "thinking" ticker.
- **Atlas translation**: this is a small, cheap, high-perceived-polish feature to add verbatim
  in concept (not verbatim text — write Atlas-flavored charm strings): after N seconds of an
  unresponsive-feeling tool call, inject a one-off ambient status line ("still working…",
  "almost there…") capped at 1-2 occurrences per call, spaced ≥10s apart, cancelled the moment
  the call finishes or the whole turn is interrupted. This single mechanism does a lot to
  fight the "is it hung?" anxiety during long tool calls — currently probably Atlas's most
  visible "feels unpolished" gap versus something like Claude Code or Codex CLI.

---

## 3. Theme & color system — summary (see §2 `theme.ts`/`color.ts` for full detail)

Seeds (≈15 colors) → `deriveTones()` mix-ladder → skin overrides → `adaptColorsToBackground`
contrast/polarity guard → final `ThemeColors` (30+ tokens). Dark defaults: accent `#FFBF00`,
primary `#FFD700`, bg `#101014`, text `#FFF8DC`, border `#CD7F32`, surface `#1a1a2e`. This is
the #1 architectural idea to port — see the dedicated recommendation in §6.

## 4. Layout patterns — Ink flexbox → Lipgloss mapping

| Ink pattern | Hermes usage | Lipgloss/Bubbletea equivalent |
|---|---|---|
| `flexDirection: column` root Box | whole-screen vertical stack | `lipgloss.JoinVertical(lipgloss.Top, ...)` |
| `flexDirection: row`, `flexGrow`/`flexShrink` siblings | ambient rails flanking transcript | `lipgloss.JoinHorizontal` with pre-computed widths (Go must compute widths manually — no true flex) |
| `position: absolute, bottom: 100%` | completions dropdown floats above composer | render dropdown as a separate string, place it directly above the composer block in the vertical join (order matters, not CSS position) |
| `position: absolute, right/bottom` | pet mascot, GoodVibesHeart | overlay via manual cursor positioning / separate render pass composited with `lipgloss.Place` |
| `ScrollBox` + virtualization (`virtualHistory.start/end`, spacer Boxes) | transcript windowing | maintain a `[]string` of rendered rows + track a scroll offset; only join the visible slice, pad with blank lines for above/below spacers |
| `WidgetGrid` (columns tracks + gap) | session panel 2-col layout, banner | manual 2-column layout via `lipgloss.JoinHorizontal` of two fixed-width blocks |
| `Box borderStyle="round"/"double"` | panels, approval/confirm dialogs | `lipgloss.RoundedBorder()` / `lipgloss.DoubleBorder()` |
| `opaque` prop | floating overlays only | Lipgloss backgrounds always paint (no direct "opaque" concept needed, but the *lesson* — never paint a background color under content unless it's genuinely floating over something — is a good discipline for Atlas's own header/status bar styling, avoid filled backgrounds on in-flow banner/transcript content) |

Column-count breakpoints used throughout (banner: 34/46/52/58/64/90; status bar: 72/76/80/84/
88/92/96/104/110) demonstrate a general practice: **define named breakpoints as constants and
gate whole segments on them**, rather than ad hoc `if width > 80` scattered through render code.

## 5. Components catalog (behavior + keybindings + Atlas translation)

| Component | Looks like | Behavior / keys | Atlas note |
|---|---|---|---|
| Banner | 4-tier responsive ASCII logo/rule/name | none (static, width-driven) | pick from named breakpoints, not continuous reflow |
| SessionPanel | rounded panel, 2-col on wide term, 4 accordions | click-to-toggle (mouse) | build once, reuse `Accordion` type 4×; needs a keybinding fallback since Bubbletea mouse support is optional |
| Status bar | 1-row, priority-ordered segments | click on session-count segment opens sessions overlay | adopt the `fits()` width-budget-with-priority-drop pattern |
| Busy indicator (FaceTicker) | glyph + rotating verb + elapsed clock | 4 selectable styles via `/indicator` | add verb rotation + elapsed clock to Atlas's spinner |
| Approval prompt | double-border, warn color, wrapped command preview | ↑↓ Enter, 1-4 quick-pick, o/s/a/d letters, Esc=deny | wrap long commands + line cap; add number quick-pick |
| Clarify prompt | numbered choices + "Other" free-text, batch mode w/ Tab cycling | ↑↓ Enter, digit quick-pick, Tab cycles batch questions | batch-question mode is a bigger lift; skip unless Atlas needs multi-Q flows |
| Confirm prompt | double-border Y/N | y/n letters, ↑↓ Enter, Esc=cancel(no) | straightforward, Atlas likely has equivalent |
| Todo panel | collapsible tree, per-status glyph/color | click toggle | build as controlled/uncontrolled dual-mode component |
| Accordion | ▸/▾ chevron + bold title + (count) + suffix | click toggle | THE shared primitive — build once |
| Model/session pickers | pinned-width list, fuzzy filter, multi-stage wizard | type-to-filter, ↑↓, Esc=back-a-stage | pin overlay width; add fuzzy filter to long lists |
| Completions dropdown | floats above input, 2-col name/desc grid, windowed | Tab apply, ↑↓ cycle | windowed-centered scroll for long lists |
| Scrollbar | draggable thumb+track, hover/grab recolor | mouse drag | Bubbletea+Lipgloss can render a static thumb/track without drag if mouse isn't wired; still worth the visual (┃/│ chars) |
| Shimmer/skeleton loader | diagonal sweeping highlight band over `▁` blocks | none (auto-animates), 30s cap | good "waiting for gateway" visual for Atlas |
| Tool/thinking trail | tree-rail (├─/└─/│ ) nested rows, variant spinners | Chevron shift-click = expand-all | tree-rail rendering is pure string logic, directly portable |
| Streaming markdown | incrementally frozen blocks, only tail re-parses | n/a | biggest performance/polish win if Atlas re-renders full markdown per delta |
| Ambient "long tool" charm | one-off status line after 8s, capped 2×/10s | n/a | cheap, high-perceived-polish; implement almost verbatim in concept |

## 6. Animation & motion summary

- **Spinners**: variant-specific frame pools (`unicode-animations` package) — different braille
  animations for "thinking" vs "running a tool," randomized per-mount so repeated turns don't
  look identical. Fallback simple frame sets (`ascii: |/-\`, `emoji: ⚕🌀🤔✨🍵🔮`) exist for
  users who disable unicode spinners.
- **Blinking stream cursor**: `▍` toggling every 420ms at the tail of in-flight text, frozen
  solid the instant streaming stops.
- **Shimmer/skeleton**: 7-cell highlight band sweeping over `▁` at a shared 90ms tick, offset
  per-row for a diagonal effect, capped at 30s total animation.
- **GoodVibesHeart**: random-colored `♥` flash for 650ms on trigger — pure delight/positive-
  feedback micro-animation, no functional purpose.
- **Ambient "long-running tool" charms**: text-only "motion" — injecting a fresh status line
  rather than animating anything, after an 8s delay, max 2 per tool, ≥10s apart.
- **Batched streaming render**: text is NOT rendered token-by-token; deltas are buffered and
  flushed at 16ms baseline (60fps-equivalent), throttled to 80-100ms while the user types or
  the view is scrolling, to avoid competing for repaint budget.
- **Resize-drag coalescing**: SIGWINCH bursts collapsed to ~30fps (32ms) so a terminal
  drag-resize doesn't stutter through full re-layouts on every pixel.
- **Occlusion-aware timers**: any component with a `setInterval` (busy ticker, session-duration
  clock, idle-since clock) explicitly checks `$isStatusRuleOccluded` and stops ticking while an
  overlay covers it — invisible re-renders are treated as a real bug class, not ignored.
- **Double-Esc-to-clear** (500ms window): a small but real "feel" detail — a single Esc might
  dismiss a completion or clear a selection; only a second Esc within 500ms clears the whole
  draft, avoiding accidental data loss.

---

## 7. Prioritized recommendations for Atlas (ranked by impact/effort)

Given Atlas already has: root model (`app.go`), arrow-key picker (`picker.go`), diff viewer
(`diff.go`), theme file (`styles.go`), slash-command dropdown, y/n approval overlay, header bar
+ status bar with filled backgrounds.

1. **[High impact / Low-Medium effort] Adopt the seed→derive→contrast-guard theme
   architecture.** Replace hardcoded palette entries in `styles.go` with ~12-15 named seed
   colors (bg, text, primary, accent, border, ok/warn/error, status*4) and a small `color.go`
   port of `mix`/`desaturate`/`relativeLuminance`/`liftForContrast` (all pure math, directly
   portable — formulas given in full in §2). Derive `muted`/`label`/`selection`/`border`-
   fallback from the seeds instead of hand-picking them. This single change fixes "looks bad"
   at its root: incoherent muted/dim tones and low-contrast text against arbitrary terminal
   backgrounds are usually *the* reason a homegrown TUI palette reads as amateurish, and the
   fix is almost entirely math, not design taste.

2. **[High impact / Low effort] Priority-ordered, width-budgeted status bar segments.**
   Rewrite Atlas's status bar to compute a `fits(width)`-style budget: pin the
   always-visible segments (status/spinner + model), then add optional segments (context %,
   duration, cwd/branch, tokens/sec, cache-hit%) in a fixed priority order, dropping
   lowest-priority first as the terminal narrows — exactly the `statusRuleWidths`/
   `statusBarSegments` pattern (breakpoints at 72/76/80/84/88/92/96/104/110 cols, adjustable).
   Currently a fixed-width status bar either overflows ugly on narrow terminals or wastes
   space on wide ones; this is a contained, mechanical fix.

3. **[High impact / Low effort] Busy indicator upgrade: rotating verb + elapsed clock +
   ambient "still working" charms for long tool calls.** Add a small `VERBS` pool + a
   `time.Since(start)` elapsed clock next to Atlas's spinner (cheap: one string pool + one
   ticker), and implement the `useLongRunToolCharms` pattern — after 8s of a tool running,
   inject a one-off "still working…" style line, max 2 per call, ≥10s apart, cancelled on
   completion/interrupt. This directly targets the most common "does this look alive?"
   complaint in long-running CLI agents.

4. **[Medium impact / Low effort] Never-shrinking selected-row contrast + no reverse-video/dim
   for "receded" UI.** Adopt `listRowStyle`/`chipRowProps`'s pattern: a selected-row
   background chip with the foreground re-lifted for contrast *against that specific chip*
   (not just the base theme's default text color), and stop using ANSI reverse-video or dim
   attributes for anything meant to look "selected" or "muted" — always blend real theme
   colors instead, since dim/inverse render inconsistently (or as ugly black slabs) across
   terminals with transparent or unusual backgrounds — directly relevant on Windows Terminal.

5. **[Medium impact / Medium effort] Incremental/batched markdown rendering for streaming
   text.** If Atlas re-renders the full Glamour-rendered markdown on every stream delta,
   port the `streamingMarkdown.tsx` approach: freeze completed blocks (split at
   blank-line-outside-fence boundaries) into a cache, only re-render the small in-flight tail.
   Pair with batched delta flushing (~16ms baseline, slower while the user is typing or
   scrolling) rather than rendering every token — this removes visible stutter/CPU spikes
   during long responses, which is a common source of "feels janky" in Bubbletea TUIs driving
   an expensive renderer per update.

(Runners-up, lower priority but still concrete: build one shared `Accordion` component and use
it for all of Atlas's collapsible Tools/Skills/System-Prompt/MCP-style sections instead of
bespoke toggle code per section; wrap long approval-prompt commands to panel width with a
"+N more lines" footer instead of single-line truncation; add a shimmer/skeleton loading state
for anything that waits on the gateway/session init; add a double-Esc-to-clear-draft guard.)
