// Package windsurf implements a fantasy.Provider for the Codeium
// backend that fronts a Windsurf Pro/Teams coding plan subscription.
//
// Scope: this SCAFFOLD implements the fantasy.Provider contract so
// the coordinator can dispatch to it, but Generate/Stream are
// intentionally stubs. Codeium's Windsurf backend does not publish
// its request shape; finishing this package requires observing the
// real Windsurf IDE to capture the exact path, headers, and body
// the IDE uses. See the companion package internal/oauth/windsurf
// for the OAuth side.
package windsurf

import (
	"context"
	"errors"

	fantasy "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm/object"
)

const Name = "windsurf"

const defaultBaseURL = "https://codeium.com"

type options struct {
	accessToken string
	baseURL     string
	apiKey      string
}

type Option = func(*options)

func WithAccessToken(tok string) Option {
	return func(o *options) { o.accessToken = tok }
}

func WithBaseURL(baseURL string) Option {
	return func(o *options) { o.baseURL = baseURL }
}

// WithAPIKey sets the Codeium API key obtained from the Windsurf
// settings page, which is a separate path from the OAuth session
// token and what most third-party integrations (including the
// Windsurf CLI) actually use.
func WithAPIKey(key string) Option {
	return func(o *options) { o.apiKey = key }
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
	if o.accessToken == "" && o.apiKey == "" {
		return nil, errors.New("windsurf: missing access token or API key; sign in with `atlas login windsurf` or set a Codeium API key")
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

// ProviderOptions carries windsurf-specific per-call options.
type ProviderOptions struct{}

// Options implements fantasy.ProviderOptionsData.
func (o *ProviderOptions) Options() {}

// Generate implements fantasy.LanguageModel.
//
// TODO(windsurf): wire this up to Codeium's /api/v1/... endpoint
// once the request envelope is known.
func (m *languageModel) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	return nil, errors.New("windsurf: Generate not yet implemented; finish the Codeium/Windsurf request envelope (see internal/oauth/windsurf package docs)")
}

// Stream implements fantasy.LanguageModel.
//
// TODO(windsurf): same as Generate.
func (m *languageModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	return nil, errors.New("windsurf: Stream not yet implemented; finish the Codeium/Windsurf SSE envelope (see internal/oauth/windsurf package docs)")
}

// ParseOptions parses provider options from a merged options map for
// the windsurf provider.
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
