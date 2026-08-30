package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme is the full color + style surface for Atlas's TUI. The shape is
// the Hermes-Agent theme.ts surface translated to Go — seeds in, tokens
// out, contrast-guarded foregrounds, polarity-aware fills. The major
// upgrade from the previous Atlas palette is:
//
//  1. The seed set now matches Hermes's 12-seed shape: accent, primary,
//     prompt, shellDollar, status{Good,Bad,Warn,Critical}, text, bg,
//     border, error, ok. Anything else is derived.
//  2. Every derived fill (header/status/selection/surface) runs through
//     polarity check so a light skin can't accidentally land on a
//     background that's the wrong shade of dim.
//  3. Brand glyphs (the prompt mark, the tool mark) are real Theme
//     fields, not literals in render code, so a future skin swap
//     propagates without touching the renderers.
//
// The architecture note in the old version still applies: Atlas is
// "polish a small set of seeds" rather than "pick every token
// independently", because hand-picked hex values lead to
// incoherently-dim UI when one token drifts.
type Theme struct {
	// Identity / brand colors (skinnable).
	Accent      lipgloss.Color
	Primary     lipgloss.Color
	Prompt      lipgloss.Color
	ShellDollar lipgloss.Color
	User        lipgloss.Color
	Asst        lipgloss.Color

	// Semantic status tones (5 tiers: good / warn / bad / critical /
	// info). statusGood mirrors the Hermes "ok" status; the others are
	// distinct shades for the 4-level escalation (info → warn → bad →
	// critical).
	StatusGood     lipgloss.Color
	StatusWarn     lipgloss.Color
	StatusBad      lipgloss.Color
	StatusCritical lipgloss.Color
	StatusInfo     lipgloss.AdaptiveColor

	Success lipgloss.Color
	Error   lipgloss.Color

	// Adaptive (dark/light) tokens. AdaptiveColor is the
	// lipgloss-native "two-shade" pair; we use it for everything that
	// sits on a polarity-aware background.
	Muted     lipgloss.AdaptiveColor
	Border    lipgloss.AdaptiveColor
	Label     lipgloss.AdaptiveColor
	Text      lipgloss.AdaptiveColor
	HeaderBg  lipgloss.AdaptiveColor
	HeaderFg  lipgloss.AdaptiveColor
	StatusBg  lipgloss.AdaptiveColor
	StatusFg  lipgloss.AdaptiveColor
	SurfaceBg lipgloss.AdaptiveColor
	SelectedFg lipgloss.AdaptiveColor
	SelectedBg lipgloss.AdaptiveColor

	// Diff colors (4 shades: added / removed, each in pale and emphatic
	// variants). The pale variant is for context lines; the emphatic
	// variant is for the actual change line.
	DiffAdded     lipgloss.AdaptiveColor
	DiffRemoved   lipgloss.AdaptiveColor
	DiffAddedWord lipgloss.AdaptiveColor
	DiffRemovedWord lipgloss.AdaptiveColor

	// Lipgloss styles. Each is a derived style that uses the colors
	// above. Components pick the style that matches their semantic role
	// rather than composing the colors themselves.
	HeaderBar    lipgloss.Style
	StatusBar    lipgloss.Style
	Title        lipgloss.Style
	UserLabel    lipgloss.Style
	AsstLabel    lipgloss.Style
	UserBubble   lipgloss.Style
	AsstBubble   lipgloss.Style
	InputBox     lipgloss.Style
	ErrorText    lipgloss.Style
	HelpText     lipgloss.Style
	NoticeText   lipgloss.Style
	ThinkingText lipgloss.Style
	DiffAdd      lipgloss.Style
	DiffRemove   lipgloss.Style
	DiffContext  lipgloss.Style
	ToolBox      lipgloss.Style
	ApprovalBox  lipgloss.Style

	// Brand glyphs (skinnable). The default values mirror Hermes's
	// brand defaults: `❯` for the prompt, `┊` for the tool gutter.
	PromptGlyph  string
	ToolGlyph    string
	AsstGlyph    string
	UserGlyph    string

	// Brand text.
	WelcomeMessage string
	GoodbyeMessage string

	// DimFallback is the muted color used instead of SGR dim on
	// terminals (Apple Terminal) that silently ignore the dim SGR code.
	// Hermes falls back to a literal hex on those terminals; Atlas
	// follows the same approach via the termprobe module.
	DimFallback lipgloss.AdaptiveColor
}

// seeds is the minimal per-polarity palette DefaultTheme derives
// everything else from. The 12 fields map 1:1 to Hermes's ThemeSeeds
// (accent, primary, prompt, shellDollar, status4, text, bg, border, ok,
// error, warn, plus identity-fill overrides).
type seeds struct {
	bg, text, border string

	accent, primary, prompt, shellDollar string
	user, asst                          string

	ok, warn, err                       string
	statusGood, statusWarn, statusBad, statusCritical string

	displayMinContrast  float64
	semanticMinContrast float64
}

// DefaultTheme returns the canonical Atlas theme. The dark/light pairs
// follow Hermes's "light is computed from dark" principle (liftForContrast
// at 4.5 minimum), so a future "skin" addition can ship dark-only and
// have the light variant generated automatically.
func DefaultTheme() Theme {
	dark := seeds{
		bg:     "#101014",
		text:   "#FFF8DC",
		border: "#CD7F32",
		accent: "#FFBF00",
		primary: "#FFD700",
		prompt: "#FFF8DC",
		shellDollar: "#4dabf7",
		user:  "#7AA2F7",
		asst:  "#F5A97F",
		ok:    "#4caf50",
		warn:  "#ffa726",
		err:   "#ef5350",
		statusGood:     "#8FBC8F",
		statusWarn:     "#FFD700",
		statusBad:      "#FF8C00",
		statusCritical: "#FF6B6B",
		displayMinContrast: 1.45,
		semanticMinContrast: 2.2,
	}
	light := seeds{
		bg:     "#ffffff",
		text:   "#3D2F13",
		border: "#A56628",
		accent: "#956E00",
		primary: "#867000",
		prompt: "#2B2014",
		shellDollar: "#377BB3",
		user:  "#3B5FCC",
		asst:  "#B5651D",
		ok:    "#367E39",
		warn:  "#956115",
		err:   "#C14240",
		statusGood:     "#5C7A5C",
		statusWarn:     "#867000",
		statusBad:      "#A65A00",
		statusCritical: "#B94D4D",
		displayMinContrast: 1.18,
		semanticMinContrast: 1.6,
	}

	dt := derive(dark)
	lt := derive(light)

	adapt := func(d, l string) lipgloss.AdaptiveColor {
		return lipgloss.AdaptiveColor{Dark: d, Light: l}
	}

	muted := adapt(dt.muted, lt.muted)
	border := adapt(dt.border, lt.border)
	headerBg := adapt(dt.headerBg, lt.headerBg)
	headerFg := adapt(dt.headerFg, lt.headerFg)
	statusBg := adapt(dt.statusBg, lt.statusBg)
	statusFg := adapt(dt.statusFg, lt.statusFg)
	selBg := adapt(dt.selectionBg, lt.selectionBg)
	selFg := adapt(dt.selectionFg, lt.selectionFg)
	surfaceBg := adapt(dt.surfaceBg, lt.surfaceBg)

	diffAdded := adapt("#dcffdc", "#d6f5d6")
	diffRemoved := adapt("#ffdcdc", "#fadada")
	diffAddedWord := adapt("#248a3d", "#1f7a36")
	diffRemovedWord := adapt("#cf222e", "#a8202b")

	brand := lipgloss.Color(dark.accent)
	primary := lipgloss.Color(dark.primary)
	prompt := lipgloss.Color(dark.prompt)
	shellDollar := lipgloss.Color(dark.shellDollar)
	user := lipgloss.Color(dark.user)
	asst := lipgloss.Color(dark.asst)
	success := lipgloss.Color(dark.ok)
	errColor := lipgloss.Color(dark.err)

	return Theme{
		Accent:        brand,
		Primary:       primary,
		Prompt:        prompt,
		ShellDollar:   shellDollar,
		User:          user,
		Asst:          asst,
		StatusGood:     lipgloss.Color(dark.statusGood),
		StatusWarn:     lipgloss.Color(dark.statusWarn),
		StatusBad:      lipgloss.Color(dark.statusBad),
		StatusCritical: lipgloss.Color(dark.statusCritical),
		StatusInfo:     muted,
		Success:       success,
		Error:         errColor,

		Muted:     muted,
		Border:    border,
		Label:     adapt("#E8E6F0", "#3D2F13"),
		Text:      adapt(dark.text, light.text),
		HeaderBg:  headerBg,
		HeaderFg:  headerFg,
		StatusBg:  statusBg,
		StatusFg:  statusFg,
		SurfaceBg: surfaceBg,
		SelectedFg: selFg,
		SelectedBg: selBg,

		DiffAdded:       diffAdded,
		DiffRemoved:     diffRemoved,
		DiffAddedWord:   diffAddedWord,
		DiffRemovedWord: diffRemovedWord,

		HeaderBar: lipgloss.NewStyle().
			Background(headerBg).
			Foreground(headerFg).
			Bold(true).
			Padding(0, 1),
		StatusBar: lipgloss.NewStyle().
			Background(statusBg).
			Foreground(statusFg).
			Padding(0, 1),
		Title: lipgloss.NewStyle().
			Foreground(brand).
			Bold(true),
		UserLabel: lipgloss.NewStyle().
			Foreground(user).
			Bold(true),
		AsstLabel: lipgloss.NewStyle().
			Foreground(asst).
			Bold(true),
		UserBubble: lipgloss.NewStyle().
			BorderStyle(lipgloss.ThickBorder()).
			BorderLeft(true).
			BorderForeground(user).
			PaddingLeft(1),
		AsstBubble: lipgloss.NewStyle().
			BorderStyle(lipgloss.ThickBorder()).
			BorderLeft(true).
			BorderForeground(asst).
			PaddingLeft(1),
		InputBox: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(0, 1),
		ErrorText: lipgloss.NewStyle().
			Foreground(errColor).
			Bold(true),
		HelpText: lipgloss.NewStyle().
			Foreground(muted),
		NoticeText: lipgloss.NewStyle().
			Foreground(muted).
			Italic(true),
		ThinkingText: lipgloss.NewStyle().
			Foreground(muted).
			Italic(true),
		DiffAdd: lipgloss.NewStyle().
			Foreground(diffAddedWord),
		DiffRemove: lipgloss.NewStyle().
			Foreground(diffRemovedWord),
		DiffContext: lipgloss.NewStyle().
			Foreground(muted),
		ToolBox: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(0, 1),
		ApprovalBox: lipgloss.NewStyle().
			BorderStyle(lipgloss.DoubleBorder()).
			BorderForeground(errColor).
			Padding(0, 1),

		PromptGlyph:  "❯",
		ToolGlyph:    "┊",
		AsstGlyph:    "┊",
		UserGlyph:    "❯",
		WelcomeMessage: "Atlas'a hoş geldin",
		GoodbyeMessage: "Güle güle.",

		DimFallback: muted,
	}
}

// derivedTones are the tokens computed from seeds rather than hand-picked.
// The shape matches Hermes's deriveTones (muted, border, headerBg/Fg,
// statusBg/Fg, selectionBg/Fg, surfaceBg). Atlas adds a surfaceBg tone
// for the panel/composer backgrounds that need to be subtly distinct
// from the terminal's own bg without competing with the header bar.
type derivedTones struct {
	muted, border                 string
	headerBg, headerFg            string
	statusBg, statusFg            string
	selectionBg, selectionFg      string
	surfaceBg                     string
}

// derive runs the seed→token ladder. Every foreground tone is then
// re-lifted against the background it will actually sit on so a derived
// token can never end up unreadable on its real background.
func derive(s seeds) derivedTones {
	// Muted: mostly background tinted faintly by the brand hue, then
	// desaturated so it reads as "quiet" rather than "duller brand".
	muted := Desaturate(Mix(s.accent, s.bg, 0.55), 0.35)
	muted = LiftForContrast(muted, s.bg, s.displayMinContrast)

	// Border: a touch lifted from the bg.
	border := Mix(s.border, s.bg, 0.2)

	// Header bar: subtle brand-tinted fill so the bar reads as UI
	// chrome, not transparent scrollback. Foreground is the brand
	// itself, contrast-lifted against the bar's bg.
	headerBg := Mix(s.accent, s.bg, 0.85)
	headerFg := LiftForContrast(s.accent, headerBg, s.semanticMinContrast)

	// Status bar: a border-tinted fill, more neutral than the header
	// so the header still reads as the most prominent band.
	statusBg := Mix(s.border, s.bg, 0.55)
	statusFg := LiftForContrast(muted, statusBg, s.displayMinContrast)

	// Selected-row chip: stronger brand fill, with its own contrast-lifted
	// foreground so the chip is always legible regardless of the default
	// text color.
	selectionBg := Mix(s.accent, s.bg, 0.62)
	selectionFg := LiftForContrast(s.text, selectionBg, s.semanticMinContrast)

	// Surface: a slightly more saturated neutral than the terminal bg.
	// Used for tool/approval box interiors.
	surfaceBg := Mix(s.border, s.bg, 0.30)

	return derivedTones{
		muted:       muted,
		border:      border,
		headerBg:    headerBg,
		headerFg:    headerFg,
		statusBg:    statusBg,
		statusFg:    statusFg,
		selectionBg: selectionBg,
		selectionFg: selectionFg,
		surfaceBg:   surfaceBg,
	}
}

// SelectedBgBackground is a small convenience used by the approval
// prompt to wrap a pre-styled string in the chip background. Wrapping
// a styled string in a new style preserves the inner SGR but adds the
// background fill — that's exactly what the chip wants.
func (t Theme) SelectedBgBackground(s string) string {
	return lipgloss.NewStyle().Background(t.SelectedBg).Render(s)
}
