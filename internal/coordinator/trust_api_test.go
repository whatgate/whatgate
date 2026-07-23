package coordinator

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/whatgate/whatgate/internal/trust"
)

// admit registers a peer through the invite flow so it may register presence.
func admit(t *testing.T, c *Client, code, peerID string) {
	t.Helper()
	if _, err := c.Join(code, peerID); err != nil {
		t.Fatalf("join %s: %v", peerID, err)
	}
}

func TestGroupsAndTrustAnnotatedDirectory(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 100)
	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	defer ts.Close()
	c := NewClient(ts.URL)

	// Admit three peers and have them register as JP exits.
	for _, p := range []string{"alice", "bob", "carol"} {
		admit(t, c, "welcome", p)
		if err := c.Register(NodeInfo{PeerID: p, Region: "JP", WantExit: true}); err != nil {
			t.Fatalf("register %s: %v", p, err)
		}
	}

	// alice in g1; bob in g2; g1 endorses g2; carol ungrouped.
	if err := c.CreateGroup("g1", "alice"); err != nil {
		t.Fatalf("create g1: %v", err)
	}
	if err := c.CreateGroup("g2", "bob"); err != nil {
		t.Fatalf("create g2: %v", err)
	}
	if err := c.JoinGroup("g1", "dave"); err != nil {
		t.Fatalf("join g1: %v", err)
	}
	if err := c.EndorseGroup("g1", "g2"); err != nil {
		t.Fatalf("endorse: %v", err)
	}

	// Directory as seen by dave (a g1 member):
	_, tiers, err := c.DirectoryFor("dave")
	if err != nil {
		t.Fatalf("DirectoryFor: %v", err)
	}
	if tiers["alice"] != trust.TierSameGroup {
		t.Errorf("dave->alice tier = %v, want same-group", tiers["alice"])
	}
	if tiers["bob"] != trust.TierEndorsed {
		t.Errorf("dave->bob tier = %v, want endorsed", tiers["bob"])
	}
	if tiers["carol"] != trust.TierStranger {
		t.Errorf("dave->carol tier = %v, want stranger", tiers["carol"])
	}
}
