// Package jetbrains implements a fantasy.Provider for the JetBrains
// AI Assistant gateway, reached with the Bearer JWT obtained by
// exchanging a JB-ACCESS-TOKEN cookie (see internal/oauth/jetbrains).
//
// Scope: this SCAFFOLD implements the fantasy.Provider contract so
// the coordinator can dispatch to it, but Generate/Stream are
// intentionally stubs. The api.jetbrains.ai gateway does not
// publish its request shape; finishing this package requires
// observing the real JetBrains IDE to capture the exact path,
// headers, and body the IDE uses.
package jetbrains

import (
	"context"
	"errors"

	fantasy "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm/object"
)

const Name = "jetbrains"

const defaultBaseURL = "https://api.jetbrains.ai"

type options struct {
	accessToken string
	baseURL     string
}

type Option = func(*options)

func WithAccessToken(tok string) Option {
	return func(o *options) { o.accessToken = tok }
}

func WithBaseURL(baseURL string) Option {
	return func(o *options) { o.baseURL = baseURL }
}

type provider struct{ options options }

func New(opts ...Option) (fantasy.Provider, error) {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	if o.baseURL == "" {
		o.baseURL = defaultBaseURL
	}
	if o.accessToken == "" {
		return nil, errors.New("jetbrains: missing access token; sign in with `atlas login jetbrains`")
	}
	return &provider{options: o}, nil
}

func (*provider) Name() string { return Name }

type languageModel struct {
	modelID string
	options options
}

func (p *provider) LanguageModel(_ context.Context, modelID string) (fantasy.LanguageModel, error) {
	return &languageModel{modelID: modelID, options: p.options}, nil
}

func (m *languageModel) Model() string    { return m.modelID }
func (m *languageModel) Provider() string { return Name }

// ProviderOptions carries jetbrains-specific per-call options.
type ProviderOptions struct{}

// Options implements fantasy.ProviderOptionsData.
func (o *ProviderOptions) Options() {}

// Generate implements fantasy.LanguageModel.
//
// TODO(jetbrains): wire this up to api.jetbrains.ai's /user/v5/...
// or /chat/v5/... endpoint once the request envelope is known.
func (m *languageModel) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	return nil, errors.New("jetbrains: Generate not yet implemented; finish the api.jetbrains.ai request envelope (see internal/oauth/jetbrains package docs)")
}

// Stream implements fantasy.LanguageModel.
//
// TODO(jetbrains): same as Generate.
func (m *languageModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	return nil, errors.New("jetbrains: Stream not yet implemented; finish the api.jetbrains.ai SSE envelope (see internal/oauth/jetbrains package docs)")
}

// ParseOptions parses provider options from a merged options map for
// the jetbrains provider.
func ParseOptions(data map[string]any) (*ProviderOptions, error) {
	var options ProviderOptions
	if err := fantasy.ParseOptions(data, &options); err != nil {
		return nil, err
	}
	return &options, nil
}

// GenerateObject implements fantasy.LanguageModel via the generic
// tool-call-based fallback (no native structured-output mode here).
func (m *languageModel) GenerateObject(ctx context.Context, call fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return object.GenerateWithTool(ctx, m, call)
}

// StreamObject implements fantasy.LanguageModel via the generic
// tool-call-based fallback.
func (m *languageModel) StreamObject(ctx context.Context, call fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return object.StreamWithTool(ctx, m, call)
}
