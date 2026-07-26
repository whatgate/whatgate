package routing

import (
	"github.com/whatgate/whatgate/internal/coordinator"
	"github.com/whatgate/whatgate/internal/trust"
)

// DHTExit is an exit discovered and verified on the private DHT (Tier C). It is
// deliberately minimal — the trust/reputation dimension is absent because the DHT
// plane cannot supply it (§4 four-state: DHT gives authorized/eligible/reachable,
// never recommended). Callers build these from node.VerifiedExit records.
type DHTExit struct {
	PeerID string
	Region string
	Addrs  []string
}

// MergeExits unions the coordinator directory (authoritative: carries trust
// tiers, and — via the reputation/graph — the "recommended" dimension) with
// DHT-resolved exits, deduping by peer ID. Coordinator entries win on conflict
// and keep their authoritative annotation; a DHT-only exit is added as a stranger
// because the DHT plane cannot vouch for its trust. It returns the merged node
// list and a tierOf wrapper reporting the coordinator tier for known peers and
// TierStranger for DHT-only ones. Feeding this to RankExits means a conservative
// scope naturally excludes DHT-only strangers (no authoritative trust) while an
// open scope includes them — the four-state merge without a bespoke state machine.
func MergeExits(coordNodes []coordinator.NodeInfo, coordTierOf func(string) trust.Tier, dhtExits []DHTExit) ([]coordinator.NodeInfo, func(string) trust.Tier) {
	known := make(map[string]struct{}, len(coordNodes))
	for _, n := range coordNodes {
		known[n.PeerID] = struct{}{}
	}

	merged := append([]coordinator.NodeInfo(nil), coordNodes...)
	dhtOnly := make(map[string]struct{})
	for _, e := range dhtExits {
		if _, ok := known[e.PeerID]; ok {
			continue // coordinator's authoritative entry wins
		}
		if _, dup := dhtOnly[e.PeerID]; dup {
			continue
		}
		dhtOnly[e.PeerID] = struct{}{}
		merged = append(merged, coordinator.NodeInfo{
			PeerID:   e.PeerID,
			Addrs:    e.Addrs,
			Region:   e.Region,
			WantExit: true,
		})
	}

	tierOf := func(id string) trust.Tier {
		if _, ok := known[id]; ok {
			return coordTierOf(id)
		}
		return trust.TierStranger // DHT-only: the DHT cannot vouch trust
	}
	return merged, tierOf
}
