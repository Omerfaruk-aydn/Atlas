//go:build windows
// +build windows

package tea

import "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-term"

func (p *Program) checkOptimizedMovements(*term.State) {
	p.useHardTabs = true
	p.useBackspace = true
}
