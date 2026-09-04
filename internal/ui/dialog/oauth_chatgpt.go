package dialog

import (
	"context"
	"fmt"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-models/pkg/catwalk"
	tea "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-ui/v2"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth/codex"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/common"
)

// NewOAuthChatGPT opens the OAuth dialog driving a ChatGPT (Codex-style)
// sign-in: unlike Copilot's device code, this is a PKCE + localhost
// redirect flow, so there is no code for the user to type -- just a
// browser tab to open and wait on.
func NewOAuthChatGPT(
	com *common.Common,
	isOnboarding bool,
	provider catwalk.Provider,
	model config.SelectedModel,
	modelType config.SelectedModelType,
) (*OAuth, tea.Cmd) {
	return newOAuth(com, isOnboarding, provider, model, modelType, &OAuthChatGPT{})
}

type OAuthChatGPT struct {
	session    *codex.AuthSession
	cancelFunc func()
}

var _ OAuthProvider = (*OAuthChatGPT)(nil)

func (m *OAuthChatGPT) name() string {
	return "ChatGPT"
}

func (m *OAuthChatGPT) initiateAuth() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session, err := codex.Start(ctx)
	if err != nil {
		return ActionOAuthErrored{Error: fmt.Errorf("failed to start ChatGPT sign-in: %w", err)}
	}
	m.session = session

	return ActionInitiateOAuth{
		// No device code: this is a PKCE/redirect flow, so the dialog
		// shows only the sign-in link instead of a code box.
		DeviceCode:      "",
		UserCode:        "",
		VerificationURL: session.AuthURL(),
		// A generous window to complete sign-in in the browser.
		ExpiresIn: 10 * 60,
		Interval:  0,
	}
}

func (m *OAuthChatGPT) startPolling(_ string, expiresIn int) tea.Cmd {
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

func (m *OAuthChatGPT) stopPolling() tea.Msg {
	if m.cancelFunc != nil {
		m.cancelFunc()
	}
	if m.session != nil {
		m.session.Close()
	}
	return nil
}
