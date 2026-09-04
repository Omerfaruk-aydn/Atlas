// Package dockerx parses a Dockerfile into its build stages and flags the
// handful of instruction-level mistakes that are easy to make and hard to
// notice by reading the file top to bottom: a mutable base image tag, an
// image that ends up running as root, ADD used where COPY was meant, and
// a credential baked into an ENV or ARG layer.
//
// This is a line-oriented parser, not a full Dockerfile grammar -- it
// tracks directives, stage boundaries, and continuation lines, which is
// everything the checks below need.
package dockerx

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Instruction is one directive in a Dockerfile, with continuation lines
// already joined.
type Instruction struct {
	Line      int
	Directive string // Upper-cased: FROM, RUN, COPY, ADD, ENV, ...
	Args      string
}

// Stage is one FROM..until-the-next-FROM section of a multi-stage build.
type Stage struct {
	Index        int
	Name         string // The "AS name" part of FROM, empty if none.
	BaseImage    string
	Instructions []Instruction
}

// Finding is one issue spotted in the Dockerfile.
type Finding struct {
	// Kind is one of "latest-tag", "root-user", "add-for-local-copy",
	// "secret-in-env".
	Kind    string
	Line    int
	Stage   string
	Message string
}

// Result is the outcome of parsing and checking one Dockerfile.
type Result struct {
	Stages   []Stage
	Findings []Finding
}

var secretEnvHints = []string{"password", "passwd", "secret", "apikey", "api_key", "token", "credential", "privatekey", "private_key"}

// Parse reads path and returns its stages plus any findings.
func Parse(path string) (Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()

	instructions, err := readInstructions(f)
	if err != nil {
		return Result{}, fmt.Errorf("reading %s: %w", path, err)
	}

	result := Result{Stages: buildStages(instructions)}
	stageNames := map[string]bool{}
	for _, s := range result.Stages {
		if s.Name != "" {
			stageNames[strings.ToLower(s.Name)] = true
		}
	}

	for i := range result.Stages {
		result.Findings = append(result.Findings, checkStage(&result.Stages[i], stageNames)...)
	}
	return result, nil
}

// readInstructions splits the file into directive lines, joining
// backslash continuations and dropping comments and blank lines.
func readInstructions(f *os.File) ([]Instruction, error) {
	var out []Instruction
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	var (
		pending    strings.Builder
		firstLine  int
		continuing bool
	)

	lineNo := 0
	flush := func() {
		text := strings.TrimSpace(pending.String())
		pending.Reset()
		if text == "" {
			return
		}
		directive, args, ok := strings.Cut(text, " ")
		if !ok {
			directive, args = text, ""
		}
		out = append(out, Instruction{
			Line:      firstLine,
			Directive: strings.ToUpper(directive),
			Args:      strings.TrimSpace(args),
		})
	}

	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)

		if !continuing {
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			firstLine = lineNo
		}

		if after, ok := strings.CutSuffix(trimmed, "\\"); ok {
			pending.WriteString(strings.TrimSpace(after))
			pending.WriteString(" ")
			continuing = true
			continue
		}

		pending.WriteString(trimmed)
		continuing = false
		flush()
	}
	if continuing {
		flush()
	}
	return out, scanner.Err()
}

// buildStages groups instructions by FROM boundaries.
func buildStages(instructions []Instruction) []Stage {
	var stages []Stage
	for _, inst := range instructions {
		if inst.Directive == "FROM" {
			image, name := parseFrom(inst.Args)
			stages = append(stages, Stage{
				Index:     len(stages),
				Name:      name,
				BaseImage: image,
			})
			continue
		}
		if len(stages) == 0 {
			continue // Instructions before any FROM are not part of a stage.
		}
		last := &stages[len(stages)-1]
		last.Instructions = append(last.Instructions, inst)
	}
	return stages
}

// parseFrom reads "image[:tag] [AS name]" out of a FROM instruction's
// arguments, ignoring a leading --platform=... flag.
func parseFrom(args string) (image, name string) {
	fields := strings.Fields(args)
	var rest []string
	for _, f := range fields {
		if strings.HasPrefix(f, "--") {
			continue
		}
		rest = append(rest, f)
	}
	if len(rest) == 0 {
		return "", ""
	}
	image = rest[0]
	if len(rest) >= 3 && strings.EqualFold(rest[1], "AS") {
		name = rest[2]
	}
	return image, name
}

func checkStage(s *Stage, stageNames map[string]bool) []Finding {
	var findings []Finding

	if f := checkLatestTag(s, stageNames); f != nil {
		findings = append(findings, *f)
	}

	hasUser := false
	for _, inst := range s.Instructions {
		switch inst.Directive {
		case "USER":
			hasUser = true
		case "ADD":
			if f := checkAddForLocalCopy(s, inst); f != nil {
				findings = append(findings, *f)
			}
		case "ENV", "ARG":
			findings = append(findings, checkSecretInEnv(s, inst)...)
		}
	}
	if !hasUser {
		findings = append(findings, Finding{
			Kind: "root-user", Stage: stageLabel(s),
			Message: "no USER instruction in this stage -- the container runs as root by default",
		})
	}

	return findings
}

func checkLatestTag(s *Stage, stageNames map[string]bool) *Finding {
	image := s.BaseImage
	if image == "" || image == "scratch" {
		return nil
	}
	if stageNames[strings.ToLower(image)] {
		return nil // References an earlier build stage, not a registry image.
	}

	// A digest pin (@sha256:...) is at least as immutable as a tag, so it
	// is not a "latest" problem even without a ":tag" part.
	if strings.Contains(image, "@") {
		return nil
	}

	lastSlash := strings.LastIndex(image, "/")
	tagPart := image
	if lastSlash >= 0 {
		tagPart = image[lastSlash+1:]
	}
	if !strings.Contains(tagPart, ":") {
		return &Finding{
			Kind: "latest-tag", Stage: stageLabel(s),
			Message: fmt.Sprintf("FROM %s has no tag, which resolves to :latest -- a build today and a build tomorrow can pull different images", image),
		}
	}
	if strings.HasSuffix(tagPart, ":latest") {
		return &Finding{
			Kind: "latest-tag", Stage: stageLabel(s),
			Message: fmt.Sprintf("FROM %s pins the mutable :latest tag -- a build today and a build tomorrow can pull different images", image),
		}
	}
	return nil
}

func checkAddForLocalCopy(s *Stage, inst Instruction) *Finding {
	fields := strings.Fields(inst.Args)
	var src string
	for _, f := range fields {
		if !strings.HasPrefix(f, "--") {
			src = f
			break
		}
	}
	if src == "" {
		return nil
	}
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return nil // Fetching a URL is exactly what ADD is for.
	}
	for _, ext := range []string{".tar", ".tar.gz", ".tgz", ".tar.bz2", ".tar.xz", ".zip"} {
		if strings.HasSuffix(src, ext) {
			return nil // Auto-extraction is exactly what ADD is for.
		}
	}
	return &Finding{
		Line: inst.Line, Stage: stageLabel(s), Kind: "add-for-local-copy",
		Message: fmt.Sprintf("ADD %s copies a local path with none of ADD's extra behavior (URL fetch, archive extraction) in play -- COPY says the same thing without the surprise", inst.Args),
	}
}

func checkSecretInEnv(s *Stage, inst Instruction) []Finding {
	var findings []Finding
	for _, assignment := range splitEnvAssignments(inst.Args) {
		key, value, ok := strings.Cut(assignment, "=")
		if !ok {
			continue
		}
		value = strings.Trim(value, `"'`)
		if !looksLikeSecretKey(key) || isPlaceholderValue(value) {
			continue
		}
		findings = append(findings, Finding{
			Line: inst.Line, Stage: stageLabel(s), Kind: "secret-in-env",
			Message: fmt.Sprintf("%s %s bakes a credential-shaped value into the image layer history, where it survives even if a later layer removes it", inst.Directive, key),
		})
	}
	return findings
}

// splitEnvAssignments handles both ENV/ARG forms: "KEY value" (single,
// space-separated) and "KEY1=v1 KEY2=v2" (one or more, always with "=").
func splitEnvAssignments(args string) []string {
	if !strings.Contains(args, "=") {
		return nil
	}
	return strings.Fields(args)
}

func looksLikeSecretKey(key string) bool {
	lower := strings.ToLower(key)
	for _, hint := range secretEnvHints {
		if strings.Contains(lower, hint) {
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
	for _, placeholder := range []string{"todo", "changeme", "xxx", "example", "placeholder", "your-", "$"} {
		if strings.Contains(lower, placeholder) {
			return true
		}
	}
	return false
}

func stageLabel(s *Stage) string {
	if s.Name != "" {
		return s.Name
	}
	return fmt.Sprintf("stage %d", s.Index)
}
