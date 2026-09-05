package version

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
)

// Build-time parameters set via -ldflags.

// defaultVersion is what Version holds when no -ldflags set it, and so
// the signal that the build info below may fill it in.
const defaultVersion = "devel"

var (
	Version = defaultVersion
	Commit  = "unknown"
	// BuildID is a unique identifier for this build. For release builds it
	// equals Commit; for development builds (go run / go build without
	// ldflags) it is derived from the executable's modification time, which
	// changes on every recompilation.
	BuildID = ""
)

// A user may install Atlas-Agent using `go install github.com/Omerfaruk-aydn/Atlas-Agent@latest`
// without -ldflags, in which case the version above is unset. As a workaround
// we use the embedded build version that *is* set when using `go install` (and
// is only set for `go install` and not for `go build`).
//
// This only ever fills in a version; it never replaces one. Go also
// synthesizes info.Main.Version from VCS state, so a build made from a
// tagged but dirty tree reports something like "v0.9.18+dirty" -- and
// letting that win over -ldflags meant a release binary could disagree
// with the tag it was cut from, and that `atlas-agent update` then
// refused to run because the string contains "dirty".
func init() {
	if Version == defaultVersion {
		if info, ok := debug.ReadBuildInfo(); ok {
			mainVersion := info.Main.Version
			if mainVersion != "" && mainVersion != "(devel)" {
				Version = mainVersion
			}
		}
	}

	// Derive BuildID when not set via ldflags.
	if BuildID == "" {
		BuildID = deriveBuildID()
	}
}

// deriveBuildID uses the running executable's modification time as a unique
// build fingerprint. This changes on every recompilation (including `go run`),
// making it reliable for detecting stale servers during development.
func deriveBuildID() string {
	exe, err := os.Executable()
	if err != nil {
		return "unknown"
	}
	fi, err := os.Stat(exe)
	if err != nil {
		return "unknown"
	}
	return strconv.FormatInt(fi.ModTime().UnixNano(), 36)
}

// BinaryName returns the command name to use in user-facing hints, such as
// the resume line printed on exit.
//
// It cannot be a constant. The published command is atlas-agent, a local
// build is usually atlas, and the npm wrapper launches a file named after
// the platform (atlas-agent-windows-x64.exe) — so a hardcoded name is
// wrong for someone. A resume hint naming a command that does not exist is
// worse than no hint at all, which is what "atlas -s <id>" was for every
// npm install.
//
// The launcher states the name it was invoked as; otherwise the running
// executable's own name is the best available answer.
func BinaryName() string {
	if v, ok := os.LookupEnv(invokedAsEnv); ok && v != "" {
		return v
	}
	name := filepath.Base(os.Args[0])
	if ext := filepath.Ext(name); strings.EqualFold(ext, ".exe") {
		name = strings.TrimSuffix(name, ext)
	}
	if name == "" || name == "." || name == string(filepath.Separator) {
		return defaultBinaryName
	}
	return name
}

const (
	// invokedAsEnv lets a wrapper script report the name the user actually
	// typed, which the process it spawns cannot otherwise know.
	invokedAsEnv = "ATLAS_AGENT_INVOKED_AS"

	defaultBinaryName = "atlas-agent"
)
