package codeintel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeEnvFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func findUsage(t *testing.T, usages []EnvVarUsage, name string) *EnvVarUsage {
	t.Helper()
	for i := range usages {
		if usages[i].Name == name {
			return &usages[i]
		}
	}
	return nil
}

func TestAuditEnvVarsFindsGetenv(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, "a.go", `package a

import "os"

func f() string {
	return os.Getenv("API_KEY")
}
`)

	got, err := AuditEnvVars(dir, false)
	require.NoError(t, err)
	u := findUsage(t, got.Usages, "API_KEY")
	require.NotNil(t, u)
	require.Equal(t, "read", u.Kind)
}

func TestAuditEnvVarsFindsLookupEnvAndSetenv(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, "a.go", `package a

import "os"

func f() {
	os.LookupEnv("DEBUG")
	os.Setenv("PORT", "8080")
}
`)

	got, err := AuditEnvVars(dir, false)
	require.NoError(t, err)
	require.Equal(t, "read", findUsage(t, got.Usages, "DEBUG").Kind)
	require.Equal(t, "write", findUsage(t, got.Usages, "PORT").Kind)
}

func TestAuditEnvVarsHandlesDynamicKeys(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, "a.go", `package a

import "os"

func f(key string) {
	os.Getenv(key)
}
`)

	got, err := AuditEnvVars(dir, false)
	require.NoError(t, err)
	require.NotNil(t, findUsage(t, got.Usages, "(dynamic)"))
}

func TestAuditEnvVarsIgnoresUnrelatedSelectorCalls(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, "a.go", `package a

import "strings"

func f() {
	strings.Getenv("nope")
}
`)

	got, err := AuditEnvVars(dir, false)
	require.NoError(t, err)
	require.Empty(t, got.Usages)
}

func TestAuditEnvVarsReportsNoEnvFile(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, "a.go", `package a

import "os"

func f() { os.Getenv("API_KEY") }
`)

	got, err := AuditEnvVars(dir, false)
	require.NoError(t, err)
	require.False(t, got.EnvFileFound)
	require.Empty(t, got.Undocumented)
}

func TestAuditEnvVarsFindsUndocumentedVars(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, "a.go", `package a

import "os"

func f() {
	os.Getenv("API_KEY")
	os.Getenv("DATABASE_URL")
}
`)
	writeEnvFile(t, dir, ".env.example", "API_KEY=changeme\n")

	got, err := AuditEnvVars(dir, false)
	require.NoError(t, err)
	require.True(t, got.EnvFileFound)
	require.Equal(t, []string{"DATABASE_URL"}, got.Undocumented)
}

func TestAuditEnvVarsAllDocumentedIsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, "a.go", `package a

import "os"

func f() { os.Getenv("API_KEY") }
`)
	writeEnvFile(t, dir, ".env.example", "API_KEY=changeme\n")

	got, err := AuditEnvVars(dir, false)
	require.NoError(t, err)
	require.Empty(t, got.Undocumented)
}

func TestAuditEnvVarsIgnoresDynamicKeysInUndocumentedList(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, "a.go", `package a

import "os"

func f(key string) { os.Getenv(key) }
`)
	writeEnvFile(t, dir, ".env.example", "SOMETHING=x\n")

	got, err := AuditEnvVars(dir, false)
	require.NoError(t, err)
	require.Empty(t, got.Undocumented)
}

func TestAuditEnvVarsSkipsTestFilesByDefault(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, "a_test.go", `package a

import "os"

func f() { os.Getenv("API_KEY") }
`)

	got, err := AuditEnvVars(dir, false)
	require.NoError(t, err)
	require.Equal(t, 0, got.FilesScanned)
}

func TestAuditEnvVarsFallsBackToSampleFile(t *testing.T) {
	dir := t.TempDir()
	writeEnvFile(t, dir, "a.go", `package a

import "os"

func f() { os.Getenv("API_KEY") }
`)
	writeEnvFile(t, dir, ".env.sample", "API_KEY=x\n")

	got, err := AuditEnvVars(dir, false)
	require.NoError(t, err)
	require.True(t, got.EnvFileFound)
	require.Contains(t, got.EnvFilePath, ".env.sample")
}
