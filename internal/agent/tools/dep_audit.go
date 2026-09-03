package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/depaudit"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
)

const DepAuditToolName = "dep_audit"

//go:embed dep_audit.md
var depAuditDescription string

const (
	defaultAuditTimeout = 2 * time.Minute
	maxAuditTimeout     = 15 * time.Minute
	// maxOutdatedShown bounds the outdated list. A long-untouched
	// project has hundreds of indirect dependencies behind by a patch
	// release, and printing them all buries the direct ones that matter.
	maxOutdatedShown = 25
)

type DepAuditParams struct {
	Dir         string `json:"dir,omitempty" description:"A directory inside the module. Defaults to the working directory."`
	SkipUpdates *bool  `json:"skip_updates,omitempty" description:"Skip the outdated-dependency check, which reaches the network and is the slower half."`
	SkipVulns   *bool  `json:"skip_vulns,omitempty" description:"Skip the vulnerability scan and only report what is out of date."`
	Timeout     string `json:"timeout,omitempty" description:"How long each command may take, as a Go duration. Default 2m."`
}

type DepAuditResponseMetadata struct {
	Vulnerabilities int  `json:"vulnerabilities"`
	Reachable       int  `json:"reachable"`
	Outdated        int  `json:"outdated"`
	Modules         int  `json:"modules"`
	VulnsChecked    bool `json:"vulns_checked"`
}

func NewDepAuditTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		DepAuditToolName,
		depAuditDescription,
		func(ctx context.Context, params DepAuditParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			timeout := defaultAuditTimeout
			if params.Timeout != "" {
				parsed, err := time.ParseDuration(params.Timeout)
				if err != nil || parsed <= 0 {
					return fantasy.NewTextErrorResponse(
						fmt.Sprintf("timeout %q is not a positive duration (try '2m')", params.Timeout)), nil
				}
				timeout = min(parsed, maxAuditTimeout)
			}

			skipUpdates := params.SkipUpdates != nil && *params.SkipUpdates
			skipVulns := params.SkipVulns != nil && *params.SkipVulns
			if skipUpdates && skipVulns {
				return fantasy.NewTextErrorResponse(
					"skip_updates and skip_vulns cannot both be set -- that leaves nothing to audit"), nil
			}

			dir := cmp.Or(params.Dir, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			result, err := depaudit.Run(ctx, dir, depaudit.Options{
				Timeout:     timeout,
				SkipUpdates: skipUpdates,
				SkipVulns:   skipVulns,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatAudit(result, skipUpdates, skipVulns)),
				DepAuditResponseMetadata{
					Vulnerabilities: len(result.Vulnerabilities),
					Reachable:       result.Called(),
					Outdated:        len(result.Outdated()),
					Modules:         len(result.Modules),
					VulnsChecked:    result.VulnToolAvailable,
				},
			), nil
		},
	)
}

func formatAudit(r depaudit.Result, skipUpdates, skipVulns bool) string {
	var b strings.Builder

	if !skipVulns {
		writeVulnerabilities(&b, r)
	}
	if !skipUpdates {
		writeOutdated(&b, r)
	}

	fmt.Fprintf(&b, "\n%d module(s) in the dependency graph.\n", len(r.Modules))
	return b.String()
}

func writeVulnerabilities(b *strings.Builder, r depaudit.Result) {
	// "Not checked" must never be presentable as "none found". This is
	// the one place in this tool where the wrong wording turns a missing
	// tool into a false clean bill of health.
	if !r.VulnToolAvailable {
		b.WriteString("VULNERABILITIES: NOT CHECKED.\n")
		if r.VulnToolError != "" {
			fmt.Fprintf(b, "  %s\n", r.VulnToolError)
		}
		b.WriteString("  This is not the same as \"no vulnerabilities\" -- nothing was scanned. Do not report this project as clean.\n")
		return
	}

	if len(r.Vulnerabilities) == 0 {
		b.WriteString("No known vulnerabilities affect this module.\n")
		return
	}

	reachable := r.Called()
	fmt.Fprintf(b, "%d advisory(ies) affect this module's dependencies; %d reach code this module actually calls.\n",
		len(r.Vulnerabilities), reachable)

	if reachable > 0 {
		b.WriteString("\nREACHABLE -- the affected code is called from here:\n")
		for _, v := range r.Vulnerabilities {
			if !v.Called {
				continue
			}
			writeVuln(b, v)
		}
	}

	if reachable < len(r.Vulnerabilities) {
		b.WriteString("\nPresent but not called -- worth upgrading, not urgent:\n")
		for _, v := range r.Vulnerabilities {
			if v.Called {
				continue
			}
			writeVuln(b, v)
		}
	}
}

func writeVuln(b *strings.Builder, v depaudit.Vulnerability) {
	fmt.Fprintf(b, "  %s", v.ID)
	if len(v.Aliases) > 0 {
		fmt.Fprintf(b, " (%s)", strings.Join(v.Aliases, ", "))
	}
	b.WriteString("\n")
	if v.Summary != "" {
		fmt.Fprintf(b, "    %s\n", v.Summary)
	}
	if v.Module != "" {
		fmt.Fprintf(b, "    %s %s", v.Module, v.Found)
		if v.Fixed != "" {
			fmt.Fprintf(b, " -> fixed in %s", v.Fixed)
		} else {
			b.WriteString(" -> no fixed version published")
		}
		b.WriteString("\n")
	}
	if v.Trace != "" {
		fmt.Fprintf(b, "    via %s\n", v.Trace)
	}
}

func writeOutdated(b *strings.Builder, r depaudit.Result) {
	outdated := r.Outdated()
	if len(outdated) == 0 {
		b.WriteString("\nAll dependencies are at their latest versions.\n")
		return
	}

	// Direct dependencies first: an indirect one usually moves when its
	// parent does, so it is not separately actionable.
	var direct, indirect []depaudit.Module
	for _, m := range outdated {
		if m.Indirect {
			indirect = append(indirect, m)
		} else {
			direct = append(direct, m)
		}
	}

	fmt.Fprintf(b, "\n%d dependency(ies) behind (%d direct, %d indirect):\n",
		len(outdated), len(direct), len(indirect))

	shown := 0
	for _, group := range []struct {
		label string
		mods  []depaudit.Module
	}{{"direct", direct}, {"indirect", indirect}} {
		if len(group.mods) == 0 {
			continue
		}
		fmt.Fprintf(b, "\n  %s:\n", group.label)
		for _, m := range group.mods {
			if shown >= maxOutdatedShown {
				fmt.Fprintf(b, "    ... and %d more\n", len(outdated)-shown)
				return
			}
			shown++
			fmt.Fprintf(b, "    %s  %s -> %s\n", m.Path, m.Version, m.Update)
		}
	}

	b.WriteString("\nA newer version existing is not by itself a reason to upgrade.\n")
}
