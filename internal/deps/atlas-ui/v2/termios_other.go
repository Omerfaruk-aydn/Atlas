//go:build !windows && !darwin && !dragonfly && !freebsd && !linux && !solaris && !aix
// +build !windows,!darwin,!dragonfly,!freebsd,!linux,!solaris,!aix

package tea

import "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-term"

func (*Program) checkOptimizedMovements(*term.State) {}
