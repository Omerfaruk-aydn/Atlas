package agent

import "testing"

func TestCandidateAPIKeysCombinesPrimaryAndExtra(t *testing.T) {
	got := candidateAPIKeys("$KEY1", []string{"$KEY2", "$KEY3"})
	want := []string{"$KEY1", "$KEY2", "$KEY3"}
	if len(got) != len(want) {
		t.Fatalf("candidateAPIKeys() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidateAPIKeys()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCandidateAPIKeysSkipsBlanks(t *testing.T) {
	got := candidateAPIKeys("", []string{"$KEY2", "", "$KEY3"})
	if len(got) != 2 || got[0] != "$KEY2" || got[1] != "$KEY3" {
		t.Errorf("candidateAPIKeys() = %v, want [$KEY2 $KEY3]", got)
	}
}

func TestCredentialRotatorPickSingleKeyNeverAdvances(t *testing.T) {
	r := newCredentialRotator()
	for range 3 {
		if got := r.Pick("openai", []string{"$KEY1"}); got != "$KEY1" {
			t.Errorf("Pick() = %q, want $KEY1", got)
		}
	}
}

func TestCredentialRotatorPickRoundRobins(t *testing.T) {
	r := newCredentialRotator()
	keys := []string{"$KEY1", "$KEY2", "$KEY3"}

	seen := []string{
		r.Pick("openai", keys),
		r.Pick("openai", keys),
		r.Pick("openai", keys),
		r.Pick("openai", keys), // wraps back to the first
	}
	want := []string{"$KEY1", "$KEY2", "$KEY3", "$KEY1"}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("Pick() call %d = %q, want %q", i, seen[i], want[i])
		}
	}
}

func TestCredentialRotatorPicksAreIndependentPerProvider(t *testing.T) {
	r := newCredentialRotator()
	keys := []string{"$KEY1", "$KEY2"}

	if got := r.Pick("openai", keys); got != "$KEY1" {
		t.Errorf("openai first pick = %q, want $KEY1", got)
	}
	if got := r.Pick("anthropic", keys); got != "$KEY1" {
		t.Errorf("anthropic first pick = %q, want $KEY1 (independent from openai)", got)
	}
}

func TestCredentialRotatorAdvanceSkipsAKey(t *testing.T) {
	r := newCredentialRotator()
	keys := []string{"$KEY1", "$KEY2", "$KEY3"}

	if got := r.Pick("openai", keys); got != "$KEY1" {
		t.Fatalf("first pick = %q, want $KEY1", got)
	}
	r.Advance("openai")
	if got := r.Pick("openai", keys); got != "$KEY3" {
		t.Errorf("pick after Advance = %q, want $KEY3", got)
	}
}

func TestCredentialRotatorNilReceiverIsSafe(t *testing.T) {
	var r *credentialRotator
	if got := r.Pick("openai", []string{"$KEY1"}); got != "$KEY1" {
		t.Errorf("nil Pick() with one key = %q, want $KEY1", got)
	}
	r.Advance("openai") // must not panic
}
