package node

import (
	"sync"

	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// MemberSet is the set of peer IDs a node currently treats as network members —
// the coarse (PeerID-granular) admission layer for the private discovery plane
// (C1.3 §15.3). It is seeded from what the node already knows are members (the
// coordinator directory, the bootstrap bundle, verified records) and gates who
// may enter the routing table or hold a connection. It is NOT the full
// authorization check — a member's certificate is verified separately, after the
// connection, by the member-auth protocol (C1.3b). It is safe for concurrent use.
type MemberSet struct {
	mu  sync.RWMutex
	ids map[peer.ID]struct{}
}

// NewMemberSet returns an empty member set.
func NewMemberSet() *MemberSet {
	return &MemberSet{ids: make(map[peer.ID]struct{})}
}

// Set replaces the membership with exactly ids.
func (m *MemberSet) Set(ids []peer.ID) {
	next := make(map[peer.ID]struct{}, len(ids))
	for _, id := range ids {
		next[id] = struct{}{}
	}
	m.mu.Lock()
	m.ids = next
	m.mu.Unlock()
}

// Add adds one member.
func (m *MemberSet) Add(id peer.ID) {
	m.mu.Lock()
	m.ids[id] = struct{}{}
	m.mu.Unlock()
}

// Has reports whether id is a known member.
func (m *MemberSet) Has(id peer.ID) bool {
	m.mu.RLock()
	_, ok := m.ids[id]
	m.mu.RUnlock()
	return ok
}

// Len reports how many members are known.
func (m *MemberSet) Len() int {
	m.mu.RLock()
	n := len(m.ids)
	m.mu.RUnlock()
	return n
}

// memberGater is a libp2p ConnectionGater that admits only known members. It
// rejects a stranger both when dialing out (InterceptPeerDial / InterceptAddrDial)
// and, decisively, after the inbound security handshake reveals the peer ID
// (InterceptSecured) — so a stranger's connection is dropped server-side and
// never enters the peerstore, routing table, or any RPC path.
type memberGater struct{ ms *MemberSet }

func (g memberGater) InterceptPeerDial(p peer.ID) bool { return g.ms.Has(p) }

func (g memberGater) InterceptAddrDial(p peer.ID, _ multiaddr.Multiaddr) bool {
	return g.ms.Has(p)
}

// InterceptAccept allows the raw inbound connection; the peer ID is not known
// until the security handshake, so the real decision is made in InterceptSecured.
func (g memberGater) InterceptAccept(network.ConnMultiaddrs) bool { return true }

func (g memberGater) InterceptSecured(_ network.Direction, p peer.ID, _ network.ConnMultiaddrs) bool {
	return g.ms.Has(p)
}

func (g memberGater) InterceptUpgraded(network.Conn) (bool, control.DisconnectReason) {
	return true, 0
}
