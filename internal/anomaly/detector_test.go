package anomaly

import (
	"testing"
	"time"
)

// A single key producing many *distinct* identities within the window is the
// Sybil pattern the rate limiter (which only bounds request rate) can't catch.
func TestDetectorFlagsDistinctIdentityBurst(t *testing.T) {
	d := New(time.Hour, 3)
	if d.Observe("ip1", "a") {
		t.Fatal("1 distinct identity should not flag")
	}
	if d.Observe("ip1", "b") {
		t.Fatal("2 distinct identities should not flag")
	}
	if !d.Observe("ip1", "c") {
		t.Fatal("3rd distinct identity should flag (>= threshold)")
	}
}

// Re-observing the same identity (e.g. a peer retrying join) is not new Sybil
// growth and must not inflate the distinct count.
func TestDetectorDedupsSameIdentity(t *testing.T) {
	d := New(time.Hour, 3)
	for i := 0; i < 5; i++ {
		if d.Observe("ip1", "a") {
			t.Fatalf("attempt %d: one identity should never flag", i)
		}
	}
}

// Distinct keys are isolated: one abusive IP doesn't taint another.
func TestDetectorIndependentKeys(t *testing.T) {
	d := New(time.Hour, 2)
	d.Observe("ip1", "a")
	d.Observe("ip1", "b") // ip1 now flagged
	if d.Observe("ip2", "x") {
		t.Fatal("ip2 with one identity should not be flagged by ip1's activity")
	}
}

// Identities older than the window fall out, so a slow trickle under the
// threshold never accumulates into a false positive.
func TestDetectorPrunesOldIdentities(t *testing.T) {
	d := New(10*time.Minute, 3)
	now := time.Unix(0, 0)
	d.now = func() time.Time { return now }

	d.Observe("ip1", "a")
	d.Observe("ip1", "b")
	now = now.Add(11 * time.Minute) // a and b now expired
	if d.Observe("ip1", "c") {
		t.Fatal("stale identities should be pruned; only c is in-window")
	}
	if got := d.Count("ip1"); got != 1 {
		t.Fatalf("Count = %d, want 1 after prune", got)
	}
}

// Count reports the current in-window distinct identities for observability.
func TestDetectorCount(t *testing.T) {
	d := New(time.Hour, 100)
	d.Observe("ip1", "a")
	d.Observe("ip1", "b")
	d.Observe("ip1", "a") // dup
	if got := d.Count("ip1"); got != 2 {
		t.Fatalf("Count = %d, want 2", got)
	}
	if got := d.Count("unknown"); got != 0 {
		t.Fatalf("Count unknown = %d, want 0", got)
	}
}
