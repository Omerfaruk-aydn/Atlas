package claude

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"

	xxhash "github.com/cespare/xxhash/v2"
)

// cchSeed is the secret XXH64 seed Claude Code's fetch wrapper uses
// for the cch billing attestation. The seed is not confidential --
// what matters is that the API expects XXH64 with this exact seed,
// computed over the request body with the cch=00000 placeholder in
// place. Source: oh-my-pi `claude-code-fingerprint.ts` (CCH_SEED).
const cchSeed = 0x4d659218e32a3268

// cchPlaceholder is the literal `cch=00000` segment of the billing
// header block. Claude Code's fetch wrapper patches the body in
// place to replace this with XXH64(body) & 0xfffff formatted as 5
// hex chars. We do the same in our custom RoundTripper so the API
// sees an attested request.
//
// Length of this string is 8 (5-digit placeholder + 3 prefix chars).
// It must be unique within the body; the billing header is the only
// place it appears, by construction (we inject it as the first system
// block in the wrapped provider).
const cchPlaceholder = "cch=00000"

// cchPatcher is an http.RoundTripper that patches the cch=00000
// billing-attestation placeholder in outgoing request bodies with
// the actual XXH64 attestation. It is a no-op on bodies that don't
// contain the placeholder, so installing it on every OAuth flow is
// safe even if the billing header is later removed.
type cchPatcher struct {
	inner http.RoundTripper
}

// RoundTrip reads the request body, patches the cch placeholder if
// present, and forwards the modified request to the underlying
// transport. If the body cannot be read, the request is forwarded
// unchanged; failing the request would be worse than sending the
// unattested variant because the API still answers (with a
// different error) instead of timing out.
func (p *cchPatcher) RoundTrip(req *http.Request) (*http.Response, error) {
	inner := p.inner
	if inner == nil {
		inner = http.DefaultTransport
	}
	if req.Body == nil {
		return inner.RoundTrip(req)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		// We can't see the body, so we can't patch it; close the
		// original body and let the transport do its thing.
		_ = req.Body.Close()
		return inner.RoundTrip(req)
	}
	_ = req.Body.Close()

	patched, didPatch := patchCCH(body)
	if didPatch {
		// ContentLength needs to follow the new body size; some
		// transports use it to decide on chunked encoding. -1 tells
		// Go to re-derive it.
		req.ContentLength = int64(len(patched))
		req.Body = io.NopCloser(bytes.NewReader(patched))
	} else {
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	return inner.RoundTrip(req)
}

// patchCCH replaces the cch=00000 placeholder in body with the
// XXH64 attestation. Returns the patched body and a flag indicating
// whether the placeholder was found. The flag exists so the
// caller can short-circuit setting ContentLength when nothing
// changed.
//
// XXH64 is computed over the body with the placeholder in place;
// that is, the placeholder is part of the bytes that get hashed.
// The 5-hex-char output is the low 20 bits of the 64-bit hash,
// zero-padded on the left.
func patchCCH(body []byte) ([]byte, bool) {
	idx := bytes.Index(body, []byte(cchPlaceholder))
	if idx == -1 {
		return body, false
	}
	digest := xxhash.NewWithSeed(cchSeed)
	_, _ = digest.Write(body)
	cch := digest.Sum64() & 0xfffff
	patched := make([]byte, 0, len(body))
	patched = append(patched, body[:idx]...)
	patched = append(patched, []byte("cch=")...)
	patched = append(patched, []byte(fmt.Sprintf("%05x", cch))...)
	patched = append(patched, body[idx+len(cchPlaceholder):]...)
	return patched, true
}

// computeCCH is exposed for tests so they can assert on the exact
// hash output without having to scan the body.
func computeCCH(body []byte) string {
	digest := xxhash.NewWithSeed(cchSeed)
	_, _ = digest.Write(body)
	return fmt.Sprintf("%05x", digest.Sum64()&0xfffff)
}

// mustHaveCCH is a debug helper: encodes body as hex and returns
// the body with the placeholder replaced by the computed hash, so a
// test failure prints both halves side by side.
func mustHaveCCH(body []byte) string {
	patched, _ := patchCCH(body)
	return hex.EncodeToString(patched)
}
