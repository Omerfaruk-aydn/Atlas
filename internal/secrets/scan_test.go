package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The fixtures below are shaped exactly like real credentials -- that is
// the only way to test detectors that match on shape -- but none of them
// is a live secret; the random halves are keyboard mash.
//
// They are assembled from pieces rather than written whole so that no
// contiguous run of bytes in this file matches a provider's pattern.
// Otherwise every credential scanner that ever reads this repository,
// including GitHub's own push protection, flags the test suite for the
// scanner as a leak -- which blocks pushes and trains everyone to click
// past the warning.
var (
	fakeAWSKey      = "AKIA" + "Q3ZKJXNVBWRTPLMD"
	fakeGitHubToken = "ghp" + "_R7kQm2WvLdXpYb4TnZaCsEuFgHj3Ki9M"
	fakeGenericKey  = "Xk9Rm2QwZ7pLtBvN4cHy"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func ruleNames(r Result) []string {
	out := make([]string, 0, len(r.Findings))
	for _, f := range r.Findings {
		out = append(out, f.Rule)
	}
	return out
}

func TestScanFindsAnAWSAccessKeyID(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "conf.txt", "aws_key = "+fakeAWSKey+"\n")

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.Contains(t, ruleNames(got), "AWS access key ID")
	require.Equal(t, ConfidenceHigh, got.Findings[0].Confidence)
}

func TestScanFindsAGitHubToken(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.env", "GH="+fakeGitHubToken+"\n")

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.Contains(t, ruleNames(got), "GitHub token")
}

func TestScanFindsAPrivateKeyHeader(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "id_rsa", "-----BEGIN RSA PRIVATE KEY-----\nZm9vYmFy\n")

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.Contains(t, ruleNames(got), "private key block")
}

func TestScanFindsAPasswordInAConnectionString(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "db.yml", "url: postgres://admin:Rk4mZq7Xw2@db.internal:5432/app\n")

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.Contains(t, ruleNames(got), "connection string with password")
}

// The whole point of a redacting scanner: a report that quoted the key
// would put it into a transcript, a log, and probably a bug tracker.
func TestScanNeverReturnsTheSecretItself(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "conf.txt", "aws_key = "+fakeAWSKey+"\n")

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.NotEmpty(t, got.Findings)
	for _, f := range got.Findings {
		require.NotContains(t, f.Redacted, fakeAWSKey)
		require.NotContains(t, f.Context, fakeAWSKey)
		require.Contains(t, f.Redacted, "*")
	}
}

// Enough has to survive redaction to identify which credential to rotate.
func TestRedactKeepsTheEndsAndHidesTheMiddle(t *testing.T) {
	require.Equal(t, "AKIA********PLMD", Redact(fakeAWSKey))
	// A short value gives up nothing at all.
	require.Equal(t, "*****", Redact("short"))
}

func TestScanFindsAGenericSecretAssignment(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "cfg.py", `API_KEY = "`+fakeGenericKey+`"`+"\n")

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.Contains(t, ruleNames(got), "secret-looking assignment")
}

// Reporting documentation placeholders trains the reader to ignore the
// tool, which is worse than missing them.
func TestScanIgnoresPlaceholders(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "README.md", strings.Join([]string{
		`password = "changeme"`,
		`api_key = "your_api_key_here"`,
		`token = "xxxxxxxxxxxxxxxx"`,
		`secret = "${SECRET_FROM_ENV}"`,
		`api_key = os.getenv("API_KEY")`,
		`password = "example-password"`,
	}, "\n"))

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

// A low-entropy value under a secret-sounding name is nearly always
// prose, not a key.
func TestScanRejectsLowEntropyAssignments(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.go", `const password = "aaaaaaaaaaaa"`+"\n")

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

func TestScanHonoursAnAllowComment(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "fixture.go", `const k = "`+fakeAWSKey+`" // atlas:allow-secret`+"\n")

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

// A key in node_modules is not this repository's leak, and reporting it
// buries the ones that are.
func TestScanSkipsDependencyAndBuildTrees(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "node_modules/pkg/a.js", `var k = "`+fakeGitHubToken+`"`+"\n")
	write(t, dir, "vendor/x/b.go", `var k = "`+fakeGitHubToken+`"`+"\n")
	write(t, dir, ".git/config", "token = "+fakeGitHubToken+"\n")

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

func TestScanSkipsBinaryExtensions(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "logo.png", fakeGitHubToken)

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

func TestScanSkipsOversizedFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "big.txt", strings.Repeat("x", 300)+"\n"+fakeGitHubToken+"\n")

	got, err := Scan(dir, Options{MaxFileSize: 100})
	require.NoError(t, err)
	require.Empty(t, got.Findings)
	require.Equal(t, 1, got.FilesSkipped)
}

func TestScanReportsHighConfidenceFirst(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.txt", `api_key = "`+fakeGenericKey+`"`+"\n")
	write(t, dir, "b.txt", fakeAWSKey+"\n")

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(got.Findings), 2)
	require.Equal(t, ConfidenceHigh, got.Findings[0].Confidence)
}

func TestScanFiltersByMinConfidence(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.txt", `api_key = "`+fakeGenericKey+`"`+"\n")
	write(t, dir, "b.txt", fakeAWSKey+"\n")

	got, err := Scan(dir, Options{MinConfidence: ConfidenceHigh})
	require.NoError(t, err)
	require.Len(t, got.Findings, 1)
	require.Equal(t, "AWS access key ID", got.Findings[0].Rule)
}

func TestScanCanSkipTheGenericLayer(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.txt", `api_key = "`+fakeGenericKey+`"`+"\n")

	got, err := Scan(dir, Options{SkipGeneric: true})
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

func TestScanStopsAtTheFindingLimit(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for range 50 {
		b.WriteString(fakeAWSKey + "\n")
	}
	write(t, dir, "a.txt", b.String())
	write(t, dir, "b.txt", fakeGitHubToken+"\n")

	got, err := Scan(dir, Options{MaxFindings: 5})
	require.NoError(t, err)
	require.Len(t, got.Findings, 5)
	require.True(t, got.Truncated)
}

func TestScanRecordsFileAndLine(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "a.txt", "line one\nline two\n"+fakeAWSKey+"\n")

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.Len(t, got.Findings, 1)
	require.Equal(t, path, got.Findings[0].File)
	require.Equal(t, 3, got.Findings[0].Line)
}

func TestScanAcceptsASingleFile(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "a.txt", fakeAWSKey+"\n")

	got, err := Scan(path, Options{})
	require.NoError(t, err)
	require.Len(t, got.Findings, 1)
	require.Equal(t, 1, got.FilesScanned)
}

func TestScanFailsClearlyOnAMissingPath(t *testing.T) {
	_, err := Scan(filepath.Join(t.TempDir(), "nope"), Options{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot scan")
}

func TestScanFindsNothingInACleanTree(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "main.go", `package main

import "fmt"

func main() { fmt.Println("hello") }
`)

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.Empty(t, got.Findings)
	require.Equal(t, 1, got.FilesScanned)
}

func TestShannonEntropySeparatesRandomFromWords(t *testing.T) {
	require.Less(t, shannonEntropy("password"), 3.0)
	require.Greater(t, shannonEntropy(fakeGenericKey), 3.5)
	require.Zero(t, shannonEntropy(""))
}

// A minified bundle on one line produces nothing but noise and is slow to
// run every rule against.
func TestScanSkipsAbsurdlyLongLines(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "bundle.js", strings.Repeat("a", 5000)+fakeAWSKey+"\n")

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

func TestScanDedupesTheSameSecretOnOneLine(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.txt", fakeAWSKey+" "+fakeAWSKey+"\n")

	got, err := Scan(dir, Options{})
	require.NoError(t, err)
	require.Len(t, got.Findings, 1)
}
