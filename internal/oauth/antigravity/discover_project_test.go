package antigravity

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// withFastPolling shrinks the poll/backoff delays so a test that drives
// several onboardUser rounds doesn't actually wait tens of seconds, and
// restores them afterward so other tests (and any future ones added to
// this package) see the real production values.
func withFastPolling(t *testing.T) {
	t.Helper()
	prevPoll, prevBackoff := pollDelay, rateLimitBackoff
	pollDelay = time.Millisecond
	rateLimitBackoff = time.Millisecond
	t.Cleanup(func() { pollDelay, rateLimitBackoff = prevPoll, prevBackoff })
}

// pointCodeAssistAt redirects postCodeAssist's requests to srv for the
// duration of the test.
func pointCodeAssistAt(t *testing.T, srv *httptest.Server) {
	t.Helper()
	prev := codeAssistEndpoint
	codeAssistEndpoint = srv.URL
	t.Cleanup(func() { codeAssistEndpoint = prev })
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// A 429 from onboardUser used to abort discoverProject outright, on the
// theory that any non-200 response was a hard failure. In practice it
// is Google's onboarding backend asking the client to slow down, not a
// rejection of the account -- a real login was observed failing this
// way after seventeen clean "not done yet" polls, with attempts still
// left in its budget. This pins the fix: the poll continues past 429s
// and still succeeds once the backend stops rate-limiting it.
func TestDiscoverProjectRetriesPast429AndSucceeds(t *testing.T) {
	withFastPolling(t)

	var onboardCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "loadCodeAssist"):
			writeJSON(w, loadCodeAssistResponse{AllowedTiers: []tierInfo{{ID: "free"}}})
		case strings.HasSuffix(r.URL.Path, "onboardUser"):
			n := onboardCalls.Add(1)
			if n <= 2 {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED"}}`))
				return
			}
			resp := onboardUserResponse{Done: true}
			resp.Response.CloudaicompanionProject.ID = "proj-123"
			writeJSON(w, resp)
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	pointCodeAssistAt(t, srv)

	var reports []string
	project, tier, err := discoverProject(t.Context(), "token", func(s string) { reports = append(reports, s) })

	require.NoError(t, err)
	require.Equal(t, "proj-123", project)
	require.Equal(t, "free", tier)
	require.Equal(t, int32(3), onboardCalls.Load(), "two rate-limited attempts, then the successful third")
	require.True(t, containsSubstring(reports, "rate-limited"), "the rate-limit rounds must be reported distinctly, not silently retried")
}

// If onboardUser is rate-limited on every attempt, discoverProject must
// still give up eventually (never hang) and say so, distinguishing the
// case from an ordinary "still provisioning" timeout.
func TestDiscoverProjectGivesUpAfterPersistent429s(t *testing.T) {
	withFastPolling(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "loadCodeAssist"):
			writeJSON(w, loadCodeAssistResponse{AllowedTiers: []tierInfo{{ID: "free"}}})
		case strings.HasSuffix(r.URL.Path, "onboardUser"):
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED"}}`))
		}
	}))
	defer srv.Close()
	pointCodeAssistAt(t, srv)

	_, _, err := discoverProject(t.Context(), "token", nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "timed out")
	require.Contains(t, err.Error(), "24 rate-limited", "the timeout message must say every attempt was rate-limited, not just that it timed out")
}

// A non-429 failure (a real rejection, not a rate limit) must still
// abort immediately rather than burning through the whole poll budget.
func TestDiscoverProjectStillFailsFastOnANonRateLimitError(t *testing.T) {
	withFastPolling(t)

	var onboardCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "loadCodeAssist"):
			writeJSON(w, loadCodeAssistResponse{AllowedTiers: []tierInfo{{ID: "free"}}})
		case strings.HasSuffix(r.URL.Path, "onboardUser"):
			onboardCalls.Add(1)
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":403,"status":"PERMISSION_DENIED"}}`))
		}
	}))
	defer srv.Close()
	pointCodeAssistAt(t, srv)

	_, _, err := discoverProject(t.Context(), "token", nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "onboardUser")
	require.Equal(t, int32(1), onboardCalls.Load(), "a non-rate-limit error must abort on the first attempt")
}

func containsSubstring(reports []string, substr string) bool {
	for _, r := range reports {
		if strings.Contains(r, substr) {
			return true
		}
	}
	return false
}
