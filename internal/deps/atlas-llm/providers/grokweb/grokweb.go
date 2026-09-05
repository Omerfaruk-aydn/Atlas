// Package grokweb implements a fantasy.Provider for xAI's grok.com
// web console backend: the same account a signed-in grok.com browser
// session uses, reached through whatever internal/protected endpoint
// grok.com's console API exposes, rather than the public xAI API at
// api.x.ai, which uses a separate XAI_API_KEY.
//
// Scope: this SCAFFOLD implements the fantasy.Provider contract so
// the coordinator can dispatch to it, but Generate/Stream are
// intentionally stubs that return "not implemented". grok.com's
// backend does not publish its request shape; finishing this package
// requires observing the real client (browser DevTools) to capture
// the exact path, headers, and body the console uses. See the
// companion package internal/oauth/grok for the OAuth side.
package grokweb

import (
	"context"
	"errors"

	fantasy "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm/object"
)

const Name = "grok-web"

const defaultBaseURL = "https://grok.com"

type options struct {
	accessToken string
	baseURL     string
	accountID   string
}

type Option = func(*options)

func WithAccessToken(tok string) Option {
	return func(o *options) { o.accessToken = tok }
}

func WithBaseURL(baseURL string) Option {
	return func(o *options) { o.baseURL = baseURL }
}

func WithAccountID(id string) Option {
	return func(o *options) { o.accountID = id }
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
		return nil, errors.New("grok-web: missing access token; sign in with `atlas login grok`")
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

// ProviderOptions carries grok-web-specific per-call options.
type ProviderOptions struct{}

// Options implements fantasy.ProviderOptionsData.
func (o *ProviderOptions) Options() {}

// Generate implements fantasy.LanguageModel.
//
// TODO(grok-web): wire this up to grok.com's real /rest/.../chat
// endpoint once the request envelope is known.
func (m *languageModel) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	return nil, errors.New("grok-web: Generate not yet implemented; finish the grok.com request envelope (see internal/oauth/grok package docs)")
}

// Stream implements fantasy.LanguageModel.
//
// TODO(grok-web): same as Generate; wire this up once the SSE
// envelope the grok.com console uses is captured.
func (m *languageModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	return nil, errors.New("grok-web: Stream not yet implemented; finish the grok.com SSE envelope (see internal/oauth/grok package docs)")
}

// ParseOptions parses provider options from a merged options map for
// the grok-web provider.
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
