package routing

import (
	"sort"

	"github.com/whatgate/whatgate/internal/coordinator"
	"github.com/whatgate/whatgate/internal/trust"
)

// Metrics are the measured qualities of a candidate exit used for ranking.
type Metrics struct {
	LatencyMs int // round-trip latency; lower is better
	Load      int // exit's current active connection count; lower is better
}

// RankExits returns the trust-scope-eligible exits for a region, best first.
// Ranking is lexicographic: more trusted first, then lower latency, then lower
// load. This makes trust the primary criterion while still preferring fast,
// lightly loaded exits among equally trusted ones. Not yet implemented.
func RankExits(
	nodes []coordinator.NodeInfo,
	region, selfID string,
	scope trust.Scope,
	tierOf func(peerID string) trust.Tier,
	metricsOf func(peerID string) Metrics,
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
		ta, tb := tierOf(a.PeerID), tierOf(b.PeerID)
		if ta != tb {
			return ta > tb // more trusted first
		}
		ma, mb := metricsOf(a.PeerID), metricsOf(b.PeerID)
		if ma.LatencyMs != mb.LatencyMs {
			return ma.LatencyMs < mb.LatencyMs // lower latency first
		}
		return ma.Load < mb.Load // lower load first
	})
	return eligible
}
