package node

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	dht "github.com/libp2p/go-libp2p-kad-dht"
)

// whatgateDHTPrefix namespaces WhatGate's DHT off the public IPFS DHT — DHT
// protocol IDs become /whatgate/kad/1.0.0. The prefix is isolation, not access
// control; membership is enforced by the connection gater and routing-table
// filter (C1.3 §5).
const whatgateDHTPrefix = protocol.ID("/whatgate/kad")

// WithMemberGater installs a connection gater that admits only peers in ms, so
// strangers cannot hold a connection to this node or enter its routing table.
// Enabling this restricts ALL of the host's connections to members, so ms must
// also include any infrastructure peers the node legitimately talks to (relays,
// bootstrap). It is opt-in and off by default.
func WithMemberGater(ms *MemberSet) Option {
	return func(c *config) { c.gater = memberGater{ms: ms} }
}

// PrivateDHT is WhatGate's private, member-scoped Kademlia DHT — the redundant
// discovery plane that survives the coordinator being blocked (Tier C1).
type PrivateDHT struct {
	d *dht.IpfsDHT
}

// StartPrivateDHT attaches a private-prefix Kademlia DHT to the node, filtering
// its routing table to members of ms, connects to the given bootstrap peers, and
// bootstraps. The node should have been built WithMemberGater(ms) so non-members
// are refused at the connection layer as well.
func (n *Node) StartPrivateDHT(ctx context.Context, ms *MemberSet, bootstrap []peer.AddrInfo) (*PrivateDHT, error) {
	d, err := dht.New(n.h,
		dht.Mode(dht.ModeServer),
		dht.ProtocolPrefix(whatgateDHTPrefix),
		dht.RoutingTableFilter(func(_ any, p peer.ID) bool { return ms.Has(p) }),
	)
	if err != nil {
		return nil, fmt.Errorf("private dht: %w", err)
	}
	for _, ai := range bootstrap {
		// Best-effort: an unreachable bootstrap peer must not abort startup.
		_ = n.h.Connect(ctx, ai)
	}
	if err := d.Bootstrap(ctx); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("private dht bootstrap: %w", err)
	}
	return &PrivateDHT{d: d}, nil
}

// RoutingTableSize reports how many members are in the DHT routing table.
func (p *PrivateDHT) RoutingTableSize() int { return p.d.RoutingTable().Size() }

// Bootstrap re-runs DHT bootstrapping (e.g. after the member set or peers change).
func (p *PrivateDHT) Bootstrap(ctx context.Context) error { return p.d.Bootstrap(ctx) }

// Close shuts down the DHT.
func (p *PrivateDHT) Close() error { return p.d.Close() }
