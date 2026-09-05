package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/clipboard"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-style/v2"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth/antigravity"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth/codex"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/oauth/copilot"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/workspace"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Aliases: []string{"auth"},
	Use:     "login [platform]",
	Short:   "Login ATLAS-AGENT to a platform",
	Long: `Login ATLAS-AGENT to a specified platform.
	The platform should be provided as an argument.
	Available platforms are: copilot, chatgpt, antigravity.

	Note: antigravity only supports its Gemini-family models
	(gemini-3-pro-high/low); Claude and GPT-OSS models served through an
	Antigravity account are not supported.`,
	Example: `
# Authenticate with GitHub Copilot
atlas login copilot

# Authenticate with a ChatGPT subscription
atlas login chatgpt

# Authenticate with a Google Antigravity (AI Pro/Ultra) account
atlas login antigravity

# Force re-authentication even if already logged in
atlas login -f copilot
  `,
	ValidArgs: []cobra.Completion{
		"copilot",
		"github",
		"github-copilot",
		"chatgpt",
		"codex",
		"antigravity",
	},
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, cleanup, err := setupWorkspaceWithProgressBar(cmd)
		if err != nil {
			return err
		}
		defer cleanup()

		provider := "copilot"
		if len(args) > 0 {
			provider = args[0]
		}
		force, _ := cmd.Flags().GetBool("force")
		switch provider {
		case "copilot", "github", "github-copilot":
			return loginCopilot(ws, force)
		case "chatgpt", "codex":
			return loginChatGPT(ws, force)
		case "antigravity":
			return loginAntigravity(ws, force)
		default:
			return fmt.Errorf("unknown platform: %s", args[0])
		}
	},
}

func init() {
	loginCmd.Flags().BoolP("force", "f", false, "Force re-authentication even if already logged in")
}

func loginCopilot(ws workspace.Workspace, force bool) error {
	loginCtx := getLoginContext()

	if !force {
		cfg := ws.Config()
		if cfg != nil {
			if pc, ok := cfg.Providers.Get("copilot"); ok && pc.OAuthToken != nil {
				fmt.Println("You are already logged in to GitHub Copilot.")
				fmt.Println("Use --force to re-authenticate.")
				return nil
			}
		}
	}

	diskToken, hasDiskToken := copilot.RefreshTokenFromDisk()
	var token *oauth.Token

	switch {
	case hasDiskToken:
		fmt.Println("Found existing GitHub Copilot token on disk. Using it to authenticate...")

		t, err := copilot.RefreshToken(loginCtx, diskToken)
		if err != nil {
			return fmt.Errorf("unable to refresh token from disk: %w", err)
		}
		token = t
	default:
		fmt.Println("Requesting device code from GitHub...")
		dc, err := copilot.RequestDeviceCode(loginCtx)
		if err != nil {
			return err
		}

		clipboard.WriteText(dc.UserCode)
		fmt.Println()
		fmt.Println("The following code should be on clipboard already:")
		fmt.Println()
		lipgloss.Println(lipgloss.NewStyle().Bold(true).Render(dc.UserCode))
		fmt.Println()
		fmt.Println("Press enter to open this URL and authenticate with GitHub Copilot:")
		fmt.Println()
		lipgloss.Println(lipgloss.NewStyle().Hyperlink(dc.VerificationURI, "id=copilot").Render(dc.VerificationURI))
		fmt.Println()
		waitEnter()
		if err := browser.OpenURL(dc.VerificationURI); err != nil {
			fmt.Println("Could not open the URL. You'll need to manually open the URL in your browser.")
		}

		fmt.Println("Waiting for authorization...")

		t, err := copilot.PollForToken(loginCtx, dc)
		if err == copilot.ErrNotAvailable {
			fmt.Println()
			fmt.Println("GitHub Copilot is unavailable for this account. To signup, go to the following page:")
			fmt.Println()
			lipgloss.Println(lipgloss.NewStyle().Hyperlink(copilot.SignupURL, "id=copilot-signup").Render(copilot.SignupURL))
			fmt.Println()
			fmt.Println("You may be able to request free access if eligible. For more information, see:")
			fmt.Println()
			lipgloss.Println(lipgloss.NewStyle().Hyperlink(copilot.FreeURL, "id=copilot-free").Render(copilot.FreeURL))
		}
		if err != nil {
			return err
		}
		token = t
	}

	if err := ws.SetProviderAPIKey(config.ScopeGlobal, "copilot", token); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("You're now authenticated with GitHub Copilot!")
	return nil
}

func loginChatGPT(ws workspace.Workspace, force bool) error {
	loginCtx := getLoginContext()

	if !force {
		cfg := ws.Config()
		if cfg != nil {
			if pc, ok := cfg.Providers.Get("chatgpt"); ok && pc.OAuthToken != nil {
				fmt.Println("You are already logged in to ChatGPT.")
				fmt.Println("Use --force to re-authenticate.")
				return nil
			}
		}
	}

	session, err := codex.Start(loginCtx)
	if err != nil {
		return fmt.Errorf("failed to start ChatGPT sign-in: %w", err)
	}

	fmt.Println("Press enter to open your browser and sign in with ChatGPT:")
	fmt.Println()
	lipgloss.Println(lipgloss.NewStyle().Hyperlink(session.AuthURL(), "id=chatgpt").Render(session.AuthURL()))
	fmt.Println()
	waitEnter()
	if err := browser.OpenURL(session.AuthURL()); err != nil {
		fmt.Println("Could not open the URL. You'll need to manually open the URL in your browser.")
	}

	fmt.Println("Waiting for authorization...")
	token, err := session.Wait(loginCtx)
	if err != nil {
		return err
	}

	if err := ws.SetProviderAPIKey(config.ScopeGlobal, "chatgpt", token); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("You're now authenticated with ChatGPT!")
	return nil
}

func loginAntigravity(ws workspace.Workspace, force bool) error {
	loginCtx := getLoginContext()

	if !force {
		cfg := ws.Config()
		if cfg != nil {
			if pc, ok := cfg.Providers.Get("antigravity"); ok && pc.OAuthToken != nil {
				fmt.Println("You are already logged in to Antigravity.")
				fmt.Println("Use --force to re-authenticate.")
				return nil
			}
		}
	}

	fmt.Println("Note: only Antigravity's Gemini-family models are supported; Claude and GPT-OSS models served through an Antigravity account are not.")
	fmt.Println()

	session, err := antigravity.Start(loginCtx)
	if err != nil {
		return fmt.Errorf("failed to start Antigravity sign-in: %w", err)
	}

	fmt.Println("Press enter to open your browser and sign in with your Google account:")
	fmt.Println()
	lipgloss.Println(lipgloss.NewStyle().Hyperlink(session.AuthURL(), "id=antigravity").Render(session.AuthURL()))
	fmt.Println()
	waitEnter()
	if err := browser.OpenURL(session.AuthURL()); err != nil {
		fmt.Println("Could not open the URL. You'll need to manually open the URL in your browser.")
	}

	fmt.Println("Waiting for authorization...")
	// Bounded on top of getLoginContext's signal-only cancellation: a
	// network-level stall here (proxy, AV, DNS) would otherwise hang with
	// no feedback at all instead of failing with a clear timeout. 10
	// minutes rather than 5: project provisioning has no fixed attempt
	// cap of its own anymore (see antigravity.discoverProject) and backs
	// off up to 60s between retries while Google's onboarding backend
	// reports itself busy, so it needs real room to ride that out.
	waitCtx, waitCancel := context.WithTimeout(loginCtx, 10*time.Minute)
	defer waitCancel()
	token, err := session.WaitWithProgress(waitCtx, func(msg string) {
		fmt.Println(msg)
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("timed out waiting for Google sign-in to complete: %w", err)
		}
		return err
	}

	if err := ws.SetProviderAPIKey(config.ScopeGlobal, "antigravity", token); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("You're now authenticated with Antigravity!")
	return nil
}

func getLoginContext() context.Context {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	go func() {
		<-ctx.Done()
		cancel()
		os.Exit(1)
	}()
	return ctx
}

func waitEnter() {
	_, _ = fmt.Scanln()
}
