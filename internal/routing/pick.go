// Package routing selects which exit a client should tunnel through. In M2 it
// filters the directory by region and exit-willingness; later milestones extend
// it with latency/load measurement and trust weighting.
package routing

import (
	"github.com/whatgate/whatgate/internal/coordinator"
	"github.com/whatgate/whatgate/internal/trust"
)

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

// PickExitScoped is PickExit with a trust-scope filter: a candidate is only
// eligible if its trust tier (from tierOf) is within the user's scope. This is
// how "only my circle" vs "whole network" is enforced during exit selection.
func PickExitScoped(nodes []coordinator.NodeInfo, region, selfID string, scope trust.Scope, tierOf func(peerID string) trust.Tier) (coordinator.NodeInfo, bool) {
	for _, n := range nodes {
		if n.PeerID == selfID || !n.WantExit || n.Region != region {
			continue
		}
		if !scope.Allows(tierOf(n.PeerID)) {
			continue
		}
		return n, true
	}
	return coordinator.NodeInfo{}, false
}
