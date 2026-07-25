package node

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	libp2pprotocol "github.com/libp2p/go-libp2p/core/protocol"

	"github.com/whatgate/whatgate/internal/exit"
	"github.com/whatgate/whatgate/pkg/protocol"
)

// UDPTunnelProtocol identifies WhatGate UDP-datagram tunnel streams.
const UDPTunnelProtocol = libp2pprotocol.ID("/whatgate/udp/0.1.0")

// udpIdleTimeout closes a per-target UDP relay socket after this much silence.
const udpIdleTimeout = 60 * time.Second

// UDPAuthorizer authorizes forwarding to a target on the UDP exit, returning a
// release func to call when the per-target socket is torn down. A nil authorizer
// means allow everything.
type UDPAuthorizer func(target string) (release func(), err error)

// EnableUDPExit relays UDP datagrams for others without policy (allow all).
func (n *Node) EnableUDPExit() {
	n.h.SetStreamHandler(UDPTunnelProtocol, func(s network.Stream) {
		relayUDP(streamConn{s}, nil, false)
	})
}

// EnableGuardedUDPExit relays UDP datagrams, authorizing each new target through
// cfg.Guard (same policy as the TCP exit: trust scope, reputation, port/domain,
// concurrency). Hostname targets are additionally re-checked against their
// resolved IP so DNS names pointing at internal ranges are dropped (SSRF guard).
func (n *Node) EnableGuardedUDPExit(cfg GuardedExit) {
	authorize := cfg.authorizer()
	blockPrivate := cfg.Guard != nil && !cfg.Guard.AllowsPrivateTargets()
	n.h.SetStreamHandler(UDPTunnelProtocol, func(s network.Stream) {
		requesterID := s.Conn().RemotePeer().String()
		relayUDP(streamConn{s}, func(target string) (func(), error) {
			return authorize(requesterID, target)
		}, blockPrivate)
	})
}

// DisableUDPExit stops serving UDP tunnel streams.
func (n *Node) DisableUDPExit() {
	n.h.RemoveStreamHandler(UDPTunnelProtocol)
}

type udpTarget struct {
	conn    *net.UDPConn
	release func()
}

// relayUDP serves one UDP tunnel stream: it keeps a UDP socket per target
// (NAT-like), sending each framed datagram out and framing replies back. If
// authorize is non-nil, each new target must pass it (denied targets are
// dropped).
func relayUDP(stream net.Conn, authorize UDPAuthorizer, blockPrivate bool) {
	defer stream.Close()

	var wmu sync.Mutex
	writeBack := func(target string, payload []byte) {
		wmu.Lock()
		defer wmu.Unlock()
		_ = protocol.WriteDatagram(stream, target, payload)
	}

	var cmu sync.Mutex
	targets := make(map[string]*udpTarget)
	defer func() {
		cmu.Lock()
		for _, t := range targets {
			_ = t.conn.Close()
			if t.release != nil {
				t.release()
			}
		}
		cmu.Unlock()
	}()

	for {
		target, payload, err := protocol.ReadDatagram(stream)
		if err != nil {
			return
		}

		cmu.Lock()
		t, ok := targets[target]
		if !ok {
			var release func()
			if authorize != nil {
				rel, err := authorize(target)
				if err != nil {
					cmu.Unlock()
					continue // policy denied: drop datagrams to this target
				}
				release = rel
			}
			uaddr, err := net.ResolveUDPAddr("udp", target)
			if err != nil {
				if release != nil {
					release()
				}
				cmu.Unlock()
				continue
			}
			// SSRF guard: drop datagrams to a target that resolves into a
			// private/loopback/link-local range (catches hostnames; IP literals
			// are already refused by the guard's Authorize).
			if blockPrivate && exit.DisallowedTargetIP(uaddr.IP) {
				if release != nil {
					release()
				}
				cmu.Unlock()
				continue
			}
			c, err := net.DialUDP("udp", nil, uaddr)
			if err != nil {
				if release != nil {
					release()
				}
				cmu.Unlock()
				continue
			}
			t = &udpTarget{conn: c, release: release}
			targets[target] = t
			go func(name string, c *net.UDPConn) {
				buf := make([]byte, 64*1024)
				for {
					_ = c.SetReadDeadline(time.Now().Add(udpIdleTimeout))
					nr, err := c.Read(buf)
					if err != nil {
						return
					}
					writeBack(name, buf[:nr])
				}
			}(target, c)
		}
		cmu.Unlock()

		_, _ = t.conn.Write(payload)
	}
}

// UDPSession is the client end of a UDP tunnel to an exit: datagrams sent to any
// target travel over one libp2p stream, and replies come back tagged by target.
type UDPSession struct {
	stream net.Conn
	wmu    sync.Mutex
}

// OpenUDPSession opens a UDP tunnel stream to the exit.
func (n *Node) OpenUDPSession(ctx context.Context, exit peer.ID) (*UDPSession, error) {
	sctx := network.WithAllowLimitedConn(ctx, "whatgate-udp")
	s, err := n.h.NewStream(sctx, exit, UDPTunnelProtocol)
	if err != nil {
		return nil, err
	}
	return &UDPSession{stream: streamConn{s}}, nil
}

// Send tunnels one datagram to target ("host:port").
func (s *UDPSession) Send(target string, payload []byte) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return protocol.WriteDatagram(s.stream, target, payload)
}

// Receive blocks for the next reply datagram, returning its source target and
// payload.
func (s *UDPSession) Receive() (target string, payload []byte, err error) {
	return protocol.ReadDatagram(s.stream)
}

// Close ends the session.
func (s *UDPSession) Close() error { return s.stream.Close() }
