package llm_test

import (
	"sort"
	"testing"

	"github.com/omerfarukaydin/atlas/internal/llm"

	_ "github.com/omerfarukaydin/atlas/internal/llm/anthropic"
	_ "github.com/omerfarukaydin/atlas/internal/llm/gemini"
	_ "github.com/omerfarukaydin/atlas/internal/llm/openai"
)

func TestAllProvidersRegistered(t *testing.T) {
	got := llm.Available()
	sort.Strings(got)
	want := []string{"anthropic", "gemini", "openai"}
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("expected %v registered, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("expected %v registered, got %v", want, got)
			break
		}
	}
}

func TestNewReturnsCorrectlyNamedProvider(t *testing.T) {
	for _, name := range []string{"anthropic", "openai", "gemini"} {
		p, err := llm.New(name, "fake-key", "fake-model")
		if err != nil {
			t.Fatalf("New(%q) returned error: %v", name, err)
		}
		if p.Name() != name {
			t.Errorf("expected provider name %q, got %q", name, p.Name())
		}
	}
}

func TestNewUnknownProviderErrors(t *testing.T) {
	if _, err := llm.New("does-not-exist", "key", "model"); err == nil {
		t.Error("expected error for unknown provider")
	}
}
