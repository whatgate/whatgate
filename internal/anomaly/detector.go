// Package anomaly provides a sliding-window behavioral detector that flags a key
// (e.g. a client IP) producing too many *distinct* identities (e.g. PeerIDs)
// within a window. It complements the token-bucket rate limiter: the limiter
// bounds request *rate*, but a patient attacker registering one new identity per
// second still assembles a large Sybil fleet under a single IP. Counting distinct
// identities per key catches that pattern and lets the coordinator auto-isolate
// the source.
//
// Honest limitation: keyed by IP, so many legitimate users behind one CGNAT/proxy
// IP could trip a low threshold. It is opt-in and the threshold should be set
// generously; it is a coarse safety net, not a precise classifier.
package anomaly

import (
	"sync"
	"time"
)

// Detector tracks, per key, the set of distinct identities seen within a sliding
// time window. It is safe for concurrent use.
type Detector struct {
	window    time.Duration
	threshold int
	now       func() time.Time

	mu   sync.Mutex
	seen map[string]map[string]time.Time // key -> identity -> last-seen
}

// New returns a detector that flags a key once it has produced threshold or more
// distinct identities within window.
func New(window time.Duration, threshold int) *Detector {
	return &Detector{
		window:    window,
		threshold: threshold,
		now:       time.Now,
		seen:      make(map[string]map[string]time.Time),
	}
}

// Observe records that key produced identity and reports whether key is now
// anomalous (distinct in-window identities >= threshold). Re-observing an
// existing identity refreshes its timestamp without inflating the count.
func (d *Detector) Observe(key, identity string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.now()
	ids := d.seen[key]
	if ids == nil {
		ids = make(map[string]time.Time)
		d.seen[key] = ids
		d.pruneKeysLocked(now)
	}
	ids[identity] = now
	d.pruneIdentitiesLocked(key, now)
	return len(d.seen[key]) >= d.threshold
}

// Count returns the current number of distinct in-window identities for key.
func (d *Detector) Count(key string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pruneIdentitiesLocked(key, d.now())
	return len(d.seen[key])
}

// pruneIdentitiesLocked drops identities under key whose last-seen is older than
// the window, deleting the key entirely if it becomes empty.
func (d *Detector) pruneIdentitiesLocked(key string, now time.Time) {
	ids := d.seen[key]
	for id, last := range ids {
		if now.Sub(last) > d.window {
			delete(ids, id)
		}
	}
	if len(ids) == 0 {
		delete(d.seen, key)
	}
}

// pruneKeysLocked bounds memory by sweeping fully-expired keys when the map grows
// large; cheap and only runs when a new key first appears.
func (d *Detector) pruneKeysLocked(now time.Time) {
	if len(d.seen) < 1024 {
		return
	}
	for key := range d.seen {
		d.pruneIdentitiesLocked(key, now)
	}
}
