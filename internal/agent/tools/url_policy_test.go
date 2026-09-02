package tools

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestURLPolicyPermitsEverythingByDefault(t *testing.T) {
	t.Parallel()
	var p URLPolicy
	require.NoError(t, p.Check("https://anything.example.com/path"))
}

func TestURLPolicyDeny(t *testing.T) {
	t.Parallel()
	p := URLPolicy{Deny: []string{"bad.example.com"}}

	require.Error(t, p.Check("https://bad.example.com/x"))
	require.Error(t, p.Check("https://sub.bad.example.com/x"), "a subdomain of a denied domain is also denied")
	require.NoError(t, p.Check("https://good.example.com/x"))
}

func TestURLPolicyAllow(t *testing.T) {
	t.Parallel()
	p := URLPolicy{Allow: []string{"docs.example.com"}}

	require.NoError(t, p.Check("https://docs.example.com/x"))
	require.NoError(t, p.Check("https://api.docs.example.com/x"), "a subdomain of an allowed domain is also allowed")
	require.Error(t, p.Check("https://other.example.com/x"))
}

func TestURLPolicyDenyWinsOverAllow(t *testing.T) {
	t.Parallel()
	p := URLPolicy{
		Allow: []string{"example.com"},
		Deny:  []string{"internal.example.com"},
	}

	require.NoError(t, p.Check("https://example.com/x"))
	require.Error(t, p.Check("https://internal.example.com/x"))
}

func TestURLPolicyDoesNotMatchUnrelatedSuffixes(t *testing.T) {
	t.Parallel()
	p := URLPolicy{Deny: []string{"example.com"}}

	// notexample.com ends in "example.com" as a raw string but is not a
	// subdomain of it -- the check must compare host labels, not just
	// call strings.HasSuffix on the unqualified strings.
	require.NoError(t, p.Check("https://notexample.com/x"))
}

func TestURLPolicyIsCaseInsensitive(t *testing.T) {
	t.Parallel()
	p := URLPolicy{Deny: []string{"Example.COM"}}
	require.Error(t, p.Check("https://EXAMPLE.com/x"))
}

func TestURLPolicyRejectsAnUnparseableURL(t *testing.T) {
	t.Parallel()
	var p URLPolicy
	require.Error(t, p.Check("://not a url"))
}
