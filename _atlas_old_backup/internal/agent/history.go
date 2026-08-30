package agent

import "github.com/omerfarukaydin/atlas/internal/llm"

// History holds the conversation so far in provider-agnostic form.
type History struct {
	messages []llm.Message
}

func (h *History) AppendUser(text string) {
	h.messages = append(h.messages, llm.TextMessage(llm.RoleUser, text))
}

func (h *History) AppendAssistant(text string) {
	h.messages = append(h.messages, llm.TextMessage(llm.RoleAssistant, text))
}

// Append adds a fully-formed message (e.g. one carrying tool_use/tool_result
// content blocks) to the conversation.
func (h *History) Append(msg llm.Message) {
	h.messages = append(h.messages, msg)
}

func (h *History) Messages() []llm.Message {
	return h.messages
}
