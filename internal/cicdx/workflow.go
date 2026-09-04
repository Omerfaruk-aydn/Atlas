// Package cicdx reads a GitHub Actions workflow file and flags the
// handful of mistakes that run green today and become a supply-chain
// risk, a runaway job, or a leaked secret later.
//
// It parses generically (into map[string]any via yaml.v3) rather than
// against a full workflow schema -- the same tradeoff internal/k8sx makes
// for Kubernetes manifests, for the same reason: three or four field
// reads out of a YAML tree don't need a dedicated schema type.
package cicdx

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Finding is one issue spotted in a workflow file.
type Finding struct {
	// Kind is one of "unpinned-action", "missing-timeout",
	// "secret-in-run".
	Kind string
	Job  string
	// Step is the step's "name", or its "uses"/"run" value truncated,
	// when the step has no name. Empty for a job-level finding like
	// missing-timeout.
	Step    string
	Message string
}

// Result is the outcome of scanning one workflow file.
type Result struct {
	Findings  []Finding
	JobsFound int
}

var (
	pinnedSHA    = regexp.MustCompile(`^[0-9a-f]{40}$`)
	pinnedTag    = regexp.MustCompile(`^v?\d+(\.\d+){0,2}$`)
	secretRef    = regexp.MustCompile(`secrets\.[A-Za-z0-9_]+`)
	echoLikeLine = regexp.MustCompile(`(?i)\b(echo|print|printf|cat|console\.log)\b`)
)

// Parse reads path and checks every job and step it defines.
func Parse(path string) (Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Result{}, fmt.Errorf("parsing %s: %w", path, err)
	}

	jobs, _ := doc["jobs"].(map[string]any)
	result := Result{JobsFound: len(jobs)}
	for name, raw := range jobs {
		job, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		result.Findings = append(result.Findings, checkJob(name, job)...)
	}
	return result, nil
}

func checkJob(name string, job map[string]any) []Finding {
	var findings []Finding

	if _, ok := job["timeout-minutes"]; !ok {
		findings = append(findings, Finding{
			Kind: "missing-timeout", Job: name,
			Message: "no timeout-minutes set -- a hung step runs until GitHub's own default (six hours) cuts it off",
		})
	}

	steps, _ := job["steps"].([]any)
	for _, raw := range steps {
		step, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		findings = append(findings, checkStep(name, step)...)
	}
	return findings
}

func checkStep(job string, step map[string]any) []Finding {
	var findings []Finding
	label := stepLabel(step)

	if uses, ok := step["uses"].(string); ok {
		if f := checkUnpinnedAction(job, label, uses); f != nil {
			findings = append(findings, *f)
		}
	}
	if run, ok := step["run"].(string); ok {
		findings = append(findings, checkSecretInRun(job, label, run)...)
	}
	return findings
}

func checkUnpinnedAction(job, step, uses string) *Finding {
	// Local actions ("./.github/actions/x") and Docker actions
	// ("docker://image:tag") are not fetched from a mutable ref the same
	// way a marketplace action is.
	if strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, "docker://") {
		return nil
	}
	_, ref, ok := strings.Cut(uses, "@")
	if !ok || ref == "" {
		return &Finding{
			Kind: "unpinned-action", Job: job, Step: step,
			Message: fmt.Sprintf("uses: %s has no @ref at all, which resolves to the action's default branch", uses),
		}
	}
	if pinnedSHA.MatchString(ref) || pinnedTag.MatchString(ref) {
		return nil
	}
	return &Finding{
		Kind: "unpinned-action", Job: job, Step: step,
		Message: fmt.Sprintf("uses: %s pins a branch, not a version tag or commit SHA -- the action's code can change without this workflow changing", uses),
	}
}

func checkSecretInRun(job, step, run string) []Finding {
	var findings []Finding
	for _, line := range strings.Split(run, "\n") {
		if secretRef.MatchString(line) && echoLikeLine.MatchString(line) {
			findings = append(findings, Finding{
				Kind: "secret-in-run", Job: job, Step: step,
				Message: fmt.Sprintf("this line both prints output and references a secret: %s", strings.TrimSpace(line)),
			})
		}
	}
	return findings
}

func stepLabel(step map[string]any) string {
	if name, ok := step["name"].(string); ok && name != "" {
		return name
	}
	if uses, ok := step["uses"].(string); ok {
		return "uses: " + uses
	}
	if run, ok := step["run"].(string); ok {
		first, _, _ := strings.Cut(strings.TrimSpace(run), "\n")
		return "run: " + first
	}
	return "(unnamed step)"
}
