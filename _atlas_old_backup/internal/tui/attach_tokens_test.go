package tui

import "testing"

func TestExpandTokensImage(t *testing.T) {
	in := "look at [[ Image 1 ]] and [[ Image 2 ]] please"
	out, imgs := expandTokens(in, nil)
	// Image tokens consume the trailing adjacent space so the user's
	// typed "at[[ Image ]] " doesn't leave a double-space artifact.
	if out != "look at and please" {
		t.Errorf("expected images expanded + trailing space consumed, got %q", out)
	}
	if len(imgs) != 2 || imgs[0] != 1 || imgs[1] != 2 {
		t.Errorf("expected image indices [1, 2], got %v", imgs)
	}
}

func TestExpandTokensPaste(t *testing.T) {
	in := "before [[ first.. [3 lines] .. last ]] after"
	payloads := map[string]string{
		"[[ first.. [3 lines] .. last ]]": "full paste content\nline2\nline3",
	}
	out, imgs := expandTokens(in, payloads)
	// Same consume-trailing-space rule applies to paste tokens.
	if out != "before full paste content\nline2\nline3after" {
		t.Errorf("expected paste expansion with trailing space consumed, got %q", out)
	}
	if len(imgs) != 0 {
		t.Errorf("expected no images, got %v", imgs)
	}
}

func TestNextImageIndex(t *testing.T) {
	if got := nextImageIndex(""); got != 1 {
		t.Errorf("expected first index 1, got %d", got)
	}
	if got := nextImageIndex("[[ Image 2 ]]"); got != 3 {
		t.Errorf("expected next-after-2 = 3, got %d", got)
	}
}

func TestDroppedTokens(t *testing.T) {
	old := "keep [[ Image 1 ]] and [[ Image 2 ]]"
	new := "keep [[ Image 2 ]]"
	dropped := droppedTokens(old, new)
	if len(dropped) != 1 || dropped[0] != "[[ Image 1 ]]" {
		t.Errorf("expected [[ Image 1 ]] dropped, got %v", dropped)
	}
}
