# Hermes Agent `ui-tui/src/lib/` — Exhaustive Analysis (58/58 files)

All 58 files in the manifest were cloned and read in full (test files skimmed
lightly per instructions). This directory is almost entirely pure TypeScript
utility logic with zero React/Ink dependency — an unusually high proportion of
directly-portable Go material.

---

## color.ts / color.test.ts

- THE color primitive: hex/`rgb()` parsing, sRGB lerp `mix()`, WCAG relative
  luminance + contrast ratio, `readableOn()` (luminance>0.5 → black ink else
  white), `ensureContrast()` (steps toward the readable pole re-mixing from the
  ORIGINAL color each step — 5% increments, up to 20 steps, so hue decays
  linearly not exponentially), `liftForContrast()` — **xterm.js's own
  minimum-contrast algorithm**, ported faithfully: multiplicative 10%-of-remaining-headroom
  channel steps toward the readable pole (darken: `c -= ceil(c*0.1)`; lighten:
  `c += ceil((255-c)*0.1)`), preserving hue/chroma ratios far better than
  linear mixing. Also: `lighten`/`darken` (mix toward white/black), `grayOf`
  (luminance-weighted gray), `desaturate`, HSL round-trip (`toHsl`/`fromHsl`),
  `retone` (hue-preserving re-tone: clamp saturation floor + pin lightness —
  used for light-terminal variants of dark-terminal pastel accents), `boostSaturation`,
  and a chainable `color(v).mix().darken().ensureContrast().hex()` API.
- **Verify against Atlas's `internal/tui/color.go` port**: check specifically
  whether Atlas already has `liftForContrast` (the xterm.js algorithm) — this
  is the most novel/valuable piece, distinct from the simpler `ensureContrast`.
  If Atlas only ported `ensureContrast`, it's missing the multiplicative
  luminance-preserving variant that terminals/IDEs actually use for their own
  contrast guarantees. Also confirm `retone` and `boostSaturation` made the
  cut — used for theme variant derivation.
- HIGH PRIORITY / directly portable: 100% pure math, trivial Go translation
  (arrays of 3 floats instead of `Rgb` tuples).

## emoji.ts

- **This is the file Atlas needs for its emoji problem — but it does NOT do
  terminal-capability detection.** It's actually the opposite: it's a
  presentation-selector *injector*, not a filter. `ensureEmojiPresentation(text)`
  walks a string and, for any codepoint in a fixed 100-entry `TEXT_DEFAULT_EMOJI`
  set (heart, tools, arrows, weather glyphs, etc. — chars whose Unicode default
  is monochrome/text presentation), appends U+FE0F (VS16) so they render as
  color emoji instead of text glyphs — skipping ones that already carry an
  explicit VS16/VS15 selector. It is lazy (only allocates a new string if a
  substitution is actually needed) and iterates by codepoint (handles
  surrogate pairs via `codePointAt` + size 1 or 2).
- **Important correction for Atlas's plan**: this file assumes the terminal
  *can* render emoji and color glyphs correctly — it's solving the opposite
  problem (some terminals show `☎` as monochrome text instead of the colorful
  emoji version unless VS16 is present). It does nothing for detecting
  cmd.exe's inability to render astral-plane (surrogate-pair, code point >
  0xFFFF) emoji at all. **No file in this directory implements terminal-capability
  gating for emoji width/support** — that logic apparently lives elsewhere in
  hermes-agent (likely in the `packages/hermes-ink` slice another agent is
  covering, which is closer to the renderer/stringWidth layer). Atlas's
  "ban all emoji on legacy cmd.exe" approach may still be the right call for
  astral-plane glyphs; this file's technique (VS16 injection) is only useful
  for BMP glyphs that already have both text and emoji presentations.
- Portable takeaway: if Atlas wants finer-grained emoji handling than a blanket
  ban, it could still adopt this exact pattern — inject/strip VS16 based on a
  detected terminal capability level (only worth doing if Atlas separately
  detects "renders BMP emoji fine, but not astral-plane").

## forceTruecolor.ts

- **Directly relevant to Atlas's truecolor detection woes.** Small env-driven
  override module, runs at import time (side-effecting top-level code):
  - `shouldForceTruecolor(env)`: `HERMES_TUI_TRUECOLOR` truthy (`1|true|yes|on`,
    case-insensitive) forces truecolor; explicit falsy value or `NO_COLOR`
    present forces it off.
  - Key insight: **macOS Terminal.app (pre-"Tahoe 26") lies about truecolor
    support** — even when it advertises `COLORTERM=truecolor`, RGB SGR
    rendering is broken. `shouldDowngradeAppleTerminalTruecolor()` detects
    `TERM_PROGRAM=Apple_Terminal` + an advertised-truecolor env and forcibly
    deletes `COLORTERM`/`FORCE_COLOR=3` unless the user explicitly opts back
    in via `HERMES_TUI_TRUECOLOR=1`.
  - `isAdvertisedTruecolor`: checks `COLORTERM` == `truecolor`/`24bit` OR
    `FORCE_COLOR` == `'3'`.
- **Translation note for Atlas on Windows**: there is no Windows-specific
  quirk handled here (this file is Mac-only), but the *pattern* — "don't
  trust an env var blindly; downgrade known-lying terminals; allow explicit
  override" — is exactly the architecture Atlas needs for cmd.exe: detect
  `WT_SESSION` (Windows Terminal, real truecolor) vs. plain `cmd.exe` (legacy
  console host, unreliable RGB SGR even on Windows 10 1909+) and downgrade to
  256-color unless overridden. HIGH PRIORITY pattern to port, though the
  Windows-specific detection logic itself must be written fresh (this file
  doesn't have it).

## platform.ts (large file, mostly keybinding logic, not OS-color detection)

- Contrary to the task brief's expectation, this is NOT primarily OS/Windows
  detection — it's platform-aware **keybinding semantics**: `isMac`,
  `isActionMod` (Cmd on mac, Ctrl elsewhere), `isMacActionFallback` (handles
  macOS terminals that rewrite Cmd+arrows into readline Ctrl+A/E/U),
  `isCopyShortcut`, `isRemoteShell` (SSH env detection), and a very elaborate
  voice-record-key parser/validator (`parseVoiceRecordKey`,
  `isVoiceToggleKey`) with reserved-key collision detection per platform.
- Not directly useful for Atlas's rendering problems, but the `isRemoteShell`
  pattern (checking `SSH_CONNECTION`/`SSH_CLIENT`/`SSH_TTY`) is a reusable
  one-liner worth having in Atlas for capability decisions (SSH sessions often
  have degraded terminal capability negotiation).
- LOW PRIORITY for the rendering-fix task, but the whole file is trivially
  portable pure-logic if Atlas ever wants Mac-parity keybindings.

## terminalModes.ts

- Defines `TERMINAL_MODE_RESET`: a giant escape-sequence constant that
  resets every mouse-tracking mode (X10, click, button-motion, any-motion,
  UTF-8/SGR/urxvt/SGR-pixels/passive), focus events, bracketed paste,
  alternate screen, kitty keyboard protocol, modifyOtherKeys, attributes, and
  cursor visibility — the canonical terminal-mode cleanup sequence on exit.
- `defaultColorSlot(osc)`: manages OSC 10/11 (set default fg/bg) with tracked
  "painted" state so exit-time reset (`resetTerminalModes`) only restores
  colors it actually touched (OSC 110/111 restore). Uses `writeSync` on exit
  (not async) so it completes before process death, with a `stream.write`
  fallback for mocked/unusual streams.
- HIGH PRIORITY, directly portable: this exact reset-sequence catalog is
  something Atlas's Bubbletea program should send on `tea.Program` teardown
  (Bubbletea has its own cleanup for altscreen/mouse, but the padding for the
  more obscure modes like kitty keyboard, modifyOtherKeys, DEC locator, and
  the OSC 10/11 restoration is worth cross-checking against). If Atlas ever
  leaves stray terminal state after a crash (common complaint pattern: "my
  terminal is stuck in a weird mode after quitting"), this is the reference
  sequence.

## terminalParity.ts

- Builds a small set of contextual hint messages surfaced to the user at
  startup: `detectMacTerminalContext()` (Apple Terminal / remote / tmux /
  VSCode-like) drives messages like "Detected cursor terminal · run
  /terminal-setup", "Apple Terminal detected · use /paste for image clipboard
  fallback, try Ctrl+A/E/U if Cmd+←/→ gets rewritten", "tmux detected ·
  clipboard uses passthrough", "SSH session detected · clipboard bridges via
  OSC52". All Mac/tmux/SSH focused, not Windows.
- MEDIUM PRIORITY pattern (not code): Atlas could adopt this exact "detect
  environment class → surface a short actionable hint banner" pattern for
  Windows cmd.exe (e.g. "Legacy Console detected · astral-plane emoji and RGB
  colors may render incorrectly · try Windows Terminal for full support").

## terminalSetup.ts

- VSCode/Cursor/Windsurf-specific keybindings.json patcher (adds Shift+Enter,
  Ctrl+Enter, Cmd+Enter, Cmd+Z/Shift+Cmd+Z as kitty-protocol CSI-u sequences;
  migrates legacy `\\\r\n` escape sequences to CSI-u). Includes a hand-rolled
  JSONC comment/trailing-comma stripper (`stripJsonComments`) that correctly
  preserves comment-like text inside string literals — a nice small reusable
  utility if Atlas ever needs to read a JSONC config. Also
  `getVSCodeStyleConfigDir` per-platform (darwin: `~/Library/Application
  Support/<app>/User`; **win32: `%APPDATA%\<app>\User`**; else
  `~/.config/<app>/User`) and a `when`-clause overlap/contradiction checker for
  detecting keybinding conflicts.
- Not relevant to Atlas's core rendering fix. LOW PRIORITY — only useful if
  Atlas ever ships an analogous "optimize my terminal" setup command.

## text.ts / text.test.ts

- Large grab-bag of ANSI/text utilities: `stripAnsi`/`sanitizeAnsiForRender`
  (regex-based ANSI CSI/OSC/DCS stripping, handling incomplete sequences at
  string boundaries — sanitizeAnsiForRender preserves SGR `m` sequences but
  strips everything else, useful if Atlas needs to sanitize untrusted tool
  output before rendering); `hasAnsi`; markdown-to-plain-text line estimator
  (`renderEstimateLine` — strips bold/italic/strikethrough/links/images/headers/
  lists/blockquotes to estimate rendered width); `compactPreview`,
  `estimateTokensRough` (`(len+3)>>2`, i.e. ~4 chars/token); `edgePreview`
  (head+tail truncation with `..` ellipsis marker, careful about not producing
  `]]` inside output that would break paste-token markup); `cleanThinkingText`
  (strips streaming status-verb noise like "Thinking..." from reasoning
  text via a verb-list regex, with a pre-bound tail-slice optimization to keep
  the O(n) regex pass from becoming O(n²) over a growing stream — **directly
  relevant if Atlas streams reasoning/thinking text and re-renders the full
  accumulated buffer on every token**); `boundedLiveRenderText`
  (caps live-rendered text to N chars/N lines, walking backward from the tail
  to find a clean line boundary, emitting an `[omitted N lines/N chars]`
  header — this is a good pattern for Atlas's own render throttle to avoid
  unbounded buffer growth during long tool output streams); tool-trail
  formatting helpers (`formatToolCall`, `buildToolTrailLine`,
  `buildVerboseToolTrailLine`, parsing helpers); `fmtK` (compact number
  formatter via `Intl.NumberFormat`).
- HIGH PRIORITY, portable: `stripAnsi`, `boundedLiveRenderText`,
  `cleanThinkingText`'s tail-bound-before-regex trick, and `estimateRows`
  (renders row-count including fenced-code-block detection, table-separator-row
  skipping, and markdown-to-plain estimation) are all generically useful for
  any streaming TUI and have zero framework coupling.

## mathUnicode.ts

- Best-effort LaTeX→Unicode converter for terminal rendering of math (Glamour
  doesn't do this). Comprehensive symbol table (Greek, blackboard/calligraphic/
  fraktur letter maps, big operators, set theory, logic, relations, arrows,
  sub/superscript maps), balanced-brace parser (`readBraced`) for correctly
  handling `\frac{a^{n}}{b}`-style nesting that a naive `[^{}]*` regex would
  break on, `\frac` → `a/b` collapse with smart parenthesization
  (`wrapForFrac`), `\boxed`/`\fbox` → sentinel control chars (U+0001/U+0002)
  for the renderer to apply inverse-video highlighting, `\xrightarrow{label}`
  → `─label→`, and word-boundary-safe regex ordering (longest-match-first,
  split by punctuation-ending vs letter-ending commands).
- **HIGH PRIORITY / directly portable and something Atlas doesn't have**: if
  Atlas renders any model output that could include LaTeX (`$...$`,
  `\frac{}{}`, etc.), this is a complete, self-contained algorithm doable in
  a single Go file with no dependencies. Given Atlas already discovered
  Glamour's syntax-highlighting gaps (see `syntax.ts` below), this is a
  similarly-scoped "gap Glamour doesn't fill" opportunity.

## resizeCoalescer.ts / resizeCoalescer.test.ts

- Leading+trailing throttle for terminal resize event storms. `schedule()`
  fires `reflow()` immediately on first call (`Date.now() - lastReflow >=
  intervalMs`), then collapses subsequent calls into a single trailing
  `setTimeout` firing after the interval elapses since the last reflow, so a
  drag-resize gets exactly one immediate reflow + one final reflow instead of
  reflowing per pixel. `cancel()` clears pending trailing timer. Uses
  `Date.now()`/`setTimeout` directly (not RAF) for deterministic fake-timer
  testability.
- **HIGH PRIORITY, directly portable**: this is precisely the algorithm to use
  for Bubbletea's `tea.WindowSizeMsg` handling if Atlas doesn't already
  throttle resize-triggered re-renders — trivial Go port (time.Now(),
  time.AfterFunc). Exact same shape as Atlas's existing 40ms streaming
  throttle, just for the resize axis instead.

## themeBoot.ts

- Flash-free theme boot: persists the last-resolved `Theme` + background hex
  + light/dark mode pin to `~/.hermes/tui-theme-boot.json` (atomic write via
  tmp-file + rename, debounced 400ms, `unref()`'d timer so it doesn't block
  exit). On next launch, replays the cached theme as the FIRST frame (before
  async detection — OSC-11 background probe, gateway skin, config sync — can
  resolve) to avoid a visible default→final theme flash. Cache is a hint only:
  explicit env vars beat it, and it self-invalidates when the current
  terminal's live OSC-11 probe returns an untrusted value (pure black
  `#000000`, the "unset default" fingerprint) so a stale cached background
  can't outrank live detection forever. Test-run guarded (`VITEST`/`NODE_ENV=test`)
  so tests never touch the real `~/.hermes` dir.
- **HIGH PRIORITY pattern for Atlas's `DefaultTheme()` in styles.go**: if
  Atlas currently always boots into a hardcoded default before OSC-11/COLORFGBG
  detection resolves, this exact cache-and-replay technique would eliminate
  a visible flash. Fully portable to Go (encoding/json + os.Rename for atomic
  write); the "untrusted value invalidates the cache" provenance-tracking
  detail is the subtle but important part worth copying faithfully.

## termux.ts

- Trivial: `isTermuxEnv` (checks `TERMUX_VERSION` env or `PREFIX` containing
  `/data/data/com.termux/files/usr`), `isTermuxTuiMode` (defaults on in Termux,
  overridable via `HERMES_TUI_TERMUX_MODE=0`). Not relevant to Windows/cmd.exe,
  but the "vendor-string env sniffing with explicit override env var" pattern
  is reusable if Atlas ever wants a `HERMES_TUI_TERMUX_MODE`-style toggle.
  LOW PRIORITY.

## syntax.ts

- Minimal but real syntax highlighter Atlas is explicitly missing: regex
  tokenizer (`TOKEN_RE` matches quoted strings incl. escapes, numbers,
  identifiers) with per-language keyword sets (go/ts/py/rust/sh/sql/json/yaml)
  plus alias table (js→ts, bash→sh, etc.), single-line comment detection per
  language, and 4-way token classification (comment/string/number/
  keyword/plain) mapped to theme colors. No block comments, no nested
  strings-in-strings, no multi-line constructs — deliberately simple, one
  line at a time.
- **HIGH PRIORITY, directly portable, and matches the task's flagged gap**:
  Atlas has "no code syntax highlighting beyond Glamour's defaults" — this is
  a ~120-line self-contained algorithm, trivially portable to Go (map[string]bool
  keyword sets, same regex-based tokenizer using Go's regexp). Given Atlas
  already has a `go` keyword set to write anyway (dogfooding), this is very
  low effort for real payoff.

## fuzzy.ts / fuzzy.test.ts

- Ordered-subsequence fuzzy matcher for picker/palette filtering.
  `fuzzyScore(target, query)`: null if not a subsequence; else scores +1 per
  matched char, +5 for contiguous runs, -1..-3 penalty for skipped gaps
  (capped), +3 for word-boundary matches (after `-_/.` space or camelCase
  transition), +5 for matching index 0, +8 prefix-match bonus, +20 exact-match
  bonus, -0.01×length tiebreak favoring shorter targets. Returns matched
  character indices for highlight rendering. `fuzzyScoreMulti` does AND-semantics
  across whitespace-separated query tokens, unioning positions. `fuzzyRank`
  filters+sorts a list by a derived text key, stable-sorted by original index
  on ties.
- **HIGH PRIORITY, directly portable**: zero dependencies, this is exactly the
  algorithm behind fzf-style fuzzy pickers. If Atlas has any command palette,
  model picker, or file picker needing fuzzy filtering, this scoring heuristic
  (well-tuned per its own test suite: `son4` correctly ranks `claude-sonnet-4`
  top) is worth porting verbatim rather than re-deriving from scratch.

## circularBuffer.ts

- Generic ring buffer: fixed capacity, `push` (wraps head, tracks length up to
  cap), `tail(n)` (last n items in insertion order), `drain()` (tail + clear),
  `clear()`. Straightforward, ~45 lines.
- HIGH PRIORITY, directly portable: Go generics make this a near 1:1 port
  (`type CircularBuffer[T any] struct`). Useful for Atlas's log/history
  ring buffers if not already using a container/ring equivalent.

## clipboard.ts

- Cross-platform clipboard read/write via native tools: macOS `pbpaste`/`pbcopy`;
  **Windows: PowerShell `Get-Clipboard -Raw`/`Set-Clipboard`, with UTF-8
  content base64-encoded before crossing the PowerShell boundary** — because
  PowerShell decodes piped stdin using the system ANSI codepage (e.g. CP936),
  which corrupts CJK/emoji; base64+`[Convert]::FromBase64String`+`UTF8.GetString`
  sidesteps the codepage entirely. WSL detected via `WSL_INTEROP`/`WSL_DISTRO_NAME`
  and routed to `powershell.exe`. Linux: Wayland (`wl-paste`/`wl-copy`) then X11
  (`xclip`, falling back to `xsel` for writes). `isUsableClipboardText` rejects
  NUL bytes and flags "suspicious" (control chars / U+FFFD replacement char)
  content beyond a 2%-or-2-char threshold, guarding against reading garbage
  from a binary clipboard payload.
- **HIGH PRIORITY / directly relevant to Windows work**: the base64 PowerShell
  encoding trick is a concrete, battle-tested fix for a real Windows Unicode
  bug class Atlas may be hitting today if it shells out to `Set-Clipboard`/
  `Get-Clipboard` directly with UTF-8 text or CJK/emoji content. Worth
  checking whether Atlas's clipboard code (if any) has this exact bug.

## osc52.ts

- OSC 52 clipboard read/write over the terminal itself (works over SSH without
  needing a native clipboard tool). `buildOsc52ClipboardQuery` wraps the OSC
  sequence for tmux (`ESC Ptmux;` + doubled ESCs + `ESC \`) or GNU screen
  (`ESC P ... ESC \`) passthrough — **multiplexer wrapping is required or the
  escape sequence gets eaten by tmux/screen instead of reaching the real
  terminal**. `parseOsc52ClipboardData` decodes the `c;<base64>` or `p;<base64>`
  response. `readOsc52Clipboard` races the query against a timeout Promise.
  `writeOsc52Clipboard` just base64-encodes and writes `ESC]52;c;<b64>BEL`.
- MEDIUM PRIORITY: relevant if Atlas ever wants clipboard support over SSH
  sessions where native tools aren't available; the tmux/screen wrapping
  detail is easy to miss and worth copying if Atlas implements OSC52.

## gracefulExit.ts

- Signal handling: registers SIGINT/SIGTERM/SIGHUP handlers once (`wired` guard),
  maps each to a POSIX-standard exit code (128+signum: 130/143/129), runs
  registered async cleanups via `Promise.allSettled`, and races them against a
  `failsafeMs` (default 4000ms) hard-exit timer so a hung cleanup can't block
  process exit forever. Also wires `uncaughtException`/`unhandledRejection` to
  a caller-supplied `onError`. `shouldExitForSignal` allows selectively
  ignoring specific signals (used elsewhere to let Ctrl+C be handled by the
  app instead of exiting).
- MEDIUM PRIORITY: Go's signal handling idiom differs enough (channels +
  select) that this isn't a direct port, but the **"race cleanups against a
  failsafe timer so a hang can't prevent exit"** pattern is worth adopting
  in Atlas's shutdown path if it isn't already there.

## parentLog.ts

- Best-effort crash-log breadcrumb writer (`~/.hermes/logs/tui_gateway_crash.log`)
  used to correlate the Node "parent" process's actions (killing a gateway
  child, replacing it, or receiving OS signals) with the Python gateway child's
  own panic log — because a bare `SIGTERM` to the child is ambiguous (could be
  the parent's own graceful action, or an external OOM-reaper/supervisor kill).
  Caps each breadcrumb line to 4096 chars, collapses embedded newlines,
  disabled under `VITEST`, warns once on write failure then goes silent.
- LOW-MEDIUM PRIORITY: only relevant if Atlas spawns/supervises a child
  process (e.g. driving an external gateway/backend) and wants crash forensics.
  Pattern-only, no code to port for pure single-binary Atlas.

## externalCli.ts

- Trivial: spawns `hermes` (or `$HERMES_BIN`) with `stdio: 'inherit'`, resolves
  with exit code or error message. Not relevant to Atlas.

## externalLink.ts

- Fetches an external URL's `<title>` for rendering a friendly link preview in
  the transcript. In-memory LRU-ish cache (500 entries, evict-oldest-key),
  in-flight-request dedup, 5s timeout, 96KB byte-budget streamed read (aborts
  early), HTML-entity decoding, private/local-IP/hostname filtering
  (`isPrivateOrLocalHost` — blocks RFC1918, loopback, link-local, `.local`/`.lan`/
  `.internal`/`.corp` suffixes, IPv6 ULA/link-local — prevents SSRF against
  internal services via link previews), and a title-quality filter
  (`TITLE_ERROR_RE` rejects "access denied"/"captcha"/"just a moment" pages).
- MEDIUM PRIORITY: not rendering-related, but the SSRF-prevention private-IP
  filter (`isPrivateIpv4`/`isPrivateIpv6`/`isPrivateOrLocalHost`) is a solid,
  fully-portable security utility worth having if Atlas's tools ever fetch
  user-supplied URLs (its `internal/tools/web` package, per the brief).

## openExternalUrl.ts / openExternalUrl.test.ts

- Opens a URL in the OS default handler, **explicitly avoiding `cmd.exe /c
  start` on Windows** — uses `explorer.exe <url>` directly instead, because
  `start` is a cmd.exe builtin whose tokenizer reinterprets `&`, `|`, `^`, `<`,
  `>` in the URL (breaking commands or, worse, letting a malicious/model-supplied
  URL with those characters do something unintended). macOS: `open`; Linux/BSD:
  `xdg-open` (explicit allow-list of platforms — returns `null`/no-opener for
  aix/sunos/cygwin/haiku rather than optimistically guessing). Protocol
  allowlist restricts to `http:`/`https:` only (blocks `file:`, `data:`,
  `javascript:`, `mailto:`, etc. — defends against a model emitting a
  malicious link that would trigger a local handler on click). Spawned via
  `child_process.spawn` with an argv array (no shell), so shell metacharacters
  in the URL cannot be reinterpreted. A no-op `'error'` listener is attached
  before `unref()` so an async ENOENT (missing `xdg-open`/`explorer.exe`)
  doesn't crash the whole process via an unhandled EventEmitter error.
- **HIGH PRIORITY / directly actionable for Atlas on Windows**: if Atlas's
  `internal/tools` (or the TUI) ever opens URLs, the `explorer.exe` (not
  `cmd /c start`) technique plus the protocol allowlist plus the
  no-op-error-listener-before-unref pattern are all concrete, tested fixes
  worth replicating in Go (`exec.Command("explorer.exe", url)`, no shell).

## editor.ts / editor.test.ts

- `$VISUAL`/`$EDITOR` resolution with shell-tokenization (`"code --wait"` →
  `['code','--wait']`), POSIX fallback chain (`editor`, `nano`, `pico`, `vi`,
  `emacs` — checked via `X_OK` on each `$PATH` dir), **Windows fallback:
  `notepad.exe`**, ultimate POSIX floor `vi`. `openInEditor` writes to a
  temp file, spawns the editor synchronously (suspending the Ink render loop
  meanwhile), reads back the result only if exit code was 0, cleans up the
  temp dir in a `finally`.
- HIGH PRIORITY, directly portable: if Atlas has (or wants) an "open in
  external editor" feature (e.g. `/edit` for long compose), this exact
  fallback chain — including the Windows `notepad.exe` default — is a
  complete, tested reference implementation.

## history.ts

- Command history persisted to `~/.hermes/.hermes_history` in a
  multi-line-safe custom format: each entry's lines are prefixed with `+`,
  entries separated by blank-line-delimited `# <timestamp>` headers. Caps to
  last 1000 entries in memory (lazy-loaded, cached), dedupes consecutive
  identical entries, appends are best-effort (swallowed errors).
- MEDIUM PRIORITY: directly portable file-format + logic if Atlas wants
  persistent multi-line-safe history (readline libraries often mangle
  multi-line entries — this custom `+`-prefix format avoids that ambiguity
  entirely). Worth adopting the format if Atlas's history doesn't already
  handle multi-line input safely.

## prompt.ts

- `composerPromptText`: tiny prompt-string composer — shell-mode gives `$`;
  Termux mode forces a strictly single-cell ASCII `>` prompt (explicitly
  avoiding decorative Unicode glyphs because "Termux fonts/terminal backends
  can render decorative prompt glyphs with ambiguous width", causing stale
  arrow artifacts) and only shows profile name prefix on very wide panes
  (≥90 cols); normal mode prefixes profile name if not default/custom.
- MEDIUM PRIORITY: the specific insight — **avoid ambiguous-width Unicode
  glyphs in performance-critical single-line prompt indicators, prefer plain
  ASCII** — is a generically useful defensive principle for Atlas given its
  own width-detection struggles, even though this file's code itself is
  trivial.

## inputMetrics.ts

- Composer cursor-position math built to stay in exact lock-step with
  `wrap-ansi`'s line-wrapping (used by Ink's `<Text wrap="wrap">`), because a
  hand-rolled word-wrap previously drifted from wrap-ansi's actual behavior in
  subtle ways (exact-fill rows, mid-word breaks), parking the hardware cursor
  off by several cells. `visualLines()` runs the real wrapper and does a
  parallel character-by-character walk to map wrapped output back to original
  string offsets, with a defensive re-sync scan (`value.indexOf`) if the two
  streams ever desync. `cursorLayout` (position→line/col), `offsetFromPosition`
  (line/col→position, using Intl.Segmenter grapheme widths, not raw chars, so
  multi-codepoint graphemes/emoji count as one cursor cell), `composerPromptWidth`,
  `transcriptGutterWidth`, `transcriptBodyWidth`, `stableComposerColumns`
  (reserves scrollbar gutter width only when there's room).
- **HIGH PRIORITY conceptual lesson for Atlas** (Bubbletea+Lipgloss instead of
  wrap-ansi/Ink): the core insight — "cursor-position math for a text input
  MUST be derived from the exact same wrapping algorithm the renderer uses,
  not a parallel reimplementation, or cursor drift results" — applies directly
  if Atlas has any custom text input with a visible cursor and Lipgloss-based
  wrapping. Also: **grapheme-cluster-aware width measurement via a
  segmenter** (not naive rune counting) is the correct way to place a cursor
  among multi-codepoint emoji/combining characters — Go's
  `golang.org/x/text/unicode/norm` or a grapheme-cluster library (e.g.
  `github.com/rivo/uniseg`, which Lipgloss's own `ansi`/`uniseg` dependency
  chain likely already uses) is the equivalent tool.

## precisionWheel.ts

- Tiny "precision scroll mode" state machine: while a modifier is held (or
  within an 80ms sticky window after the last event), forces single-row
  scroll steps at max 1 step per 16ms frame, vs. normal (faster) wheel
  acceleration. LOW-MEDIUM PRIORITY, only relevant if Atlas implements
  fine-grained scroll-wheel handling with a "precision mode" modifier.

## wheelAccel.ts

- Detailed mouse-wheel acceleration state machine with two code paths: native
  terminals (Ghostty/iTerm2/WezTerm — ramps a multiplier +0.3 per event within
  a 40ms window, resets on gaps >40ms) vs. xterm.js-hosted terminals
  (VS Code/Cursor — exponential decay `0.5^(gap/150ms)` blended with a "+5 decay
  step", capped 3 (slow) or 6 (fast) depending on the inter-event gap, carrying
  a fractional remainder across `scrollBy`-style flooring calls so mult=1.5
  yields an alternating 1,2,1,2 pattern rather than always flooring to 1).
  Also handles "encoder bounce" — a rapid direction-flip-then-flip-back within
  200ms is detected as mechanical-wheel bounce (not real reversal) and engages
  a sticky "wheel mode" with its own ramp/cap; 5 consecutive <5ms events
  disengages wheel-mode as a trackpad flick. Reads a user-configurable base
  speed from `HERMES_TUI_SCROLL_SPEED`/`CLAUDE_CODE_SCROLL_SPEED` env vars
  (clamped 0, 20].
- MEDIUM PRIORITY: highly polished but narrow-scope logic; worth porting only
  if Atlas's mouse-wheel scrolling currently feels bad (1 row/event, or
  uncontrolled acceleration) — this is a complete, tested reference for
  getting wheel feel right across both native and xterm.js-hosted terminals
  (relevant to Atlas users running inside VS Code's integrated terminal).

## billingDialog.ts / billingDialog.test.ts

- Trivial copy-generation function for an out-of-credits dialog (Nous vs
  third-party provider branching). Not relevant to Atlas (no billing system).
  NOT PORTABLE / not applicable.

## charts.ts

- Pure string-builder chart primitives, zero framework dependency:
  `sparkline` (8-level block-character `▁▂▃▄▅▆▇█` ramp, min/max-normalized
  over a trailing window, left-padded so short series don't resize the
  container), `sparkRows` (multi-row column chart with `rows*8` levels of
  resolution using partial eighth-blocks), `gauge` (horizontal fill bar),
  `hbars` (horizontal bar chart with eighth-block-resolution tips via
  `▏▎▍▌▋▊▉█`).
- **HIGH PRIORITY, directly portable**: if Atlas ever wants inline
  sparklines/gauges/bar charts (e.g. a token-usage or latency HUD), this is a
  complete self-contained algorithm — trivial Go translation (same block-char
  constants, same min/max normalize + eighth-block interpolation math).

## fpsStore.ts

- Dev-only FPS tracker fed by Ink's per-frame callback: rolling 30-frame
  timestamp window, computes fps as `(count-1)/elapsedSeconds`, zero-cost
  when a `SHOW_FPS` flag is off (the tracking function itself is `undefined`
  rather than gated inside, so the call site's optional-chain short-circuits
  entirely — no branch cost). Uses `nanostores` `atom` for reactive state.
- LOW-MEDIUM PRIORITY: the "expose the debug function as `undefined` when
  disabled, not an internal branch" zero-cost-when-off pattern is a nice
  Go-portable idiom (`var TrackFrame func(time.Duration)` left nil).

## memory.ts / memory.test.ts

- Heavy diagnostics/heapdump module: `captureMemoryDiagnostics` gathers
  `process.memoryUsage()`, V8 heap stats, resource usage, active handle/request
  counts, `/proc/self/fd` count and `/proc/self/smaps_rollup` (Linux-only,
  swallowed elsewhere), and computes a REAL growth rate relative to a
  `STARTED_AT` snapshot captured at module load (explicitly NOT
  `rss/uptime`, which would misreport a long-stable process as "growing").
  Heuristic leak-indicator list (detached V8 contexts, active-handle count
  >100, native-memory > heap-used, growth >100MB/hr, FD count >500).
  `performHeapDump` writes a diagnostics JSON sidecar always, but the full
  `.heapsnapshot` (can be multi-GiB) only fires for `manual` triggers or when
  `HERMES_AUTO_HEAPDUMP` is explicitly opted in — auto (threshold-crossing)
  triggers write diagnostics-only by default to avoid filling the user's disk.
  `pruneHeapdumps` caps total heapdump-dir bytes (default 2GB), evicting
  oldest-by-mtime first while always keeping at least the single newest file.
- LOW PRIORITY for Atlas (a Go binary has fundamentally different memory
  diagnostics — pprof, not V8 heap snapshots), but the **retention/pruning
  pattern** (cap total bytes in a directory, evict oldest-first, always keep
  the newest) is a reusable Go utility if Atlas writes any kind of debug
  artifact directory (logs, session recordings) that needs bounding.

## memoryMonitor.ts

- Polls `process.memoryUsage()` every 10s; thresholds are RELATIVE to V8's
  actual `--max-old-space-size` heap ceiling (critical ~88%, high ~70%) rather
  than a hardcoded absolute — the old hardcoded 2.5GB threshold was firing at
  only 31% of an 8GB ceiling, misdiagnosing normal long-session growth as OOM.
  Includes a clever **early-warning below the "high" threshold**: fires once
  when heap crosses an absolute floor (600MB) AND grew ≥150MB in one 10s tick
  (signature of a render-tree blowup) — the exact silent-death regime that
  used to show up only as an unexplained `stdin EOF` when the gateway's child
  process died. A cooldown (default 600s) prevents repeated auto-dumps when
  heap oscillates around a threshold. Deferred (lazy dynamic `import()`) of
  the Ink-cache-eviction module so the cold-start critical path never pays for
  it unless heap pressure actually triggers a tick.
- LOW PRIORITY for direct Go porting (Go's memory model + Bubbletea's small
  footprint make OOM unlikely to be Atlas's problem), but the **threshold-relative-to-actual-ceiling
  instead of hardcoded-absolute** design principle, and the **"warn early
  based on growth RATE, not just absolute level"** idea, are both
  transferable if Atlas ever adds its own memory-pressure diagnostics
  (Go's `runtime.MemStats` + `debug.SetMemoryLimit()` is the analogous API).

## messages.ts / messages.test.ts

- Thin wrapper: `appendTranscriptMessage` stamps `createdAt` at append time
  (wall-clock, not later at persist/re-render time) unless the message
  already carries one (rehydrated history), then delegates to
  `appendToolShelfMessage` (see liveProgress.ts). `capTranscriptHistory` caps
  transcript length to `MAX_HISTORY`, specially preserving an `intro`-kind
  first item. `upsert` merges same-role adjacent messages.
- MEDIUM PRIORITY: the "stamp timestamp at append, not at persist" rule is a
  small but real correctness detail worth checking in Atlas's own history
  code if it stamps times lazily.

## model-search-text.ts / model-search-text.test.ts

- Tiny alias table so opaque model wire-IDs (e.g. `k3` for "Kimi Coding
  flagship", `x-preview-f-free` for "Ox Alpha") are still findable via their
  marketing name in a fuzzy search box, without changing the actual wire ID
  used for API calls. NOT PORTABLE (Atlas doesn't share Hermes's model catalog),
  but the PATTERN — augment the search haystack with aliases while keeping the
  canonical ID untouched — is reusable if Atlas has any model/provider picker
  with non-obvious IDs.

## liveProgress.ts / liveProgress.test.ts

- Tool-execution "shelf" merging logic for the live transcript: groups
  consecutive tool-call trail messages (and interleaved "thinking" shelf
  messages) into a single collapsed row instead of one row per tool call,
  walking backward through recent messages to find the nearest mergeable
  holder, stopping at any "barrier" message (assistant text, user input,
  intro/panel/diff kind, or a message with actual text). `isTodoDone` treats
  an empty todo list as NOT done (only non-empty all-completed/cancelled).
- MEDIUM PRIORITY: the visual-grouping algorithm (merge adjacent
  ephemeral/status rows into one collapsible shelf, breaking at semantic
  barriers) is a nice UX pattern if Atlas's own tool-call trail currently
  renders one line per call with no grouping — directly portable logic (pure
  slice manipulation, easy in Go).

## reasoning.ts

- Extracts `<think>`/`<reasoning>`/`<thinking>`/`<thought>`/`<REASONING_SCRATCHPAD>`
  tags (checked case-insensitively) from streamed model output, both
  well-formed paired tags AND unclosed tags anchored to the START of the
  input only (`^\s*<tag>...$`) — deliberately NOT matching a stray mid-text
  `<think>` mention, because real unclosed reasoning blocks always lead the
  message (how reasoning models actually stream); a model quoting the literal
  word "think" mid-paragraph must not eat all trailing prose.
- HIGH PRIORITY, directly portable: if Atlas's agent loop handles any
  reasoning-model output with think-tags (Deepseek-R1-style, o1-style, or any
  future model that emits inline `<think>` blocks instead of a separate API
  field), this exact extraction algorithm — including the important
  anchored-to-start guard against false positives — is copy-ready logic
  (Go regexp with the same case-insensitive tag list).

## rpc.ts

- Type-narrowing helpers for a gateway RPC command-dispatch response
  (`exec`/`plugin`/`alias`/`skill`/`send`/`prefill` variants), plus
  `rpcErrorMessage` (extracts a message from an `Error`, string, or falls
  back to a generic "request failed"). Entirely tied to Hermes's own RPC
  protocol shape — NOT PORTABLE to Atlas's tool-calling architecture, though
  the small `rpcErrorMessage` idiom (normalize unknown thrown value to a
  displayable string) is a trivial universally-useful helper.

## starmapPalette.ts

- Derives a small themed color palette (background, dim, label, "memory" ink,
  "skill" ink) for a "Star Map" visualization overlay from a theme's primary
  + foreground colors: computes a complementary hue (+165°, not exactly 180°,
  clamped saturation ≥0.5 and lightness to [0.5, 0.7]) for the "skill" ink so
  it visually differs from the primary-colored "memory" ink, and fades either
  ink toward the background by an alpha blend for depth cues. Uses its own
  local RGB/HSL conversion functions (duplicating logic already in
  `color.ts` — the file's header comment says it mirrors
  `apps/desktop/src/app/starmap/color.ts`, suggesting this exists as a
  standalone module rather than importing `color.ts`, likely for
  desktop/TUI code-sharing reasons that don't apply to Atlas).
- LOW PRIORITY / NOT DIRECTLY APPLICABLE unless Atlas builds an analogous
  graph/map visualization; the complementary-hue-with-a-165°-offset-not-180°
  trick (to avoid a jarring exact-opposite hue) is a nice small color-theory
  detail worth remembering for any future "derive a second accent color from
  the primary" need.

## subagentTree.ts

- Reconstructs a tree from a flat, event-ordered list of subagent-progress
  records keyed by `parentId` (missing/unknown parent → treated as
  top-level), sorts siblings by `(depth, index)` for stable ordering
  regardless of network reordering, and recursively aggregates rollup stats
  (tool count, duration, descendant count, active count, max depth, token
  counts, cost, files touched) plus a "hotness" metric (tools/second) used to
  color tree rails by activity. Includes formatting helpers
  (`fmtDuration`, `fmtTokens`, `fmtCost`, `sparkline` via 8-level block ramp)
  and a `hotnessBucket` normalizer for palette-index mapping.
- MEDIUM PRIORITY: only relevant if Atlas has (or plans) a
  multi-agent/sub-agent orchestration feature with a visual tree — the
  tree-building-from-flat-parentId-list + rollup-aggregation algorithm is
  directly portable Go logic (recursive struct walk) if that feature exists
  or is planned.

## todo.ts / todo.test.ts

- Renders a todo-list hierarchy: `todoGlyph` (fixed-width ASCII markers
  `[x]`/`[-]`/`[>]`/`[ ]` — explicitly avoiding wide/emoji-like glyphs),
  `todoTone` (neutral coloring: only `in_progress` gets an "active" highlight,
  everything else is body/dim — explicitly NOT red/green for cancelled/
  completed), and `todoTree` (DFS parent-before-children ordering from a flat
  list with a `parent` field, degrading dangling/self-referencing parents to
  root level, and preserving true cycle members instead of dropping them
  entirely by appending unvisited nodes flat at the end).
- MEDIUM-HIGH PRIORITY if Atlas has a todo/task-list feature (the task
  description doesn't confirm this, but it's a common agent-CLI feature): the
  fixed-width-ASCII-glyph choice for status markers is directly relevant to
  Atlas's broader "ambiguous-width glyph" problem — using `[x]`/`[>]`/`[ ]`
  instead of any Unicode checkbox/circle glyph sidesteps width-detection
  entirely. The DFS-with-cycle-safety tree builder is directly portable Go.

## viewportStore.ts

- React-specific scroll-viewport snapshot store built on
  `useSyncExternalStore` — computes `atBottom`/`top`/`bottom`/`scrollHeight`/
  `pending` from an imperative `ScrollBoxHandle`, with a "cached vs fresh"
  two-tier scroll-height lookup (only calls the more expensive
  `getFreshScrollHeight()` when the cheap cached value suggests we might not
  actually be at the bottom, avoiding a full remeasure on every render tick).
  Snapshot equality is done via a serialized string key (rounding top/scrollHeight
  to multiples of 8) so `useSyncExternalStore`'s reference-equality check
  doesn't cause unnecessary re-renders for sub-pixel-equivalent scroll state.
- NOT PORTABLE (React-specific state-management idiom with no Bubbletea
  analog — Bubbletea's own model update loop doesn't need `useSyncExternalStore`),
  but the **two-tier cached-vs-fresh expensive-remeasure-only-when-needed**
  optimization pattern, and the **quantize-to-reduce-redundant-updates**
  trick, are both transferable ideas if Atlas's own viewport/scrollback
  tracking does expensive height recomputation on every frame.

## virtualHeights.ts

- Estimates transcript-row heights BEFORE actual layout/measurement, for
  virtualized-list scroll-position math: `messageHeightKey` builds a cache
  key from a fast DJB2-style string hash (`hashText`) of the message's
  content-relevant fields (text, thinking, tools, todo signature, panel
  signature, intro version) — avoiding a full-string cache key while staying
  correctly invalidated when content actually changes. `wrappedLines`
  estimates wrapped row count from `text.length / width` with a hard
  `MAX_ESTIMATE_LINES` (800) cap and a bounded walk-budget so a
  multi-megabyte single-line message can't cause an O(text) scan — explicitly
  documented as necessary because otherwise cold-mounting a 1000-row
  transcript becomes a multi-million-char wrap walk that blocks the UI.
  `estimatedMsgHeight` composes per-message-kind height rules (intro/panel/
  todo-trail get fixed estimates; normal messages get wrapped-line count +
  paragraph-gap bonus + tool/thinking-detail row additions + separator/gutter
  padding).
- **HIGH PRIORITY conceptual pattern for Atlas**: if Atlas's transcript
  scrollback ever needs virtualization/windowing (only render visible rows)
  for performance on long sessions, this exact "cheap pre-layout height
  estimate, refined by real measurement after mount" strategy — with a hard
  cap + budget on the estimation walk itself — is the right architecture and
  is directly portable arithmetic (Go string length / width division, no
  framework dependency). The DJB2 hash-based cache-key trick is also
  reusable.

## widgetGrid.ts

- Two independent, sophisticated pure-logic grid solvers with zero UI
  dependency:
  1. `layoutWidgetGrid` — auto-flowing widget-dashboard grid (like CSS grid
     `auto-flow`): computes column count from available width /
     `minColumnWidth` (or accepts explicit column count / track list), then
     places items greedily (explicit `colStart` pins respected if free, else
     first-fit scan), wrapping to a new row when an item can't fit.
  2. `resolveGridTracks` — a fixed-vs-`fr`-weighted track solver (the
     terminal-cell equivalent of CSS `grid-template-columns`): fixed tracks
     take their exact size, `fr` tracks share the leftover proportionally via
     floor-division-with-remainder-spread (`distributeByWeight`), with a
     `min` floor per track that iteratively re-pins any track violating its
     minimum and re-solves the remaining tracks — genuinely careful
     constraint-solving logic, not a simple divide.
  3. `layoutGridAreas` — a full 2D grid (both column AND row tracks, explicit
     `col`/`row`/`colSpan`/`rowSpan` item pins) with CSS `grid-auto-flow: row
     dense` placement (dense first-fit — smaller items can fill gaps left by
     larger explicitly-placed ones) and absolute `{x,y,width,height}` rect
     output per cell — this is the terminal-cell equivalent of a full CSS
     Grid engine.
- **HIGH PRIORITY, directly portable, and a genuine engineering asset**: this
  is real constraint-solving logic (not just arithmetic), thoroughly
  reusable if Atlas ever wants a dashboard/widget layout mode, a multi-pane
  split-view layout, or any scenario needing more than Lipgloss's basic
  horizontal/vertical joins. Straightforward to port to Go — no external
  dependencies, pure math over slices/structs.

## petPolling.ts

- Cosmetic "desktop pet" gateway-polling helper: single-flight guard (drops
  overlapping poll requests rather than queueing them), two-stage
  request (cheap metadata first, then the more expensive frame-cell payload
  only if metadata says it's needed), deliberately bypasses error-surfacing
  since a pet failing to load shouldn't show an error to the user (swallowed
  in a catch, returns null). NOT APPLICABLE to Atlas (no pet feature), but the
  **single-flight + two-stage cheap-probe-then-expensive-fetch** pattern is a
  reusable idea for any optional/cosmetic polling Atlas might do (e.g.
  update-check pings, telemetry).

## perfPane.tsx

- Dev-only performance instrumentation, gated entirely behind
  `HERMES_DEV_PERF=1` (React `Profiler` wrapper `PerfPane` returns children
  directly when disabled — zero overhead; `logFrameEvent` is `undefined` when
  disabled so Ink's `onFrame` callsite short-circuits without a function
  call). Logs JSON-lines to `~/.hermes/perf.log` (or `HERMES_DEV_PERF_LOG`)
  tagged by `src: 'react'|'frame'`, with a configurable minimum-duration
  filter (`HERMES_DEV_PERF_MS`, default 2ms, so sub-threshold idle frames
  don't spam the log) — captures per-pane React commit timings and per-frame
  renderer phase breakdowns (yoga/renderer/diff/optimize/write + a scroll
  "fast path" cumulative-stats counter).
- MEDIUM PRIORITY: not portable code (React-Profiler-specific), but the
  overall **dev-perf-logging architecture** — env-gated, zero-cost-when-off
  via undefined-function-short-circuit rather than an internal branch,
  JSON-lines output for `jq`-based analysis, threshold filtering to avoid log
  spam — is a good blueprint if Atlas wants to add its own render-timing
  diagnostics (Bubbletea's own Update/View functions could be wrapped
  similarly, logging duration to a JSONL file behind an env var).

---

## Top recommendations for Atlas (ranked by impact/effort)

1. **Windows clipboard base64/PowerShell encoding fix (`clipboard.ts`)** — If
   Atlas shells out to `Set-Clipboard`/`Get-Clipboard` with raw UTF-8 text
   today, it is very likely silently corrupting CJK/emoji content due to
   PowerShell's ANSI-codepage stdin decoding. This is a concrete, almost
   certainly applicable bug fix, not a "nice to have." Low effort (a few
   lines), high impact if the bug exists.

2. **`liftForContrast` in `color.ts` (xterm.js's real contrast algorithm)** —
   Verify whether Atlas's `color.go` port has this exact multiplicative
   10%-step luminance-preserving contrast lift, distinct from the simpler
   linear `ensureContrast`. This is the algorithm real terminals/IDEs use for
   their own "minimum contrast" feature, so matching it means Atlas's theme
   colors degrade the same way professional tools do. Low effort (pure
   function), directly requested by the task brief's "verify our port"
   instruction.

3. **`openExternalUrl.ts`'s `explorer.exe`-not-`cmd/c-start` + protocol
   allowlist pattern** — If Atlas ever opens URLs (from tool output, link
   clicks, etc.) on Windows, using `cmd.exe /c start` is a known footgun
   (shell metacharacter reinterpretation) that this file explicitly avoids.
   Directly actionable, security-relevant, low effort.

4. **`syntax.ts` — minimal syntax highlighter** — Task brief explicitly flags
   this as a gap Atlas has. ~120 lines, zero dependencies, directly portable.
   High value for visual polish (the user's core complaint is "TUI looks
   bad") relative to effort.

5. **`mathUnicode.ts` — LaTeX-to-Unicode converter** — Another complete,
   self-contained, zero-dependency algorithm filling a real Glamour gap
   (LaTeX math rendering in a terminal). Medium effort (more code than
   syntax.ts) but comprehensive and well-tested.

6. **`widgetGrid.ts`'s grid/track solver** — The most sophisticated pure
   logic in the whole directory (real constraint-solving, not just
   division). Only relevant if Atlas wants dashboard/multi-pane layouts
   beyond what Lipgloss's basic joins offer, but if so, this is a
   ready-made, well-designed engine — high value relative to how hard this
   would be to design from scratch.

7. **`themeBoot.ts` — flash-free theme boot cache** — If Atlas's TUI visibly
   flashes from a default theme to the final resolved theme on startup
   (common in terminal apps that detect background color asynchronously),
   this cache-and-replay-with-self-invalidation pattern is a complete fix.
   Medium effort, meaningfully improves perceived polish (the user's stated
   complaint).

8. **`resizeCoalescer.ts` — leading+trailing resize throttle** — If Atlas
   currently reflows on every `tea.WindowSizeMsg` during a drag-resize,
   users see flicker; this ~30-line algorithm (already the same shape as
   Atlas's own streaming throttle) fixes it. Very low effort.

9. **`fuzzy.ts` — tuned subsequence fuzzy matcher** — If Atlas has any
   picker/palette needing fuzzy filtering, this is a well-tested,
   ready-to-port scoring heuristic rather than something to design and tune
   from scratch. Low effort, meaningful UX quality for any picker feature.

10. **`todo.ts`'s fixed-width-ASCII status glyphs (`[x]`/`[>]`/`[ ]`/`[-]`)
    and `text.ts`'s `estimateRows`/`boundedLiveRenderText`** — Small but
    directly relevant to Atlas's stated width-ambiguity problems: using
    plain ASCII brackets instead of any Unicode status glyph sidesteps
    width-detection risk entirely, and the bounded-tail-rendering pattern
    for live-streaming text prevents unbounded buffer growth during long
    tool output. Very low effort, directly aligned with Atlas's core pain
    point (rendering robustness on constrained terminals).

**Important caveat on the emoji/terminal-capability-detection angle the task
specifically flagged**: neither `emoji.ts` nor `platform.ts` nor
`forceTruecolor.ts` contain genuine terminal-capability *detection* for
astral-plane emoji width or general Unicode-support gating — `emoji.ts` only
injects presentation selectors (VS16) assuming emoji rendering already works;
`forceTruecolor.ts`'s pattern (env-based detection + explicit override +
known-bad-terminal downgrade) is reusable in spirit for Windows cmd.exe
truecolor detection, but Windows-specific detection code doesn't exist in this
directory. The actual astral-plane-emoji/width-detection logic Atlas needs
most likely lives in the `packages/hermes-ink` renderer slice (stringWidth /
`isXtermJs` and similar low-level terminal-capability primitives referenced
but not defined in this directory) — worth checking that other parallel
agent's findings closely.
