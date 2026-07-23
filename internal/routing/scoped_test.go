package routing

import (
	"testing"

	"github.com/whatgate/whatgate/internal/coordinator"
	"github.com/whatgate/whatgate/internal/trust"
)

// tierMap builds a tierOf lookup for tests.
func tierMap(m map[string]trust.Tier) func(string) trust.Tier {
	return func(peerID string) trust.Tier { return m[peerID] }
}

func TestPickExitScopedConservativeSkipsStranger(t *testing.T) {
	nodes := []coordinator.NodeInfo{
		{PeerID: "stranger", Region: "JP", WantExit: true},
		{PeerID: "friend", Region: "JP", WantExit: true},
	}
	tiers := tierMap(map[string]trust.Tier{
		"stranger": trust.TierStranger,
		"friend":   trust.TierEndorsed,
	})

	got, ok := PickExitScoped(nodes, "JP", "self", trust.ScopeConservative, tiers)
	if !ok {
		t.Fatal("expected an eligible endorsed exit")
	}
	if got.PeerID != "friend" {
		t.Fatalf("picked %q, want friend (stranger must be excluded in conservative scope)", got.PeerID)
	}
}

func TestPickExitScopedConservativeNoneWhenAllStrangers(t *testing.T) {
	nodes := []coordinator.NodeInfo{
		{PeerID: "stranger", Region: "JP", WantExit: true},
	}
	tiers := tierMap(map[string]trust.Tier{"stranger": trust.TierStranger})

	if _, ok := PickExitScoped(nodes, "JP", "self", trust.ScopeConservative, tiers); ok {
		t.Fatal("conservative scope should find no exit among strangers")
	}
}

func TestPickExitScopedOpenAllowsStranger(t *testing.T) {
	nodes := []coordinator.NodeInfo{
		{PeerID: "stranger", Region: "JP", WantExit: true},
	}
	tiers := tierMap(map[string]trust.Tier{"stranger": trust.TierStranger})

	got, ok := PickExitScoped(nodes, "JP", "self", trust.ScopeOpen, tiers)
	if !ok || got.PeerID != "stranger" {
		t.Fatalf("open scope should accept a stranger exit; got %q ok=%v", got.PeerID, ok)
	}
}
