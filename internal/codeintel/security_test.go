package codeintel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeSecurityFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func findSecurityKind(t *testing.T, findings []SecurityFinding, kind string) *SecurityFinding {
	t.Helper()
	for i := range findings {
		if findings[i].Kind == kind {
			return &findings[i]
		}
	}
	return nil
}

func TestSecurityScanFlagsAHardcodedCredentialAssignment(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "a.go", `package a

func connect() {
	password := "hunter2-real-secret"
	_ = password
}
`)

	got, err := SecurityScan(dir, SecurityScanOptions{})
	require.NoError(t, err)
	f := findSecurityKind(t, got.Findings, "hardcoded-credential")
	require.NotNil(t, f)
	require.Equal(t, "connect", f.Func)
}

func TestSecurityScanIgnoresAPlaceholderCredential(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "a.go", `package a

func connect() {
	password := "changeme"
	_ = password
}
`)

	got, err := SecurityScan(dir, SecurityScanOptions{})
	require.NoError(t, err)
	require.Nil(t, findSecurityKind(t, got.Findings, "hardcoded-credential"))
}

func TestSecurityScanIgnoresAShortCredentialLiteral(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "a.go", `package a

func connect() {
	token := "abc"
	_ = token
}
`)

	got, err := SecurityScan(dir, SecurityScanOptions{})
	require.NoError(t, err)
	require.Nil(t, findSecurityKind(t, got.Findings, "hardcoded-credential"))
}

func TestSecurityScanFlagsAHardcodedCredentialConst(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "a.go", `package a

const apiKey = "sk-live-abcdefgh12345678"
`)

	got, err := SecurityScan(dir, SecurityScanOptions{})
	require.NoError(t, err)
	require.NotNil(t, findSecurityKind(t, got.Findings, "hardcoded-credential"))
}

func TestSecurityScanFlagsWeakCryptoImport(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "a.go", `package a

import "crypto/md5"

func hash(b []byte) [16]byte {
	return md5.Sum(b)
}
`)

	got, err := SecurityScan(dir, SecurityScanOptions{})
	require.NoError(t, err)
	f := findSecurityKind(t, got.Findings, "weak-crypto")
	require.NotNil(t, f)
	require.Contains(t, f.Message, "crypto/md5")
}

func TestSecurityScanAcceptsStrongCrypto(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "a.go", `package a

import "crypto/sha256"

func hash(b []byte) [32]byte {
	return sha256.Sum256(b)
}
`)

	got, err := SecurityScan(dir, SecurityScanOptions{})
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

func TestSecurityScanFlagsInsecureTLS(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "a.go", `package a

import "crypto/tls"

func cfg() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true}
}
`)

	got, err := SecurityScan(dir, SecurityScanOptions{})
	require.NoError(t, err)
	require.NotNil(t, findSecurityKind(t, got.Findings, "insecure-tls"))
}

func TestSecurityScanAcceptsTLSWithVerificationOn(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "a.go", `package a

import "crypto/tls"

func cfg() *tls.Config {
	return &tls.Config{InsecureSkipVerify: false}
}
`)

	got, err := SecurityScan(dir, SecurityScanOptions{})
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

func TestSecurityScanFlagsSQLBuiltWithSprintf(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "a.go", `package a

import (
	"database/sql"
	"fmt"
)

func lookup(db *sql.DB, name string) {
	db.Query(fmt.Sprintf("SELECT * FROM users WHERE name = '%s'", name))
}
`)

	got, err := SecurityScan(dir, SecurityScanOptions{})
	require.NoError(t, err)
	f := findSecurityKind(t, got.Findings, "sql-injection-risk")
	require.NotNil(t, f)
	require.Equal(t, "lookup", f.Func)
}

func TestSecurityScanAcceptsParameterizedSQL(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "a.go", `package a

import "database/sql"

func lookup(db *sql.DB, name string) {
	db.Query("SELECT * FROM users WHERE name = ?", name)
}
`)

	got, err := SecurityScan(dir, SecurityScanOptions{})
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

func TestSecurityScanFlagsCommandBuiltWithSprintf(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "a.go", `package a

import (
	"fmt"
	"os/exec"
)

func run(userInput string) {
	exec.Command("sh", "-c", fmt.Sprintf("echo %s", userInput))
}
`)

	got, err := SecurityScan(dir, SecurityScanOptions{})
	require.NoError(t, err)
	require.NotNil(t, findSecurityKind(t, got.Findings, "command-injection-risk"))
}

func TestSecurityScanAcceptsCommandWithSeparateArgs(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "a.go", `package a

import "os/exec"

func run(userInput string) {
	exec.Command("echo", userInput)
}
`)

	got, err := SecurityScan(dir, SecurityScanOptions{})
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

func TestSecurityScanSkipsTestFilesByDefault(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "a_test.go", `package a

func TestX() {
	password := "hunter2-real-secret"
	_ = password
}
`)

	got, err := SecurityScan(dir, SecurityScanOptions{})
	require.NoError(t, err)
	require.Equal(t, 0, got.FilesScanned)
}

func TestSecurityScanIncludesTestFilesWhenAsked(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "a_test.go", `package a

func TestX() {
	password := "hunter2-real-secret"
	_ = password
}
`)

	got, err := SecurityScan(dir, SecurityScanOptions{IncludeTests: true})
	require.NoError(t, err)
	require.NotNil(t, findSecurityKind(t, got.Findings, "hardcoded-credential"))
}

func TestSecurityScanCountsByKind(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "a.go", `package a

const apiKey = "sk-live-abcdefgh12345678"
const apiKey2 = "sk-live-zyxwvutsrqponml"
`)

	got, err := SecurityScan(dir, SecurityScanOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, got.ByKind["hardcoded-credential"])
}

func TestSecurityScanSkipsUnparsableFiles(t *testing.T) {
	dir := t.TempDir()
	writeSecurityFile(t, dir, "broken.go", "not valid go (((")

	got, err := SecurityScan(dir, SecurityScanOptions{})
	require.NoError(t, err)
	require.Equal(t, 0, got.FilesScanned)
	require.Empty(t, got.Findings)
}
