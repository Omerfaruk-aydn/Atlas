// Package depaudit inspects a Go module's dependencies: which are
// vulnerable, and which are behind.
//
// Vulnerabilities come from govulncheck when it is installed, because it
// is the only tool that checks whether vulnerable code is actually
// reachable from this module rather than merely present in the module
// graph. That distinction is the whole value: a repository with fifty
// "vulnerable" dependencies usually calls none of the affected
// functions, and a report that cannot tell the difference gets ignored.
package depaudit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Vulnerability is one advisory affecting this module.
type Vulnerability struct {
	// ID is the Go vulnerability database identifier, e.g. GO-2024-1234.
	ID      string
	Aliases []string
	Summary string
	Module  string
	// Found is the version in use; Fixed is the first version that is
	// not affected, empty when no fix has been published.
	Found string
	Fixed string
	// Called reports whether affected code is actually reachable from
	// this module. An uncalled vulnerability is real but not urgent, and
	// conflating the two is how a vulnerability report stops being read.
	Called bool
	// Trace names the first reachable affected symbol, when there is one.
	Trace string
}

// Module is one dependency and whether a newer version exists.
type Module struct {
	Path    string
	Version string
	// Update is the newest available version, empty when current.
	Update string
	// Indirect reports a dependency this module does not import
	// directly.
	Indirect bool
	// Main marks the module being audited.
	Main bool
}

// Result is one audit.
type Result struct {
	Vulnerabilities []Vulnerability
	Modules         []Module
	// VulnToolAvailable reports whether govulncheck ran. When it did
	// not, an empty vulnerability list means "not checked", not "none" --
	// the difference matters more than almost anything else here.
	VulnToolAvailable bool
	VulnToolError     string
}

// Called counts vulnerabilities whose affected code is reachable.
func (r Result) Called() int {
	n := 0
	for _, v := range r.Vulnerabilities {
		if v.Called {
			n++
		}
	}
	return n
}

// Outdated returns modules with a newer version available.
func (r Result) Outdated() []Module {
	var out []Module
	for _, m := range r.Modules {
		if m.Update != "" && !m.Main {
			out = append(out, m)
		}
	}
	return out
}

// Options tune an audit.
type Options struct {
	// Timeout bounds each external command. Zero means two minutes.
	Timeout time.Duration
	// SkipUpdates skips the network round trip that finds newer
	// versions, which is by far the slower half.
	SkipUpdates bool
	// SkipVulns skips govulncheck.
	SkipVulns bool
}

const defaultAuditTimeout = 2 * time.Minute

// Run audits the module rooted at dir.
func Run(ctx context.Context, dir string, opts Options) (Result, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = defaultAuditTimeout
	}
	if _, err := exec.LookPath("go"); err != nil {
		return Result{}, errors.New("the go toolchain is not on PATH")
	}

	var result Result

	if !opts.SkipVulns {
		vulns, err := runGovulncheck(ctx, dir, opts.Timeout)
		switch {
		case errors.Is(err, errGovulncheckMissing):
			result.VulnToolError = "govulncheck is not installed (go install golang.org/x/vuln/cmd/govulncheck@latest)"
		case err != nil:
			result.VulnToolError = err.Error()
		default:
			result.VulnToolAvailable = true
			result.Vulnerabilities = vulns
		}
	}

	modules, err := listModules(ctx, dir, opts)
	if err != nil {
		// Without the module list there is still a vulnerability report
		// worth returning, so this is not fatal on its own.
		if len(result.Vulnerabilities) == 0 && result.VulnToolError == "" {
			return result, err
		}
	}
	result.Modules = modules

	return result, nil
}

var errGovulncheckMissing = errors.New("govulncheck not installed")

// govulnMessage is one line of `govulncheck -json` output. The format is
// a stream of single-key objects rather than one document.
type govulnMessage struct {
	OSV *struct {
		ID      string   `json:"id"`
		Aliases []string `json:"aliases"`
		Summary string   `json:"summary"`
	} `json:"osv"`
	Finding *struct {
		OSV          string `json:"osv"`
		FixedVersion string `json:"fixed_version"`
		Trace        []struct {
			Module   string `json:"module"`
			Version  string `json:"version"`
			Package  string `json:"package"`
			Function string `json:"function"`
		} `json:"trace"`
	} `json:"finding"`
}

func runGovulncheck(ctx context.Context, dir string, timeout time.Duration) ([]Vulnerability, error) {
	binary, err := exec.LookPath("govulncheck")
	if err != nil {
		return nil, errGovulncheckMissing
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, binary, "-json", "./...")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// A non-zero exit is how govulncheck reports findings, so it is not
	// on its own an error.
	runErr := cmd.Run()

	vulns, parseErr := parseGovulncheck(stdout.Bytes())
	if parseErr != nil {
		if runErr != nil {
			return nil, fmt.Errorf("govulncheck: %s", firstLine(stderr.String()))
		}
		return nil, parseErr
	}
	return vulns, nil
}

// parseGovulncheck reads the JSON message stream.
//
// Findings arrive separately from advisories and are joined by OSV id.
// A finding whose trace ends in a function is reachable; one that names
// only a module is present but not called. Both are reported, flagged
// differently, because treating them alike is what makes a vulnerability
// report unreadable.
func parseGovulncheck(out []byte) ([]Vulnerability, error) {
	dec := json.NewDecoder(bytes.NewReader(out))

	osvs := map[string]*Vulnerability{}
	var order []string

	decoded := 0
	for {
		var msg govulnMessage
		if err := dec.Decode(&msg); err != nil {
			break
		}
		decoded++

		if msg.OSV != nil {
			v, ok := osvs[msg.OSV.ID]
			if !ok {
				v = &Vulnerability{ID: msg.OSV.ID}
				osvs[msg.OSV.ID] = v
				order = append(order, msg.OSV.ID)
			}
			v.Aliases = msg.OSV.Aliases
			v.Summary = msg.OSV.Summary
			continue
		}

		if msg.Finding == nil || msg.Finding.OSV == "" {
			continue
		}
		v, ok := osvs[msg.Finding.OSV]
		if !ok {
			v = &Vulnerability{ID: msg.Finding.OSV}
			osvs[msg.Finding.OSV] = v
			order = append(order, msg.Finding.OSV)
		}
		if msg.Finding.FixedVersion != "" {
			v.Fixed = msg.Finding.FixedVersion
		}
		if len(msg.Finding.Trace) == 0 {
			continue
		}
		// The trace runs from the vulnerable symbol outward, so the
		// first frame is the affected code itself.
		frame := msg.Finding.Trace[0]
		if v.Module == "" {
			v.Module = frame.Module
			v.Found = frame.Version
		}
		if frame.Function != "" {
			v.Called = true
			if v.Trace == "" {
				v.Trace = frame.Package + "." + frame.Function
			}
		}
	}

	if decoded == 0 && len(bytes.TrimSpace(out)) > 0 {
		return nil, errors.New("govulncheck produced output that could not be parsed")
	}

	vulns := make([]Vulnerability, 0, len(order))
	for _, id := range order {
		vulns = append(vulns, *osvs[id])
	}
	// Reachable first: that is the part that needs action today.
	sort.SliceStable(vulns, func(i, j int) bool {
		if vulns[i].Called != vulns[j].Called {
			return vulns[i].Called
		}
		return vulns[i].ID < vulns[j].ID
	})
	return vulns, nil
}

// listModule is one record of `go list -m -json`.
type listModule struct {
	Path     string `json:"Path"`
	Version  string `json:"Version"`
	Indirect bool   `json:"Indirect"`
	Main     bool   `json:"Main"`
	Update   *struct {
		Version string `json:"Version"`
	} `json:"Update"`
}

func listModules(ctx context.Context, dir string, opts Options) ([]Module, error) {
	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	args := []string{"list", "-m", "-json"}
	if !opts.SkipUpdates {
		// -u reaches the network to find newer versions, which is by far
		// the slower half of this and the reason it can be skipped.
		args = append(args, "-u")
	}
	args = append(args, "all")

	cmd := exec.CommandContext(runCtx, "go", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil && stdout.Len() == 0 {
		return nil, fmt.Errorf("go list: %s", firstLine(stderr.String()))
	}

	var modules []Module
	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	for {
		var lm listModule
		if err := dec.Decode(&lm); err != nil {
			break
		}
		m := Module{
			Path:     lm.Path,
			Version:  lm.Version,
			Indirect: lm.Indirect,
			Main:     lm.Main,
		}
		if lm.Update != nil {
			m.Update = lm.Update.Version
		}
		modules = append(modules, m)
	}

	sort.Slice(modules, func(i, j int) bool { return modules[i].Path < modules[j].Path })
	return modules, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	if s == "" {
		return "no output"
	}
	return s
}
