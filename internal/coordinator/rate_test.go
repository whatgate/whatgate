package coordinator

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// With a rate limit configured, repeated requests from one client to a mutating
// endpoint are rejected with 429 once the burst is exhausted — even malformed
// ones, since the limit is the outermost gate (before decoding/auth).
func TestRateLimitsMutatingEndpoint(t *testing.T) {
	srv := NewServer(NewDirectory(60_000_000_000, nil), NewInviteStore(nil))
	srv.SetRateLimit(0.0001, 2) // burst 2, negligible refill

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	post := func() int {
		resp, err := http.Post(ts.URL+"/join", "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	// The two burst requests reach the handler (bad body → 400); the third is
	// rejected by the limiter.
	s1, s2, s3 := post(), post(), post()
	if s1 == http.StatusTooManyRequests || s2 == http.StatusTooManyRequests {
		t.Fatalf("burst requests should not be limited: %d %d", s1, s2)
	}
	if s3 != http.StatusTooManyRequests {
		t.Fatalf("third request status = %d, want 429", s3)
	}
}

// Read endpoints are not rate-limited (they are cheap and needed frequently).
func TestRateLimitLeavesReadsAlone(t *testing.T) {
	srv := NewServer(NewDirectory(60_000_000_000, nil), NewInviteStore(nil))
	srv.SetRateLimit(0.0001, 1)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for i := 0; i < 5; i++ {
		resp, err := http.Get(ts.URL + "/directory")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			t.Fatalf("directory read %d was rate-limited", i+1)
		}
	}
}
