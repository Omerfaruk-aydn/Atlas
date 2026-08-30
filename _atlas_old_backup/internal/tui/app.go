// Package tui implements Atlas's terminal UI as a single Bubbletea root
// model. The architecture mirrors Hermes Agent's CLI TUI: a flat App
// struct (Bubbletea has no nested reducer hierarchy), stateful
// sub-controllers (the face ticker, the notice board, the slash
// registry, the block-layout group walker), and a single Update method
// that routes incoming events to the right controller.
//
// What follows is the Atlas-flavored port — every "theme.ts / appLayout
// / turnController / prompts" pattern from Hermes Agent's
// ui-tui/ is here, translated from React+Ink+Yoga to Bubbletea+Lipgloss.
// The package is intentionally monolithic (single file per concept)
// because Bubbletea has no component model — composition happens by
// direct method calls in the App.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/omerfarukaydin/atlas/internal/agent"
	"github.com/omerfarukaydin/atlas/internal/llm"
)

// ProviderSwitcher builds a new llm.Provider for the given provider name.
type ProviderSwitcher func(name string) (llm.Provider, error)

// chatMessage is the transcript's unit of content. role drives the
// block-layout grouping; kind is the optional sub-classifier for trail /
// event / diff / slash / intro kinds.
type chatMessage struct {
	role string // "user" | "assistant" | "error" | "info" | "tool" | "trail"
	kind string // "trail" | "event" | "diff" | "slash" | "intro" — empty for plain rows
	text string
}

// approvalRequest tracks a tool call awaiting the user's decision.
type approvalRequest struct {
	toolName string
	input    json.RawMessage

	previewPath string
	previewOld  string
	previewNew  string
}

func (r *approvalRequest) hasPreview() bool {
	return r.previewPath != "" || r.previewNew != ""
}

// TRAIL_LIMIT caps the number of tool/reasoning rows the transcript
// retains per turn. Mirrors Hermes's TRAIL_LIMIT = 8.
const trailLimit = 8

// renderTickInterval is the streaming-render coalesce interval. The
// real Hermes value is dynamic (16/80/96ms) based on whether the user
// is typing or scrolling; Atlas uses the idle 16ms baseline plus the
// streamDelay-boost on user activity (see bumpStreamDelay).
const renderTickInterval = 16 * time.Millisecond

// faceTickInterval is how often the busy face rotates its verb. Matches
// Hermes's FACE_TICK_MS = 2500.
const faceTickInterval = 2500 * time.Millisecond

// shimmerTickInterval is the shared sweep cadence. 90ms matches Hermes.
const shimmerTickInterval = 90 * time.Millisecond

// streamDelayBoostMS is how much the render tick widens when the user is
// actively typing mid-stream (mirrors Hermes's STREAM_TYPING_BATCH_MS).
const streamDelayBoostMS = 80

// streamDelayIdleAfterMS is the cooldown after the last typing event
// before the tick relaxes back to the idle baseline. Mirrors Hermes's
// TYPING_IDLE_MS = 250.
const streamDelayIdleAfterMS = 250

// App is the root Bubbletea model.
type App struct {
	agent      *agent.Agent
	switcher   ProviderSwitcher
	slash      *SlashRegistry
	notices    *NoticeBoard
	shimmer    *sharedShimmerClock
	turn       *TurnController
	hist       *InputHistory
	queue      *MessageQueue
	subm       *submissionState
	config     *ConfigSync
	themeBoot  *ThemeBoot
	fps        *FpsOverlay
	details    DetailsState
	paste      PasteState
	completion *CompletionPipeline

	// systemPromptText is the prompt configured for the agent (used
	// by the SessionPanel display). Set in New() when the agent
	// exposes it; Atlas's agent doesn't currently expose a getter
	// so the field defaults to "".
	systemPromptText string

	chat  viewport.Model
	input textarea.Model
	spin  spinner.Model
	theme Theme
	rend  *glamour.TermRenderer

	messages  []chatMessage
	streaming bool
	cancel    context.CancelFunc
	dirty     bool

	picker     *list.Model
	pickerKind pickerKind
	approvalSelected int

	pendingApproval *approvalRequest

	// Slash command completion.
	cmdSuggestIndex  int
	cmdSuggestAll    []FuzzyScoreItem
	cmdSuggestFilter string
	completionFlight int64

	// Stream delay state.
	streamDelay      time.Duration
	lastTypingAt     time.Time

	// Face ticker.
	faceTickCount int
	faceStartedAt time.Time

	// Pager overlay.
	pager *PagerState

	// Help overlay.
	helpOpen bool

	// "?" detected as a single-character input.
	helpPending bool

	// Session metrics.
	sessionInTok  int64
	sessionOutTok int64
	turnStart     time.Time
	lastTurnMS    int64
	thinkingVerb  string

	// History + queue.
	messageHistory []string
	historyIdx     int
	queuedMessages []string

	gitBranch string
	width, height int

	// showWelcome controls whether the SessionPanel is shown in the
	// chat pane (overrides the empty-transcript path). Toggled by
	// the /welcome slash command.
	showWelcome bool

	// sessionSwitcher is the /sessions overlay state, nil when closed.
	sessionSwitcher *SessionSwitcherState

	// busyMode is the /busy policy: how Enter during a streaming
	// turn behaves. Hermes supports queue/steer/interrupt; Atlas
	// only wires queue today but stores the others so the policy
	// can be promoted to full behavior without changing the field.
	busyMode BusyMode

	// detailedDebug turns on per-event payload logging (Hermes's
	// /debug-detailed). Useful for the rare "why did the agent do
	// that" investigation.
	detailedDebug bool
}

// toolCharms is the canonical "still working…" string pool (Hermes's
// LONG_RUN_CHARMS equivalent). Package scope so the test file can
// reference it directly.
var toolCharms = []string{
	"hâlâ çalışıyor", "biraz sürüyor", "neredeyse bitti", "işleniyor",
}

// New builds the App, wiring up the slash registry with the Atlas
// command set, the notice board, and the shared shimmer clock.
func New(ag *agent.Agent, switcher ProviderSwitcher) *App {
	ta := textarea.New()
	ta.Placeholder = "Bir mesaj yaz... (? ile kısayollar)"
	ta.Prompt = "┃ "
	ta.CharLimit = 0
	ta.SetHeight(2)
	ta.Focus()
	ta.ShowLineNumbers = false

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))

	vp := viewport.New(80, 20)

	rend, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(78),
	)

	return &App{
		agent:      ag,
		switcher:   switcher,
		slash:      buildSlashRegistry(ag, switcher),
		notices:    newNoticeBoard(time.Now),
		shimmer:    newSharedShimmerClock(90),
		turn:       newTurnController(),
		hist:       newInputHistory(),
		queue:      &MessageQueue{},
		subm:       &submissionState{},
		themeBoot:  newThemeBoot(),
		fps:        newFpsOverlay(),
		details:    DetailsState{Global: DetailsExpanded},
		paste:      PasteState{},
		completion: newCompletionPipeline(),
		chat:       vp,
		input:      ta,
		spin:       sp,
		theme:      DefaultTheme(),
		rend:       rend,
		historyIdx: -1,
	}
}

// buildSlashRegistry composes the master registry from per-group
// command slices. Mirrors Hermes's registry.ts (one file per group,
// flat-concatenated into a master list, lookup-map built at startup).
func buildSlashRegistry(ag *agent.Agent, switcher ProviderSwitcher) *SlashRegistry {
	a := &appForRegistry{ag: ag, switcher: switcher}
	return newSlashRegistry(
		coreCommands(a),
		sessionCommands(a),
		sessionExtraCommands(a),
		setupCommands(a),
		debugCommands(),
		miscCommands(a),
	)
}

// appForRegistry is a tiny seam so the slash-command handlers can
// receive a stable *App-shaped struct without having to thread every
// field through. The real App updates its fields at runtime; the
// handlers always see the current values because they capture the
// pointer.
type appForRegistry struct {
	ag       *agent.Agent
	switcher ProviderSwitcher
}

// ProviderName helper for the registry-bound handlers.
func (a *appForRegistry) ProviderName() string { return a.ag.ProviderName() }
func (a *appForRegistry) CurrentModel() string { return a.ag.CurrentModel() }
func (a *appForRegistry) Available() []string  { return llm.Available() }

// Init starts the spinner, the git-branch probe, the shimmer clock, and
// the face ticker. The face ticker is a separate command from the
// existing spinner so the verb-pairing logic can be Hermes-faithful
// (the spinner renders the glyph; the face ticker decides the verb).
func (a *App) Init() tea.Cmd {
	a.shimmer.Start()
	return tea.Batch(
		textarea.Blink,
		gitBranchCmd(),
		faceTick(),
		shimmerTick(),
	)
}

type gitBranchMsg string

// gitBranchCmd shells out once at startup to label the status bar with
// the current git branch, matching Hermes Agent's own status bar. Not
// refreshed mid-session — cheap enough to accept and avoids running git
// on every tick.
func gitBranchCmd() tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
		if err != nil {
			return gitBranchMsg("")
		}
		return gitBranchMsg(strings.TrimSpace(string(out)))
	}
}

// Update is the central event router. Every tea.Msg the App receives
// lands here; the case-by-case dispatch mirrors Hermes's
// createGatewayEventHandler.ts but flattened (no React effects to
// route around — Bubbletea is purely synchronous).
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.layout()
		return a, nil

	case tea.KeyMsg:
		return a.handleKey(msg)

	case agentEventMsg:
		a.applyEvent(msg.ev)
		return a, waitForAgentEvent(msg.ch)

	case agentDoneMsg:
		a.streaming = false
		a.cancel = nil
		// Flush any pending notice that was held while the spinner
		// owned the status slot. NEVER inside resetTurn — the flush
		// has to fire at this specific site so a notice from session A
		// can't leak into session B via a generic reset.
		a.notices.flush()
		if a.dirty {
			a.render()
			a.dirty = false
		}
		// Release the submit gate.
		if a.subm != nil {
			a.subm.Release()
		}
		// Persist the sent message to the on-disk input history.
		if len(a.messages) >= 2 {
			lastUser := a.messages[len(a.messages)-2]
			if lastUser.role == "user" && a.hist != nil {
				_ = a.hist.Append(lastUser.text)
			}
		}
		if len(a.queuedMessages) > 0 {
			next := a.queuedMessages[0]
			a.queuedMessages = a.queuedMessages[1:]
			return a.startTurn(next)
		}
		// Decay stream delay back to idle baseline.
		a.streamDelay = 0
		return a, nil

	case spinner.TickMsg:
		if a.streaming {
			var cmd tea.Cmd
			a.spin, cmd = a.spin.Update(msg)
			return a, cmd
		}
		return a, nil

	case faceTickMsg:
		a.faceTickCount++
		// Only re-render while the busy indicator is on screen.
		if a.streaming {
			return a, faceTick()
		}
		return a, nil

	case shimmerTickMsg:
		return a, shimmerTick()

	case renderTickMsg:
		if !a.streaming {
			return a, nil
		}
		stillThinking := len(a.messages) > 0 && a.messages[len(a.messages)-1].role == "assistant" && a.messages[len(a.messages)-1].text == ""
		if a.dirty || stillThinking {
			a.render()
			a.dirty = false
		}
		// Re-arm with the current (possibly-boosted) delay.
		delay := renderTickInterval
		if a.streamDelay > 0 {
			now := time.Now()
			if now.Sub(a.lastTypingAt) > streamDelayIdleAfterMS*time.Millisecond {
				// Quiet long enough — relax back to idle.
				a.streamDelay = 0
			} else {
				delay = a.streamDelay
			}
		}
		return a, tea.Tick(delay, func(time.Time) tea.Msg { return renderTickMsg{} })

	case gitBranchMsg:
		a.gitBranch = string(msg)
		return a, nil

	case modelsListMsg:
		if msg.err != nil {
			a.notices.enqueue(Notice{Key: "models_err", Text: "Modeller alınamadı: " + msg.err.Error(), Level: NoticeLevelError, Kind: NoticeFlash, TTLMS: 5000})
		} else if len(msg.models) == 0 {
			a.notices.enqueue(Notice{Key: "models_empty", Text: "Sağlayıcı hiç model döndürmedi.", Level: NoticeLevelWarn, Kind: NoticeFlash, TTLMS: 5000})
		} else {
			items := make([]pickerItem, 0, len(msg.models))
			for _, m := range msg.models {
				desc := ""
				if m == a.agent.CurrentModel() {
					desc = "şu an aktif"
				}
				items = append(items, pickerItem{name: m, desc: desc})
			}
			a.openPicker(fmt.Sprintf("Model seç (%s)", a.agent.ProviderName()), pickerKindModel, items)
		}
		return a, nil

	case backgroundDetectMsg:
		// Stub slot for the OSC-11 probe when it lands; right now we
		// trust the env-driven detectLightMode() at startup.
		_ = msg
		return a, nil
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	cmds = append(cmds, cmd)
	a.chat, cmd = a.chat.Update(msg)
	cmds = append(cmds, cmd)
	return a, tea.Batch(cmds...)
}

// handleKey dispatches one key to the right sub-handler: pager > help >
// picker > approval > completion > slash suggestion > history >
// global. Mirrors Hermes's useOverlayKeys plus the per-overlay
// keymaps — flattened because Bubbletea has no effect hook.
func (a *App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.pager != nil {
		return a.handlePagerKey(msg)
	}
	if a.helpOpen {
		return a.handleHelpKey(msg)
	}
	if a.sessionSwitcher != nil {
		return a.handleSessionSwitcherKey(msg)
	}
	if a.picker != nil {
		return a.handlePickerKey(msg)
	}
	if a.pendingApproval != nil {
		return a.handleApprovalKey(msg)
	}

	// Slash command completion menu (only when the user is mid-token,
	// not mid-Enter / mid-arrow).
	if sugs := a.activeCommandSuggestions(); len(sugs) > 0 {
		switch msg.Type {
		case tea.KeyUp, tea.KeyDown:
			delta := 1
			if msg.Type == tea.KeyUp {
				delta = -1
			}
			a.cmdSuggestIndex = (a.cmdSuggestIndex + delta + len(sugs)) % len(sugs)
			return a, nil
		case tea.KeyTab:
			// Tab applies the active suggestion (Hermes: "Tab apply
			// completion").
			idx := a.cmdSuggestIndex
			if idx >= len(sugs) {
				idx = 0
			}
			return a.applyCompletion(sugs[idx])
		case tea.KeyEnter:
			// Enter: if applying the active completion would only
			// append a trailing space to an already-complete command,
			// submit instead. Otherwise apply.
			idx := a.cmdSuggestIndex
			if idx >= len(sugs) {
				idx = 0
			}
			cur := a.input.Value()
			comp := "/" + sugs[idx].ID
			if apply, isNoOp := completionToApplyOnSubmit(cur, comp); !isNoOp {
				_ = apply
				return a.applyCompletion(sugs[idx])
			}
			// Otherwise fall through to normal submit.
		case tea.KeyEsc:
			// 3-level Esc semantics (mirrors Hermes's model picker):
			//   1st Esc: clear filter
			//   2nd Esc: dismiss menu
			//   3rd Esc: n/a (we don't have a deeper context here)
			if a.cmdSuggestFilter != "" {
				a.cmdSuggestFilter = ""
				a.cmdSuggestAll = nil
				a.cmdSuggestIndex = 0
				return a, nil
			}
			a.cmdSuggestAll = nil
			a.cmdSuggestIndex = 0
			return a, nil
		}
	}

	// Number quick-pick (1-9) — only when the completion menu is open.
	if a.cmdSuggestAll != nil {
		if r := msg.Runes; len(r) == 1 && r[0] >= '1' && r[0] <= '9' {
			idx := int(r[0] - '1')
			if idx < len(a.cmdSuggestAll) {
				return a.applyCompletion(a.cmdSuggestAll[idx])
			}
		}
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		if a.streaming && a.cancel != nil {
			a.cancel()
			return a, nil
		}
		if strings.TrimSpace(a.input.Value()) != "" {
			a.input.Reset()
			a.historyIdx = -1
			return a, nil
		}
		return a, tea.Quit

	case tea.KeyEnter:
		text := strings.TrimSpace(a.input.Value())
		if text == "" {
			return a, nil
		}
		if a.streaming {
			a.queuedMessages = append(a.queuedMessages, text)
			a.input.Reset()
			a.notices.enqueue(Notice{Key: "queued_" + text, Text: fmt.Sprintf("↳ kuyruğa eklendi: %s", truncateForNote(text)), Level: NoticeLevelInfo, Kind: NoticeFlash, TTLMS: 3000})
			return a, nil
		}
		return a.submit(text)

	case tea.KeyUp:
		if val := a.input.Value(); val == "" || a.historyIdx != -1 {
			if handled, cmd := a.recallHistory(1); handled {
				return a, cmd
			}
		}

	case tea.KeyDown:
		if a.historyIdx != -1 {
			if handled, cmd := a.recallHistory(-1); handled {
				return a, cmd
			}
		}
	}

	// Any other key — let textarea handle it (insertion, navigation).
	var cmd tea.Cmd
	prev := a.input.Value()
	a.input, cmd = a.input.Update(msg)
	if a.input.Value() != prev {
		// The user typed something — bump the stream delay so the
		// render tick widens while typing, then decays after 250ms.
		a.streamDelay = streamDelayBoostMS * time.Millisecond
		a.lastTypingAt = time.Now()
		a.refreshSuggestions()
		// Detect a single "?" character as the help trigger.
		a.helpPending = (a.input.Value() == "?")
	} else {
		a.helpPending = false
	}
	a.cmdSuggestIndex = 0
	if a.historyIdx != -1 {
		a.historyIdx = -1
	}
	return a, cmd
}

// handleHelpKey is the keymap for the "?" help popover. Esc / q closes.
func (a *App) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyEnter:
		a.helpOpen = false
		a.input.Reset()
		return a, nil
	}
	if msg.String() == "q" {
		a.helpOpen = false
		a.input.Reset()
		return a, nil
	}
	return a, nil
}

// handlePagerKey is the keymap for the long-content pager.
func (a *App) handlePagerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.pager == nil {
		return a, nil
	}
	key := pagerKeyName(msg)
	next, res := handlePagerKey(*a.pager, key)
	a.pager = &next
	if res == PagerClose {
		a.pager = nil
	}
	return a, nil
}

// pagerKeyName maps a tea.KeyMsg to the pager's pure key vocabulary.
func pagerKeyName(msg tea.KeyMsg) string {
	switch msg.Type {
	case tea.KeyEsc:
		return "esc"
	case tea.KeyEnter:
		return "enter"
	case tea.KeyUp:
		return "up"
	case tea.KeyDown:
		return "down"
	case tea.KeyPgUp:
		return "pageup"
	case tea.KeyPgDown:
		return "pagedown"
	case tea.KeyHome:
		return "home"
	case tea.KeyEnd:
		return "end"
	}
	if r := msg.Runes; len(r) == 1 {
		return strings.ToLower(string(r))
	}
	return ""
}

// handleApprovalKey applies a key to the multi-option approval prompt.
// The pure function approvalKeyAction is the single source of truth for
// the keymap; this wrapper just dispatches the resulting action.
func (a *App) handleApprovalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.pendingApproval == nil {
		return a, nil
	}
	opts := NewApprovalOptions(false, false)
	act := approvalKeyAction(msg.String(), a.approvalSelected, opts)
	switch act.kind {
	case "move":
		a.approvalSelected = act.idx
		return a, nil
	case "choose":
		choice := opts[act.idx]
		a.pendingApproval = nil
		a.approvalSelected = 0
		a.agent.Approve(choice.Kind != "deny")
		// Sticky notice for "always" — sticks across the rest of the
		// session so the user knows what they decided.
		level := NoticeLevelSuccess
		if choice.Kind == "deny" {
			level = NoticeLevelWarn
		}
		sticky := choice.Kind == "always" || choice.Kind == "session"
		kind := NoticeFlash
		if sticky {
			kind = NoticeSticky
		}
		a.notices.set(Notice{
			Key:   "approval_" + a.pendingApprovalToolName() + "_" + choice.Kind,
			Text:  approvalActionLabel(choice) + " — " + a.pendingApprovalToolName(),
			Level: level,
			Kind:  kind,
			TTLMS: 4000,
		})
		return a, nil
	case "cancel":
		a.pendingApproval = nil
		a.approvalSelected = 0
		a.agent.Approve(false)
		a.notices.set(Notice{Key: "approval_deny", Text: "✗ reddedildi: " + a.pendingApprovalToolName(), Level: NoticeLevelWarn, Kind: NoticeFlash, TTLMS: 3000})
		return a, nil
	}
	return a, nil
}

func (a *App) pendingApprovalToolName() string {
	if a.pendingApproval == nil {
		return ""
	}
	return a.pendingApproval.toolName
}

func approvalActionLabel(o ApprovalOption) string {
	if o.Kind == "once" {
		return "✓ onaylandı (bir kez)"
	}
	if o.Kind == "session" {
		return "✓ onaylandı (oturum)"
	}
	if o.Kind == "always" {
		return "✓ onaylandı (her zaman)"
	}
	return "✗ reddedildi"
}

// applyCompletion replaces the current input with "/<name>" (or appends
// the arg if the caller already provided a full command), preserving
// the cursor-at-end convention.
func (a *App) applyCompletion(item FuzzyScoreItem) (tea.Model, tea.Cmd) {
	cur := a.input.Value()
	// Parse the current input as a slash command; replace the name
	// segment, keep whatever argument the user has already typed.
	name, arg := parseSlashInput(cur)
	if name == "" {
		a.input.SetValue("/" + item.ID + " ")
	} else if arg == "" {
		// Same name, no arg — just refresh.
		if name == item.ID {
			a.input.SetValue("/" + item.ID)
		} else {
			a.input.SetValue("/" + item.ID + " ")
		}
	} else {
		a.input.SetValue("/" + item.ID + " " + arg)
	}
	a.input.CursorEnd()
	a.cmdSuggestAll = nil
	a.cmdSuggestIndex = 0
	a.cmdSuggestFilter = ""
	return a, nil
}

// refreshSuggestions recomputes the slash-command suggestion list for
// the current input. Called on every keystroke. Filters by the
// fragment after the leading slash.
func (a *App) refreshSuggestions() {
	val := a.input.Value()
	if !strings.HasPrefix(val, "/") {
		a.cmdSuggestAll = nil
		a.cmdSuggestIndex = 0
		return
	}
	// Only suggest while typing the command name (no whitespace yet).
	if strings.ContainsAny(val, " \n") {
		a.cmdSuggestAll = nil
		return
	}
	// Detect the "?" trigger.
	if val == "?" {
		a.helpOpen = true
		a.input.Reset()
		a.cmdSuggestAll = nil
		return
	}
	filter := strings.TrimPrefix(val, "/")
	a.cmdSuggestFilter = filter
	all := a.slash.MatchFilter()
	ranked := rankFuzzy(all, filter)
	a.cmdSuggestAll = make([]FuzzyScoreItem, 0, len(ranked))
	for _, r := range ranked {
		a.cmdSuggestAll = append(a.cmdSuggestAll, r.item)
	}
	if a.cmdSuggestIndex >= len(a.cmdSuggestAll) {
		a.cmdSuggestIndex = 0
	}
}

// activeCommandSuggestions returns the current suggestion list, or nil
// if the input doesn't warrant a menu.
func (a *App) activeCommandSuggestions() []FuzzyScoreItem {
	if a.input.Value() == "?" {
		// ? opens help, not the slash menu.
		return nil
	}
	if !strings.HasPrefix(a.input.Value(), "/") {
		return nil
	}
	if strings.ContainsAny(a.input.Value(), " \n") {
		return nil
	}
	return a.cmdSuggestAll
}

// recallHistory moves the history cursor by delta (-1 = older, +1 = newer)
// and loads that message into the input.
func (a *App) recallHistory(delta int) (handled bool, cmd tea.Cmd) {
	if len(a.messageHistory) == 0 {
		return false, nil
	}
	next := a.historyIdx + delta
	if next < -1 || next >= len(a.messageHistory) {
		return true, nil
	}
	a.historyIdx = next
	if next == -1 {
		a.input.Reset()
		return true, nil
	}
	a.input.SetValue(a.messageHistory[len(a.messageHistory)-1-next])
	a.input.CursorEnd()
	return true, nil
}

// submit handles a confirmed text submission: routes to slash commands
// or starts a new turn.
func (a *App) submit(text string) (tea.Model, tea.Cmd) {
	a.messageHistory = append(a.messageHistory, text)
	a.historyIdx = -1
	a.input.Reset()
	if strings.HasPrefix(text, "/") {
		return a.handleSlashCommand(text)
	}
	return a.startTurn(text)
}

// startTurn begins one agent turn.
func (a *App) startTurn(text string) (tea.Model, tea.Cmd) {
	a.messages = append(a.messages, chatMessage{role: "user", text: text})
	a.messages = append(a.messages, chatMessage{role: "assistant", text: ""})
	a.streaming = true
	a.turnStart = time.Now()
	a.faceStartedAt = a.turnStart
	a.faceTickCount = 0
	a.thinkingVerb = thinkingVerbs[rand.IntN(len(thinkingVerbs))]
	a.render()

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	ch := a.agent.Run(ctx, text)

	return a, tea.Batch(waitForAgentEvent(ch), a.spin.Tick, renderTick(), faceTick())
}

// handleSlashCommand processes a "/"-prefixed input line through the
// registry.
func (a *App) handleSlashCommand(text string) (tea.Model, tea.Cmd) {
	name, arg := parseSlashInput(text)
	cmd := a.slash.Find(name)
	if cmd == nil {
		a.notices.set(Notice{Key: "unknown_cmd", Text: "Bilinmeyen komut: /" + name + " (yardım için /help)", Level: NoticeLevelWarn, Kind: NoticeFlash, TTLMS: 4000})
		return a, nil
	}
	note, followup, prefilled := cmd.Run(arg, a)
	if prefilled != "" {
		a.input.SetValue(prefilled)
		a.input.CursorEnd()
	}
	if note != "" {
		a.notices.set(Notice{Key: "slash_" + cmd.Name, Text: note, Level: NoticeLevelInfo, Kind: NoticeFlash, TTLMS: 5000})
	}
	return a, followup
}

// switchProvider applies a provider switch by name.
func (a *App) switchProvider(name string) (tea.Model, tea.Cmd) {
	if a.switcher == nil {
		a.notices.set(Notice{Key: "no_switcher", Text: "Sağlayıcı değişimi bu oturumda desteklenmiyor.", Level: NoticeLevelWarn, Kind: NoticeFlash, TTLMS: 4000})
		return a, nil
	}
	provider, err := a.switcher(name)
	if err != nil {
		a.notices.set(Notice{Key: "switch_err", Text: "Hata: " + err.Error(), Level: NoticeLevelError, Kind: NoticeFlash, TTLMS: 5000})
		return a, nil
	}
	a.agent.SetProvider(provider)
	a.notices.set(Notice{Key: "switched_provider", Text: fmt.Sprintf("Sağlayıcı değiştirildi: %s (model: %s)", provider.Name(), provider.Model()), Level: NoticeLevelSuccess, Kind: NoticeFlash, TTLMS: 4000})
	return a, nil
}

// openPicker shows a keyboard-navigable selection list.
func (a *App) openPicker(title string, kind pickerKind, items []pickerItem) {
	pickerW, pickerH := a.width-4, a.chat.Height
	if pickerH < 5 {
		pickerH = 5
	}
	p := a.newPicker(title, items, pickerW, pickerH)
	a.picker = &p
	a.pickerKind = kind
}

// openSessionSwitcher opens the /sessions overlay. Populates Live and
// History from the App's current state (Atlas currently has no live-
// session tracking, so Live is empty; History is built from
// messageHistory). The poll loop would refresh Live in the background.
func (a *App) openSessionSwitcher() {
	hist := make([]SessionEntry, 0, len(a.messageHistory))
	for i, m := range a.messageHistory {
		hist = append(hist, SessionEntry{
			ID:        fmt.Sprintf("hist-%d", i),
			Title:     truncateToWidth(m, 64),
			Model:     a.agent.CurrentModel(),
			StartedAt: time.Now().Add(-time.Duration(i+1) * time.Hour),
			Status:    SessionStatusIdle,
		})
	}
	prev := ""
	if a.sessionSwitcher != nil {
		rows := a.sessionSwitcher.mergedRows()
		if a.sessionSwitcher.Sel < len(rows) {
			prev = rows[a.sessionSwitcher.Sel].Entry.ID
		}
	}
	st := &SessionSwitcherState{
		Live:    nil,
		History: hist,
		Sel:     0,
		Visible: 12,
		Width:   a.width - 4,
	}
	if prev != "" {
		*st = reanchorSel(*st, prev)
	}
	a.sessionSwitcher = st
}

// handleSessionSwitcherKey routes keys when the session switcher
// overlay is open.
func (a *App) handleSessionSwitcherKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.sessionSwitcher == nil {
		return a, nil
	}
	key := msg.String()
	next, action := sessionSwitcherKeyAction(*a.sessionSwitcher, key)
	a.sessionSwitcher = &next
	switch action {
	case "cancel":
		a.sessionSwitcher = nil
	case "refresh":
		a.openSessionSwitcher()
	case "delete":
		// Atlas doesn't actually persist history, so the delete is
		// a no-op — but the switcher closes as if it succeeded.
		a.openSessionSwitcher()
		a.notices.set(Notice{Key: "session_delete", Text: "Silme isteği gönderildi (yerel depolama yok).", Level: NoticeLevelInfo, Kind: NoticeFlash, TTLMS: 3000})
	}
	return a, nil
}

func (a *App) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		a.picker = nil
		return a, nil
	case tea.KeyEnter:
		item, ok := a.picker.SelectedItem().(pickerItem)
		kind := a.pickerKind
		a.picker = nil
		if !ok {
			return a, nil
		}
		switch kind {
		case pickerKindProvider:
			return a.switchProvider(item.name)
		case pickerKindModel:
			a.agent.SetModel(item.name)
			a.notices.set(Notice{Key: "model_changed", Text: "Model değiştirildi: " + item.name, Level: NoticeLevelSuccess, Kind: NoticeFlash, TTLMS: 4000})
		}
		return a, nil
	}
	var cmd tea.Cmd
	*a.picker, cmd = a.picker.Update(msg)
	return a, cmd
}

func listModelsCmd(ag *agent.Agent) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		models, err := ag.ListModels(ctx)
		return modelsListMsg{models: models, err: err}
	}
}

// note appends a local info message.
func (a *App) note(text string) {
	a.messages = append(a.messages, chatMessage{role: "info", text: text})
	a.render()
}

// noteTool appends a tool-activity line.
func (a *App) noteTool(text string) {
	a.messages = append(a.messages, chatMessage{role: "tool", text: text})
	a.render()
}

func (a *App) applyEvent(ev agent.Event) {
	switch ev.Type {
	case agent.EventTextDelta:
		if n := len(a.messages); n > 0 {
			cleaned := cleanThinkingText(ev.TextDelta)
			if cleaned != "" {
				a.messages[n-1].text += cleaned
			}
		}
		a.dirty = true
	case agent.EventUsage:
		a.sessionInTok += ev.InputTok
		a.sessionOutTok += ev.OutputTok
	case agent.EventError:
		a.messages = append(a.messages, chatMessage{role: "error", text: ev.Err.Error()})
		a.render()
		a.dirty = false
		a.notices.set(Notice{Key: "turn_error", Text: "Hata: " + ev.Err.Error(), Level: NoticeLevelError, Kind: NoticeFlash, TTLMS: 6000})
	case agent.EventTurnDone:
		a.streaming = false
		if !a.turnStart.IsZero() {
			a.lastTurnMS = time.Since(a.turnStart).Milliseconds()
		}
		if a.dirty {
			a.render()
			a.dirty = false
		}
	case agent.EventToolStart:
		a.appendTrailRow("▸ " + ev.ToolName + " çalıştırılıyor...")
	case agent.EventAmbient:
		charm := toolCharms[rand.IntN(len(toolCharms))]
		a.appendTrailRow(fmt.Sprintf("… %s (%s · %s)", charm, ev.ToolName, fmtDuration(time.Duration(ev.ElapsedMS)*time.Millisecond)))
	case agent.EventApprovalRequest:
		a.pendingApproval = &approvalRequest{
			toolName:    ev.ToolName,
			input:       ev.ToolInput,
			previewPath: ev.PreviewPath,
			previewOld:  ev.PreviewOld,
			previewNew:  ev.PreviewNew,
		}
		a.approvalSelected = 0
	case agent.EventToolResult:
		status := "✓"
		if ev.ToolIsError {
			status = "✗"
		}
		a.appendTrailRow(fmt.Sprintf("%s %s → %s", status, ev.ToolName, truncateForNote(ev.ToolOutput)))
	}
}

// appendTrailRow caps the per-turn trail to trailLimit rows by evicting
// the oldest when the cap is reached. Mirrors Hermes's TRAIL_LIMIT = 8.
func (a *App) appendTrailRow(text string) {
	// Find the current trail block: the contiguous run of trail rows
	// starting from the end of messages.
	cut := len(a.messages)
	for cut > 0 && a.messages[cut-1].role == "trail" {
		cut--
	}
	// cut now points to the first trail row (or end of slice).
	trail := a.messages[cut:]
	if len(trail) >= trailLimit {
		// Drop the oldest row.
		a.messages = append(a.messages[:cut], a.messages[cut+1:]...)
		cut--
	}
	a.messages = append(a.messages, chatMessage{role: "trail", kind: "trail", text: text})
	a.render()
}

func truncateForNote(s string) string {
	s = strings.TrimSpace(s)
	const max = 200
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func (a *App) layout() {
	headerH := 1
	statusH := 1
	inputH := a.input.Height() + 2
	chatH := a.height - headerH - statusH - inputH
	if chatH < 3 {
		chatH = 3
	}
	a.chat.Width = a.width - 2 // reserve 1 col for scrollbar + 1 for the left bubble bar
	if a.chat.Width < 8 {
		a.chat.Width = 8
	}
	a.chat.Height = chatH
	a.input.SetWidth(a.width - 4)
	if a.rend != nil {
		a.rend, _ = glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(a.width-4),
		)
	}
	if a.picker != nil {
		a.picker.SetSize(a.width-4, chatH)
	}
	a.render()
}

// render rebuilds the chat content using the block-layout group walker
// for proper inter-block spacing.
func (a *App) render() {
	if len(a.messages) == 0 {
		a.chat.SetContent(a.renderWelcome())
		return
	}

	var blocks []string
	prevGroup := GroupIntro
	for i, m := range a.messages {
		cur := messageGroup(m.role, m.kind)
		// Skip invisible trail rows for layout decisions.
		if m.kind == "trail" && isInvisibleTrail(m) {
			continue
		}
		if i > 0 && hasLeadGap(prevGroup, cur) {
			blocks = append(blocks, "")
		}
		// User-turn separator (Hermes: ─── above every user message
		// after the first). Cheap visual chunking in long transcripts.
		if cur == GroupUser && prevGroup != GroupIntro && prevGroup != GroupUser {
			sepStyle := lipgloss.NewStyle().Foreground(a.theme.Border)
			blocks = append(blocks, sepStyle.Render(strings.Repeat("─", a.chat.Width)))
		}
		blocks = append(blocks, a.renderMessage(m, cur))
		prevGroup = cur
	}
	a.chat.SetContent(strings.Join(blocks, "\n"))
	a.chat.GotoBottom()
}

// renderMessage paints a single chat row.
func (a *App) renderMessage(m chatMessage, g BlockGroup) string {
	switch m.role {
	case "user":
		content := a.theme.UserLabel.Render("Sen") + "\n" + m.text
		return a.theme.UserBubble.Render(content)
	case "assistant":
		var body string
		if m.text == "" && a.streaming && a.lastAssistantEmpty() {
			elapsed := time.Since(a.turnStart).Round(time.Second)
			body = fmt.Sprintf("%s %s (%s)", a.spin.View(), a.thinkingVerb, elapsed)
		} else {
			body = a.renderMarkdown(m.text)
		}
		content := a.theme.AsstLabel.Render("Atlas") + "\n" + body
		return a.theme.AsstBubble.Render(content)
	case "error":
		return a.theme.ErrorText.Render("✗ " + m.text)
	case "trail":
		return a.theme.HelpText.Render("  " + m.text)
	case "tool":
		return a.theme.ToolBox.Render(a.theme.HelpText.Render(m.text))
	default: // info / event
		return a.theme.HelpText.Render(m.text)
	}
}

// lastAssistantEmpty is true when the most recent message is the
// in-progress assistant bubble (role=assistant, text=empty) so the
// spinner substitutes for the body.
func (a *App) lastAssistantEmpty() bool {
	n := len(a.messages)
	if n == 0 {
		return false
	}
	last := a.messages[n-1]
	return last.role == "assistant" && last.text == ""
}

// renderWelcome fills the otherwise-empty chat pane with a session-info
// panel before the first message.
func (a *App) renderWelcome() string {
	var b strings.Builder
	b.WriteString(a.theme.Title.Render(a.theme.WelcomeMessage))
	b.WriteString("\n\n")

	cwd, _ := os.Getwd()
	info := []string{
		fmt.Sprintf("sağlayıcı: %s", a.agent.ProviderName()),
		fmt.Sprintf("model: %s", a.agent.CurrentModel()),
	}
	if cwd != "" {
		info = append(info, "dizin: "+cwd)
	}
	if a.gitBranch != "" {
		info = append(info, "dal: "+a.gitBranch)
	}
	for _, line := range info {
		b.WriteString(a.theme.HelpText.Render("  " + line))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if names := a.agent.ToolNames(); len(names) > 0 {
		n := len(names)
		body := "    " + strings.Join(names, ", ")
		b.WriteString(a.renderAccordion(accordion{title: "Araçlar", count: &n, open: true, body: a.theme.HelpText.Render(body)}))
		b.WriteString("\n\n")
	}

	tips := []string{
		"Bir mesaj yazıp Enter'a bas.",
		"/model — modeli değiştir",
		"/provider — LLM sağlayıcısını değiştir",
		"/help — tüm komutlar",
		"? — kısayollar",
	}
	for _, t := range tips {
		b.WriteString(a.theme.HelpText.Render("  · " + t))
		b.WriteString("\n")
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(strings.TrimRight(b.String(), "\n"))
}

func (a *App) renderMarkdown(s string) string {
	if a.rend == nil || s == "" {
		return s
	}
	out, err := a.rend.Render(s)
	if err != nil {
		return s
	}
	return strings.TrimRight(out, "\n")
}

// View assembles the full screen. Top to bottom: banner, main pane
// (chat with scrollbar, or picker / approval / pager overlay), sticky
// prompt breadcrumb if scrolled away, queued-messages preview, input
// box, status bar. The status-bar's left slot can be replaced by a
// notice when one is active.
func (a *App) View() string {
	if a.width == 0 {
		return "başlatılıyor..."
	}

	header := a.renderBanner()
	status := a.renderStatusBar()

	mainPane := lipgloss.JoinHorizontal(lipgloss.Top, a.chat.View(), a.renderScrollbar(a.chat.Height))
	if a.pager != nil {
		mainPane = a.renderPager(*a.pager)
	} else if a.sessionSwitcher != nil {
		mainPane = a.renderSessionSwitcher(*a.sessionSwitcher)
	} else if a.helpOpen {
		mainPane = overlayBottom(mainPane, a.renderHelpHint())
	} else if a.picker != nil {
		mainPane = a.picker.View()
	} else if a.pendingApproval != nil {
		mainPane = overlayBottom(mainPane, a.renderApprovalPrompt(a.pendingApproval))
	} else if sugs := a.activeCommandSuggestions(); len(sugs) > 0 {
		mainPane = overlayBottom(mainPane, a.renderCommandMenu(sugs))
	}

	rows := []string{header, mainPane}
	if sticky := a.renderStickyPrompt(); sticky != "" {
		rows = append(rows, sticky)
	}
	if queued := a.renderQueuedMessages(); queued != "" {
		rows = append(rows, queued)
	}
	rows = append(rows,
		a.theme.InputBox.Width(a.width-2).Render(a.input.View()),
		status,
	)
	out := lipgloss.JoinVertical(lipgloss.Left, rows...)
	// FPS overlay (dev only, no-op when HERMES_TUI_FPS=0).
	if fpsView := a.fps.View(a); fpsView != "" {
		out += "\n" + fpsView
	}
	return out
}

func (a *App) renderQueuedMessages() string {
	if len(a.queuedMessages) == 0 {
		return ""
	}
	var b strings.Builder
	for i, m := range a.queuedMessages {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(a.theme.HelpText.Render(fmt.Sprintf("↳ %d. %s", i+1, truncateForNote(m))))
	}
	return b.String()
}

// renderBanner now delegates to banner.go's tier-based renderer.
func (a *App) renderBannerString() string { return a.renderBanner() }
func (a *App) renderHeader() string      { return a.renderBanner() }

// statusSegment is one optional piece of the status bar's right-hand side.
type statusSegment struct {
	text     string
	priority int
}

// renderStatusBar shows live session state with the Hermes
// priority-disclosure ladder: pinned (status / model) segments never
// drop, optional segments shed from lowest to highest priority as the
// terminal narrows.
func (a *App) renderStatusBar() string {
	// Pinned left: status indicator + provider/model.
	left := fmt.Sprintf("%s  %s/%s", statusIndicator(a.streaming), a.agent.ProviderName(), a.agent.CurrentModel())
	if n := len(a.queuedMessages); n > 0 {
		left += fmt.Sprintf("  [kuyrukta %d]", n)
	}

	// Optional segments, lowest priority first (so they shed first).
	var segments []statusSegment
	if a.lastTurnMS > 0 {
		segments = append(segments, statusSegment{fmt.Sprintf("%.1fs", float64(a.lastTurnMS)/1000), 1})
	}
	// Tokens segment is always present (Hermes pins it) so the user
	// can see the running total even before the first turn completes.
	segments = append(segments, statusSegment{"tokens " + fmtTokens(a.sessionInTok+a.sessionOutTok), 2})
	if a.gitBranch != "" {
		segments = append(segments, statusSegment{a.gitBranch, 3})
	}
	if cwd, err := os.Getwd(); err == nil {
		segments = append(segments, statusSegment{filepath.Base(cwd), 4})
	}

	// If a notice is active, replace the trailing "ready" slot.
	noticeText := a.notices.currentText()

	const sep = "  •  "
	budget := a.width - lipgloss.Width(left) - 4
	var kept []string
	for p := 1; p <= 4; p++ {
		for _, s := range segments {
			if s.priority != p {
				continue
			}
			need := lipgloss.Width(s.text)
			if len(kept) > 0 {
				need += lipgloss.Width(sep)
			}
			if need <= budget {
				kept = append(kept, s.text)
				budget -= need
			}
		}
	}
	right := strings.Join(kept, sep)
	if noticeText != "" {
		right = noticeText + "  " + right
		if lipgloss.Width(right) > budget+lipgloss.Width(right)-lipgloss.Width(noticeText)-2 {
			right = noticeText
		}
	}

	gap := a.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right
	return a.theme.StatusBar.Width(a.width).Padding(0, 1).Render(line)
}

// renderCommandMenu paints the slash-command completion dropdown with
// a centered-window viewport (start = active - 8) so a long list
// doesn't grow/shrink as the user scrolls past row 8.
func (a *App) renderCommandMenu(sugs []FuzzyScoreItem) string {
	idx := a.cmdSuggestIndex
	if idx >= len(sugs) {
		idx = 0
	}

	const visibleRows = 8
	half := visibleRows / 2
	start := idx - half
	if start < 0 {
		start = 0
	}
	if start > len(sugs)-visibleRows && len(sugs) > visibleRows {
		start = len(sugs) - visibleRows
	}
	if start < 0 {
		start = 0
	}
	end := start + visibleRows
	if end > len(sugs) {
		end = len(sugs)
	}

	// Compute name column width as the longest visible name.
	nameW := 0
	for _, s := range sugs[start:end] {
		if len(s.ID) > nameW {
			nameW = len(s.ID)
		}
	}
	nameW += 2

	var b strings.Builder
	for i := start; i < end; i++ {
		s := sugs[i]
		desc := s.Description
		if i == idx {
			// Chip-style highlight on the active row.
			b.WriteString(a.theme.SelectedBgBackground(a.theme.UserLabel.Render("▸ /" + s.ID)))
		} else {
			b.WriteString(a.theme.HelpText.Render("  /" + s.ID))
		}
		if desc != "" {
			b.WriteString(a.theme.HelpText.Render("  " + truncateForNote(desc)))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	width := a.width - 4
	if width < 20 {
		width = 20
	}
	return a.theme.InputBox.Width(width).Render(b.String())
}

// renderScrollbar draws the chat scrollbar.
func (a *App) renderScrollbar(height int) string {
	if height < 1 {
		return ""
	}
	total, visible := a.chat.TotalLineCount(), a.chat.VisibleLineCount()
	if total <= visible {
		return strings.Repeat(" \n", height-1) + " "
	}
	thumbSize := height * visible / total
	if thumbSize < 1 {
		thumbSize = 1
	}
	maxThumbStart := height - thumbSize
	thumbStart := int(a.chat.ScrollPercent() * float64(maxThumbStart))
	var b strings.Builder
	trackStyle := lipgloss.NewStyle().Foreground(a.theme.Border)
	thumbStyle := lipgloss.NewStyle().Foreground(a.theme.Accent)
	for i := 0; i < height; i++ {
		if i > 0 {
			b.WriteString("\n")
		}
		if i >= thumbStart && i < thumbStart+thumbSize {
			b.WriteString(thumbStyle.Render("┃"))
		} else {
			b.WriteString(trackStyle.Render("│"))
		}
	}
	return b.String()
}

// capLines keeps a long body from pushing the box off screen.
func capLines(s string, max int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= max {
		return s
	}
	extra := len(lines) - max
	kept := append([]string{}, lines[:max]...)
	kept = append(kept, fmt.Sprintf("… (+%d satır daha)", extra))
	return strings.Join(kept, "\n")
}

// overlayBottom replaces the bottom lines of base with overlay's lines.
func overlayBottom(base, overlay string) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")
	if len(overlayLines) > len(baseLines) {
		overlayLines = overlayLines[len(overlayLines)-len(baseLines):]
	}
	start := len(baseLines) - len(overlayLines)
	if start < 0 {
		start = 0
	}
	for i, line := range overlayLines {
		baseLines[start+i] = line
	}
	return strings.Join(baseLines, "\n")
}

func statusIndicator(streaming bool) string {
	if streaming {
		return "● yanıtlanıyor"
	}
	return "○ hazır"
}
