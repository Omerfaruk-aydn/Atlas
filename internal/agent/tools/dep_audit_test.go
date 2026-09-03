package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/depaudit"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runDepAuditTool(t *testing.T, workingDir string, params DepAuditParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewDepAuditTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  DepAuditToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func auditFixtureModule(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/auditmod\n\ngo 1.24\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"),
		[]byte("package a\n"), 0o644))
	return dir
}

func TestDepAuditToolReportsTheModuleGraph(t *testing.T) {
	dir := auditFixtureModule(t)

	yes := true
	resp := runDepAuditTool(t, dir, DepAuditParams{SkipUpdates: &yes})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "module(s) in the dependency graph")
}

// The single most important property: a missing scanner must never be
// presentable as a clean result.
func TestDepAuditToolNeverPassesOffNotCheckedAsClean(t *testing.T) {
	if _, err := exec.LookPath("govulncheck"); err == nil {
		t.Skip("govulncheck is installed, so this path does not apply here")
	}
	dir := auditFixtureModule(t)

	yes := true
	resp := runDepAuditTool(t, dir, DepAuditParams{SkipUpdates: &yes})
	require.Contains(t, resp.Content, "NOT CHECKED")
	require.Contains(t, resp.Content, "not the same as")
	require.Contains(t, resp.Content, "Do not report this project as clean")
	require.NotContains(t, resp.Content, "No known vulnerabilities")

	var meta DepAuditResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.False(t, meta.VulnsChecked)
}

func TestDepAuditToolRejectsSkippingEverything(t *testing.T) {
	yes := true
	resp := runDepAuditTool(t, t.TempDir(), DepAuditParams{SkipUpdates: &yes, SkipVulns: &yes})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "nothing to audit")
}

func TestDepAuditToolRejectsABadTimeout(t *testing.T) {
	resp := runDepAuditTool(t, t.TempDir(), DepAuditParams{Timeout: "soonish"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "not a positive duration")
}

func TestDepAuditToolErrorsOutsideAModule(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	yes := true
	resp := runDepAuditTool(t, t.TempDir(), DepAuditParams{SkipUpdates: &yes, SkipVulns: &yes})
	require.True(t, resp.IsError)
}

// Formatting is exercised against constructed results, because producing
// a genuinely vulnerable module in a test would mean depending on one.

func TestFormatAuditSeparatesReachableFromPresent(t *testing.T) {
	r := depaudit.Result{
		VulnToolAvailable: true,
		Vulnerabilities: []depaudit.Vulnerability{
			{
				ID: "GO-2024-0001", Aliases: []string{"CVE-2024-1111"},
				Summary: "Reachable problem", Module: "example.com/vuln",
				Found: "v1.0.0", Fixed: "v1.2.3",
				Called: true, Trace: "example.com/vuln/pkg.Exploitable",
			},
			{
				ID: "GO-2024-0002", Summary: "Present only",
				Module: "example.com/other", Found: "v1.0.0", Fixed: "v2.0.0",
			},
		},
	}

	got := formatAudit(r, true, false)
	require.Contains(t, got, "REACHABLE")
	require.Contains(t, got, "example.com/vuln/pkg.Exploitable")
	require.Contains(t, got, "Present but not called")
	require.Contains(t, got, "not urgent")
	require.Contains(t, got, "1 reach code this module actually calls")
	// The reachable one must be listed before the merely present one.
	require.Less(t, indexOf(got, "GO-2024-0001"), indexOf(got, "GO-2024-0002"))
}

// Reporting a vulnerability as "fixed in " with an empty version would
// send someone chasing a release that does not exist.
func TestFormatAuditSaysWhenThereIsNoFix(t *testing.T) {
	r := depaudit.Result{
		VulnToolAvailable: true,
		Vulnerabilities: []depaudit.Vulnerability{
			{ID: "GO-2024-0003", Module: "example.com/x", Found: "v1.0.0", Called: true},
		},
	}

	got := formatAudit(r, true, false)
	require.Contains(t, got, "no fixed version published")
	require.NotContains(t, got, "fixed in \n")
}

func TestFormatAuditReportsACleanScan(t *testing.T) {
	r := depaudit.Result{VulnToolAvailable: true}

	got := formatAudit(r, true, false)
	require.Contains(t, got, "No known vulnerabilities")
	require.NotContains(t, got, "NOT CHECKED")
}

// An indirect dependency usually moves when its parent does, so it is
// not separately actionable and must not crowd out the direct ones.
func TestFormatAuditPutsDirectDependenciesFirst(t *testing.T) {
	r := depaudit.Result{
		Modules: []depaudit.Module{
			{Path: "example.com/main", Main: true},
			{Path: "example.com/indirect", Version: "v1.0.0", Update: "v1.1.0", Indirect: true},
			{Path: "example.com/direct", Version: "v1.0.0", Update: "v2.0.0"},
		},
	}

	got := formatAudit(r, false, true)
	require.Contains(t, got, "2 dependency(ies) behind (1 direct, 1 indirect)")
	require.Less(t, indexOf(got, "example.com/direct"), indexOf(got, "example.com/indirect"))
	require.NotContains(t, got, "example.com/main  ")
	require.Contains(t, got, "not by itself a reason to upgrade")
}

func TestFormatAuditReportsEverythingUpToDate(t *testing.T) {
	r := depaudit.Result{Modules: []depaudit.Module{
		{Path: "example.com/dep", Version: "v1.0.0"},
	}}

	got := formatAudit(r, false, true)
	require.Contains(t, got, "All dependencies are at their latest versions")
}
