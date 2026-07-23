package trust

import "testing"

func TestPeerReputationAccumulates(t *testing.T) {
	r := NewReputation()
	r.AdjustPeer("alice", 5)
	r.AdjustPeer("alice", -2)

	if got := r.PeerScore("alice"); got != 3 {
		t.Fatalf("PeerScore(alice) = %d, want 3", got)
	}
	if got := r.PeerScore("nobody"); got != 0 {
		t.Fatalf("PeerScore(nobody) = %d, want 0", got)
	}
}

func TestGroupReputationAccumulates(t *testing.T) {
	r := NewReputation()
	r.AdjustGroup("g1", 10)
	r.AdjustGroup("g1", 4)

	if got := r.GroupScore("g1"); got != 14 {
		t.Fatalf("GroupScore(g1) = %d, want 14", got)
	}
}
