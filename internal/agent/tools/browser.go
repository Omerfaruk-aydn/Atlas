package tools

import (
	"context"
	_ "embed"
	"fmt"
	"slices"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/browser"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/permission"
)

const BrowserToolName = "browser"

//go:embed browser.md
var browserDescription string

// browserActions lists every value BrowserParams.Action accepts.
var browserActions = []string{
	"navigate", "click", "type", "key", "eval", "text", "html", "screenshot", "url", "close",
}

// browserReadOnlyActions are the actions that only observe the current page
// (or the manager's bookkeeping) rather than changing what's loaded or
// submitting anything -- these get Safe: true, the same way bash marks its
// read-only command allowlist, so ModeAutoAcceptEdits doesn't stop to ask
// about them while ModePlan still denies the tool outright.
var browserReadOnlyActions = map[string]bool{
	"text":       true,
	"html":       true,
	"screenshot": true,
	"url":        true,
	"close":      true,
}

type BrowserParams struct {
	Action string `json:"action" description:"One of: navigate, click, type, key, eval, text, html, screenshot, url, close. See the tool description for what each needs and returns."`
	// URL is required for navigate.
	URL string `json:"url,omitempty" description:"Destination for the navigate action. Must start with http:// or https://."`
	// Selector is required for click, type, text, html.
	Selector string `json:"selector,omitempty" description:"CSS selector identifying the target element, for click, type, text, and html."`
	// Text is required for type.
	Text string `json:"text,omitempty" description:"Text to type into the element identified by selector, for the type action. Replaces whatever was already in the field."`
	// Key is required for key.
	Key string `json:"key,omitempty" description:"Named key to send to the focused element, for the key action: enter, tab, escape, backspace, delete, arrowup, arrowdown, arrowleft, arrowright."`
	// Script is required for eval.
	Script string `json:"script,omitempty" description:"JavaScript expression to evaluate in the page, for the eval action. The expression's value is returned as JSON."`
	// FullPage only applies to screenshot.
	FullPage bool `json:"full_page,omitempty" description:"For the screenshot action, capture the full scrollable page instead of just the visible viewport."`
}

type BrowserPermissionsParams struct {
	Action   string `json:"action"`
	URL      string `json:"url,omitempty"`
	Selector string `json:"selector,omitempty"`
	Text     string `json:"text,omitempty"`
	Key      string `json:"key,omitempty"`
	Script   string `json:"script,omitempty"`
	FullPage bool   `json:"full_page,omitempty"`
}

type BrowserResponseMetadata struct {
	Action string `json:"action"`
	URL    string `json:"url,omitempty"`
}

// browserSessions is the seam NewBrowserTool depends on instead of
// *browser.Manager directly, so tests can supply a fake session without
// launching a real browser.
type browserSessions interface {
	Session(id string) (browser.Session, error)
	Close(id string)
}

func NewBrowserTool(permissions permission.Service, workingDir string, cfg config.ToolBrowser) fantasy.AgentTool {
	manager := browser.GetManager(browser.Options{
		ExecutablePath: cfg.ExecutablePath,
		Headless:       cfg.IsHeadless(),
		ActionTimeout:  cfg.GetActionTimeout(),
		IdleTimeout:    cfg.GetIdleTimeout(),
	})
	return newBrowserTool(permissions, workingDir, manager)
}

func newBrowserTool(permissions permission.Service, workingDir string, sessions browserSessions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		BrowserToolName,
		browserDescription,
		func(ctx context.Context, params BrowserParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			action := strings.ToLower(strings.TrimSpace(params.Action))
			if !slices.Contains(browserActions, action) {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("unknown action %q, must be one of: %s", params.Action, strings.Join(browserActions, ", "))), nil
			}

			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, fmt.Errorf("session ID is required for using the browser")
			}

			p, err := permissions.Request(
				ctx,
				permission.CreatePermissionRequest{
					SessionID:   sessionID,
					Path:        workingDir,
					ToolCallID:  call.ID,
					ToolName:    BrowserToolName,
					Action:      action,
					Description: browserActionDescription(action, params),
					Params:      BrowserPermissionsParams(params),
					Safe:        browserReadOnlyActions[action],
				},
			)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !p {
				return NewPermissionDeniedResponse(permissions), nil
			}

			if action == "close" {
				sessions.Close(sessionID)
				return fantasy.NewTextResponse("Browser session closed."), nil
			}

			sess, err := sessions.Session(sessionID)
			if err != nil {
				return fantasy.NewTextErrorResponse("failed to start browser: " + err.Error()), nil
			}

			return runBrowserAction(sess, action, params)
		},
	)
}

func browserActionDescription(action string, params BrowserParams) string {
	switch action {
	case "navigate":
		return "Navigate browser to: " + params.URL
	case "click":
		return "Click browser element: " + params.Selector
	case "type":
		return "Type into browser element: " + params.Selector
	case "key":
		return "Send browser key press: " + params.Key
	case "eval":
		return "Run JavaScript in browser: " + params.Script
	case "text", "html":
		return fmt.Sprintf("Read browser element %s: %s", action, params.Selector)
	case "screenshot":
		return "Take browser screenshot"
	case "url":
		return "Read current browser URL"
	case "close":
		return "Close browser session"
	default:
		return "Browser action: " + action
	}
}

func runBrowserAction(sess browser.Session, action string, params BrowserParams) (fantasy.ToolResponse, error) {
	metadata := BrowserResponseMetadata{Action: action}

	switch action {
	case "navigate":
		if params.URL == "" {
			return fantasy.NewTextErrorResponse("url is required for the navigate action"), nil
		}
		if !strings.HasPrefix(params.URL, "http://") && !strings.HasPrefix(params.URL, "https://") {
			return fantasy.NewTextErrorResponse("url must start with http:// or https://"), nil
		}
		if err := sess.Navigate(params.URL); err != nil {
			return fantasy.NewTextErrorResponse("navigate failed: " + err.Error()), nil
		}
		metadata.URL = params.URL
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse("Navigated to "+params.URL), metadata), nil

	case "click":
		if params.Selector == "" {
			return fantasy.NewTextErrorResponse("selector is required for the click action"), nil
		}
		if err := sess.Click(params.Selector); err != nil {
			return fantasy.NewTextErrorResponse("click failed: " + err.Error()), nil
		}
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse("Clicked "+params.Selector), metadata), nil

	case "type":
		if params.Selector == "" {
			return fantasy.NewTextErrorResponse("selector is required for the type action"), nil
		}
		if err := sess.Type(params.Selector, params.Text); err != nil {
			return fantasy.NewTextErrorResponse("type failed: " + err.Error()), nil
		}
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse("Typed into "+params.Selector), metadata), nil

	case "key":
		if params.Key == "" {
			return fantasy.NewTextErrorResponse("key is required for the key action"), nil
		}
		if err := sess.PressKey(strings.ToLower(params.Key)); err != nil {
			return fantasy.NewTextErrorResponse("key press failed: " + err.Error()), nil
		}
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse("Sent key "+params.Key), metadata), nil

	case "eval":
		if params.Script == "" {
			return fantasy.NewTextErrorResponse("script is required for the eval action"), nil
		}
		result, err := sess.Eval(params.Script)
		if err != nil {
			return fantasy.NewTextErrorResponse("eval failed: " + err.Error()), nil
		}
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse(result), metadata), nil

	case "text":
		if params.Selector == "" {
			return fantasy.NewTextErrorResponse("selector is required for the text action"), nil
		}
		result, err := sess.Text(params.Selector)
		if err != nil {
			return fantasy.NewTextErrorResponse("text failed: " + err.Error()), nil
		}
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse(result), metadata), nil

	case "html":
		if params.Selector == "" {
			return fantasy.NewTextErrorResponse("selector is required for the html action"), nil
		}
		result, err := sess.HTML(params.Selector)
		if err != nil {
			return fantasy.NewTextErrorResponse("html failed: " + err.Error()), nil
		}
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse(result), metadata), nil

	case "screenshot":
		data, err := sess.Screenshot(params.FullPage)
		if err != nil {
			return fantasy.NewTextErrorResponse("screenshot failed: " + err.Error()), nil
		}
		return fantasy.NewImageResponse(data, "image/png"), nil

	case "url":
		result, err := sess.URL()
		if err != nil {
			return fantasy.NewTextErrorResponse("url failed: " + err.Error()), nil
		}
		metadata.URL = result
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse(result), metadata), nil

	default:
		return fantasy.NewTextErrorResponse("unknown action: " + action), nil
	}
}
