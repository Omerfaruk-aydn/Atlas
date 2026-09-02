package event

import "time"

// AppInitialized is a no-op.
func AppInitialized() {
	_ = time.Now()
}

// AppExited is a no-op.
func AppExited() {}

// SessionCreated is a no-op.
func SessionCreated() {}

// SessionDeleted is a no-op.
func SessionDeleted() {}

// SessionSwitched is a no-op.
func SessionSwitched() {}

// FilePickerOpened is a no-op.
func FilePickerOpened() {}

// PromptSent is a no-op.
func PromptSent(...any) {}

// PromptResponded is a no-op.
func PromptResponded(...any) {}

// TokensUsed is a no-op.
func TokensUsed(...any) {}

// StatsViewed is a no-op.
func StatsViewed() {}

// SessionListed is a no-op.
func SessionListed(bool) {}

// SessionShown is a no-op.
func SessionShown(bool) {}

// SessionLastShown is a no-op.
func SessionLastShown(bool) {}

// SessionDeletedCommand is a no-op.
func SessionDeletedCommand(bool) {}

// SessionRenamed is a no-op.
func SessionRenamed(bool) {}

// SessionTagged is a no-op.
func SessionTagged(bool) {}
