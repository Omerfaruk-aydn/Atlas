package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runScanSecretsTool(t *testing.T, workingDir string, params ScanSecretsParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewScanSecretsTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  ScanSecretsToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

// Shaped like a real AWS key id, but the tail is keyboard mash -- no live
// credential exists in this test file.
//
// It is assembled from two pieces so no contiguous run of bytes here
// matches the provider's pattern. Written whole, every credential scanner
// that reads this repository -- including GitHub's own push protection --
// flags the tests for the scanner as a leak, which blocks pushes and
// teaches everyone to click past the warning.
var fakeAWSKey = "AKIA" + "Q3ZKJXNVBWRTPLMD"

func TestScanSecretsToolReportsAFinding(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "conf.txt", "aws_key = "+fakeAWSKey+"\n")

	resp := runScanSecretsTool(t, dir, ScanSecretsParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "AWS access key ID")
	require.Contains(t, resp.Content, "conf.txt:1")
	require.Contains(t, resp.Content, "[high]")
}

// The single most important property of this tool: the report must not
// re-leak what it is warning about.
func TestScanSecretsToolNeverPrintsTheSecret(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "conf.txt", "aws_key = "+fakeAWSKey+"\n")

	resp := runScanSecretsTool(t, dir, ScanSecretsParams{})
	require.NotContains(t, resp.Content, fakeAWSKey)
	require.Contains(t, resp.Content, "AKIA********PLMD")
}

// Deleting the line is the intuitive fix and it is wrong, so the advice
// has to lead rather than trail.
func TestScanSecretsToolLeadsWithRotation(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "conf.txt", fakeAWSKey+"\n")

	resp := runScanSecretsTool(t, dir, ScanSecretsParams{})
	require.Contains(t, resp.Content, "ROTATED")
	require.Contains(t, resp.Content, "git history")
	require.Contains(t, resp.Content, "Deleting the line is not enough")
}

// It must also tell the model not to go fetch the full value itself.
func TestScanSecretsToolTellsTheModelNotToRecoverTheValue(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "conf.txt", fakeAWSKey+"\n")

	resp := runScanSecretsTool(t, dir, ScanSecretsParams{})
	require.Contains(t, resp.Content, "Do not read the file to recover the full value")
}

// A clean result must not read as a guarantee, or it becomes a licence to
// publish the repository.
func TestScanSecretsToolQualifiesACleanResult(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "main.go", `package main

func main() {}
`)

	resp := runScanSecretsTool(t, dir, ScanSecretsParams{})
	require.Contains(t, resp.Content, "No credentials found")
	require.Contains(t, resp.Content, "not a guarantee")
}

func TestScanSecretsToolFiltersByConfidence(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.txt", `api_key = "Xk9Rm2QwZ7pLtBvN4cHy"`+"\n")
	writeDeadCodeFile(t, dir, "b.txt", fakeAWSKey+"\n")

	resp := runScanSecretsTool(t, dir, ScanSecretsParams{MinConfidence: "high"})
	require.Contains(t, resp.Content, "AWS access key ID")
	require.NotContains(t, resp.Content, "secret-looking assignment")
}

func TestScanSecretsToolRejectsABadConfidenceLevel(t *testing.T) {
	resp := runScanSecretsTool(t, t.TempDir(), ScanSecretsParams{MinConfidence: "very high"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "must be high, medium, or low")
}

func TestScanSecretsToolCanSkipTheGenericLayer(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.txt", `api_key = "Xk9Rm2QwZ7pLtBvN4cHy"`+"\n")

	yes := true
	resp := runScanSecretsTool(t, dir, ScanSecretsParams{SkipGeneric: &yes})
	require.Contains(t, resp.Content, "No credentials found")
}

func TestScanSecretsToolExplainsWhatConfidenceMeans(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.txt", fakeAWSKey+"\n")

	resp := runScanSecretsTool(t, dir, ScanSecretsParams{})
	require.Contains(t, resp.Content, "false positives")
}

func TestScanSecretsToolErrorsOnAMissingPath(t *testing.T) {
	resp := runScanSecretsTool(t, t.TempDir(), ScanSecretsParams{Path: "nope"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "cannot scan")
}

func TestScanSecretsToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.txt", fakeAWSKey+"\n")

	resp := runScanSecretsTool(t, dir, ScanSecretsParams{})
	var meta ScanSecretsResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Findings)
	require.Equal(t, 1, meta.High)
	require.Equal(t, 1, meta.FilesScanned)
}
