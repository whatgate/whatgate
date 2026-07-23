package coordinator

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/whatgate/whatgate/internal/trust"
)

func newTrustServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 100)
	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestGroupsAndTrustAnnotatedDirectory(t *testing.T) {
	ts := newTrustServer(t)

	// Three admitted + registered JP exits.
	aliceC, alice := admitAndRegister(t, ts.URL, "welcome")
	bobC, bob := admitAndRegister(t, ts.URL, "welcome")
	_, carol := admitAndRegister(t, ts.URL, "welcome") // registered but ungrouped

	// dave only needs an identity to be a group member (not registered).
	daveC, dave := newSignedClient(t, ts.URL)

	// alice founds g1 with a secret; dave joins g1 with the same secret.
	if err := aliceC.JoinGroup("g1", alice, "s1"); err != nil {
		t.Fatalf("alice founds g1: %v", err)
	}
	if err := daveC.JoinGroup("g1", dave, "s1"); err != nil {
		t.Fatalf("dave joins g1: %v", err)
	}
	// bob founds g2.
	if err := bobC.JoinGroup("g2", bob, "s2"); err != nil {
		t.Fatalf("bob founds g2: %v", err)
	}
	// alice (a member of g1) makes g1 endorse g2.
	if err := aliceC.EndorseGroup("g1", "g2"); err != nil {
		t.Fatalf("endorse: %v", err)
	}

	// Directory as seen by dave (a g1 member).
	_, tiers, err := daveC.DirectoryFor(dave)
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

func TestGroupJoinWrongSecretRejected(t *testing.T) {
	ts := newTrustServer(t)

	aliceC, alice := newSignedClient(t, ts.URL)
	if err := aliceC.JoinGroup("fam", alice, "correct"); err != nil {
		t.Fatalf("alice founds fam: %v", err)
	}

	strangerC, stranger := newSignedClient(t, ts.URL)
	err := strangerC.JoinGroup("fam", stranger, "guess")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("wrong-secret join should be 403, got %v", err)
	}
}

func TestGroupJoinRejectsImpersonation(t *testing.T) {
	ts := newTrustServer(t)

	// aliceC signs as alice but tries to add a different peer ID.
	aliceC, _ := newSignedClient(t, ts.URL)
	_, other := newSignedClient(t, ts.URL)

	err := aliceC.JoinGroup("fam", other, "s")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("impersonated join should be 401, got %v", err)
	}
}

func TestGroupEndorseRequiresMembership(t *testing.T) {
	ts := newTrustServer(t)

	aliceC, alice := newSignedClient(t, ts.URL)
	if err := aliceC.JoinGroup("g1", alice, "s1"); err != nil {
		t.Fatalf("alice founds g1: %v", err)
	}

	// A non-member of g1 cannot make g1 endorse anything.
	strangerC, _ := newSignedClient(t, ts.URL)
	err := strangerC.EndorseGroup("g1", "g2")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("non-member endorse should be 403, got %v", err)
	}
}
