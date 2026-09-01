package fsext

import "os"

// PreferExisting chooses between two spellings of the same path: the current
// one when it is present, the legacy one when it is the only one there, and
// the current one when neither exists so that whatever gets created lands on
// the current name.
//
// It exists because this program was renamed and the names it writes to disk
// carried the old one. Reading both, and moving nothing, is what keeps an
// installation made before the rename working after it.
func PreferExisting(current, legacy string) string {
	if current == legacy {
		return current
	}
	if _, err := os.Stat(current); err == nil {
		return current
	}
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return current
}
