package routing

import (
	"testing"

	"github.com/whatgate/whatgate/internal/coordinator"
	"github.com/whatgate/whatgate/internal/trust"
)

func stableTier(m map[string]trust.Tier) func(string) trust.Tier {
	return func(id string) trust.Tier { return m[id] }
}

// With latency weighted heavily, the lower-latency exit ranks first among
// equally trusted exits.
func TestRankWeightedLatencyDominant(t *testing.T) {
	nodes := []coordinator.NodeInfo{
		{PeerID: "slow", Region: "JP", WantExit: true},
		{PeerID: "fast", Region: "JP", WantExit: true},
	}
	tierOf := stableTier(map[string]trust.Tier{"slow": trust.TierSameGroup, "fast": trust.TierSameGroup})
	metrics := func(id string) Metrics {
		if id == "fast" {
			return Metrics{LatencyMs: 20}
		}
		return Metrics{LatencyMs: 200}
	}
	ranked := RankExitsWeighted(nodes, "JP", "self", trust.ScopeOpen, tierOf, metrics,
		Weights{Trust: 1, Latency: 1, Load: 1})
	if len(ranked) != 2 || ranked[0].PeerID != "fast" {
		t.Fatalf("ranked = %v, want fast first", ranked)
	}
}

// With trust weighted heavily, the more-trusted exit ranks first even if it is
// slower.
func TestRankWeightedTrustDominant(t *testing.T) {
	nodes := []coordinator.NodeInfo{
		{PeerID: "trusted-slow", Region: "JP", WantExit: true},
		{PeerID: "stranger-fast", Region: "JP", WantExit: true},
	}
	tierOf := stableTier(map[string]trust.Tier{"trusted-slow": trust.TierSameGroup, "stranger-fast": trust.TierStranger})
	metrics := func(id string) Metrics {
		if id == "stranger-fast" {
			return Metrics{LatencyMs: 10}
		}
		return Metrics{LatencyMs: 100}
	}
	ranked := RankExitsWeighted(nodes, "JP", "self", trust.ScopeOpen, tierOf, metrics,
		Weights{Trust: 1000, Latency: 1, Load: 1})
	if len(ranked) != 2 || ranked[0].PeerID != "trusted-slow" {
		t.Fatalf("ranked = %v, want trusted-slow first", ranked)
	}
}

// Scope filtering still applies: a conservative scope excludes strangers.
func TestRankWeightedRespectsScope(t *testing.T) {
	nodes := []coordinator.NodeInfo{
		{PeerID: "trusted", Region: "JP", WantExit: true},
		{PeerID: "stranger", Region: "JP", WantExit: true},
	}
	tierOf := stableTier(map[string]trust.Tier{"trusted": trust.TierSameGroup, "stranger": trust.TierStranger})
	metrics := func(string) Metrics { return Metrics{} }
	ranked := RankExitsWeighted(nodes, "JP", "self", trust.ScopeConservative, tierOf, metrics,
		Weights{Trust: 1, Latency: 1, Load: 1})
	if len(ranked) != 1 || ranked[0].PeerID != "trusted" {
		t.Fatalf("ranked = %v, want only trusted", ranked)
	}
}
