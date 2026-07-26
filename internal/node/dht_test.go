package node

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func rawHost(t *testing.T) host.Host {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func addrInfo(h host.Host) peer.AddrInfo {
	return peer.AddrInfo{ID: h.ID(), Addrs: h.Addrs()}
}

// waitConnectedness polls up to ~3s for h's connectedness to p to reach want.
func waitConnectedness(h host.Host, p peer.ID, want network.Connectedness) network.Connectedness {
	for i := 0; i < 30; i++ {
		if h.Network().Connectedness(p) == want {
			return want
		}
		time.Sleep(100 * time.Millisecond)
	}
	return h.Network().Connectedness(p)
}

// A member-gated node retains a connection from a known member but rejects one
// from a stranger (server-side enforcement — the stranger ends up NotConnected).
func TestMemberGaterAdmitsMembersRejectsStrangers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	member := rawHost(t)
	stranger := rawHost(t)

	ms := NewMemberSet()
	ms.Set([]peer.ID{member.ID()}) // only `member` is a known member

	gated, err := New(ctx, WithMemberGater(ms))
	if err != nil {
		t.Fatalf("new gated node: %v", err)
	}
	defer gated.Close()
	gatedInfo := peer.AddrInfo{ID: gated.ID(), Addrs: gated.h.Addrs()}

	if err := member.Connect(ctx, gatedInfo); err != nil {
		t.Fatalf("member connect: %v", err)
	}
	if got := waitConnectedness(gated.h, member.ID(), network.Connected); got != network.Connected {
		t.Fatalf("member connectedness = %v, want Connected", got)
	}

	_ = stranger.Connect(ctx, gatedInfo) // dialer may see transient success
	if got := waitConnectedness(gated.h, stranger.ID(), network.NotConnected); got != network.NotConnected {
		t.Fatalf("stranger connectedness = %v, want NotConnected (gater must reject non-members)", got)
	}
}

// Two member nodes running the private-prefix DHT populate each other's routing
// table (the private authenticated discovery plane forms among members).
func TestPrivateDHTMembersFormRoutingTable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msA := NewMemberSet()
	msB := NewMemberSet()

	a, err := New(ctx, WithMemberGater(msA))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := New(ctx, WithMemberGater(msB))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	// Each treats the other as a known member.
	msA.Set([]peer.ID{b.ID()})
	msB.Set([]peer.ID{a.ID()})

	dhtA, err := a.StartPrivateDHT(ctx, msA, []peer.AddrInfo{addrInfo(b.h)})
	if err != nil {
		t.Fatalf("A StartPrivateDHT: %v", err)
	}
	defer dhtA.Close()
	dhtB, err := b.StartPrivateDHT(ctx, msB, []peer.AddrInfo{addrInfo(a.h)})
	if err != nil {
		t.Fatalf("B StartPrivateDHT: %v", err)
	}
	defer dhtB.Close()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if dhtA.RoutingTableSize() > 0 && dhtB.RoutingTableSize() > 0 {
			return // success: members formed the private DHT
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("private DHT routing tables did not form: A=%d B=%d", dhtA.RoutingTableSize(), dhtB.RoutingTableSize())
}
