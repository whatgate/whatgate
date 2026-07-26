package routing

import (
	"testing"

	"github.com/whatgate/whatgate/internal/coordinator"
	"github.com/whatgate/whatgate/internal/trust"
)

// MergeExits unions the coordinator directory with DHT-resolved exits, deduping
// by peer ID; coordinator entries keep their authoritative tier while DHT-only
// exits are strangers (the DHT plane cannot vouch trust).
func TestMergeExitsUnionsAndDedups(t *testing.T) {
	coordNodes := []coordinator.NodeInfo{{PeerID: "A", Region: "JP", WantExit: true}}
	coordTierOf := func(id string) trust.Tier {
		if id == "A" {
			return trust.TierSameGroup
		}
		return trust.TierStranger
	}
	dht := []DHTExit{
		{PeerID: "B", Region: "JP", Addrs: []string{"/ip4/1.2.3.4/tcp/443/ws"}},
		{PeerID: "A", Region: "JP", Addrs: []string{"/ip4/5.6.7.8/tcp/443/ws"}}, // dup of coordinator A
	}

	merged, tierOf := MergeExits(coordNodes, coordTierOf, dht)
	if len(merged) != 2 {
		t.Fatalf("merged has %d entries, want 2 (A,B deduped)", len(merged))
	}
	if tierOf("A") != trust.TierSameGroup {
		t.Fatalf("coordinator A should keep its tier, got %v", tierOf("A"))
	}
	if tierOf("B") != trust.TierStranger {
		t.Fatalf("DHT-only B should be a stranger, got %v", tierOf("B"))
	}
}

// A DHT-only exit is a stranger, so a conservative scope excludes it (no
// authoritative trust) while an open scope includes it — the four-state merge
// realized through the existing scope/tier ranking.
func TestMergeExitsDHTOnlyFilteredByScope(t *testing.T) {
	coordNodes := []coordinator.NodeInfo{{PeerID: "A", Region: "JP", WantExit: true}}
	coordTierOf := func(id string) trust.Tier {
		if id == "A" {
			return trust.TierSameGroup
		}
		return trust.TierStranger
	}
	dht := []DHTExit{{PeerID: "B", Region: "JP", Addrs: []string{"/ip4/1.2.3.4/tcp/443/ws"}}}

	merged, tierOf := MergeExits(coordNodes, coordTierOf, dht)
	metrics := func(string) Metrics { return Metrics{} }

	conservative := RankExits(merged, "JP", "self", trust.ScopeConservative, tierOf, metrics)
	if len(conservative) != 1 || conservative[0].PeerID != "A" {
		t.Fatalf("conservative scope = %v, want only trusted A", conservative)
	}
	open := RankExits(merged, "JP", "self", trust.ScopeOpen, tierOf, metrics)
	if len(open) != 2 {
		t.Fatalf("open scope = %v, want both A and DHT-only B", open)
	}
}
