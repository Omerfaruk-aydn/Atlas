//go:build !darwin

package notification

import (
	_ "embed"
)

//go:embed atlas-icon-solo.png
var Icon []byte
