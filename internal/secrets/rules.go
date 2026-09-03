// Package secrets finds credentials that have been committed to a
// working tree by accident.
//
// Detection is two-layered. Named rules match the shapes that particular
// providers issue -- an AWS access key, a GitHub token, a private key
// header -- and those carry high confidence because the shape is
// unambiguous. A generic assignment rule catches the rest: anything
// assigned to a name like "password" or "api_key" whose value looks
// random. The generic layer is where the false positives live, and every
// finding says which layer produced it so the reader knows how much to
// trust it.
package secrets

import (
	"math"
	"regexp"
	"strings"
)

// Confidence says how much a finding should be trusted, which matters
// more here than in most analyses: a false positive wastes a minute, and
// a missed key is a breach.
type Confidence string

const (
	// ConfidenceHigh is a provider-specific shape that has essentially no
	// other meaning -- an AWS key id, a private key header.
	ConfidenceHigh Confidence = "high"
	// ConfidenceMedium is a provider shape with a looser pattern, or a
	// secret-looking assignment with high entropy.
	ConfidenceMedium Confidence = "medium"
	// ConfidenceLow is a keyword match whose value did not look
	// especially random. Usually a placeholder, occasionally real.
	ConfidenceLow Confidence = "low"
)

// Rule is one named detector.
type Rule struct {
	Name       string
	Pattern    *regexp.Regexp
	Confidence Confidence
	// Group is the submatch holding the secret itself, for redaction. 0
	// means the whole match.
	Group int
	// MinEntropy, when above zero, rejects matches whose captured value
	// is not random enough. It keeps keyword rules from firing on
	// "password = example".
	MinEntropy float64
}

// namedRules match specific credential formats. Each pattern is anchored
// on the parts a provider actually fixes -- the prefix and the length --
// rather than on surrounding syntax, so they fire in any file type.
var namedRules = []Rule{
	{
		Name:       "AWS access key ID",
		Pattern:    regexp.MustCompile(`\b((?:AKIA|ASIA|ABIA|ACCA)[0-9A-Z]{16})\b`),
		Confidence: ConfidenceHigh,
		Group:      1,
	},
	{
		Name:       "AWS secret access key",
		Pattern:    regexp.MustCompile(`(?i)aws[_\-. ]?(?:secret[_\-. ]?)?access[_\-. ]?key[^\n]{0,20}?["'\x60]([A-Za-z0-9/+=]{40})["'\x60]`),
		Confidence: ConfidenceHigh,
		Group:      1,
	},
	{
		Name:       "GitHub token",
		Pattern:    regexp.MustCompile(`\b((?:ghp|gho|ghu|ghs|ghr|github_pat)_[A-Za-z0-9_]{22,})\b`),
		Confidence: ConfidenceHigh,
		Group:      1,
	},
	{
		Name:       "GitLab token",
		Pattern:    regexp.MustCompile(`\b(glpat-[A-Za-z0-9_\-]{20,})\b`),
		Confidence: ConfidenceHigh,
		Group:      1,
	},
	{
		Name:       "Slack token",
		Pattern:    regexp.MustCompile(`\b(xox[baprs]-[A-Za-z0-9\-]{10,})\b`),
		Confidence: ConfidenceHigh,
		Group:      1,
	},
	{
		Name:       "Slack webhook",
		Pattern:    regexp.MustCompile(`(https://hooks\.slack\.com/services/[A-Za-z0-9/+]{20,})`),
		Confidence: ConfidenceHigh,
		Group:      1,
	},
	{
		Name:       "Stripe key",
		Pattern:    regexp.MustCompile(`\b((?:sk|rk)_(?:live|test)_[A-Za-z0-9]{20,})\b`),
		Confidence: ConfidenceHigh,
		Group:      1,
	},
	{
		Name:       "Google API key",
		Pattern:    regexp.MustCompile(`\b(AIza[A-Za-z0-9_\-]{35})\b`),
		Confidence: ConfidenceHigh,
		Group:      1,
	},
	{
		Name:       "OpenAI API key",
		Pattern:    regexp.MustCompile(`\b(sk-(?:proj-)?[A-Za-z0-9_\-]{32,})\b`),
		Confidence: ConfidenceHigh,
		Group:      1,
	},
	{
		Name:       "Anthropic API key",
		Pattern:    regexp.MustCompile(`\b(sk-ant-[A-Za-z0-9_\-]{20,})\b`),
		Confidence: ConfidenceHigh,
		Group:      1,
	},
	{
		Name:       "npm token",
		Pattern:    regexp.MustCompile(`\b(npm_[A-Za-z0-9]{36})\b`),
		Confidence: ConfidenceHigh,
		Group:      1,
	},
	{
		Name:       "SendGrid API key",
		Pattern:    regexp.MustCompile(`\b(SG\.[A-Za-z0-9_\-]{20,}\.[A-Za-z0-9_\-]{20,})\b`),
		Confidence: ConfidenceHigh,
		Group:      1,
	},
	{
		Name:       "Twilio API key",
		Pattern:    regexp.MustCompile(`\b(SK[0-9a-fA-F]{32})\b`),
		Confidence: ConfidenceMedium,
		Group:      1,
	},
	{
		Name:       "private key block",
		Pattern:    regexp.MustCompile(`(-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY(?: BLOCK)?-----)`),
		Confidence: ConfidenceHigh,
		Group:      1,
	},
	{
		Name:       "JSON Web Token",
		Pattern:    regexp.MustCompile(`\b(eyJ[A-Za-z0-9_\-]{10,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,})\b`),
		Confidence: ConfidenceMedium,
		Group:      1,
	},
	{
		Name:       "connection string with password",
		Pattern:    regexp.MustCompile(`(?i)\b((?:postgres|postgresql|mysql|mongodb(?:\+srv)?|redis|amqp)://[^\s:@/]+:([^\s:@/]{3,})@[^\s/]+)`),
		Confidence: ConfidenceHigh,
		Group:      1,
	},
	{
		Name:       "basic auth in URL",
		Pattern:    regexp.MustCompile(`\b(https?://[^\s:@/]+:([^\s:@/]{3,})@[^\s/]+)`),
		Confidence: ConfidenceMedium,
		Group:      1,
	},
}

// genericRule catches an assignment to a secret-sounding name. It is the
// layer that finds credentials no named rule knows about, and also the
// layer that produces nearly every false positive -- hence the entropy
// floor and the placeholder filter.
var genericRule = Rule{
	Name: "secret-looking assignment",
	Pattern: regexp.MustCompile(
		`(?i)\b(?:api[_\-]?key|apikey|secret[_\-]?key|secret|passwd|password|pwd|token|auth[_\-]?token|access[_\-]?token|private[_\-]?key|client[_\-]?secret|credential)\b\s*[:=]\s*["'\x60]([^"'\x60\n]{8,})["'\x60]`),
	Confidence: ConfidenceMedium,
	Group:      1,
	MinEntropy: 3.0,
}

// placeholders are values that appear in documentation and templates.
// Reporting them trains the reader to ignore the tool, which is worse
// than missing them.
var placeholders = []string{
	"example", "changeme", "change_me", "placeholder", "your_", "yourkey",
	"xxxxxx", "aaaaaa", "123456", "password", "secret", "dummy", "sample",
	"redacted", "insert", "todo", "fixme", "notreal", "fake", "test_key",
	"<your", "${", "{{", "%s", "process.env", "os.getenv", "os.environ",
	"getenv(", "npm_token", "abcdef", "foobar", "hunter2", "s3cret",
}

// isPlaceholder reports whether a value is obviously not a real secret.
func isPlaceholder(value string) bool {
	lower := strings.ToLower(value)
	for _, p := range placeholders {
		if strings.Contains(lower, p) {
			return true
		}
	}
	// A value made of one repeated character carries no information.
	if len(lower) > 0 {
		first := lower[0]
		same := true
		for i := range len(lower) {
			if lower[i] != first {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	return false
}

// shannonEntropy returns the bits-per-character entropy of s. Real keys
// sit above 3.5; English words and snake_case identifiers sit below 3.
func shannonEntropy(s string) float64 {
	if s == "" {
		return 0
	}
	var counts [256]int
	for i := range len(s) {
		counts[s[i]]++
	}
	n := float64(len(s))
	entropy := 0.0
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / n
		entropy -= p * math.Log2(p)
	}
	return entropy
}
