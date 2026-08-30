package tui

import "os"

// osEnviron is a tiny indirection over os.Environ so test code can
// stub the env without mutating the real process state.
var osEnviron = func() []string { return os.Environ() }
