// Package terraformx scans Terraform (.tf) source for a small set of
// misconfigurations that are valid HCL, apply cleanly, and only turn into
// an incident later: a security group open to the internet, a credential
// typed directly into a provider or variable default, or a storage
// resource set to a public ACL.
//
// It reads line by line and tracks block nesting by counting braces
// rather than parsing full HCL. That is a real limitation -- a brace
// inside a string, or several block headers sharing one line, can throw
// the nesting off -- but it covers `terraform fmt`-formatted source,
// which is the overwhelming majority of real Terraform, without adding
// an HCL parser dependency for three pattern checks.
package terraformx

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Finding is one issue spotted in a Terraform file.
type Finding struct {
	// Kind is one of "open-ingress", "hardcoded-credential",
	// "public-acl".
	Kind     string
	File     string
	Line     int
	Resource string // e.g. "aws_security_group.web", empty when unknown.
	Message  string
}

// Result is the outcome of a scan.
type Result struct {
	Findings     []Finding
	FilesScanned int
}

var (
	resourceHeader = regexp.MustCompile(`^resource\s+"([^"]+)"\s+"([^"]+)"\s*\{`)
	providerHeader = regexp.MustCompile(`^provider\s+"([^"]+)"\s*\{`)
	variableHeader = regexp.MustCompile(`^variable\s+"([^"]+)"\s*\{`)
	ingressHeader  = regexp.MustCompile(`^\s*(dynamic\s+"ingress"|ingress)\s*\{`)
	egressHeader   = regexp.MustCompile(`^\s*(dynamic\s+"egress"|egress)\s*\{`)

	openCIDRPattern       = regexp.MustCompile(`"(0\.0\.0\.0/0|::/0)"`)
	credentialAssignment  = regexp.MustCompile(`^(access_key|secret_key|token|password|default)\s*=\s*"([^"]*)"`)
	publicACLAssignment   = regexp.MustCompile(`^acl\s*=\s*"(public-read|public-read-write)"`)
	credentialFieldHints  = []string{"access_key", "secret_key", "token", "password"}
	credentialVarNameHint = regexp.MustCompile(`(?i)password|secret|token|api_key|apikey|credential`)
)

type frame struct {
	depth int
	label string // "resource", "provider", "variable", "ingress", "egress", or "".
	extra string
}

// Scan walks root and checks every .tf file it finds.
func Scan(root string) (Result, error) {
	files, err := collectTerraformFiles(root)
	if err != nil {
		return Result{}, err
	}

	result := Result{}
	for _, path := range files {
		findings, err := scanFile(path)
		if err != nil {
			continue
		}
		result.FilesScanned++
		result.Findings = append(result.Findings, findings...)
	}

	sort.SliceStable(result.Findings, func(i, j int) bool {
		if result.Findings[i].File != result.Findings[j].File {
			return result.Findings[i].File < result.Findings[j].File
		}
		return result.Findings[i].Line < result.Findings[j].Line
	})
	return result, nil
}

func scanFile(path string) ([]Finding, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var findings []Finding
	var stack []frame
	depth := 0
	lineNo := 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		pushHeaderFrame(&stack, depth, trimmed)

		if f := checkOpenIngress(path, lineNo, trimmed, stack); f != nil {
			findings = append(findings, *f)
		}
		if f := checkHardcodedCredential(path, lineNo, trimmed, stack); f != nil {
			findings = append(findings, *f)
		}
		if f := checkPublicACL(path, lineNo, trimmed, stack); f != nil {
			findings = append(findings, *f)
		}

		depth += strings.Count(trimmed, "{") - strings.Count(trimmed, "}")
		for len(stack) > 0 && stack[len(stack)-1].depth >= depth {
			stack = stack[:len(stack)-1]
		}
	}
	return findings, scanner.Err()
}

func pushHeaderFrame(stack *[]frame, depth int, line string) {
	switch {
	case resourceHeader.MatchString(line):
		m := resourceHeader.FindStringSubmatch(line)
		*stack = append(*stack, frame{depth: depth, label: "resource", extra: m[1] + "." + m[2]})
	case providerHeader.MatchString(line):
		m := providerHeader.FindStringSubmatch(line)
		*stack = append(*stack, frame{depth: depth, label: "provider", extra: m[1]})
	case variableHeader.MatchString(line):
		m := variableHeader.FindStringSubmatch(line)
		*stack = append(*stack, frame{depth: depth, label: "variable", extra: m[1]})
	case ingressHeader.MatchString(line):
		*stack = append(*stack, frame{depth: depth, label: "ingress"})
	case egressHeader.MatchString(line):
		*stack = append(*stack, frame{depth: depth, label: "egress"})
	}
}

func enclosingLabel(stack []frame, label string) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].label == label {
			return true
		}
	}
	return false
}

func enclosingResource(stack []frame) string {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].label == "resource" {
			return stack[i].extra
		}
	}
	return ""
}

func checkOpenIngress(path string, line int, trimmed string, stack []frame) *Finding {
	if !openCIDRPattern.MatchString(trimmed) || !enclosingLabel(stack, "ingress") {
		return nil
	}
	return &Finding{
		Kind: "open-ingress", File: path, Line: line, Resource: enclosingResource(stack),
		Message: "ingress rule allows traffic from 0.0.0.0/0 (or ::/0) -- open to the entire internet",
	}
}

func checkHardcodedCredential(path string, line int, trimmed string, stack []frame) *Finding {
	m := credentialAssignment.FindStringSubmatch(trimmed)
	if m == nil {
		return nil
	}
	field, value := m[1], m[2]

	inProvider := enclosingLabel(stack, "provider")
	inVariable := false
	varLooksLikeSecret := false
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].label == "variable" {
			inVariable = true
			varLooksLikeSecret = credentialVarNameHint.MatchString(stack[i].extra)
			break
		}
	}

	switch {
	case inProvider && contains(credentialFieldHints, field):
	case inVariable && field == "default" && varLooksLikeSecret:
	default:
		return nil
	}

	if isPlaceholderValue(value) || strings.HasPrefix(value, "${") {
		return nil
	}
	return &Finding{
		Kind: "hardcoded-credential", File: path, Line: line, Resource: enclosingResource(stack),
		Message: fmt.Sprintf("%s is set to a literal value that looks like a real credential -- use a variable sourced from the environment or a secrets manager instead", field),
	}
}

func checkPublicACL(path string, line int, trimmed string, stack []frame) *Finding {
	m := publicACLAssignment.FindStringSubmatch(trimmed)
	if m == nil {
		return nil
	}
	return &Finding{
		Kind: "public-acl", File: path, Line: line, Resource: enclosingResource(stack),
		Message: fmt.Sprintf("acl is set to %q, making this resource's contents publicly accessible", m[1]),
	}
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func isPlaceholderValue(v string) bool {
	if len(v) < 8 {
		return true
	}
	lower := strings.ToLower(v)
	for _, placeholder := range []string{"todo", "changeme", "xxx", "example", "placeholder", "your-"} {
		if strings.Contains(lower, placeholder) {
			return true
		}
	}
	return false
}

func collectTerraformFiles(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{root}, nil
	}

	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".terraform" {
				return filepath.SkipDir
			}
			if path != root && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".tf") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
