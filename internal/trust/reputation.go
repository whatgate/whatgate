package trust

import "sync"

// Reputation tracks two levels of standing: per individual peer and per group.
// Scores start at zero and are nudged by behavior (positive for good conduct,
// negative for abuse). Later milestones feed it from exit outcomes and use it in
// routing rank and exit-serving thresholds.
type Reputation struct {
	mu    sync.RWMutex
	peer  map[string]int
	group map[string]int
}

// NewReputation returns an empty reputation store.
func NewReputation() *Reputation {
	return &Reputation{
		peer:  make(map[string]int),
		group: make(map[string]int),
	}
}

// AdjustPeer changes a peer's reputation by delta.
func (r *Reputation) AdjustPeer(peerID string, delta int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.peer[peerID] += delta
}

// PeerScore returns a peer's reputation (0 if unknown).
func (r *Reputation) PeerScore(peerID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.peer[peerID]
}

// AdjustGroup changes a group's reputation by delta.
func (r *Reputation) AdjustGroup(groupID string, delta int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.group[groupID] += delta
}

// GroupScore returns a group's reputation (0 if unknown).
func (r *Reputation) GroupScore(groupID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.group[groupID]
}
