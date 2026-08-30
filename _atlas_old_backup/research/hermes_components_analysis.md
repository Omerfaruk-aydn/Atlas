# Hermes Agent — `ui-tui/src/components/` Analysis (36/36 files read)

Scope note: this is one of 5 parallel slices. Billing/subscription/plugin-marketplace
*business logic* is skipped per instructions, but generic dialog/UX shapes from those
files are still captured.

---

## accordion.tsx
- THE expand/collapse primitive, shared by session-panel sections and widget-app accordions.
- Uncontrolled (`defaultOpen`) or controlled (`open`+`onToggle`) — same component either way.
- Visuals: `▾ ` open / `▸ ` glyph in accent color, bold title, optional `(count)` and muted suffix text, all on one clickable row.
- Go/Bubbletea: a small `Accordion` struct holding `open bool`, a `Toggle()` method, and a `View(title string, count *int, body string) string` — trivial to port; use lipgloss for the glyph+bold title style.

## activeSessionSwitcher.tsx
- The `/resume`/session-switcher overlay. Rows: pinned `+new` row (always index 0) then live sessions then resumable history, windowed to `VISIBLE=12` rows, width clamped `MIN_WIDTH=64`/`MAX_WIDTH=128`, title truncated at `TITLE_MAX=64` chars.
- Status glyphs: idle `✓`, starting `…`, waiting `?`, working `▶`; colored ok/label/muted by status.
- Live-session poll every **1500ms** (`quiet=true`, skips full history refetch) re-anchors selection by session ID identity (not flat index) so growth/shrink of the list doesn't desync the cursor — this identity-preserving re-anchor pattern is the standout technique here.
- Two-press delete confirm on history rows (`d` arms, second `d` deletes, any other key cancels) — tracked by session ID not row index so background repoll can't misdirect a delete.
- Column layout: fixed-width columns (2/11/11/18) then a flex-grow title column, all `wrap="truncate-end"`.
- Keybindings: ↑↓ move, Ctrl+N new, Ctrl+R refresh, Tab picks model (only on new-row), Ctrl+D close live session, `d` arm-delete on history row, Enter switch/resume/start.
- Go/Bubbletea translation: use `bubbles/list` won't fit the mixed-kind rows well; better to hand-roll a windowed slice-render like this file does, with a `rowKind(i)` discriminator function. The identity-preserving selection re-anchor on background poll is the single most valuable pattern for Atlas's own session/list UI if Atlas ever polls live state under a selection cursor.

## agentsOverlay.tsx
- Full-screen `/agents` subagent-tree monitor: list mode (flat rows + a Gantt-chart mini-timeline) and detail mode (scrollable node inspector), replay mode over `$spawnHistory` snapshots.
- Sort modes cycle: `depth-first → tools-desc → duration-desc → status`; filter modes cycle: `all → running → failed → leaf`.
- Status glyph/color table: running=`●` accent, queued=`○` muted, completed=`✓` statusGood, interrupted=`■` warn, failed=`✗` error, timeout=`⌛` warn, error=`⚠` error.
- **GanttStrip**: ASCII timeline — 5-col id gutter, 10-col right label reserve, bar drawn with `█` fill on blank track, ruler line uses `┼` every 10 cols / `·` every 5 / `─` elsewhere, with a derived numeric ruler-label row underneath (`0`, `5s`, `10s`...). Auto-picks label step of 5 vs 10 based on total span. This is a genuinely nice reusable "mini process timeline" widget — Atlas has nothing like it.
- Heat-map coloring: cold→hot palette `[border, accent, primary, warn, error]`, bucketed by `hotnessBucket(hotness, peak, paletteLen)`; only buckets ≥2 get a colored `▍` marker so cool branches stay muted.
- Auto-follow: when live subagents drain to 0, auto-jumps to `history[1]` with a flash message `"turn finished · inspect freely · q to close"` — nice UX for "your background job just finished, look here."
- Keybindings: q/Esc close, `<`/`[` and `>`/`]` step replay history, `p` pause/resume spawning, `x` kill selected, `X` kill subtree, in list mode `g`/`G` top/bottom, `s` cycle sort, `f` cycle filter, Enter/→/`l` open detail; in detail mode `h`/←  back, PageUp/Down or Ctrl+U/D page-scroll, `j`/`k` line-scroll.
- Go translation: the Gantt strip and heat-map bucket coloring are the two standout pieces worth porting into Atlas if it ever visualizes concurrent tool/subagent execution.

## appChrome.tsx
- **StatusRule**: the width-budgeted, priority-ordered status bar (Atlas already has one, so only novel deltas below).
  - Busy indicator styles: `kaomoji` (FACES array, 2500ms tick), `emoji` (6-frame array, 600ms tick), `ascii` (`|/-\\`, 100ms tick), `unicode` (braille spinner, spinner's authored interval, min 100ms, no verb shown). This "pluggable indicator style" concept (`/indicator` command swaps busy-spinner glyph sets) is not in Atlas and is a nice user-facing customization to consider.
  - Segment gating thresholds (`statusBarSegments`): compactCtx <72 cols, bar ≥72, duration ≥76, compressions ≥80, voice ≥84, bg ≥88, subagents ≥92, cacheHit ≥96, latency ≥104, tps ≥110 — a concrete, tunable progressive-disclosure ladder Atlas's status bar could mirror numerically.
  - Context bar coloring thresholds: ≥95% critical, >80% bad, ≥50% warn, else good.
  - `MAX_DURATION_WIDTH` reserved from `fmtDuration` output itself (`"59m 59s"` / `"99h 59m"`) rather than a magic number — self-documenting width budget technique.
  - `GoodVibesHeart`: a random-colored `♥` glyph that flashes for 650ms on a `tick` bump (random palette pick from error/warn/accent) — a tiny celebratory micro-animation, could be a fun optional touch for Atlas on successful task completion.
  - `SpawnHud`: compact ` │ d2/3 ⚡1/2` HUD appended to status bar only while subagents are fanning out; color escalates muted→warn (ratio≥0.66)→error (ratio≥1, i.e. at cap) using max(depthRatio, concurrencyRatio against the widest tree level, not naive sum).
  - `TranscriptScrollbar`: mouse-draggable scrollbar with 3-state coloring (thumb: accent while grabbed/hovered else primary; track: blend toward completionBg, more so on hover) — thumb size `round((vp*vp)/total)` min 1.
- Go/Bubbletea: none of Atlas's existing status bar needs replacing, but the segment-priority width table and the pluggable indicator-style concept are worth adopting as literal ported values/features.

## appLayout.tsx
- Top-level layout: transcript pane / composer pane / prompt zone / floating overlays, assembled via `AlternateScreen` (or `Fragment` for INLINE_MODE, skipping alt-screen so native scrollback stays live).
- **PetPane**: a floating mascot anchored bottom-right (3 rows above composer, 2px left pad, 1px right pad), which publishes its own footprint (`$petBox`) so the transcript can either reserve a right gutter (wide terminal, ≥72 cols remaining) or reserve bottom rows (narrow terminal) to avoid overlapping it. This "self-reporting layout obstacle" pattern (a floating decorative element that publishes its bounding box so scrolling content can route around it) is a genuinely reusable idea if Atlas ever adds any floating chrome.
- Composer: computes `promptWidth`/`promptBlank` from `composerPromptText`, mouse-drag-to-position-cursor logic maps drag row/col into TextInput's internal coordinate space (accounting for the prompt-prefix width offset).
- Multi-line prompt buffer (`inputBuf`) renders each line with the prompt prefix on line 0 only, blank-padded on continuation lines.
- Message separator: every user message after the first gets a `───` dash rule above it for turn segmentation in a long transcript — cheap visual chunking Atlas could adopt directly.
- Go translation: the self-reporting-footprint pattern for a floating pet/overlay is the most portable idea; the turn-separator dash rule is a 2-line win for any Bubbletea transcript.

## appOverlays.tsx
- `PromptZone`: routes single active blocking prompt (approval/billing/subscription/confirm/clarify/sudo/secret) each wrapped in a `PromptCell` — a single-cell 1-column `WidgetGrid` with 1-padding, used purely as a layout-engine surface (keeps all overlay-hosting uniform).
- `FloatingOverlays`: renders session switcher / model picker / pet picker / skills hub / plugins hub / pager / slash-command completions as widgets in a single-column grid positioned absolutely above the composer (`bottom="100%"`).
- Completions popover: fixed `COMPLETION_WINDOW=16` viewport, **centered on the active index** (`start = compIdx - 8`, clamped) rather than growing from a fixed top, avoiding a bouncy resize as you scroll past row 8 — this fixed-centered-window technique is worth adopting for Atlas's slash-command dropdown if it currently grows/shrinks.
- Completions render as 2-column grid: name column auto-sized to widest visible item + 2, meta/description column in a neutral tone (explicitly NOT the same as "label" color to avoid two near-identical grays being confused).
- Pager panel: title centered, footer hint dynamically switches text depending on whether more content remains (`↑↓/jk line · Enter/Space/PgDn page · b/PgUp back...` vs `end · ...`).

## billingOverlay.tsx (business logic mostly skipped; generic shapes kept)
- Reusable **multi-screen state-machine modal shape**: `overview → buy → confirm` (+ `autoreload`, `limit`, `stepup`), Esc from sub-screen returns to overview, Esc from overview closes. This exact shape (a bordered double-line-ish round box containing swappable "screen" components driven by a `screen` string + `onPatch` partial-merge) is a solid general pattern for any multi-step wizard dialog in Atlas.
- Generic UI bits worth reuse regardless of billing: `MenuRow`/`ActionRow`/`footer()` primitives from `overlayPrimitives.tsx` (see below) are used consistently across billing/subscription screens — number-key quick-pick (`1-N`) alongside arrow+Enter selection is a nice redundant-input affordance.
- Resumable OAuth/device-flow step-up screen (`prompt → waiting → granted → resuming`) that keeps the modal mounted across a browser round-trip and requires an explicit "press Enter to resume" rather than auto-firing on grant — good pattern for any Atlas flow that hands off to a browser and must resume deterministically.
- Two-field form editing (`AutoReloadScreen`): Tab cycles focused field, arrow up/down also moves focus row, Enter in field 1 jumps to field 2, Enter in field 2 jumps to first action row — a full inline multi-field-form input scheme, potentially useful if Atlas ever needs a settings-form overlay.

## branding.tsx
- **Banner** responsive tiers by terminal width: full logo+tagline at `cols >= logoWidth+2`, compact single-tagline-rule banner from `COMPACT_FROM=58`, name+tag-only fallback below that, hidden entirely below `HIDE_BELOW=34`. Tag text itself degrades: `TAG_FULL` → `TAG_MID` → `TAG_TINY` at further width breakpoints (52/46/64 cutoffs). This tiered-degradation ladder (not just "fits or truncates" but multiple named fallback layouts) is the standout idea — Atlas's banner/header (if any) could use the same 3-4 tier scheme instead of one-size truncation.
- `CompactBanner` explicitly avoids `bold` on full-width box-drawing dash rules because on some transparent terminals (e.g. Cursor) a bold run of `─` renders with an opaque black cell background bug — a concrete rendering gotcha noted inline; if Atlas hits weird bold-rule artifacts on transparent terminal backgrounds this is the exact cause to check for.
- **SessionPanel**: the big bordered round-box startup panel — two-column wide layout (hero art + info) when `cols>=90` and `leftW+40<cols`, else single narrow column. Uses lazy-loading skeletons (`ShimmerRows`/`InlineLoader`) while tool/skill lists are still loading (`info.lazy`).
- Truncation-with-overflow-count list rendering pattern (`truncLine`): joins comma-separated items until budget exceeded, then appends `, …+N` — reusable for any "show first few + count" list line.
- Accordion sections used for Tools (open by default)/Skills/System Prompt/MCP Servers (all collapsed by default) — consistent with the shared Accordion primitive.
- "Behind N commits" update-nag banner rendered inline in warn color with a highlighted update command.

## fpsOverlay.tsx
- Trivial: `HERMES_TUI_FPS=1` env-gated dev overlay. Color thresholds: ≥50fps good, ≥30fps warn, else error. Zero-cost when disabled (guard clause bails before subscribing to the store). Worth copying verbatim as a dev-mode Bubbletea profiling overlay if Atlas doesn't have one — trivial to implement (`tea.Cmd` ticking + a moving-average frame timer).

## gridStreamsDemo.tsx / gridTestOverlay.tsx
- Internal dev/demo surfaces (`/grid-test`) exercising the widgetGrid layout engine: nested grids, cell promotion/re-flow with stable per-panel state via React keys, an area-span demo (2fr weighted column, rowSpan/colSpan merged cells). Not directly useful to port into Atlas's product UI, but the *underlying* CSS-grid-like layout engine (`widgetGrid.tsx`, see below) is worth studying as a design reference even though Atlas won't need the full engine.
- One nugget: `SparkStream`'s inline mini-sparkline-plus-label component (`sparkRows(history, width, rows)`) is a compact reusable pattern for any live numeric telemetry strip (tokens/sec, latency, memory) — Atlas could use a similar ring-buffer + row-rendered-sparkline utility for a debug/perf HUD.

## helpHint.tsx
- The `?` quick-help popup shown when composer input is exactly `"?"`. Two aligned two-column lists (Common Commands / Hotkeys) with label column padded to the longest key across BOTH lists (`labelW`) so alignment is consistent — small technique worth using anywhere Atlas renders label:value help columns. Rendered in a bordered round box, absolutely positioned above the composer (`bottom:"100%"`).

## journey.tsx
- `/journey` — a full learning-history browser: static ASCII chart (last N frames from `learning.frames` RPC) + a scrollable slice/item tree below it, item detail view with edit (opens `$EDITOR`)/delete (two-press `d`+`y` confirm). Chart rows come pre-rendered as `[text, styleKey, alpha, hexOverride]` tuples from the backend and are faded via `fadeInk`/`fadeHex` (alpha-blended into the theme). Not directly portable to Atlas (no equivalent "learning graph" concept), but the **item tree with gap-rows-skipped cursor navigation** (`stepRow`/`snapRow` — cursor never lands on a blank spacer row, wrapping around it) is a nice small technique for any tree-with-section-headers list.

## loaders.tsx
- **Shimmer skeleton rows** — THE loading-state primitive. A sweeping highlight band (`BAND=7` cells wide) moves across a placeholder bar; `shimmerSegments(width, phase, band)` returns pure `[pre, band, post]` widths for the sweep, easily portable to Go as a pure function. Rows offset their phase by `i*2` for a diagonal shimmer look.
- Shared clock: **ONE** `setInterval` (90ms tick, unref'd) drives every mounted shimmer instance via a pub/sub `Set` of listeners — explicitly called out as a fix for a previous bug where each lazy-loading section spawned its own 90ms interval (~22 renders/sec on an idle TUI). This "one shared ticker, many subscribers" pattern is directly applicable to Atlas's Bubbletea `tea.Tick` usage — if Atlas has multiple independently-animating spinners, consolidating to one shared tick message is a real perf win worth doing.
- Animation budget: shimmer stops animating after `SHIMMER_ANIMATE_MS=30_000` (30s) and freezes in place rather than animating forever on a stuck lazy load — good defensive default for Atlas's own spinners on genuinely-stuck operations.

## markdown.tsx
- Very large custom markdown renderer (fences, tables, math via `texToUnicode`, footnotes, definition lists, blockquotes with nested `>` depth, task lists `☐`/`☑`, headings, setext headings, HR, autolinks, image placeholders `[image: alt] url`). Atlas already has a Glamour-based renderer, so most of this is not a gap — but a few specific numeric/behavioral details are worth checking against Glamour's output:
  - Table rendering has **3 tiers**: (1) fits at ideal per-column widths → simple; (2) fits at minimum-word-width with proportional extra-space distribution → column shrink with wrapped cells; (3) doesn't even fit at minimums → proportional scale + hard grapheme-level breaks. When `tallestBodyRow` exceeds a column-count-scaled threshold (`numCols<=3 ? 8 : numCols<=6 ? 5 : 4`) OR the max rendered line overflows, it falls back to a **vertical "Label: value" card layout per row** instead of a horizontal table — this graceful table→card fallback for narrow terminals is a genuinely valuable idea if Atlas's Glamour tables currently just clip/wrap badly on narrow widths.
  - `$...$`/`\(...\)` inline math and `$$`/`\[...\]` block math converted via a Unicode math-symbol table (`texToUnicode`) rather than rendering LaTeX — if Atlas ever needs to show math from an LLM response, this Unicode-substitution approach (Greek letters, ℕℤℚℝ, sub/superscripts, fractions) is a lightweight no-dependency option instead of pulling in a LaTeX renderer.
  - `\boxed{X}` regions rendered as inverted+bold "highlighter" spans using sentinel control chars (U+0001/U+0002) as delimiters set by the math converter — a decent trick for marking spans for later special rendering without touching the main token stream.
  - Cross-instance LRU parse cache (512 entries, keyed by `theme+cols+compact+text`) so virtualized scroll-back-into-view doesn't re-tokenize markdown it already rendered — relevant if Atlas virtualizes its transcript and re-parses markdown on scroll.

## maskedPrompt.tsx
- Trivial masked (`*`) password-style prompt used for sudo/secret-env prompts. Nothing novel beyond the icon+label+optional-sub-label+masked TextInput shape; useful as a direct template if Atlas needs a masked input.

## messageLine.tsx
- Per-role rendering: pulls `{ body, glyph, prefix }` from a `ROLE` lookup table keyed by role (user/assistant/tool/system/event). Tool-role messages render in a bordered round box (indented `marginLeft={3}`) rather than inline text — visually distinguishes raw tool output from narration.
- Timeline "event" rows (model switches, delegation completions) render as dim `◈` markers with **no gutter at all** — deliberately un-opaque, distinct from a real chat bubble.
- Collapsible long system messages (>400 chars) show only the first 120 chars + char count until clicked open — same accordion-glyph convention.
- "Response" separator: when a message has hidden tool/thinking details above it, inserts a `└─ Response` divider line with the gutter color, so users understand the assistant's visible reply follows collapsed reasoning — a nice small affordance to visually connect a collapsed trail to the response it produced.
- `display.timestamps` support: dim `[HH:MM]` stamp rendered on its own row above user/assistant messages only (never event/trail/system chrome) when timestamps are enabled — simple, could be ported directly.
- "Long pasted message" truncation renders `[long message]` inline in dim/muted rather than showing the full multi-KB paste in the transcript.

## modelPicker.tsx
- Two-stage flow: provider list (`step 1/2`) → model list (`step 2/2`), each independently fuzzy-filterable by typing (uses a shared `fuzzyRank` util) with `Ctrl+U` to clear filter, Backspace to edit filter char-by-char, Esc first clears filter then navigates back then cancels (3-level Esc semantics).
- Inline API-key entry stage for `auth_type==='api_key'` providers — masked bullet echo (`•` up to 40 chars) with save-to-`~/.hermes/.env` messaging, matching maskedPrompt's masking style but built ad hoc here rather than reusing the `TextInput mask` prop (worth noting as slight inconsistency in the source project).
- Disconnect confirmation sub-stage (`Ctrl+D` on an authenticated provider) with plain y/Enter confirm, n/Esc cancel.
- Persist-scope toggle (`Ctrl+G`): session-only vs `--global`, shown as a status line at the bottom of both stages.
- Provider row prefix glyphs: `○` unauthenticated, `*` current, `●` authenticated-but-not-current.
- Go translation: the 3-level Esc semantics (clear filter → go back a stage → cancel) plus the "filter box that also accepts printable chars as extend-filter, arrows to navigate, numbers 1-N as quick-pick" combo is a solid, complete recipe for Atlas's own filterable picker components (model picker, session picker, etc.) if not already matching this exactly.

## overlay.tsx
- Two small primitives: `Overlay` (9-zone placement: top/bottom/left/right/center + 4 corners, optional full-screen dimmed backdrop scrim painted as literal space-rows with a background color since Ink doesn't paint bg on empty boxes) and `Dialog` (bordered round box with optional centered title + hint footer).
- The **explicit backdrop scrim as painted space-rows** (rather than relying on a semi-transparent overlay Box) is the concrete technique: `Array.from({length: rows}, (_,i) => <Text backgroundColor={scrimBg}>{' '.repeat(cols)}</Text>)`. In Bubbletea/Lipgloss, dimming a background under a modal is usually solved differently (rendering both layers and compositing), but if Atlas ever needs a literal "clear space with background paint" fallback, this is the reference technique.

## overlayControls.tsx
- `useOverlayKeys({onBack, onClose})`: single reusable hook binding `q`→close, `Esc`→back-or-close, used by nearly every overlay in the codebase (skills hub, plugins hub, model picker, etc.) — this is exactly the kind of tiny reusable hook Atlas should have if it doesn't already (a `WithOverlayKeymap` helper in the Bubbletea model).
- `windowOffset`/`windowItems`: the canonical "keep selection visible in an N-row viewport, centered-ish, clamped to bounds" utility — `offset = clamp(selected - floor(visible/2), 0, count-visible)`. This exact formula recurs in nearly every list overlay in the codebase (agents overlay Gantt strip, session switcher, model picker, plugins hub, skills hub) — if Atlas's `bubbles/list` picker doesn't already center the cursor this way, this is the formula to adopt.

## overlayPrimitives.tsx
- `clampOverlayWidth(preferred, maxWidth, min=24)`: width-negotiation helper — honors a grid cell's hard cap absolutely, only enforces the floor when the cap allows it.
- `scrollbarColors(t, hover, grabbed)`: thumb = accent (grabbed/hover) else primary; track = blend of (border on hover / muted otherwise) toward `completionCurrentBg` at 25%/55% mix ratio — concrete numbers for a themed 2-state scrollbar if Atlas wants mouse-interactive scrollbars.
- `useMenu(rows, onEscape, onKey?)`: the canonical arrow+Enter+number-quick-pick menu hook, reused by billing/subscription/many list overlays — worth adopting as-is conceptually.
- `listRowStyle`/`chipRowProps`: **the single shared "selected row" treatment across the entire app** — never uses ANSI `inverse` (explicitly called out as broken on transparent terminals/unknown default colors); instead paints an explicit `backgroundColor: completionCurrentBg` and computes readable ink via `liftForContrast(text, bg, 4.5)` (WCAG-style 4.5:1 contrast-guaranteed foreground against the actual chip color). **This is the single most valuable, concretely-portable finding in this whole file set** — Atlas's picker "chip-style contrast on selected picker rows" (mentioned as already implemented) should be double-checked against this exact approach: compute contrast via a WCAG algorithm against the *actual* resolved background, not a hardcoded guess, so cross-polarity themes (dark palette on light terminal) never produce unreadable selected rows.
- `barCells(ratio, cells=10)`: generic `█`/`░` progress-bar renderer — reusable for any percentage bar in Atlas beyond just billing usage bars.
- `UsageBars`/`usageBarsText`: two-bar (plan + top-up) dollar-usage visual, business-specific but the *pattern* (labeled bar row: `label[bar]  detail`) is generic.

## overlayScrollbar.tsx
- Mouse-draggable vertical scrollbar bound to a `ScrollBox` ref, re-rendering on an external `tick` prop so async content resize doesn't require a scroll event to repaint the thumb. Thumb size formula: `max(1, round(vp*vp/total))`; drag re-maps `localRow` to a proportional scroll position. Directly reusable if Atlas's own scrollable overlays (agents tree, journey view) need mouse-drag scrollbars — this is a complete, self-contained implementation to reference.

## petPicker.tsx / petSprite.tsx
- Petdex mascot system: `PetPicker` is a filterable list overlay (rank by active > installed > curated > rest, hides `clawd-*` placeholder entries) with install-on-demand (`pet.select` RPC). `PetSprite` renders true-color pixel art using half-block characters (`▀`/`▄`) with independent top/bottom RGBA per cell so alpha-transparent pixel edges don't bleed a black box — falls back to fg-only single-glyph render when only one half is opaque. `PetKitty` variant emits Kitty-protocol Unicode placeholder cells (U+10EEEE) whose foreground color encodes an out-of-band-transmitted image id, for terminals with the Kitty graphics protocol.
- Not directly relevant to Atlas's stated goals (no mascot mentioned) but the **half-block dual-color pixel-art rendering technique** is worth knowing about if Atlas ever wants ASCII/pixel-art rendering: 2 pixels per character cell via foreground+background coloring of `▀`.

## pluginsHub.tsx
- Business-specific but the interaction shape is generic: category/scope toggle via `Tab` (user vs all/bundled plugins), status glyphs `✓`/`✗`/`○`, Enter or Space toggles enable/disable, number-key quick toggle. Same `windowItems`+`chipRowProps` list conventions as everywhere else.

## prompts.tsx
- **ApprovalPrompt**: y/n-style tool-approval overlay (Atlas already has this) — options set varies by context: full 4-choice (`once/session/always/deny`) drops to 3 when `allowPermanent===false`, or collapses to smart-deny 2-choice (`once/deny`) when `smartDenied` — a context-aware option-set narrowing Atlas's own approval overlay could adopt if it always shows all 4 options regardless of context.
- Command preview wraps with `wrapAnsi(..., {hard:true})` per line, capped at `CMD_PREVIEW_LINES=10` with an `"…+N more lines (full text above)"` footer instead of unbounded — concrete cap value worth matching if Atlas's own command-preview overlay is unbounded.
- **ClarifyPrompt** "batch" mode: a compact multi-question flow where all N questions show as a status list (✓ answered / ▸ active / · pending) with only the ACTIVE question's choices expanded; Tab/Shift+Tab cycle which question is active (any order, not linear), re-visiting an answered question restores cursor position or pre-fills the typed "Other" answer for editing. This is a well-designed pattern for any multi-field form-in-a-list UI — worth studying if Atlas ever needs to ask multiple related questions in one overlay.
- **ConfirmPrompt**: simple danger-aware (red vs warn) yes/no dialog with Y/N single-key shortcuts plus arrow+Enter — a good minimal reference implementation if Atlas's confirm dialog lacks the Y/N single-key shortcut.

## queuedMessages.tsx
- Small windowed queue display (message-queue-while-busy feature) — shows a `QUEUE_WINDOW=3` centered slice around the item being edited, with `…` lead/tail ellipsis markers and an edit-mode hint line (`Ctrl+X delete · Esc cancel`). If Atlas supports queuing messages while the agent is busy, this exact windowing formula is directly reusable.

## skillsHub.tsx
- 3-stage drill-down (`category → skill → actions`) list overlay, same `windowItems`/`chipRowProps`/number-quick-pick conventions as pluginsHub/modelPicker. Nothing novel beyond consistent reuse of the shared primitives — a good example of how uniformly these primitives are applied project-wide (worth noting as "the discipline to enforce" more than "a new pattern").

## streamingAssistant.tsx
- Flattens live streaming content into an ordered block list (settled stream segments + active tool trail + in-flight streaming text) so each block's "leading gap" spacing can be derived from its immediate predecessor — avoids the classic bug where a streaming block's spacing jumps the instant it "flushes" into settled history, because the gap logic is anchored to stable predecessor identity rather than to the live in-flux text. This block-list-with-derived-predecessor-gap technique is the standout idea, applicable directly to how Atlas manages spacing between streaming/settled chat blocks if it currently computes spacing differently for live vs settled content.

## streamingMarkdown.tsx
- **The most valuable technical pattern in this file set for Atlas's markdown rendering.** Incremental markdown parser for in-flight streaming text: a forward scanner tracks fence/math-open state and scan position in a ref across deltas, committing "settled" top-level blocks (split on blank-line boundaries outside a fence) into an append-only array — each settled block is memoized and *never re-tokenized again*. Only the unsettled tail re-parses per delta. This converts a naive O(total_length × num_deltas) or O(blocks²) re-tokenization cost into true O(new_content_per_delta). Concretely:
  - Scanning only touches complete (`\n`-terminated) lines; a partial trailing line stays in the tail because it could still open a fence.
  - A block commits at `\n\n` (blank-line boundary) only when NOT inside an open code fence or open math block.
  - State is idempotent (re-calling with same text no-ops) so React StrictMode double-invocation is safe.
  - Resets cleanly if `text` no longer extends the previously-scanned prefix (turn reuse / front-trimming very long replies).
  - Go translation: if Atlas's Bubbletea+Glamour streaming re-renders the *entire* markdown blob on every token/delta (likely, since Glamour has no incremental API), implementing this exact "settled blocks are frozen strings rendered once, only the tail re-renders" strategy would be a meaningful performance win for long streamed responses — this is probably the single highest-value backend/rendering-perf idea in this whole slice, even though it's not a *visual* change.

## subscriptionOverlay.tsx (business logic skipped; generic shapes kept)
- Same multi-screen wizard shape as billingOverlay (`overview → picker → confirm → result`, with a `stepup` OAuth-resume screen spliced in). Two additional generic techniques:
  - **Optimistic long-poll confirmation** after an async state change: after "upgraded" a `ResultScreen` polls (`ctx.refreshState()`) every 2000ms for up to 15 attempts waiting for the tier to actually reflect server-side, showing "Applying…" then "Still applying" on timeout rather than a bare success message that might be wrong — good pattern for any Atlas action that's fire-and-forget but has eventual-consistency lag (e.g., a background task the UI wants to confirm actually completed).
  - Synchronous double-submit guards via a `useRef` boolean *alongside* React state (`submittingRef` + `setSubmitting`) — because two key-events in the same tick can both observe stale `false` state before React commits the update. This is a real, generally-applicable React gotcha (not Bubbletea-relevant directly, since Go's single-threaded update loop doesn't have this race, but worth knowing if Atlas's own event loop ever has async double-fire potential from overlapping key events + async command completion).

## textInput.tsx (1845 lines — largest file, extremely low-level terminal I/O engineering)
- This is by far the most technically dense file in the slice; almost none of it is directly portable to Bubbletea (whose `textinput`/`textarea` bubbles are already React-Ink-agnostic and don't need this kind of raw-stdout bypass), but several *concepts* are worth awareness:
  - **"Fast-echo" bypass**: for pure-ASCII single-line append/backspace at the end of the buffer, the component writes raw stdout bytes (`\b \b` for backspace, the literal char for append) instead of going through a full React/Ink re-render, cutting input latency. Strict shape guards (`canFastAppendShape`/`canFastBackspaceShape`) reject the bypass for: multi-line buffers, non-ASCII/wide/combining characters, soft-wrap boundary positions, or any edit that would re-color already-painted cells (e.g., completing a `[[ token ]]` highlight span). This is Ink-specific engineering to work around a slow React reconciler; **not applicable to Bubbletea**, which already renders synchronously and cheaply — Atlas's `bubbles/textinput` doesn't need or want this.
  - Undo/redo stack (200-entry cap) storing `{value, cursor}` pairs, standard readline-style kill-line/kill-to-start (Ctrl+K/Ctrl+U), word-left/right via a hand-rolled whitespace-boundary walker, and full grapheme-cluster-aware cursor movement via `Intl.Segmenter` (with an LRU stop-cache, max 32 entries) so cursor movement doesn't split multi-codepoint emoji/combining characters. **The grapheme-cluster cursor-movement concern is real and applicable to Atlas**: if Atlas's Go text input moves the cursor by rune instead of by grapheme cluster, multi-codepoint emoji or combining-mark input will visibly misbehave (cursor lands mid-glyph). Go's `uniseg` package (rivo/uniseg) is the equivalent of `Intl.Segmenter` here and is worth verifying Atlas's text input already uses.
  - Selection support: click-drag mouse selection, double/triple-click behavior via a 500ms multi-click timer, right-click = copy-if-selected-else-paste (mirrors xterm/iTerm/gnome-terminal convention) — worth matching if Atlas's TUI supports mouse selection at all.
  - Platform-specific Return-vs-newline disambiguation (`shouldInsertNewlineOnReturn`): Shift/Ctrl/action-modifier held → always newline; otherwise a bare LF byte sequence is treated as newline on macOS terminals and on Linux terminals detected via env vars (`WT_SESSION`, SSH vars, Ghostty vars, WSL) that are known to collapse Shift+Enter down to a bare Ctrl+J. This exact terminal-compat matrix (env-var sniffing for "does this terminal properly report Shift+Enter") is worth copying verbatim if Atlas's multi-line-input Shift+Enter behavior is unreliable across terminals.

## themed.tsx
- Trivial one-off `<Fg>` wrapper component mapping a `ThemeColor` key to `theme.color[key]`, with `literal` override escape hatch. Not novel; a straightforward "themed text span" helper Atlas likely already has via lipgloss styles.

## thinking.tsx (1256 lines)
- **ToolTrail**: the collapsible reasoning/tool-calls/subagents/activity panel shown under each assistant message (Atlas has an equivalent). Key numeric/behavioral details:
  - Section visibility resolution (`sectionMode`) with 3 states per section (thinking/tools/subagents/activity): `hidden`/`collapsed`/`expanded`, each independently overridable — thinking's "collapsed" mode is **auto-driven**: panel opens automatically while `reasoningActive` is true and auto-collapses the instant reasoning ends, rather than defaulting to always-open or always-closed. This "auto-expand only while live, then auto-collapse" behavior for a reasoning/thinking panel is a genuinely nice default Atlas's own thinking-panel toggle should consider if it currently defaults to one fixed state.
  - Shift+click (or Ctrl+click) on ANY chevron expands every non-hidden section at once (`expandAll`) — a broad "expand everything" shortcut layered onto per-section toggles; cheap to add to Atlas's own accordion tree if not present.
  - Tree-drawing primitives (`TreeRow`/`TreeNode`/`treeLead`): rail-based ASCII tree rendering — `│ ` for continuing rails, `  ` for closed rails, `├─ `/`└─ ` for mid/last branch — the standard box-drawing recursive-tree rendering approach, directly reusable if Atlas ever needs to render a nested tree (e.g., subagent hierarchies, file trees) instead of a flat list.
  - Heat-map coloring of subagent tree branches by "hotness" (tools/sec relative to peak) reuses the same 5-color palette/bucket scheme as agentsOverlay.
  - `Spinner` component randomly picks one of 7 named braille-spinner variants per mount for "thinking" (`helix, breathe, orbit, dna, waverows, snake, pulse`) vs a different 7 for "tool" execution (`cascade, scan, diagswipe, fillsweep, rain, columns, sparkle`) — using different spinner *character sets* for different semantic states (thinking vs tool-running) rather than one universal spinner is a small but nice touch — Atlas's "rotating-verb+elapsed-clock busy spinner" could similarly vary glyph style by activity type (e.g. different frames while a tool executes vs while the model reasons) if it currently uses one glyph set everywhere.
  - `StreamCursor`: a blinking `▍` cursor block at 420ms interval, shown only on the last streaming line and only while `streaming && visible` — small detail worth matching (blink rate, only-last-line placement) if Atlas has a similar streaming-text cursor.
  - "Backstop" behavior: when literally every section is hidden by user config AND there are error/warning activity items, the panel still surfaces the last 2 non-info activity items as bare inline alerts instead of rendering nothing — a good defensive UX principle (never let a details-hiding setting silently swallow real errors) applicable to Atlas's own /details-equivalent config.

## todoPanel.tsx
- Small collapsible todo-list panel (used both live during a turn and archived in transcript). Header: `▸`/`▾` toggle, bold "Todo" label, `(done/total)` count in dim statusFg, optional "· incomplete · N still pending" suffix when a turn ended with unfinished items. Body: indented tree (`todoTree()` returns `[item, depth]` pairs, indent = `min(depth,4)*2` spaces) with per-status glyph/color (`todoGlyph`/`todoTone`/`rowColor` — active=text color, "body" tone=statusFg, else muted). Directly comparable to whatever Atlas's todo/plan panel does; the incomplete-count suffix and depth-capped indentation (`min(depth,4)`) are the two concrete numeric details worth matching if Atlas's todo panel differs.

## widgetGrid.tsx
- A full CSS-grid-like layout engine for terminal UI: `WidgetGrid` (flowing multi-row grid with column tracks, spans, auto-placement, uniform gaps) and `GridAreas` (fully 2-axis grid with row+column tracks, row/col spans, absolute-positioned solved cells so a widget can span multiple rows — impossible in the flowing variant). Adaptive defaults: gap auto-shrinks to 0 below 36 cols or with ≥8 columns, to 1 with depth>0 or <72 cols or ≥4 columns, else 2; padding auto-shrinks to 0 at depth 0 or <24 cols, 1 at <56 cols, else 2.
- This is a genuinely substantial general-purpose achievement (CSS Grid semantics in a terminal), but it's almost certainly overkill to port into Atlas wholesale unless Atlas's roadmap includes a "widget dashboard" or plugin-widget system with arbitrary user-defined panel layouts. If Atlas's UI is a fixed, hand-laid-out set of panels (transcript/composer/status bar/overlays), building a general grid-layout engine is not worth the effort — Lipgloss's simpler horizontal/vertical `JoinHorizontal`/`JoinVertical` composition is sufficient. Flag this as "interesting engineering, likely not worth porting" rather than a recommendation.

---

## Top recommendations for Atlas (ranked by impact/effort)

1. **WCAG-contrast-guaranteed selected-row chip color** (`overlayPrimitives.tsx: listRowStyle`/`chipRowProps`). Instead of a fixed/guessed foreground for selected rows, compute it via a contrast algorithm (e.g., WCAG relative luminance, target ≥4.5:1) against the *actual* resolved chip background color at render time. Very low effort (a pure function), high impact for any theme where light/dark polarity might be inverted from what the palette author assumed. **Verify Atlas's stated "chip-style contrast on selected picker rows" already does the real WCAG computation and not a hardcoded assumption — this is the exact bug class it would catch.**

2. **Incremental/streaming markdown parsing** (`streamingMarkdown.tsx`). If Atlas's Glamour-based streaming re-renders the whole response text on every delta, adopt the settled-block-freeze strategy: split on blank-line boundaries outside code/math fences, memoize each settled block's rendered output permanently, only re-render the unsettled tail. Medium effort (needs the same fence/blank-line scanner ported to Go), high impact on perceived responsiveness for long streamed replies.

3. **Shared/consolidated animation ticker** (`loaders.tsx`'s single shared 90ms clock with a listener `Set`, replacing multiple independent intervals). Directly maps to Bubbletea: if Atlas has more than one component independently calling `tea.Tick`, consolidate to one ticker message fanned out to subscribers. Low effort, meaningful perf/CPU win, and the source project explicitly called out the multi-interval bug it fixed (~22 renders/sec on an idle terminal).

4. **Identity-preserving cursor re-anchoring under background polling** (`activeSessionSwitcher.tsx`). When a list is periodically refreshed in the background while the user has something selected, re-anchor selection by stable ID (not flat index) so growth/shrink of the list can't silently move the cursor to a different item. Applicable to any Atlas list view that polls live state (session list, background task list). Medium effort, prevents a real, easy-to-miss UX bug class.

5. **Windowed-viewport-centered-on-cursor formula** (`overlayControls.tsx: windowOffset`). `offset = clamp(selected - floor(visible/2), 0, count-visible)`. If Atlas's `bubbles/list`-based pickers scroll by "reveal next item" rather than keeping the cursor visually centered, this one-line formula is a cheap improvement to scrolling feel in any long list.

6. **Table→card fallback for narrow terminals** (`markdown.tsx`'s 3-tier table renderer). If Atlas's Glamour markdown tables currently just clip or ugly-wrap on narrow terminal widths, adding a vertical "Label: value" per-row fallback when a table's rendered width can't fit (even at minimum word-widths) is a meaningful readability win for real-world LLM-generated tables on 80-col terminals.

7. **Graceful multi-tier banner degradation** (`branding.tsx`). If Atlas's own banner/logo currently just truncates or disappears below some width, adopting the 3-4 tier ladder (full art+tagline → compact rule+tagline → name+short-tag → hidden) at concrete breakpoints (34/58/+logoWidth) is low effort and produces a noticeably more polished small-terminal experience — this was explicitly the user's core complaint ("TUI looks bad"), and banner/header treatment is one of the first things a user sees.

8. **Auto-expand-while-live, auto-collapse-when-done for the thinking/reasoning panel** (`thinking.tsx`). If Atlas's tool-trail/thinking panel currently has one fixed default (always open or always closed), switching "collapsed" mode to auto-track `reasoningActive` produces a livelier, more informative default without giving up the option to manually pin it open/closed.

9. **Context-aware approval option sets** (`prompts.tsx: approvalOptions`). Narrowing the approval dialog's choice set based on context (drop "always" when a policy disallows permanent grants; collapse to a 2-choice "smart deny" in certain flows) rather than always showing all 4 options is a small, low-effort refinement to Atlas's existing y/n approval overlay.

10. **Self-reporting floating-element footprint** (`appLayout.tsx: PetPane`/`$petBox`). If Atlas ever adds any floating decorative or informational overlay that sits over the transcript (not a modal, just visual chrome), having it publish its own bounding box so the transcript's layout code can route text around it (gutter on wide terminals, reserved rows on narrow ones) is a clean, reusable technique — lower priority since Atlas has no stated mascot/floating-chrome feature yet, but worth keeping in mind if one is added later.

---

## Coverage confirmation

All 36 files in the manifest were fetched and read in full (not sampled/excerpted), including completing the previously-partial files (`thinking.tsx` fully read through all 1256 lines; `modelPicker.tsx` fully read through all 710 lines; `appChrome.tsx`, `appLayout.tsx`, `branding.tsx`, `prompts.tsx`, `todoPanel.tsx`, `accordion.tsx`, `loaders.tsx`, `messageLine.tsx`, `overlayPrimitives.tsx`, `appOverlays.tsx` all fully read). No files were skipped or truncated; no fetch failures occurred.
