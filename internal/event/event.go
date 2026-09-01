// Package event is a no-op telemetry stub.
//
// Atlas Agent does not collect any usage data. All event-tracking functions
// in this package are kept as no-op shims to preserve the public API used
// throughout the codebase, but they perform no work and send nothing.
package event

// Init is a no-op.
func Init() {}

// Flush is a no-op.
func Flush() {}

// GetID always returns the empty string.
func GetID() string { return "" }

// Alias is a no-op.
func Alias(string) {}

// Error is a no-op.
func Error(any, ...any) {}

// SetNonInteractive is a no-op.
func SetNonInteractive(bool) {}

// SetContinueBySessionID is a no-op.
func SetContinueBySessionID(bool) {}

// SetContinueLastSession is a no-op.
func SetContinueLastSession(bool) {}
