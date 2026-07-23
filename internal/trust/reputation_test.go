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

func TestDecayMovesScoresTowardZero(t *testing.T) {
	r := NewReputation()
	r.AdjustPeer("punished", -10)
	r.AdjustPeer("trusted", 5)
	r.AdjustPeer("almost", -2)
	r.AdjustGroup("g", -4)

	r.Decay(3)

	if got := r.PeerScore("punished"); got != -7 {
		t.Errorf("punished = %d, want -7", got)
	}
	if got := r.PeerScore("trusted"); got != 2 {
		t.Errorf("trusted = %d, want 2", got)
	}
	if got := r.PeerScore("almost"); got != 0 {
		t.Errorf("almost = %d, want 0 (no overshoot)", got)
	}
	if got := r.GroupScore("g"); got != -1 {
		t.Errorf("group g = %d, want -1", got)
	}
}

func TestDecayZeroStepIsNoop(t *testing.T) {
	r := NewReputation()
	r.AdjustPeer("a", -10)
	r.Decay(0)
	if r.PeerScore("a") != -10 {
		t.Fatal("decay with step 0 should not change scores")
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
