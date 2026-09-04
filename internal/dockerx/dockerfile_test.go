package dockerx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeDockerfile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "Dockerfile")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func findFinding(t *testing.T, findings []Finding, kind string) *Finding {
	t.Helper()
	for i := range findings {
		if findings[i].Kind == kind {
			return &findings[i]
		}
	}
	return nil
}

func TestParseReadsAStage(t *testing.T) {
	path := writeDockerfile(t, "FROM golang:1.22\nRUN go build ./...\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.Len(t, got.Stages, 1)
	require.Equal(t, "golang:1.22", got.Stages[0].BaseImage)
	require.Len(t, got.Stages[0].Instructions, 1)
	require.Equal(t, "RUN", got.Stages[0].Instructions[0].Directive)
}

func TestParseReadsMultipleStages(t *testing.T) {
	path := writeDockerfile(t, "FROM golang:1.22 AS builder\nRUN go build -o app .\n\nFROM alpine:3.19\nCOPY --from=builder /app /app\nUSER nonroot\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.Len(t, got.Stages, 2)
	require.Equal(t, "builder", got.Stages[0].Name)
	require.Equal(t, "alpine:3.19", got.Stages[1].BaseImage)
}

func TestParseJoinsLineContinuations(t *testing.T) {
	path := writeDockerfile(t, "FROM alpine\nRUN apk add --no-cache \\\n    curl \\\n    git\nUSER app\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.Len(t, got.Stages[0].Instructions, 2)
	require.Equal(t, "RUN", got.Stages[0].Instructions[0].Directive)
	require.Contains(t, got.Stages[0].Instructions[0].Args, "curl")
	require.Contains(t, got.Stages[0].Instructions[0].Args, "git")
}

func TestParseFlagsNoTag(t *testing.T) {
	path := writeDockerfile(t, "FROM alpine\nUSER app\n")

	got, err := Parse(path)
	require.NoError(t, err)
	f := findFinding(t, got.Findings, "latest-tag")
	require.NotNil(t, f)
}

func TestParseFlagsExplicitLatestTag(t *testing.T) {
	path := writeDockerfile(t, "FROM alpine:latest\nUSER app\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.NotNil(t, findFinding(t, got.Findings, "latest-tag"))
}

func TestParseAcceptsAPinnedTag(t *testing.T) {
	path := writeDockerfile(t, "FROM alpine:3.19\nUSER app\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.Nil(t, findFinding(t, got.Findings, "latest-tag"))
}

func TestParseAcceptsADigestPin(t *testing.T) {
	path := writeDockerfile(t, "FROM alpine@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234\nUSER app\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.Nil(t, findFinding(t, got.Findings, "latest-tag"))
}

func TestParseDoesNotFlagAnIntraStageReference(t *testing.T) {
	path := writeDockerfile(t, "FROM golang:1.22 AS builder\nRUN go build -o app .\n\nFROM builder\nUSER app\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.Nil(t, findFinding(t, got.Findings, "latest-tag"))
}

func TestParseFlagsMissingUser(t *testing.T) {
	path := writeDockerfile(t, "FROM alpine:3.19\nRUN echo hi\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.NotNil(t, findFinding(t, got.Findings, "root-user"))
}

func TestParseAcceptsAUserInstruction(t *testing.T) {
	path := writeDockerfile(t, "FROM alpine:3.19\nUSER nonroot\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.Nil(t, findFinding(t, got.Findings, "root-user"))
}

func TestParseFlagsAddForLocalCopy(t *testing.T) {
	path := writeDockerfile(t, "FROM alpine:3.19\nADD app.bin /usr/local/bin/app\nUSER app\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.NotNil(t, findFinding(t, got.Findings, "add-for-local-copy"))
}

func TestParseAcceptsAddForURL(t *testing.T) {
	path := writeDockerfile(t, "FROM alpine:3.19\nADD https://example.com/app.bin /usr/local/bin/app\nUSER app\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.Nil(t, findFinding(t, got.Findings, "add-for-local-copy"))
}

func TestParseAcceptsAddForArchive(t *testing.T) {
	path := writeDockerfile(t, "FROM alpine:3.19\nADD bundle.tar.gz /opt/\nUSER app\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.Nil(t, findFinding(t, got.Findings, "add-for-local-copy"))
}

func TestParseFlagsSecretInEnv(t *testing.T) {
	path := writeDockerfile(t, "FROM alpine:3.19\nENV API_KEY=sk-live-abcdefgh12345678\nUSER app\n")

	got, err := Parse(path)
	require.NoError(t, err)
	f := findFinding(t, got.Findings, "secret-in-env")
	require.NotNil(t, f)
	require.Contains(t, f.Message, "API_KEY")
}

func TestParseFlagsSecretInArg(t *testing.T) {
	path := writeDockerfile(t, "FROM alpine:3.19\nARG DB_PASSWORD=hunter2-real-secret\nUSER app\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.NotNil(t, findFinding(t, got.Findings, "secret-in-env"))
}

func TestParseIgnoresAPlaceholderSecretValue(t *testing.T) {
	path := writeDockerfile(t, "FROM alpine:3.19\nENV API_KEY=changeme\nUSER app\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.Nil(t, findFinding(t, got.Findings, "secret-in-env"))
}

func TestParseIgnoresNonSecretEnv(t *testing.T) {
	path := writeDockerfile(t, "FROM alpine:3.19\nENV PORT=8080\nUSER app\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.Nil(t, findFinding(t, got.Findings, "secret-in-env"))
}

func TestParseIgnoresCommentsAndBlankLines(t *testing.T) {
	path := writeDockerfile(t, "# a comment\nFROM alpine:3.19\n\nUSER app\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.Len(t, got.Stages[0].Instructions, 1)
}

func TestParseReportsErrorForMissingFile(t *testing.T) {
	_, err := Parse(filepath.Join(t.TempDir(), "nope"))
	require.Error(t, err)
}
