package gemini

import "github.com/omerfarukaydin/atlas/internal/llm"

func init() {
	llm.RegisterFactory("gemini", func(apiKey, model string) llm.Provider {
		return New(apiKey, model)
	})
}
