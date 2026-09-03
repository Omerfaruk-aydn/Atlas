package tools

import (
	"crypto/sha256"
	"encoding/hex"
)

// lineAnchorHashLen is how many hex characters of a line's sha256 survive
// as its anchor. 8 hex chars is 32 bits -- far more than enough headroom
// that two different lines in the same file collide by accident, while
// staying cheap to print next to every line view returns.
const lineAnchorHashLen = 8

// lineAnchorHash returns the anchor view shows next to a line and edit's
// anchor_line/anchor_hash mode verifies before applying. Both operate on
// the same normalized form: Unix line endings, before any line-number
// prefix is added, so a line's anchor is identical whether it was produced
// while rendering a read or while checking an edit against the file on
// disk right now.
func lineAnchorHash(line string) string {
	sum := sha256.Sum256([]byte(line))
	return hex.EncodeToString(sum[:])[:lineAnchorHashLen]
}
