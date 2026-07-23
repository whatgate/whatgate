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

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"

	"github.com/whatgate/whatgate/internal/tunnel"
)

// TunnelProtocol identifies WhatGate tunnel streams on a libp2p host.
const TunnelProtocol = protocol.ID("/whatgate/tunnel/0.1.0")

// Node is a WhatGate participant backed by a libp2p host.
type Node struct {
	h host.Host
}

// New creates a Node listening on the given multiaddr strings. With no
// addresses it listens on an OS-assigned TCP port on all interfaces.
func New(ctx context.Context, listenAddrs ...string) (*Node, error) {
	if len(listenAddrs) == 0 {
		listenAddrs = []string{"/ip4/0.0.0.0/tcp/0"}
	}
	h, err := libp2p.New(libp2p.ListenAddrStrings(listenAddrs...))
	if err != nil {
		return nil, err
	}
	return &Node{h: h}, nil
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

// EnableExit makes this node serve inbound tunnel streams, dialing each
// requested target via dial. This is the opt-in that turns a node into an exit
// for others; callers must only invoke it with the user's explicit consent.
func (n *Node) EnableExit(dial tunnel.DialFunc) {
	n.h.SetStreamHandler(TunnelProtocol, func(s network.Stream) {
		_ = tunnel.ServeExit(streamConn{s}, dial)
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
			s, err := n.h.NewStream(ctx, exit, TunnelProtocol)
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
