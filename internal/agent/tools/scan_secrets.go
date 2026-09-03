package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/secrets"
)

const ScanSecretsToolName = "scan_secrets"

//go:embed scan_secrets.md
var scanSecretsDescription string

// maxSecretsShown bounds the printed list. The count of everything found
// is reported separately, so a truncated report never reads as a clean
// bill of health.
const maxSecretsShown = 60

type ScanSecretsParams struct {
	Path          string `json:"path,omitempty" description:"Directory or single file to scan. Defaults to the working directory."`
	MinConfidence string `json:"min_confidence,omitempty" description:"Report only findings at or above this level: high, medium, or low. Default reports everything."`
	SkipGeneric   *bool  `json:"skip_generic,omitempty" description:"Only run provider-specific rules. Fewer false positives, misses credentials no rule knows about."`
}

type ScanSecretsResponseMetadata struct {
	Findings     int  `json:"findings"`
	High         int  `json:"high_confidence"`
	FilesScanned int  `json:"files_scanned"`
	FilesSkipped int  `json:"files_skipped"`
	Truncated    bool `json:"truncated"`
}

func NewScanSecretsTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ScanSecretsToolName,
		scanSecretsDescription,
		func(ctx context.Context, params ScanSecretsParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}

			minConfidence, err := parseConfidence(params.MinConfidence)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			result, err := secrets.Scan(root, secrets.Options{
				MinConfidence: minConfidence,
				SkipGeneric:   params.SkipGeneric != nil && *params.SkipGeneric,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			high := 0
			for _, f := range result.Findings {
				if f.Confidence == secrets.ConfidenceHigh {
					high++
				}
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatSecrets(result, workingDir)),
				ScanSecretsResponseMetadata{
					Findings:     len(result.Findings),
					High:         high,
					FilesScanned: result.FilesScanned,
					FilesSkipped: result.FilesSkipped,
					Truncated:    result.Truncated,
				},
			), nil
		},
	)
}

func parseConfidence(s string) (secrets.Confidence, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return "", nil
	case "high":
		return secrets.ConfidenceHigh, nil
	case "medium":
		return secrets.ConfidenceMedium, nil
	case "low":
		return secrets.ConfidenceLow, nil
	}
	return "", fmt.Errorf("min_confidence must be high, medium, or low (got %q)", s)
}

func formatSecrets(r secrets.Result, workingDir string) string {
	if r.FilesScanned == 0 {
		return "No files scanned. Check the path, or the tree may be entirely skipped types (binaries, node_modules, vendor)."
	}
	if len(r.Findings) == 0 {
		return fmt.Sprintf("No credentials found across %d file(s).\n\n"+
			"This is not a guarantee: only known credential shapes and secret-looking assignments are detected, "+
			"and dependency trees, binaries and files over 1 MB were skipped.", r.FilesScanned)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d suspected credential(s) across %d file(s).\n", len(r.Findings), r.FilesScanned)

	// The remediation has to lead, not trail. Deleting the line is the
	// intuitive fix and it is the wrong one -- the value is already in
	// git history and, if pushed, already public.
	b.WriteString("\nIf any of these is real, it must be ROTATED at the provider. Deleting the line is not enough: the value remains in git history, and if the repository was ever pushed it is already exposed.\n")
	b.WriteString("Values below are redacted. Do not read the file to recover the full value -- that would copy the secret into this conversation.\n\n")

	shown := r.Findings
	if len(shown) > maxSecretsShown {
		shown = shown[:maxSecretsShown]
	}

	for _, f := range shown {
		fmt.Fprintf(&b, "[%s] %s\n  %s:%d\n  %s\n",
			f.Confidence, f.Rule, relOrAbs(f.File, workingDir), f.Line, f.Context)
	}

	if len(shown) < len(r.Findings) {
		fmt.Fprintf(&b, "\n... and %d more not shown.\n", len(r.Findings)-len(shown))
	}
	if r.Truncated {
		b.WriteString("\nThe scan stopped at its finding limit, so there may be more beyond these.\n")
	}
	b.WriteString("\n\"medium\" and \"low\" come from the keyword layer and include false positives; \"high\" is a provider-specific shape and is almost always real.\n")
	return b.String()
}
