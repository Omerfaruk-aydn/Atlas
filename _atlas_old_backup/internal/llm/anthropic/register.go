package anthropic

import "github.com/omerfarukaydin/atlas/internal/llm"

func init() {
	llm.RegisterFactory("anthropic", func(apiKey, model string) llm.Provider {
		return New(apiKey, model)
	})
}
