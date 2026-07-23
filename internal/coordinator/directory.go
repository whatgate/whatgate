// Package coordinator implements WhatGate's central coordination plane: the node
// directory, invite-based admission, and (later) the trust graph. It handles
// only metadata — never proxied traffic, which flows peer-to-peer.
//
// The Directory is the live registry of participating nodes. Nodes refresh their
// entry periodically; entries that stop refreshing expire, so the directory
// reflects who is currently reachable and willing to serve as an exit.
package coordinator

import (
	"sync"
	"time"
)

// NodeInfo is a node's advertised presence in the directory.
type NodeInfo struct {
	PeerID   string   // libp2p peer ID
	Addrs    []string // dialable multiaddrs
	Region   string   // exit region tag, e.g. "JP", "US"
	WantExit bool     // whether the node has opted in to serve as an exit
	LastSeen time.Time
}

// Directory is an in-memory, expiring registry of nodes.
type Directory struct {
	ttl time.Duration
	now func() time.Time

	mu      sync.RWMutex
	entries map[string]NodeInfo // keyed by PeerID
}

// NewDirectory creates a Directory whose entries expire after ttl. now supplies
// the current time (injectable for tests); nil defaults to time.Now.
func NewDirectory(ttl time.Duration, now func() time.Time) *Directory {
	if now == nil {
		now = time.Now
	}
	return &Directory{
		ttl:     ttl,
		now:     now,
		entries: make(map[string]NodeInfo),
	}
}

// Register inserts or refreshes a node's entry, stamping it with the current
// time so its freshness can be tracked.
func (d *Directory) Register(n NodeInfo) {
	n.LastSeen = d.now()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries[n.PeerID] = n
}

// Unregister removes a node's entry.
func (d *Directory) Unregister(peerID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.entries, peerID)
}

// List returns all currently live (non-expired) entries.
func (d *Directory) List() []NodeInfo {
	cutoff := d.now().Add(-d.ttl)
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]NodeInfo, 0, len(d.entries))
	for _, n := range d.entries {
		if n.LastSeen.After(cutoff) {
			out = append(out, n)
		}
	}
	return out
}
