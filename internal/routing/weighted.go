package routing

import (
	"sort"

	"github.com/whatgate/whatgate/internal/coordinator"
	"github.com/whatgate/whatgate/internal/trust"
)

// Weights configure the weighted composite exit score. Higher trust is better;
// lower latency and load are better. Each weight scales its term's contribution,
// so an operator can bias selection toward trust, speed, or spare capacity.
type Weights struct {
	Trust   float64
	Latency float64
	Load    float64
}

// score computes a candidate's composite score (higher is better).
func (w Weights) score(tier trust.Tier, m Metrics) float64 {
	return w.Trust*float64(tier) - w.Latency*float64(m.LatencyMs) - w.Load*float64(m.Load)
}

// RankExitsWeighted returns the trust-scope-eligible exits for a region, best
// first, ordered by a weighted composite of trust, latency, and load — unlike the
// lexicographic RankExits, this lets speed or load outweigh a small trust
// difference when the operator wants it. Ties break by the lexicographic order
// (more trusted, then lower latency, then lower load) for determinism.
func RankExitsWeighted(
	nodes []coordinator.NodeInfo,
	region, selfID string,
	scope trust.Scope,
	tierOf func(peerID string) trust.Tier,
	metricsOf func(peerID string) Metrics,
	w Weights,
) []coordinator.NodeInfo {
	var eligible []coordinator.NodeInfo
	for _, n := range nodes {
		if n.PeerID == selfID || !n.WantExit || n.Region != region {
			continue
		}
		if !scope.Allows(tierOf(n.PeerID)) {
			continue
		}
		eligible = append(eligible, n)
	}

	sort.SliceStable(eligible, func(i, j int) bool {
		a, b := eligible[i], eligible[j]
		sa := w.score(tierOf(a.PeerID), metricsOf(a.PeerID))
		sb := w.score(tierOf(b.PeerID), metricsOf(b.PeerID))
		if sa != sb {
			return sa > sb // higher score first
		}
		// Deterministic tie-break: more trusted, then lower latency, then lower load.
		ta, tb := tierOf(a.PeerID), tierOf(b.PeerID)
		if ta != tb {
			return ta > tb
		}
		ma, mb := metricsOf(a.PeerID), metricsOf(b.PeerID)
		if ma.LatencyMs != mb.LatencyMs {
			return ma.LatencyMs < mb.LatencyMs
		}
		return ma.Load < mb.Load
	})
	return eligible
}
