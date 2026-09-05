// Package factory implements a fantasy.Provider for Factory AI Droids.
//
// Scope: this SCAFFOLD implements the fantasy.Provider contract so
// the coordinator can dispatch to it, but Generate/Stream are
// intentionally stubs. Finishing this package requires observing the
// real client. See the companion package internal/oauth/factory for
// the OAuth side.
package factory

import (
	"context"
	"errors"

	fantasy "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm/object"
)

const Name = "factory"

const defaultBaseURL = "https://api.factory.ai"

type options struct {
	accessToken string
	baseURL     string
	accountID   string
}

type Option = func(*options)

func WithAccessToken(tok string) Option { return func(o *options) { o.accessToken = tok } }
func WithBaseURL(baseURL string) Option { return func(o *options) { o.baseURL = baseURL } }
func WithAccountID(id string) Option    { return func(o *options) { o.accountID = id } }

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
		return nil, errors.New("factory: missing access token; sign in with `atlas login factory`")
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

type ProviderOptions struct{}

func (o *ProviderOptions) Options() {}

func ParseOptions(data map[string]any) (*ProviderOptions, error) {
	var options ProviderOptions
	if err := fantasy.ParseOptions(data, &options); err != nil {
		return nil, err
	}
	return &options, nil
}

func (m *languageModel) GenerateObject(ctx context.Context, call fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return object.GenerateWithTool(ctx, m, call)
}

func (m *languageModel) StreamObject(ctx context.Context, call fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return object.StreamWithTool(ctx, m, call)
}

// TODO(factory): wire this up to the Factory AI backend.
func (m *languageModel) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	return nil, errors.New("factory: Generate not yet implemented; finish the Factory AI request envelope (see internal/oauth/factory package docs)")
}

func (m *languageModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	return nil, errors.New("factory: Stream not yet implemented; finish the Factory AI SSE envelope (see internal/oauth/factory package docs)")
}
