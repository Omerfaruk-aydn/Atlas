package dialog

import (
	"context"
	"fmt"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-models/pkg/catwalk"
	tea "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-ui/v2"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth/antigravity"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/common"
)

// NewOAuthAntigravity opens the OAuth dialog driving an Antigravity
// (Google AI Pro/Ultra) sign-in: a PKCE + localhost redirect flow like
// ChatGPT's, plus a Cloud project discovery/provisioning step once the
// token is in hand.
func NewOAuthAntigravity(
	com *common.Common,
	isOnboarding bool,
	provider catwalk.Provider,
	model config.SelectedModel,
	modelType config.SelectedModelType,
) (*OAuth, tea.Cmd) {
	return newOAuth(com, isOnboarding, provider, model, modelType, &OAuthAntigravity{})
}

type OAuthAntigravity struct {
	session    *antigravity.AuthSession
	cancelFunc func()
}

var _ OAuthProvider = (*OAuthAntigravity)(nil)

func (m *OAuthAntigravity) name() string {
	return "Antigravity"
}

func (m *OAuthAntigravity) initiateAuth() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session, err := antigravity.Start(ctx)
	if err != nil {
		return ActionOAuthErrored{Error: fmt.Errorf("failed to start Antigravity sign-in: %w", err)}
	}
	m.session = session

	return ActionInitiateOAuth{
		DeviceCode:      "",
		UserCode:        "",
		VerificationURL: session.AuthURL(),
		// Project discovery/provisioning after the redirect can take a
		// little while for a brand-new account, so give this more room
		// than ChatGPT's window.
		ExpiresIn: 15 * 60,
		Interval:  0,
	}
}

func (m *OAuthAntigravity) startPolling(_ string, expiresIn int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(expiresIn)*time.Second)
		m.cancelFunc = cancel

		token, err := m.session.Wait(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // cancelled, don't report error.
			}
			return ActionOAuthErrored{Error: err}
		}

		return ActionCompleteOAuth{Token: token}
	}
}

func (m *OAuthAntigravity) stopPolling() tea.Msg {
	if m.cancelFunc != nil {
		m.cancelFunc()
	}
	if m.session != nil {
		m.session.Close()
	}
	return nil
}
