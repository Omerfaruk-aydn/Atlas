package tools

import (
	"bytes"
	"context"
	"html/template"
	"os/exec"
	"testing"

	"charm.land/fantasy"
	"github.com/maincodss/atlas-agent/internal/permission"
)

type (
	sessionIDContextKey string
	messageIDContextKey string
	supportsImagesKey   string
	modelNameKey        string
)

const (
	// SessionIDContextKey is the key for the session ID in the context.
	SessionIDContextKey sessionIDContextKey = "session_id"
	// MessageIDContextKey is the key for the message ID in the context.
	MessageIDContextKey messageIDContextKey = "message_id"
	// SupportsImagesContextKey is the key for the model's image support capability.
	SupportsImagesContextKey supportsImagesKey = "supports_images"
	// ModelNameContextKey is the key for the model name in the context.
	ModelNameContextKey modelNameKey = "model_name"
)

// getContextValue is a generic helper that retrieves a typed value from context.
// If the value is not found or has the wrong type, it returns the default value.
func getContextValue[T any](ctx context.Context, key any, defaultValue T) T {
	value := ctx.Value(key)
	if value == nil {
		return defaultValue
	}
	if typedValue, ok := value.(T); ok {
		return typedValue
	}
	return defaultValue
}

// GetSessionFromContext retrieves the session ID from the context.
func GetSessionFromContext(ctx context.Context) string {
	return getContextValue(ctx, SessionIDContextKey, "")
}

// GetMessageFromContext retrieves the message ID from the context.
func GetMessageFromContext(ctx context.Context) string {
	return getContextValue(ctx, MessageIDContextKey, "")
}

// GetSupportsImagesFromContext retrieves whether the model supports images from the context.
func GetSupportsImagesFromContext(ctx context.Context) bool {
	return getContextValue(ctx, SupportsImagesContextKey, false)
}

// GetModelNameFromContext retrieves the model name from the context.
func GetModelNameFromContext(ctx context.Context) string {
	return getContextValue(ctx, ModelNameContextKey, "")
}

// NewPermissionDeniedResponse returns a tool response indicating the call
// was denied.
//
// For a real user denial it sets StopTurn so the agent loop does not retry:
// the user said no, and the turn is over.
//
// ModePlan is deliberately the opposite. The denial there is automatic, and
// the model's correct next move is to call "exit_plan_mode" to ask the user
// to leave plan mode — which it cannot do if the turn ends the moment the
// tool result lands. So plan mode leaves StopTurn false and spends the tool
// result itself on the instructions, which weaker models follow far more
// reliably than an earlier system reminder. Retries are cheap (plan mode
// denies without a dialog) and the loop's step limit still bounds them.
func NewPermissionDeniedResponse(perms permission.Service) fantasy.ToolResponse {
	if perms != nil && perms.Mode() == permission.ModePlan {
		return fantasy.NewTextErrorResponse(
			"Denied: you are in PLAN MODE (read-only), so the system auto-denied this call — the user did not reject it, and retrying any write/edit/execute tool will be denied the same way.\n\n" +
				"Your next action must be one of these two, in this turn:\n" +
				"1. If you have already presented your plan to the user, call the \"exit_plan_mode\" tool now, passing your plan as the \"plan\" argument. That asks the user to approve leaving plan mode; if they approve, plan mode turns off and you can do the real work.\n" +
				"2. If you have not presented a plan yet, write the plan out as text first, then call \"exit_plan_mode\".",
		)
	}
	resp := fantasy.NewTextErrorResponse("User denied permission")
	resp.StopTurn = true
	return resp
}

// ghAvailable indicates whether the `gh` CLI is available on PATH.
var ghAvailable = func() bool {
	if testing.Testing() {
		return false
	}
	_, err := exec.LookPath("gh")
	return err == nil
}()

// toolDescriptionData is the common data structure for tool description templates.
type toolDescriptionData struct {
	GhAvailable bool
}

// renderToolDescription renders a tool description template with the given data.
func renderToolDescription(tmpl *template.Template) string {
	data := toolDescriptionData{
		GhAvailable: ghAvailable,
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		panic("failed to execute tool description template: " + err.Error())
	}
	return out.String()
}

// renderTemplate renders a Go template with the given data.
func renderTemplate(tmpl *template.Template, data any) string {
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		panic("failed to execute tool description template: " + err.Error())
	}
	return out.String()
}
