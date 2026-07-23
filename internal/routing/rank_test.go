package routing

import (
	"testing"

	"github.com/whatgate/whatgate/internal/coordinator"
	"github.com/whatgate/whatgate/internal/trust"
)

func ids(nodes []coordinator.NodeInfo) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.PeerID
	}
	return out
}

func jpExit(id string) coordinator.NodeInfo {
	return coordinator.NodeInfo{PeerID: id, Region: "JP", WantExit: true}
}

func TestRankPrefersLowerLatencyAmongEqualTrust(t *testing.T) {
	nodes := []coordinator.NodeInfo{jpExit("slow"), jpExit("fast")}
	tiers := tierMap(map[string]trust.Tier{"slow": trust.TierEndorsed, "fast": trust.TierEndorsed})
	metrics := func(id string) Metrics {
		return map[string]Metrics{"slow": {LatencyMs: 200}, "fast": {LatencyMs: 20}}[id]
	}

	got := ids(RankExits(nodes, "JP", "self", trust.ScopeOpen, tiers, metrics))
	if len(got) != 2 || got[0] != "fast" {
		t.Fatalf("ranking = %v, want [fast slow]", got)
	}
}

func TestRankTrustBeatsLatency(t *testing.T) {
	nodes := []coordinator.NodeInfo{jpExit("fastStranger"), jpExit("slowFriend")}
	tiers := tierMap(map[string]trust.Tier{
		"fastStranger": trust.TierStranger,
		"slowFriend":   trust.TierSameGroup,
	})
	metrics := func(id string) Metrics {
		return map[string]Metrics{"fastStranger": {LatencyMs: 5}, "slowFriend": {LatencyMs: 300}}[id]
	}

	got := ids(RankExits(nodes, "JP", "self", trust.ScopeOpen, tiers, metrics))
	if len(got) != 2 || got[0] != "slowFriend" {
		t.Fatalf("ranking = %v, want [slowFriend fastStranger] (trust dominates)", got)
	}
}

func TestRankPrefersLowerLoadAsTiebreak(t *testing.T) {
	nodes := []coordinator.NodeInfo{jpExit("busy"), jpExit("idle")}
	tiers := tierMap(map[string]trust.Tier{"busy": trust.TierEndorsed, "idle": trust.TierEndorsed})
	metrics := func(id string) Metrics {
		return map[string]Metrics{"busy": {LatencyMs: 50, Load: 10}, "idle": {LatencyMs: 50, Load: 1}}[id]
	}

	got := ids(RankExits(nodes, "JP", "self", trust.ScopeOpen, tiers, metrics))
	if len(got) != 2 || got[0] != "idle" {
		t.Fatalf("ranking = %v, want [idle busy]", got)
	}
}

func TestRankExcludesIneligible(t *testing.T) {
	nodes := []coordinator.NodeInfo{
		jpExit("stranger"),
		{PeerID: "us", Region: "US", WantExit: true},
		{PeerID: "self", Region: "JP", WantExit: true},
	}
	tiers := tierMap(map[string]trust.Tier{"stranger": trust.TierStranger})
	metrics := func(string) Metrics { return Metrics{} }

	// Conservative scope excludes the stranger; wrong region and self excluded too.
	got := RankExits(nodes, "JP", "self", trust.ScopeConservative, tiers, metrics)
	if len(got) != 0 {
		t.Fatalf("expected no eligible exits, got %v", ids(got))
	}
}
