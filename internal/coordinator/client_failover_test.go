package coordinator

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A dead first endpoint (connection refused) is skipped in favor of a reachable
// one.
func TestFailsOverToReachableEndpoint(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []directoryEntry{{PeerID: "exit1"}})
	}))
	defer good.Close()

	c := NewClientEndpoints([]string{"http://127.0.0.1:1", good.URL})
	nodes, _, err := c.DirectoryFor("")
	if err != nil {
		t.Fatalf("expected failover to succeed: %v", err)
	}
	if len(nodes) != 1 || nodes[0].PeerID != "exit1" {
		t.Fatalf("nodes = %+v, want one exit1", nodes)
	}
}

// After failing over, the client remembers the healthy endpoint and does not
// keep retrying the dead one first.
func TestRemembersHealthyEndpoint(t *testing.T) {
	var hits int32
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		writeJSON(w, http.StatusOK, []directoryEntry{})
	}))
	defer good.Close()

	c := NewClientEndpoints([]string{"http://127.0.0.1:1", good.URL})
	if _, _, err := c.DirectoryFor(""); err != nil {
		t.Fatal(err)
	}
	if c.current != 1 {
		t.Fatalf("current = %d, want 1 (healthy endpoint remembered)", c.current)
	}
	if _, _, err := c.DirectoryFor(""); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("good endpoint hits = %d, want 2", got)
	}
}

// A reachable endpoint returning an error status must surface that error, not
// silently fail over — "unreachable" and "business reject" are different.
func TestDoesNotFailOverOnBusinessError(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer bad.Close()
	var goodHit int32
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&goodHit, 1)
		writeJSON(w, http.StatusOK, []directoryEntry{})
	}))
	defer good.Close()

	c := NewClientEndpoints([]string{bad.URL, good.URL})
	if _, _, err := c.DirectoryFor(""); err == nil {
		t.Fatal("expected the reachable endpoint's error to surface")
	}
	if got := atomic.LoadInt32(&goodHit); got != 0 {
		t.Fatalf("must not fail over on a business error; good endpoint hit %d times", got)
	}
}
