package openai

import "github.com/omerfarukaydin/atlas/internal/llm"

func init() {
	llm.RegisterFactory("openai", func(apiKey, model string) llm.Provider {
		return New(apiKey, model)
	})
}
