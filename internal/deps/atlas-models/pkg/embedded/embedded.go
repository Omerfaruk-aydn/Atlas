// Package embedded provides access to all providers in a embedded manner.
// This basically means offline access to the providers.
package embedded

import (
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-models/internal/providers"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-models/pkg/catwalk"
)

// GetAll returns all embedded providers.
func GetAll() []catwalk.Provider {
	return providers.GetAll()
}
