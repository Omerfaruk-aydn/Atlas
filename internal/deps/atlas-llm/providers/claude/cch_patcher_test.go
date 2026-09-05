package claude

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fantasy "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

// patchCCH must replace the cch=00000 placeholder with the XXH64
// attestation derived from the body. The output must be 5 lowercase
// hex chars; the segment is what the API checks.
func TestPatchCCHReplacesPlaceholder(t *testing.T) {
	body := []byte(`{"system":[{"text":"x-anthropic-billing-header: cc_version=2.1.257.abc; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`)
	patched, ok := patchCCH(body)
	require.True(t, ok, "patchCCH must find the placeholder")
	require.False(t, bytes.Contains(patched, []byte(cchPlaceholder)),
		"the placeholder must be gone after patching, got: %s", patched)

	// Extract the cch segment and confirm it is 5 lowercase hex chars.
	idx := bytes.Index(patched, []byte("cch="))
	require.GreaterOrEqual(t, idx, 0)
	cch := patched[idx+4 : idx+4+5]
	require.Len(t, cch, 5, "cch must be 5 chars")
	for _, c := range cch {
		require.True(t,
			(c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"cch char %q must be lowercase hex", c)
	}
}

// patchCCH must be a no-op on bodies without the placeholder, so
// installing it on every OAuth flow is safe even if the billing
// header is later removed or the response is a non-billing request.
func TestPatchCCHNoOpWhenPlaceholderAbsent(t *testing.T) {
	body := []byte(`{"messages":[]}`)
	patched, ok := patchCCH(body)
	require.False(t, ok, "patchCCH must report no patch when the placeholder is absent")
	require.Equal(t, body, patched, "body must be unchanged")
}

// computeCCH and patchCCH must agree on the attestation value. The
// round-tripper is the only place that calls patchCCH; computeCCH
// is the test seam, and if they ever diverge the tests are useless.
func TestComputeCCHMatchesPatchCCH(t *testing.T) {
	body := []byte(`{"system":[{"text":"prefix cch=00000 suffix"}]}`)
	want := computeCCH(body)
	patched, _ := patchCCH(body)
	idx := bytes.Index(patched, []byte("cch="))
	require.GreaterOrEqual(t, idx, 0)
	got := string(patched[idx+4 : idx+4+5])
	require.Equal(t, want, got, "computeCCH and patchCCH must produce the same value")
}

// The cch hash must be deterministic. Two passes over the same
// body must produce the same cch; otherwise every retry would
// change the body and the API would re-verify the request and
// possibly re-429 it.
func TestCCHIsDeterministic(t *testing.T) {
	body := []byte(`{"system":[{"text":"x-anthropic-billing-header: cc_version=2.1.257.abc; cc_entrypoint=cli; cch=00000;"}]}`)
	a := computeCCH(body)
	b := computeCCH(body)
	require.Equal(t, a, b, "cch must be deterministic for the same body")
}

// The cch hash must be content-sensitive: two distinct bodies
// must produce distinct cch values. The 5-hex-char output space
// is small (20 bits) so a few collisions are possible, but
// unrelated bodies should usually not collide.
func TestCCHIsContentSensitive(t *testing.T) {
	a := computeCCH([]byte(`{"system":[{"text":"cch=00000"}],"messages":[{"role":"user","content":"hello"}]}`))
	b := computeCCH([]byte(`{"system":[{"text":"cch=00000"}],"messages":[{"role":"user","content":"world"}]}`))
	require.NotEqual(t, a, b, "different bodies must usually produce different cch")
}

// The cchPatcher RoundTripper must read the body, patch the
// placeholder, and forward the patched body to the next transport.
// This is the integration check: the cch segment on the wire must
// not be 00000.
func TestCCHPatcherPatchesOnTheWire(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	// Build a minimal request whose body contains the placeholder.
	body := []byte(`{"system":[{"text":"prefix cch=00000 suffix"}],"messages":[]}`)
	req, err := http.NewRequestWithContext(context.Background(), "POST", srv.URL, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rt := &cchPatcher{inner: http.DefaultTransport}
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.NotContains(t, string(gotBody), cchPlaceholder,
		"the wire body must not contain the placeholder, got: %s", gotBody)
	require.Contains(t, string(gotBody), "cch=",
		"the wire body must contain a cch segment, got: %s", gotBody)

	// The body received by the server must equal the result of
	// running patchCCH on the original body.
	expected, _ := patchCCH(body)
	require.True(t, bytes.Equal(gotBody, expected),
		"server body must match patchCCH output:\n got: %s\nwant: %s",
		gotBody, expected)
}

// The cchPatcher must be a no-op on requests without a body, so
// HTTP GETs (e.g. the SDK's occasional health check) pass through
// unchanged.
func TestCCHPatcherHandlesNilBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	req, err := http.NewRequest("GET", srv.URL, nil)
	require.NoError(t, err)
	rt := &cchPatcher{inner: http.DefaultTransport}
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
}

// Integration: the cch placeholder must actually be replaced when
// the full claude provider makes a real Generate call. This is the
// test that catches the regression the user reported: a fingerprint
// that the server sees as still containing cch=00000.
func TestFullProviderCallPatchesCCH(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	p, err := New(WithAccessToken("tok"), WithBaseURL(srv.URL))
	require.NoError(t, err)
	model, err := p.LanguageModel(context.Background(), "claude-sonnet-5")
	require.NoError(t, err)
	_, err = model.Generate(context.Background(), fantasy.Call{
		Prompt: []fantasy.Message{
			fantasy.NewUserMessage("hello"),
		},
	})
	require.NoError(t, err)

	// Confirm the placeholder has been replaced in the wire body.
	// The cch segment is JSON-escaped as a substring of system[0].text.
	require.NotContains(t, string(gotBody), cchPlaceholder,
		"the wire body must not contain cch=00000, got: %s", gotBody)
	require.Contains(t, string(gotBody), `cch=`,
		"the wire body must contain a cch segment, got: %s", gotBody)

	idx := strings.Index(string(gotBody), `cch=`)
	require.GreaterOrEqual(t, idx, 0)
	segment := string(gotBody[idx+4 : idx+4+5])
	for _, c := range segment {
		require.True(t,
			(c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"cch char %q must be lowercase hex", c)
	}
}
