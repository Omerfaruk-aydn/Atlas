package dialog

import (
	"context"
	"fmt"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-models/pkg/catwalk"
	tea "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-ui/v2"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth/claude"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/common"
)

// NewOAuthClaude opens the OAuth dialog driving a Claude Pro/Max
// subscription sign-in. Like ChatGPT's, this is a PKCE + localhost
// redirect flow rather than a device code, so there is nothing for the
// user to type -- just a browser tab to open and wait on.
//
// Unlike `atlas login claude`, this dialog always runs the browser flow
// and never imports an existing Claude Code credential: the CLI keeps
// that shortcut, while picking a Claude model in the TUI is an explicit
// request to authenticate.
func NewOAuthClaude(
	com *common.Common,
	isOnboarding bool,
	provider catwalk.Provider,
	model config.SelectedModel,
	modelType config.SelectedModelType,
) (*OAuth, tea.Cmd) {
	return newOAuth(com, isOnboarding, provider, model, modelType, &OAuthClaude{})
}

type OAuthClaude struct {
	session    *claude.AuthSession
	cancelFunc func()
}

var _ OAuthProvider = (*OAuthClaude)(nil)

func (m *OAuthClaude) name() string {
	return "Claude"
}

func (m *OAuthClaude) initiateAuth() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session, err := claude.Start(ctx)
	if err != nil {
		return ActionOAuthErrored{Error: fmt.Errorf("failed to start Claude sign-in: %w", err)}
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

func (m *OAuthClaude) startPolling(_ string, expiresIn int) tea.Cmd {
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

func (m *OAuthClaude) stopPolling() tea.Msg {
	if m.cancelFunc != nil {
		m.cancelFunc()
	}
	if m.session != nil {
		m.session.Close()
	}
	return nil
}
