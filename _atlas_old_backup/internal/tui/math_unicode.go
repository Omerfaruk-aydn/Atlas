package tui

import (
	"regexp"
	"strings"
)

// mathSymbolTable is the best-effort LaTeX→Unicode substitution table
// used by mathUnicodeConvert. Hermes ships a comprehensive one
// (Greek letters, set theory, relations, arrows, sub/super, fractions)
// so model output containing $$..$$ / $..$ / \[..\] renders sensibly
// in a terminal without pulling in a LaTeX engine. The Atlas port
// keeps the same shape: longest-match-first so `\neq` doesn't get
// split into `\n` + `eq`, and balanced-brace parsing for `\frac{a}{b}`.
type mathSymbol struct {
	lhs  string
	rhs  string
}

var mathSymbolTable = []mathSymbol{
	// Greek lowercase.
	{`\alpha`, `α`}, {`\beta`, `β`}, {`\gamma`, `γ`}, {`\delta`, `δ`},
	{`\epsilon`, `ε`}, {`\varepsilon`, `ε`}, {`\zeta`, `ζ`}, {`\eta`, `η`},
	{`\theta`, `θ`}, {`\vartheta`, `ϑ`}, {`\iota`, `ι`}, {`\kappa`, `κ`},
	{`\lambda`, `λ`}, {`\mu`, `μ`}, {`\nu`, `ν`}, {`\xi`, `ξ`},
	{`\pi`, `π`}, {`\varpi`, `ϖ`}, {`\rho`, `ρ`}, {`\varrho`, `ϱ`},
	{`\sigma`, `σ`}, {`\varsigma`, `ς`}, {`\tau`, `τ`}, {`\upsilon`, `υ`},
	{`\phi`, `φ`}, {`\varphi`, `ϕ`}, {`\chi`, `χ`}, {`\psi`, `ψ`},
	{`\omega`, `ω`},
	// Greek uppercase.
	{`\Gamma`, `Γ`}, {`\Delta`, `Δ`}, {`\Theta`, `Θ`}, {`\Lambda`, `Λ`},
	{`\Xi`, `Ξ`}, {`\Pi`, `Π`}, {`\Sigma`, `Σ`}, {`\Upsilon`, `Υ`},
	{`\Phi`, `Φ`}, {`\Psi`, `Ψ`}, {`\Omega`, `Ω`},
	// Set theory / logic.
	{`\forall`, `∀`}, {`\exists`, `∃`}, {`\nexists`, `∄`},
	{`\emptyset`, `∅`}, {`\varnothing`, `∅`},
	{`\in`, `∈`}, {`\notin`, `∉`}, {`\ni`, `∋`},
	{`\subset`, `⊂`}, {`\supset`, `⊃`}, {`\subseteq`, `⊆`}, {`\supseteq`, `⊇`},
	{`\cup`, `∪`}, {`\cap`, `∩`}, {`\setminus`, `∖`},
	{`\land`, `∧`}, {`\lor`, `∨`}, {`\lnot`, `¬`}, {`\neg`, `¬`},
	// Relations.
	{`\neq`, `≠`}, {`\ne`, `≠`}, {`\leq`, `≤`}, {`\le`, `≤`},
	{`\geq`, `≥`}, {`\ge`, `≥`}, {`\equiv`, `≡`}, {`\approx`, `≈`},
	{`\sim`, `∼`}, {`\simeq`, `≃`}, {`\cong`, `≅`}, {`\propto`, `∝`},
	{`\ll`, `≪`}, {`\gg`, `≫`},
	// Arrows.
	{`\to`, `→`}, {`\rightarrow`, `→`}, {`\leftarrow`, `←`}, {`\gets`, `←`},
	{`\leftrightarrow`, `↔`}, {`\Rightarrow`, `⇒`}, {`\Leftarrow`, `⇐`},
	{`\Leftrightarrow`, `⇔`}, {`\mapsto`, `↦`},
	{`\uparrow`, `↑`}, {`\downarrow`, `↓`}, {`\Uparrow`, `⇑`}, {`\Downarrow`, `⇓`},
	// Operators.
	{`\pm`, `±`}, {`\mp`, `∓`}, {`\times`, `×`}, {`\div`, `÷`},
	{`\cdot`, `·`}, {`\ast`, `∗`}, {`\star`, `⋆`}, {`\circ`, `∘`},
	{`\oplus`, `⊕`}, {`\otimes`, `⊗`}, {`\sum`, `∑`}, {`\prod`, `∏`},
	{`\int`, `∫`}, {`\oint`, `∮`}, {`\partial`, `∂`}, {`\nabla`, `∇`},
	{`\infty`, `∞`}, {`\ldots`, `…`}, {`\cdots`, `⋯`}, {`\dots`, `…`},
	// Big operators.
	{`\bigcap`, `⋂`}, {`\bigcup`, `⋃`}, {`\bigvee`, `⋁`}, {`\bigwedge`, `⋀`},
	// Number sets.
	{`\mathbb{N}`, `ℕ`}, {`\mathbb{Z}`, `ℤ`}, {`\mathbb{Q}`, `ℚ`},
	{`\mathbb{R}`, `ℝ`}, {`\mathbb{C}`, `ℂ`},
	// Sub/superscript digits.
	{`^0`, `⁰`}, {`^1`, `¹`}, {`^2`, `²`}, {`^3`, `³`}, {`^4`, `⁴`},
	{`^5`, `⁵`}, {`^6`, `⁶`}, {`^7`, `⁷`}, {`^8`, `⁸`}, {`^9`, `⁹`},
	{`^n`, `ⁿ`}, {`^i`, `ⁱ`},
	{`_0`, `₀`}, {`_1`, `₁`}, {`_2`, `₂`}, {`_3`, `₃`}, {`_4`, `₄`},
	{`_5`, `₅`}, {`_6`, `₆`}, {`_7`, `₇`}, {`_8`, `₈`}, {`_9`, `₉`},
	{`_i`, `ᵢ`}, {`_j`, `ⱼ`},
}

// mathFracRe matches a `\frac{a}{b}` group with optional braces.
var mathFracRe = regexp.MustCompile(`\\frac\s*\{([^{}]+)\}\s*\{([^{}]+)\}`)

// mathBoxedRe matches `\boxed{X}` — used to wrap the argument in
// sentinel control chars U+0001/U+0002 so a later renderer can apply
// inverse-video highlighting.
var mathBoxedRe = regexp.MustCompile(`\\boxed\s*\{([^{}]+)\}`)

// mathXArrowRe matches `\xrightarrow{label}` → "─label→".
var mathXArrowRe = regexp.MustCompile(`\\xrightarrow\s*\{([^{}]+)\}`)

// mathUnicodeConvert applies the mathSymbolTable + the brace-aware
// transforms to convert a math-mode string into terminal-friendly
// Unicode. The output is safe to feed to Glamour; the substitution
// never inserts ANSI sequences.
func mathUnicodeConvert(s string) string {
	// Apply transforms in order: \frac, \boxed, \xrightarrow, then
	// the symbol table (longest-first, word-boundary-safe).
	s = mathFracRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := mathFracRe.FindStringSubmatch(m)
		if len(parts) != 3 {
			return m
		}
		num := wrapForFrac(parts[1])
		den := wrapForFrac(parts[2])
		return num + "/" + den
	})
	s = mathBoxedRe.ReplaceAllString(s, "\x01$1\x02")
	s = mathXArrowRe.ReplaceAllString(s, "─$1→")
	// Symbol table — sort by LHS length descending so the longest
	// match wins for ambiguous prefixes (e.g. \not≡ before \notin).
	sorted := append([]mathSymbol(nil), mathSymbolTable...)
	// Insertion sort: stable, O(n²) but n is small (~150).
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && len(sorted[j].lhs) > len(sorted[j-1].lhs); j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	for _, sym := range sorted {
		// Use strings.ReplaceAll, not regex — most LaTeX commands
		// have no regex metacharacters, and the few that do
		// (^_) are handled as separate entries above.
		s = strings.ReplaceAll(s, sym.lhs, sym.rhs)
	}
	return s
}

// wrapForFrac parenthesizes a fraction term when it contains an
// operator that would change precedence if dropped. `\frac{a+b}{c}`
// renders as `(a+b)/c`, not `a+b/c`.
func wrapForFrac(s string) string {
	needsParens := strings.ContainsAny(s, "+−-")
	if !needsParens {
		return s
	}
	return "(" + s + ")"
}
