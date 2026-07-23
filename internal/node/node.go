// Package node wires WhatGate's transport-agnostic tunnel onto a real libp2p
// host. A single Node is both a client (it can open tunnels to a chosen exit)
// and, when the user opts in, an exit (it serves inbound tunnel streams).
//
// libp2p provides peer identity (PeerID = public key), an encrypted transport,
// and stream multiplexing; this package adapts a libp2p stream to net.Conn and
// hands it to internal/tunnel.
package node

import (
	"context"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	relayclient "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	"github.com/multiformats/go-multiaddr"

	"github.com/whatgate/whatgate/internal/exit"
	"github.com/whatgate/whatgate/internal/trust"
	"github.com/whatgate/whatgate/internal/tunnel"
)

// TunnelProtocol identifies WhatGate tunnel streams on a libp2p host.
const TunnelProtocol = protocol.ID("/whatgate/tunnel/0.1.0")

// Node is a WhatGate participant backed by a libp2p host.
type Node struct {
	h        host.Host
	exitLoad atomic.Int64 // active inbound tunnel streams being served as exit
}

// ExitLoad reports how many tunnel streams this node is currently serving as an
// exit, used as a load signal for exit selection.
func (n *Node) ExitLoad() int { return int(n.exitLoad.Load()) }

// config holds Node construction options.
type config struct {
	listenAddrs  []string
	staticRelays []peer.AddrInfo
}

// Option configures a Node.
type Option func(*config)

// WithListenAddrs sets the libp2p listen multiaddrs. Defaults to an OS-assigned
// TCP port on all interfaces.
func WithListenAddrs(addrs ...string) Option {
	return func(c *config) { c.listenAddrs = addrs }
}

// WithStaticRelays configures Circuit Relay v2 fallback: the node reserves slots
// on these relays and advertises /p2p-circuit addresses, so peers can still
// reach it when direct connectivity (hole punching) fails.
func WithStaticRelays(relays ...peer.AddrInfo) Option {
	return func(c *config) { c.staticRelays = relays }
}

// New creates a Node. Without options it listens on an OS-assigned TCP port on
// all interfaces with no relay fallback.
func New(ctx context.Context, opts ...Option) (*Node, error) {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	if len(cfg.listenAddrs) == 0 {
		cfg.listenAddrs = []string{"/ip4/0.0.0.0/tcp/0"}
	}

	// NAT traversal for the P2P data plane: map ports where possible, offer an
	// AutoNAT service to peers, and hole-punch through NATs (DCUtR).
	libOpts := []libp2p.Option{
		libp2p.ListenAddrStrings(cfg.listenAddrs...),
		libp2p.NATPortMap(),
		libp2p.EnableNATService(),
		libp2p.EnableHolePunching(),
	}
	// Circuit Relay v2 fallback: reserve on the given relays when direct
	// connectivity cannot be established.
	if len(cfg.staticRelays) > 0 {
		libOpts = append(libOpts, libp2p.EnableAutoRelayWithStaticRelays(cfg.staticRelays))
	}

	h, err := libp2p.New(libOpts...)
	if err != nil {
		return nil, err
	}
	return &Node{h: h}, nil
}

// AddrStrings returns the node's listen multiaddrs as strings, for advertising
// to the coordinator directory.
func (n *Node) AddrStrings() []string {
	addrs := n.h.Addrs()
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.String())
	}
	return out
}

// AddrInfoFromStrings builds a peer.AddrInfo from a peer ID and multiaddr
// strings, e.g. a directory entry, for use with Connect.
func AddrInfoFromStrings(peerID string, addrs []string) (peer.AddrInfo, error) {
	id, err := peer.Decode(peerID)
	if err != nil {
		return peer.AddrInfo{}, err
	}
	mas := make([]multiaddr.Multiaddr, 0, len(addrs))
	for _, a := range addrs {
		m, err := multiaddr.NewMultiaddr(a)
		if err != nil {
			return peer.AddrInfo{}, err
		}
		mas = append(mas, m)
	}
	return peer.AddrInfo{ID: id, Addrs: mas}, nil
}

// ID returns this node's libp2p peer ID.
func (n *Node) ID() peer.ID { return n.h.ID() }

// Addrs returns the node's listen multiaddrs.
func (n *Node) Addrs() []multiaddr.Multiaddr { return n.h.Addrs() }

// AddrInfo returns the addressing bundle another node needs to connect here.
func (n *Node) AddrInfo() peer.AddrInfo {
	return peer.AddrInfo{ID: n.h.ID(), Addrs: n.h.Addrs()}
}

// Connect dials and connects to another node.
func (n *Node) Connect(ctx context.Context, ai peer.AddrInfo) error {
	return n.h.Connect(ctx, ai)
}

// Ping connects to a peer if needed and measures round-trip latency, used as a
// selection signal. Returns the RTT of the first successful ping.
func (n *Node) Ping(ctx context.Context, ai peer.AddrInfo) (time.Duration, error) {
	if err := n.h.Connect(ctx, ai); err != nil {
		return 0, err
	}
	res := <-ping.Ping(ctx, n.h, ai.ID)
	return res.RTT, res.Error
}

// ReserveRelay reserves a slot on the given relay so other peers can reach this
// node through it when a direct connection cannot be established. It connects to
// the relay first. This is the explicit counterpart to AutoRelay, used when a
// node knows it needs relay reachability.
func (n *Node) ReserveRelay(ctx context.Context, relay peer.AddrInfo) error {
	if err := n.h.Connect(ctx, relay); err != nil {
		return err
	}
	_, err := relayclient.Reserve(ctx, n.h, relay)
	return err
}

// CircuitAddrsVia builds /p2p-circuit multiaddrs that dial a target peer through
// the given relay. Combine with a target peer ID to form an AddrInfo that forces
// a relayed connection.
func CircuitAddrsVia(relay peer.AddrInfo) ([]multiaddr.Multiaddr, error) {
	hop, err := multiaddr.NewMultiaddr("/p2p/" + relay.ID.String() + "/p2p-circuit")
	if err != nil {
		return nil, err
	}
	out := make([]multiaddr.Multiaddr, 0, len(relay.Addrs))
	for _, a := range relay.Addrs {
		out = append(out, a.Encapsulate(hop))
	}
	return out, nil
}

// EnableExit makes this node serve inbound tunnel streams, dialing each
// requested target via dial. This is the opt-in that turns a node into an exit
// for others; callers must only invoke it with the user's explicit consent.
func (n *Node) EnableExit(dial tunnel.DialFunc) {
	n.setExitHandler(dial, nil)
}

// EnableGuardedExit is EnableExit with an ExitGuard: each request is authorized
// against guard using the requester's trust tier (from tierOf) before dialing.
// This is how an exit refuses strangers, blocked destinations, or excess load.
func (n *Node) EnableGuardedExit(dial tunnel.DialFunc, guard *exit.Guard, tierOf func(requesterID string) trust.Tier) {
	n.setExitHandler(dial, func(requesterID, target string) (func(), error) {
		host, portStr, err := net.SplitHostPort(target)
		if err != nil {
			return nil, err
		}
		port, _ := strconv.Atoi(portStr)
		return guard.Authorize(exit.Request{
			RequesterTier: tierOf(requesterID),
			Host:          host,
			Port:          port,
		})
	})
}

// setExitHandler registers the tunnel stream handler, tracking exit load and
// applying an optional per-request authorizer (ExitGuard).
func (n *Node) setExitHandler(dial tunnel.DialFunc, authWithRequester func(requesterID, target string) (func(), error)) {
	n.h.SetStreamHandler(TunnelProtocol, func(s network.Stream) {
		requesterID := s.Conn().RemotePeer().String()
		n.exitLoad.Add(1)
		defer n.exitLoad.Add(-1)

		var authorize tunnel.Authorizer
		if authWithRequester != nil {
			authorize = func(target string) (func(), error) {
				return authWithRequester(requesterID, target)
			}
		}
		_ = tunnel.ServeExit(streamConn{s}, authorize, dial)
	})
}

// DisableExit stops serving inbound tunnel streams.
func (n *Node) DisableExit() {
	n.h.RemoveStreamHandler(TunnelProtocol)
}

// NewClientDialer returns a tunnel dialer that opens streams to the given exit
// peer. The caller (e.g. the SOCKS5 ingress) uses it to reach targets through
// that exit.
func (n *Node) NewClientDialer(exit peer.ID) *tunnel.ClientDialer {
	return &tunnel.ClientDialer{
		Open: func(ctx context.Context) (net.Conn, error) {
			// Allow opening the tunnel stream over a limited (relayed) connection;
			// otherwise NewStream would wait for a direct connection that may never
			// exist when only relay connectivity is available.
			sctx := network.WithAllowLimitedConn(ctx, "whatgate-tunnel")
			s, err := n.h.NewStream(sctx, exit, TunnelProtocol)
			if err != nil {
				return nil, err
			}
			return streamConn{s}, nil
		},
	}
}

// Close shuts down the underlying host.
func (n *Node) Close() error { return n.h.Close() }

// streamConn adapts a libp2p network.Stream to net.Conn. The stream already
// provides Read/Write/Close/SetDeadline; only the addr accessors are missing.
type streamConn struct {
	network.Stream
}

func (streamConn) LocalAddr() net.Addr  { return tunnelAddr{} }
func (streamConn) RemoteAddr() net.Addr { return tunnelAddr{} }

type tunnelAddr struct{}

func (tunnelAddr) Network() string { return "libp2p" }
func (tunnelAddr) String() string  { return "libp2p-stream" }
