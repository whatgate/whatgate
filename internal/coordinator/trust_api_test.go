package coordinator

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/whatgate/whatgate/internal/trust"
)

func TestGroupsAndTrustAnnotatedDirectory(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 100)
	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	defer ts.Close()

	// Three admitted + registered JP exits.
	alice := admitAndRegister(t, ts.URL, "welcome")
	bob := admitAndRegister(t, ts.URL, "welcome")
	carol := admitAndRegister(t, ts.URL, "welcome")

	// dave only needs an identity to be a group member (not registered).
	_, dave := newSignedClient(t, ts.URL)

	// Group ops are not auth-gated yet (see backlog "小网操作鉴权").
	admin := NewClient(ts.URL)
	if err := admin.CreateGroup("g1", alice); err != nil {
		t.Fatalf("create g1: %v", err)
	}
	if err := admin.CreateGroup("g2", bob); err != nil {
		t.Fatalf("create g2: %v", err)
	}
	if err := admin.JoinGroup("g1", dave); err != nil {
		t.Fatalf("join g1: %v", err)
	}
	if err := admin.EndorseGroup("g1", "g2"); err != nil {
		t.Fatalf("endorse: %v", err)
	}

	// Directory as seen by dave (a g1 member).
	_, tiers, err := admin.DirectoryFor(dave)
	if err != nil {
		t.Fatalf("DirectoryFor: %v", err)
	}
	if tiers[alice] != trust.TierSameGroup {
		t.Errorf("dave->alice tier = %v, want same-group", tiers[alice])
	}
	if tiers[bob] != trust.TierEndorsed {
		t.Errorf("dave->bob tier = %v, want endorsed", tiers[bob])
	}
	if tiers[carol] != trust.TierStranger {
		t.Errorf("dave->carol tier = %v, want stranger", tiers[carol])
	}
}
