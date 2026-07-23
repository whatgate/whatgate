package node

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	libp2pprotocol "github.com/libp2p/go-libp2p/core/protocol"

	"github.com/whatgate/whatgate/pkg/protocol"
)

// UDPTunnelProtocol identifies WhatGate UDP-datagram tunnel streams.
const UDPTunnelProtocol = libp2pprotocol.ID("/whatgate/udp/0.1.0")

// udpIdleTimeout closes a per-target UDP relay socket after this much silence.
const udpIdleTimeout = 60 * time.Second

// EnableUDPExit makes this node relay UDP datagrams for others: each inbound UDP
// tunnel stream is served by forwarding framed datagrams to their targets and
// framing replies back. Gated by the same exit opt-in as TCP.
func (n *Node) EnableUDPExit() {
	n.h.SetStreamHandler(UDPTunnelProtocol, func(s network.Stream) {
		relayUDP(streamConn{s})
	})
}

// DisableUDPExit stops serving UDP tunnel streams.
func (n *Node) DisableUDPExit() {
	n.h.RemoveStreamHandler(UDPTunnelProtocol)
}

// relayUDP serves one UDP tunnel stream: it keeps a UDP socket per target
// (NAT-like), sending each framed datagram out and framing replies back.
func relayUDP(stream net.Conn) {
	defer stream.Close()

	var wmu sync.Mutex
	writeBack := func(target string, payload []byte) {
		wmu.Lock()
		defer wmu.Unlock()
		_ = protocol.WriteDatagram(stream, target, payload)
	}

	var cmu sync.Mutex
	conns := make(map[string]*net.UDPConn)
	defer func() {
		cmu.Lock()
		for _, c := range conns {
			_ = c.Close()
		}
		cmu.Unlock()
	}()

	for {
		target, payload, err := protocol.ReadDatagram(stream)
		if err != nil {
			return
		}

		cmu.Lock()
		conn, ok := conns[target]
		if !ok {
			uaddr, err := net.ResolveUDPAddr("udp", target)
			if err != nil {
				cmu.Unlock()
				continue
			}
			c, err := net.DialUDP("udp", nil, uaddr)
			if err != nil {
				cmu.Unlock()
				continue
			}
			conn = c
			conns[target] = c
			go func(t string, c *net.UDPConn) {
				buf := make([]byte, 64*1024)
				for {
					_ = c.SetReadDeadline(time.Now().Add(udpIdleTimeout))
					nr, err := c.Read(buf)
					if err != nil {
						return
					}
					writeBack(t, buf[:nr])
				}
			}(target, c)
		}
		cmu.Unlock()

		_, _ = conn.Write(payload)
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
