package routing

import (
	"math"
	"sync"
)

// LatencyTracker smooths per-exit round-trip latency with an exponentially
// weighted moving average (EWMA), so ranking reacts to sustained changes but
// isn't whipsawed by a single slow probe. It is safe for concurrent use and
// persists across successive discovery rounds — each fresh probe feeds one more
// sample per exit.
type LatencyTracker struct {
	alpha float64 // weight of the newest sample; 1 = no smoothing (latest wins)

	mu   sync.Mutex
	ewma map[string]float64
}

// NewLatencyTracker returns a tracker weighting each new sample by alpha (and the
// running average by 1-alpha). alpha out of (0,1] is clamped to 1, which
// disables smoothing so the latest sample is used as-is.
func NewLatencyTracker(alpha float64) *LatencyTracker {
	if alpha <= 0 || alpha > 1 {
		alpha = 1
	}
	return &LatencyTracker{alpha: alpha, ewma: make(map[string]float64)}
}

// Observe folds sampleMs into the peer's running average and returns the updated
// smoothed latency in milliseconds. The first sample for a peer seeds the value.
func (t *LatencyTracker) Observe(peerID string, sampleMs int) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	v := float64(sampleMs)
	if prev, ok := t.ewma[peerID]; ok {
		v = t.alpha*float64(sampleMs) + (1-t.alpha)*prev
	}
	t.ewma[peerID] = v
	return int(math.Round(v))
}

// Get returns the current smoothed latency for a peer, or (0, false) if it has
// no samples yet.
func (t *LatencyTracker) Get(peerID string) (int, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	v, ok := t.ewma[peerID]
	if !ok {
		return 0, false
	}
	return int(math.Round(v)), true
}
