// Package augment implements a fantasy.Provider for Augment Code's
// web IDE backend: the same account a signed-in Augment browser
// session uses, reached through whatever internal/protected endpoint
// Augment's backend exposes.
//
// Scope: this SCAFFOLD implements the fantasy.Provider contract so
// the coordinator can dispatch to it, but Generate/Stream are
// intentionally stubs that return "not implemented". Finishing this
// package requires observing the real client (browser DevTools) to
// capture the exact path, headers, and body the IDE uses. See the
// companion package internal/oauth/augment for the OAuth side.
package augment

import (
	"context"
	"errors"

	fantasy "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm/object"
)

// Name is the name of the augment provider, matched against
// catwalk.Type by the coordinator's provider dispatch.
const Name = "augment"

const defaultBaseURL = "https://augmentcode.com"

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
		return nil, errors.New("augment: missing access token; sign in with `atlas login augment`")
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

// ProviderOptions carries augment-specific per-call options.
type ProviderOptions struct{}

// Options implements fantasy.ProviderOptionsData.
func (o *ProviderOptions) Options() {}

// ParseOptions parses provider options from a merged options map for
// the augment provider.
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

// Generate implements fantasy.LanguageModel.
//
// TODO(augment): wire this up to the Augment backend.
func (m *languageModel) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	return nil, errors.New("augment: Generate not yet implemented; finish the Augment request envelope (see internal/oauth/augment package docs)")
}

// Stream implements fantasy.LanguageModel.
//
// TODO(augment): same as Generate; wire this up once the SSE envelope
// the Augment backend uses is captured.
func (m *languageModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	return nil, errors.New("augment: Stream not yet implemented; finish the Augment SSE envelope (see internal/oauth/augment package docs)")
}
