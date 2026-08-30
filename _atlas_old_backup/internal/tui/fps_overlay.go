package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// FpsOverlay is a HERMES_TUI_FPS=1 dev-only overlay that paints the
// current frame rate in the top-right corner. Color thresholds
// (mirroring Hermes's): ≥50fps = good, ≥30fps = warn, else error.
type FpsOverlay struct {
	enabled    bool
	lastFrames []time.Time
}

// newFpsOverlay returns an overlay; the enabled flag is read from
// HERMES_TUI_FPS at construction time. With env unset, this is a
// zero-cost no-op (the View method returns "").
func newFpsOverlay() *FpsOverlay {
	return &FpsOverlay{enabled: fpsEnvEnabled()}
}

// fpsEnvEnabled checks HERMES_TUI_FPS=1 to gate the dev overlay.
// Cheap to call — the env lookup is O(env-vars).
func fpsEnvEnabled() bool {
	for _, e := range envSnapshot() {
		if strings.HasPrefix(e, "HERMES_TUI_FPS=") {
			v := strings.ToLower(strings.TrimPrefix(e, "HERMES_TUI_FPS="))
			return v == "1" || v == "true" || v == "yes" || v == "on"
		}
	}
	return false
}

// RecordFrame is called from the App's render tick. Maintains a
// 60-frame rolling window and computes the average FPS.
func (f *FpsOverlay) RecordFrame(now time.Time) {
	if !f.enabled {
		return
	}
	f.lastFrames = append(f.lastFrames, now)
	if len(f.lastFrames) > 60 {
		f.lastFrames = f.lastFrames[len(f.lastFrames)-60:]
	}
}

// fps computes the rolling-average frames per second.
func (f *FpsOverlay) fps() float64 {
	if len(f.lastFrames) < 2 {
		return 0
	}
	span := f.lastFrames[len(f.lastFrames)-1].Sub(f.lastFrames[0])
	if span <= 0 {
		return 0
	}
	return float64(len(f.lastFrames)-1) / span.Seconds()
}

// View returns the rendered FPS chip, or "" when disabled.
func (f *FpsOverlay) View(a *App) string {
	if !f.enabled {
		return ""
	}
	fps := f.fps()
	style := a.theme.HelpText
	switch {
	case fps >= 50:
		style = lipgloss.NewStyle().Foreground(a.theme.Success)
	case fps >= 30:
		style = lipgloss.NewStyle().Foreground(a.theme.StatusWarn)
	default:
		style = a.theme.ErrorText
	}
	return style.Render(fmt.Sprintf("%.0f fps", fps))
}

// envSnapshot is a tiny os.Environ wrapper so fps_overlay_test can
// swap the env without touching the process state.
func envSnapshot() []string {
	return osEnviron()
}
