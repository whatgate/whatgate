// Package routing selects which exit a client should tunnel through. In M2 it
// filters the directory by region and exit-willingness; later milestones extend
// it with latency/load measurement and trust weighting.
package routing

import "github.com/whatgate/whatgate/internal/coordinator"

// PickExit chooses an exit for the given region from the directory, skipping the
// caller (selfID) and any node not offering to exit. It returns the first
// eligible candidate; richer ranking (latency/load/trust) arrives in later
// milestones.
func PickExit(nodes []coordinator.NodeInfo, region, selfID string) (coordinator.NodeInfo, bool) {
	for _, n := range nodes {
		if n.PeerID == selfID || !n.WantExit || n.Region != region {
			continue
		}
		return n, true
	}
	return coordinator.NodeInfo{}, false
}
