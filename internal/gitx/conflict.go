package gitx

import "strings"

// ConflictBlock is one merge-conflict region in a file: the range git
// could not reconcile automatically, split into what each side changed.
type ConflictBlock struct {
	// StartLine is the line of the opening "<<<<<<<" marker.
	StartLine int
	// EndLine is the line of the closing ">>>>>>>" marker.
	EndLine int
	// OursLabel and TheirsLabel are whatever followed the "<<<<<<<" and
	// ">>>>>>>" markers -- usually a branch name or "HEAD".
	OursLabel   string
	TheirsLabel string
	OursLines   []string
	TheirsLines []string
	// BaseLines and BaseLabel are set only for a diff3-style conflict,
	// which includes the common ancestor between a "|||||||" marker and
	// the "=======" divider. Most repositories use git's default
	// "merge" style, which never has this section.
	BaseLabel string
	BaseLines []string
}

// ParseConflicts reads a file's content for merge-conflict markers and
// returns each conflicted region found. A file with none returns an
// empty slice, not an error -- the absence of conflicts is the expected,
// successful case, not a failure to parse anything.
func ParseConflicts(content string) []ConflictBlock {
	lines := strings.Split(content, "\n")
	var blocks []ConflictBlock
	var current *ConflictBlock
	section := "" // "ours", "base", "theirs" -- empty when not inside a block.

	for i, line := range lines {
		lineNo := i + 1
		switch {
		case strings.HasPrefix(line, "<<<<<<<"):
			current = &ConflictBlock{StartLine: lineNo, OursLabel: strings.TrimSpace(strings.TrimPrefix(line, "<<<<<<<"))}
			section = "ours"
			continue
		case current == nil:
			continue
		case strings.HasPrefix(line, "|||||||"):
			current.BaseLabel = strings.TrimSpace(strings.TrimPrefix(line, "|||||||"))
			section = "base"
			continue
		case strings.HasPrefix(line, "======="):
			section = "theirs"
			continue
		case strings.HasPrefix(line, ">>>>>>>"):
			current.EndLine = lineNo
			current.TheirsLabel = strings.TrimSpace(strings.TrimPrefix(line, ">>>>>>>"))
			blocks = append(blocks, *current)
			current = nil
			section = ""
			continue
		}

		switch section {
		case "ours":
			current.OursLines = append(current.OursLines, line)
		case "base":
			current.BaseLines = append(current.BaseLines, line)
		case "theirs":
			current.TheirsLines = append(current.TheirsLines, line)
		}
	}
	return blocks
}
