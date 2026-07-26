package routing

import "testing"

// The first sample for a peer seeds the smoothed value directly (no history to
// blend with).
func TestLatencyTrackerSeedsFirstSample(t *testing.T) {
	tr := NewLatencyTracker(0.5)
	if got := tr.Observe("a", 100); got != 100 {
		t.Fatalf("first Observe = %d, want 100", got)
	}
	if v, ok := tr.Get("a"); !ok || v != 100 {
		t.Fatalf("Get = %d,%v want 100,true", v, ok)
	}
}

// A second sample blends with the first per the EWMA weight.
func TestLatencyTrackerSmoothsSamples(t *testing.T) {
	tr := NewLatencyTracker(0.5)
	tr.Observe("a", 100)
	if got := tr.Observe("a", 200); got != 150 { // 0.5*200 + 0.5*100
		t.Fatalf("smoothed = %d, want 150", got)
	}
}

// A single latency spike is dampened toward the established baseline rather than
// swinging the selection wildly.
func TestLatencyTrackerDampensSpike(t *testing.T) {
	tr := NewLatencyTracker(0.2)
	for i := 0; i < 5; i++ {
		tr.Observe("a", 100) // establish a steady baseline
	}
	got := tr.Observe("a", 1000) // one spike
	if got <= 100 || got >= 500 {
		t.Fatalf("dampened spike = %d, want between 100 and 500 (closer to baseline)", got)
	}
}

// With alpha = 1 there is no smoothing: the latest sample fully replaces the old
// value, matching the pre-EWMA one-shot behavior.
func TestLatencyTrackerAlphaOneNoSmoothing(t *testing.T) {
	tr := NewLatencyTracker(1)
	tr.Observe("a", 100)
	if got := tr.Observe("a", 500); got != 500 {
		t.Fatalf("alpha=1 = %d, want 500 (latest wins)", got)
	}
}

// Distinct peers keep independent histories.
func TestLatencyTrackerIndependentPeers(t *testing.T) {
	tr := NewLatencyTracker(0.5)
	tr.Observe("a", 100)
	tr.Observe("b", 400)
	if v, _ := tr.Get("a"); v != 100 {
		t.Fatalf("a = %d, want 100", v)
	}
	if v, _ := tr.Get("b"); v != 400 {
		t.Fatalf("b = %d, want 400", v)
	}
}

// An unobserved peer has no smoothed value.
func TestLatencyTrackerGetUnknown(t *testing.T) {
	tr := NewLatencyTracker(0.5)
	if v, ok := tr.Get("x"); ok || v != 0 {
		t.Fatalf("Get unknown = %d,%v want 0,false", v, ok)
	}
}
