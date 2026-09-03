package tools

import "testing"

func TestLineAnchorHashIsStableAndLengthLimited(t *testing.T) {
	t.Parallel()

	got := lineAnchorHash("some line of code")
	if len(got) != lineAnchorHashLen {
		t.Fatalf("hash length = %d, want %d", len(got), lineAnchorHashLen)
	}
	if again := lineAnchorHash("some line of code"); again != got {
		t.Fatalf("hash is not stable: %q != %q", got, again)
	}
}

func TestLineAnchorHashDiffersForDifferentLines(t *testing.T) {
	t.Parallel()

	a := lineAnchorHash("alpha")
	b := lineAnchorHash("beta")
	if a == b {
		t.Fatalf("distinct lines hashed to the same anchor: %q", a)
	}
}

func TestLineAnchorHashOfEmptyLine(t *testing.T) {
	t.Parallel()

	got := lineAnchorHash("")
	if len(got) != lineAnchorHashLen {
		t.Fatalf("hash length = %d, want %d", len(got), lineAnchorHashLen)
	}
}
