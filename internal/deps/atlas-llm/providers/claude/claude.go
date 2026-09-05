// Package claude implements a fantasy.Provider for Anthropic's
// claude.ai web console backend: the same account a signed-in
// claude.ai browser session uses, reached through whatever
// internal/protected endpoint claude.ai's console API exposes, rather
// than the public Anthropic API at api.anthropic.com, which uses a
// different request envelope and a separate API key.
//
// Scope: this SCAFFOLD implements the fantasy.Provider contract so
// the coordinator can dispatch to it, but the request/response
// envelope (and therefore Generate/Stream) is intentionally a stub
// that returns a "not implemented" error. claude.ai's console
// backend does not publish its request shape; finishing this package
// requires observing the real client (browser DevTools) to capture
// the exact path, headers, and body the console uses, the same way
// Antigravity's Gemini-shaped envelope was captured. See the
// companion package internal/oauth/claude for the OAuth side.
package claude

import (
	"context"
	"errors"

	fantasy "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm/object"
)

// Name is the name of the claude provider, matched against
// catwalk.Type by the coordinator's provider dispatch. The catwalk
// Type is also "claude", so the API-key anthropic provider (Type:
// "anthropic") and the OAuth claude provider (Type: "claude")
// live side by side: pay-per-token via anthropic, Pro/Max/Team
// subscription via claude.
const Name = "claude"

const defaultBaseURL = "https://claude.ai"

type options struct {
	accessToken string
	baseURL     string
	accountID   string
}

// Option configures the claude provider.
type Option = func(*options)

// WithAccessToken sets the OAuth access token issued by claude.ai.
func WithAccessToken(tok string) Option {
	return func(o *options) { o.accessToken = tok }
}

// WithBaseURL overrides the claude.ai endpoint. Empty keeps the default.
func WithBaseURL(baseURL string) Option {
	return func(o *options) { o.baseURL = baseURL }
}

// WithAccountID sets the account/organization id discovered during
// login (see internal/oauth/claude).
func WithAccountID(id string) Option {
	return func(o *options) { o.accountID = id }
}

type provider struct{ options options }

// New creates a new claude provider.
func New(opts ...Option) (fantasy.Provider, error) {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	if o.baseURL == "" {
		o.baseURL = defaultBaseURL
	}
	if o.accessToken == "" {
		return nil, errors.New("claude: missing access token; sign in with `atlas login claude`")
	}
	return &provider{options: o}, nil
}

func (*provider) Name() string { return Name }

type languageModel struct {
	modelID string
	options options
}

// LanguageModel implements fantasy.Provider.
func (p *provider) LanguageModel(_ context.Context, modelID string) (fantasy.LanguageModel, error) {
	return &languageModel{modelID: modelID, options: p.options}, nil
}

func (m *languageModel) Model() string    { return m.modelID }
func (m *languageModel) Provider() string { return Name }

// ProviderOptions carries claude-specific per-call options, set
// via fantasy.Call.ProviderOptions[Name]. Reserved for the eventual
// thinking/reasoning knobs once the claude.ai envelope is known.
type ProviderOptions struct{}

// Options implements fantasy.ProviderOptionsData.
func (o *ProviderOptions) Options() {}

// Generate implements fantasy.LanguageModel.
//
// TODO(claude): wire this up to claude.ai's real /api/.../messages
// endpoint once the request envelope is known. The OAuth token
// (m.options.accessToken) and account id (m.options.accountID) are
// available here; the headers and body shape are the unknowns.
func (m *languageModel) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	return nil, errors.New("claude: Generate not yet implemented; finish the claude.ai request envelope (see internal/oauth/claude package docs)")
}

// Stream implements fantasy.LanguageModel.
//
// TODO(claude): same as Generate; wire this up once the SSE
// envelope the claude.ai console uses is captured.
func (m *languageModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	return nil, errors.New("claude: Stream not yet implemented; finish the claude.ai SSE envelope (see internal/oauth/claude package docs)")
}

// ParseOptions parses provider options from a merged options map for
// the claude provider.
func ParseOptions(data map[string]any) (*ProviderOptions, error) {
	var options ProviderOptions
	if err := fantasy.ParseOptions(data, &options); err != nil {
		return nil, err
	}
	return &options, nil
}

// GenerateObject implements fantasy.LanguageModel via the generic
// tool-call-based fallback (no native structured-output mode here).
// Even when Generate is still a TODO, the structured-output path is
// routed through the same Generate so the rest of the agent loop can
// use the provider as a normal LanguageModel.
func (m *languageModel) GenerateObject(ctx context.Context, call fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return object.GenerateWithTool(ctx, m, call)
}

// StreamObject implements fantasy.LanguageModel via the generic
// tool-call-based fallback.
func (m *languageModel) StreamObject(ctx context.Context, call fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return object.StreamWithTool(ctx, m, call)
}
