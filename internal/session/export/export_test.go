package export

import (
	"strings"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/message"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/stretchr/testify/require"
)

func TestMarkdownIncludesTitleAndRoles(t *testing.T) {
	t.Parallel()

	sess := session.Session{Title: "Fixing the parser", CreatedAt: 1_700_000_000_000}
	msgs := []message.Message{
		{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "why does this crash"}}},
		{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: "because of a nil pointer"}}},
	}

	doc := Markdown(sess, msgs)

	require.Contains(t, doc, "# Fixing the parser")
	require.Contains(t, doc, "## User")
	require.Contains(t, doc, "why does this crash")
	require.Contains(t, doc, "## Assistant")
	require.Contains(t, doc, "because of a nil pointer")
}

func TestMarkdownDefaultsAnEmptyTitle(t *testing.T) {
	t.Parallel()
	doc := Markdown(session.Session{}, nil)
	require.Contains(t, doc, "# Untitled session")
}

func TestMarkdownRendersToolCallsAndResults(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{ID: "1", Name: "grep", Input: `{"pattern":"TODO"}`},
			message.ToolResult{ToolCallID: "1", Name: "grep", Content: "main.go:12: // TODO fix this"},
		},
	}}

	doc := Markdown(session.Session{}, msgs)

	require.Contains(t, doc, "Tool call: `grep`")
	require.Contains(t, doc, `"pattern":"TODO"`)
	require.Contains(t, doc, "Tool result:")
	require.Contains(t, doc, "main.go:12")
}

func TestMarkdownFlagsAToolError(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.ToolResult{ToolCallID: "1", Content: "permission denied", IsError: true}},
	}}

	doc := Markdown(session.Session{}, msgs)
	require.Contains(t, doc, "Tool error:")
}

func TestMarkdownRendersShellCommands(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.ShellCommand{Command: "go test ./...", Output: "ok"}},
	}}

	doc := Markdown(session.Session{}, msgs)
	require.Contains(t, doc, "```bash\ngo test ./...\n```")
	require.Contains(t, doc, "ok")
}

func TestMarkdownSkipsReasoningAndFinish(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ReasoningContent{Thinking: "internal chain of thought"},
			message.Finish{Reason: message.FinishReasonEndTurn},
		},
	}}

	doc := Markdown(session.Session{}, msgs)
	require.NotContains(t, doc, "internal chain of thought")
	require.Contains(t, doc, "(no content)", "a turn with only internal bookkeeping still shows as empty")
}

func TestMarkdownTruncatesLongToolOutput(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("x", maxRenderedLen+500)
	msgs := []message.Message{{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.ToolResult{ToolCallID: "1", Content: huge}},
	}}

	doc := Markdown(session.Session{}, msgs)
	require.Contains(t, doc, "truncated")
	require.Less(t, len(doc), len(huge))
}

func TestMarkdownSkipsEmptyTextParts(t *testing.T) {
	t.Parallel()

	msgs := []message.Message{{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "   "}},
	}}

	doc := Markdown(session.Session{}, msgs)
	require.Contains(t, doc, "(no content)")
}
