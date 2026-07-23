package trust

import "testing"

func TestTrustSameGroupIsHighest(t *testing.T) {
	g := NewGraph()
	g.CreateGroup("g1", "alice")
	g.AddMember("g1", "bob")

	if got := g.Trust("alice", "bob"); got != TierSameGroup {
		t.Fatalf("Trust(alice,bob) = %v, want same-group", got)
	}
}

func TestTrustEndorsedNeighbor(t *testing.T) {
	g := NewGraph()
	g.CreateGroup("g1", "alice")
	g.CreateGroup("g2", "bob")
	g.Endorse("g1", "g2") // alice's group vouches for bob's group

	if got := g.Trust("alice", "bob"); got != TierEndorsed {
		t.Fatalf("Trust(alice,bob) = %v, want endorsed", got)
	}
}

func TestTrustEndorsementIsDirectional(t *testing.T) {
	g := NewGraph()
	g.CreateGroup("g1", "alice")
	g.CreateGroup("g2", "bob")
	g.Endorse("g1", "g2") // only g1 -> g2

	// bob does not automatically trust alice back.
	if got := g.Trust("bob", "alice"); got != TierStranger {
		t.Fatalf("Trust(bob,alice) = %v, want stranger (endorsement is one-way)", got)
	}
}

func TestTrustStranger(t *testing.T) {
	g := NewGraph()
	g.CreateGroup("g1", "alice")
	g.CreateGroup("g2", "bob")

	if got := g.Trust("alice", "bob"); got != TierStranger {
		t.Fatalf("Trust(alice,bob) = %v, want stranger", got)
	}
}
