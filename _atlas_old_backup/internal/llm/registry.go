package llm

import "fmt"

// Factory constructs a Provider from an API key and model name. Each
// concrete provider subpackage registers itself via RegisterFactory in an
// init() so internal/agent and internal/tui never import provider
// subpackages directly.
type Factory func(apiKey, model string) Provider

var factories = map[string]Factory{}

func RegisterFactory(name string, f Factory) {
	factories[name] = f
}

func New(name, apiKey, model string) (Provider, error) {
	f, ok := factories[name]
	if !ok {
		return nil, fmt.Errorf("bilinmeyen sağlayıcı: %q", name)
	}
	return f(apiKey, model), nil
}

func Available() []string {
	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	return names
}
