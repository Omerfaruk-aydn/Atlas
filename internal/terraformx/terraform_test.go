package terraformx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeTF(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func findTFFinding(t *testing.T, findings []Finding, kind string) *Finding {
	t.Helper()
	for i := range findings {
		if findings[i].Kind == kind {
			return &findings[i]
		}
	}
	return nil
}

func TestScanFlagsOpenIngress(t *testing.T) {
	dir := t.TempDir()
	writeTF(t, dir, "main.tf", `resource "aws_security_group" "web" {
  ingress {
    from_port   = 443
    to_port     = 443
    cidr_blocks = ["0.0.0.0/0"]
  }
}
`)

	got, err := Scan(dir)
	require.NoError(t, err)
	f := findTFFinding(t, got.Findings, "open-ingress")
	require.NotNil(t, f)
	require.Equal(t, "aws_security_group.web", f.Resource)
}

func TestScanIgnoresOpenEgress(t *testing.T) {
	dir := t.TempDir()
	writeTF(t, dir, "main.tf", `resource "aws_security_group" "web" {
  egress {
    from_port   = 0
    to_port     = 0
    cidr_blocks = ["0.0.0.0/0"]
  }
}
`)

	got, err := Scan(dir)
	require.NoError(t, err)
	require.Nil(t, findTFFinding(t, got.Findings, "open-ingress"))
}

func TestScanFlagsHardcodedProviderCredential(t *testing.T) {
	dir := t.TempDir()
	writeTF(t, dir, "main.tf", `provider "aws" {
  access_key = "AKIA4QQJ2XVZ9T6K8H3P"
  secret_key = "wJalrXUtnFEMIK7MDENGbPxRfiCYzKq9Lm2Rst"
}
`)

	got, err := Scan(dir)
	require.NoError(t, err)
	require.Len(t, got.Findings, 2)
	for _, f := range got.Findings {
		require.Equal(t, "hardcoded-credential", f.Kind)
	}
}

func TestScanIgnoresProviderCredentialFromVariable(t *testing.T) {
	dir := t.TempDir()
	writeTF(t, dir, "main.tf", `provider "aws" {
  access_key = "${var.aws_access_key}"
}
`)

	got, err := Scan(dir)
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

func TestScanFlagsSecretVariableDefault(t *testing.T) {
	dir := t.TempDir()
	writeTF(t, dir, "main.tf", `variable "db_password" {
  default = "hunter2-real-secret-value"
}
`)

	got, err := Scan(dir)
	require.NoError(t, err)
	require.NotNil(t, findTFFinding(t, got.Findings, "hardcoded-credential"))
}

func TestScanIgnoresNonSecretVariableDefault(t *testing.T) {
	dir := t.TempDir()
	writeTF(t, dir, "main.tf", `variable "region" {
  default = "us-east-1-long-enough"
}
`)

	got, err := Scan(dir)
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

func TestScanIgnoresAPlaceholderCredential(t *testing.T) {
	dir := t.TempDir()
	writeTF(t, dir, "main.tf", `provider "aws" {
  access_key = "changeme"
}
`)

	got, err := Scan(dir)
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

func TestScanFlagsPublicACL(t *testing.T) {
	dir := t.TempDir()
	writeTF(t, dir, "main.tf", `resource "aws_s3_bucket_acl" "data" {
  acl = "public-read"
}
`)

	got, err := Scan(dir)
	require.NoError(t, err)
	f := findTFFinding(t, got.Findings, "public-acl")
	require.NotNil(t, f)
	require.Equal(t, "aws_s3_bucket_acl.data", f.Resource)
}

func TestScanIgnoresPrivateACL(t *testing.T) {
	dir := t.TempDir()
	writeTF(t, dir, "main.tf", `resource "aws_s3_bucket_acl" "data" {
  acl = "private"
}
`)

	got, err := Scan(dir)
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

func TestScanIgnoresCommentedLines(t *testing.T) {
	dir := t.TempDir()
	writeTF(t, dir, "main.tf", `# access_key = "AKIAIOSFODNN7EXAMPLE1"
resource "aws_security_group" "web" {
  ingress {
    # cidr_blocks = ["0.0.0.0/0"]
    cidr_blocks = ["10.0.0.0/16"]
  }
}
`)

	got, err := Scan(dir)
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

func TestScanSkipsTerraformDir(t *testing.T) {
	dir := t.TempDir()
	writeTF(t, dir, ".terraform/modules/x/main.tf", `provider "aws" {
  access_key = "AKIAIOSFODNN7EXAMPLE1"
}
`)
	writeTF(t, dir, "main.tf", `resource "aws_s3_bucket_acl" "data" {
  acl = "private"
}
`)

	got, err := Scan(dir)
	require.NoError(t, err)
	require.Equal(t, 1, got.FilesScanned)
}

func TestScanReportsNoFiles(t *testing.T) {
	dir := t.TempDir()
	got, err := Scan(dir)
	require.NoError(t, err)
	require.Equal(t, 0, got.FilesScanned)
	require.Empty(t, got.Findings)
}
