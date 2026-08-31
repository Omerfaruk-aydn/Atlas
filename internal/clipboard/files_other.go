//go:build !windows

package clipboard

// Only Windows has a dedicated file-list clipboard format. Elsewhere a file
// manager's copy lands on the clipboard as text — a path or a file:// URI —
// which callers already handle through [FormatText].
func readFiles() ([]string, error) {
	return nil, ErrEmpty
}
