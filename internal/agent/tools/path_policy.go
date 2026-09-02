package tools

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
)

// PathPolicy decides whether a tool may write to a path at all, before any
// permission is even requested.
//
// It is off unless a workspace turns it on: refusing writes outside the
// working directory would break the ordinary case of editing a file in a
// sibling checkout, and that decision belongs to the person configuring the
// workspace, not to this package.
type PathPolicy struct {
	// Root is the directory writes are confined to. Empty disables the
	// check however Restrict is set -- confining writes to nowhere in
	// particular would refuse everything.
	Root string
	// Restrict turns the confinement on.
	Restrict bool
}

// NewPathPolicy reads the policy out of the config's options.
func NewPathPolicy(cfg *config.Config, workingDir string) PathPolicy {
	if cfg == nil || cfg.Options == nil {
		return PathPolicy{}
	}
	return PathPolicy{Root: workingDir, Restrict: cfg.Options.RestrictWritesToWorkingDir}
}

// Check reports whether path may be written. path is expected to be
// absolute already -- every writing tool joins it against the working
// directory first -- but a relative one is resolved against Root rather than
// silently passing.
func (p PathPolicy) Check(path string) error {
	if !p.Restrict || p.Root == "" {
		return nil
	}

	root, err := filepath.Abs(p.Root)
	if err != nil {
		return fmt.Errorf("resolving the working directory %q: %w", p.Root, err)
	}
	target := path
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolving %q: %w", path, err)
	}

	rel, err := filepath.Rel(root, target)
	if err != nil {
		// Different volumes on Windows: not relative to the root at
		// all, so certainly outside it.
		return outsideErr(path, root)
	}
	// ".." alone, or any path starting with a ".." segment, has climbed
	// out. filepath.Rel has already cleaned the result, so a ".." further
	// along cannot appear.
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return outsideErr(path, root)
	}
	return nil
}

func outsideErr(path, root string) error {
	return fmt.Errorf("%s is outside the working directory %s, which this workspace does not allow writing to "+
		"(unset restrict_writes_to_working_dir to allow it)", path, root)
}
