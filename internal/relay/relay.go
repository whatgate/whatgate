// Package relay runs a libp2p Circuit Relay v2 service. WhatGate co-locates it
// with the coordinator: when two nodes cannot establish a direct connection
// (hole punching fails), they route the P2P tunnel through this relay instead.
//
// The relay only forwards the already-encrypted libp2p stream; it cannot read
// the tunneled traffic.
package relay

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/multiformats/go-multiaddr"
)

// Relay is a libp2p host offering the Circuit Relay v2 service.
type Relay struct {
	h   host.Host
	svc *relayv2.Relay
}

// Limits caps a relay's resource use, so a co-located relay can't be abused to
// exhaust the operator's bandwidth or slots. A zero field keeps the libp2p
// default; set only what you want to tighten.
type Limits struct {
	CircuitDuration      time.Duration // per-circuit time limit before reset (default 2min)
	CircuitDataBytes     int64         // per-circuit per-direction data cap before reset (default 128KB)
	MaxReservations      int           // max active reservation slots (default 128)
	MaxCircuitsPerPeer   int           // max simultaneous open circuits per peer (default 16)
	MaxReservationsPerIP int           // max reservations from one IP (default 8)
	ReservationTTL       time.Duration // reservation lifetime (default 1h)
}

// buildResources maps WhatGate Limits onto libp2p relay Resources, starting from
// the library defaults and overriding only the fields the operator set (non-zero).
func buildResources(l Limits) relayv2.Resources {
	rc := relayv2.DefaultResources()
	if l.CircuitDuration > 0 || l.CircuitDataBytes > 0 {
		lim := *rc.Limit // copy the default limit so the unset half is preserved
		if l.CircuitDuration > 0 {
			lim.Duration = l.CircuitDuration
		}
		if l.CircuitDataBytes > 0 {
			lim.Data = l.CircuitDataBytes
		}
		rc.Limit = &lim
	}
	if l.MaxReservations > 0 {
		rc.MaxReservations = l.MaxReservations
	}
	if l.MaxCircuitsPerPeer > 0 {
		rc.MaxCircuits = l.MaxCircuitsPerPeer
	}
	if l.MaxReservationsPerIP > 0 {
		rc.MaxReservationsPerIP = l.MaxReservationsPerIP
	}
	if l.ReservationTTL > 0 {
		rc.ReservationTTL = l.ReservationTTL
	}
	return rc
}

// New starts a relay listening on the given multiaddr strings (default: an
// OS-assigned TCP port on all interfaces). limits caps the relay's resource use
// (a zero Limits keeps libp2p defaults). The hop service is enabled
// unconditionally (via the circuitv2 relay directly) so it works regardless of
// the host's perceived reachability.
func New(ctx context.Context, limits Limits, listenAddrs ...string) (*Relay, error) {
	if len(listenAddrs) == 0 {
		listenAddrs = []string{"/ip4/0.0.0.0/tcp/0"}
	}
	h, err := libp2p.New(libp2p.ListenAddrStrings(listenAddrs...))
	if err != nil {
		return nil, err
	}
	svc, err := relayv2.New(h, relayv2.WithResources(buildResources(limits)))
	if err != nil {
		_ = h.Close()
		return nil, err
	}
	return &Relay{h: h, svc: svc}, nil
}

// ID returns the relay's peer ID.
func (r *Relay) ID() peer.ID { return r.h.ID() }

// Addrs returns the relay's listen multiaddrs.
func (r *Relay) Addrs() []multiaddr.Multiaddr { return r.h.Addrs() }

// AddrInfo returns the relay's addressing bundle for nodes to reserve against.
func (r *Relay) AddrInfo() peer.AddrInfo {
	return peer.AddrInfo{ID: r.h.ID(), Addrs: r.h.Addrs()}
}

// Close shuts down the relay service and host.
func (r *Relay) Close() error {
	_ = r.svc.Close()
	return r.h.Close()
}
