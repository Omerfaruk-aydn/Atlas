package dialog

import (
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/commands"
	tea "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-ui/v2"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/common"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/styles"
	"github.com/stretchr/testify/require"
)

func newArgumentsDialog(t *testing.T, args []commands.Argument) *Arguments {
	t.Helper()
	sty := styles.AtlasPantera()
	return NewArguments(&common.Common{Styles: &sty}, "Test", "", args, ActionSaveModelRole{})
}

// TestArgumentsSetValuesIsRealSubmittableText verifies SetValues sets
// actual field text (submitted even if the user never touches the
// field), unlike Placeholder which is a hint that submits blank if
// left alone -- the distinction an edit form depends on.
func TestArgumentsSetValuesIsRealSubmittableText(t *testing.T) {
	args := []commands.Argument{
		{ID: "provider", Title: "Provider", Required: true},
		{ID: "model", Title: "Model", Required: true},
	}
	a := newArgumentsDialog(t, args)
	a.SetValues(map[string]string{"provider": "openai", "model": "gpt-4o"})

	require.Equal(t, "openai", a.inputs[0].Value())
	require.Equal(t, "gpt-4o", a.inputs[1].Value())

	// Confirming without touching either field must submit the
	// pre-filled values, not blanks.
	a.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter}) // advance off field 0
	action := a.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	saved, ok := action.(ActionSaveModelRole)
	require.True(t, ok, "expected ActionSaveModelRole, got %T", action)
	require.Equal(t, "openai", saved.Args["provider"])
	require.Equal(t, "gpt-4o", saved.Args["model"])
}

func TestArgumentsSetValuesIgnoresUnknownIDs(t *testing.T) {
	args := []commands.Argument{{ID: "name", Title: "Name"}}
	a := newArgumentsDialog(t, args)

	require.NotPanics(t, func() {
		a.SetValues(map[string]string{"unrelated": "x"})
	})
	require.Empty(t, a.inputs[0].Value())
}
