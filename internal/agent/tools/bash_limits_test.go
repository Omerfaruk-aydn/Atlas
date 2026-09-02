package tools

import (
	"strings"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/stretchr/testify/require"
)

// The zero value has to be usable: it means "the built-in defaults", not
// "truncate everything and background immediately".
func TestZeroLimitsMeanTheDefaults(t *testing.T) {
	var l BashLimits
	require.Equal(t, DefaultAutoBackgroundAfter, l.autoBackgroundAfter(0))

	content := strings.Repeat("x", MaxOutputLength+100)
	require.Len(t, TruncateOutputTo(content, 0), len(TruncateOutput(content)))
}

func TestConfiguredAutoBackgroundThreshold(t *testing.T) {
	l := BashLimits{AutoBackgroundAfter: 5}
	require.Equal(t, 5, l.autoBackgroundAfter(0))
}

// The model knows which command it just wrote, so an explicit per-call value
// wins over the workspace setting.
func TestPerCallThresholdWinsOverTheConfiguredOne(t *testing.T) {
	l := BashLimits{AutoBackgroundAfter: 5}
	require.Equal(t, 120, l.autoBackgroundAfter(120))
}

func TestTruncateOutputToRespectsTheGivenWidth(t *testing.T) {
	content := strings.Repeat("x", 500)

	require.Equal(t, content, TruncateOutputTo(content, 500))

	got := TruncateOutputTo(content, 100)
	require.Less(t, len(got), len(content))
	require.Contains(t, got, "lines truncated")
}

func TestNewBashLimitsReadsTheToolsSection(t *testing.T) {
	require.Equal(t, BashLimits{}, NewBashLimits(nil))
	require.Equal(t, BashLimits{}, NewBashLimits(&config.Config{}))

	maxOutput, after := 1234, 30
	got := NewBashLimits(&config.Config{Tools: config.Tools{Bash: config.ToolBash{
		MaxOutputLength:     &maxOutput,
		AutoBackgroundAfter: &after,
	}}})
	require.Equal(t, BashLimits{MaxOutputLength: 1234, AutoBackgroundAfter: 30}, got)
}

// The description tells the model how much output it will get back, so a
// configured limit has to reach it.
func TestBashDescriptionReportsTheConfiguredOutputLimit(t *testing.T) {
	var attribution config.Attribution
	require.Contains(t, bashDescription(&attribution, "m", CommandPolicy{}, BashLimits{MaxOutputLength: 4242}), "4242")
	require.Contains(t, bashDescription(&attribution, "m", CommandPolicy{}, BashLimits{}), "30000")
}
