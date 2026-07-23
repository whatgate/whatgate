// Package trust models WhatGate's layered federated trust: individuals belong to
// small groups (小网), groups endorse other groups, and trust between two peers
// is derived from that graph. It also tracks two-level reputation (per-peer and
// per-group) and enforces trust-scope policy when selecting exits.
//
// The graph is pure in-memory logic with no transport dependency, so it can be
// tested directly and reused by the coordinator (authoritative store) and by
// routing (trust-aware exit selection).
package trust

import "sync"

// Graph holds groups, their memberships, and inter-group endorsements.
type Graph struct {
	mu           sync.RWMutex
	groupMembers map[string]map[string]bool // groupID -> set of member peerIDs
	endorsements map[string]map[string]bool // fromGroup -> set of endorsed toGroups
}

// NewGraph returns an empty trust graph.
func NewGraph() *Graph {
	return &Graph{
		groupMembers: make(map[string]map[string]bool),
		endorsements: make(map[string]map[string]bool),
	}
}

// CreateGroup creates a group with the given founder as its first member.
func (g *Graph) CreateGroup(groupID, founderPeerID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.addMemberLocked(groupID, founderPeerID)
}

// AddMember adds a peer to a group, creating the group if needed.
func (g *Graph) AddMember(groupID, peerID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.addMemberLocked(groupID, peerID)
}

func (g *Graph) addMemberLocked(groupID, peerID string) {
	members, ok := g.groupMembers[groupID]
	if !ok {
		members = make(map[string]bool)
		g.groupMembers[groupID] = members
	}
	members[peerID] = true
}

// Export returns the graph's membership and endorsement edges as plain maps,
// for persistence.
func (g *Graph) Export() (groups map[string][]string, endorsements map[string][]string) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	groups = make(map[string][]string, len(g.groupMembers))
	for gid, members := range g.groupMembers {
		for p := range members {
			groups[gid] = append(groups[gid], p)
		}
	}
	endorsements = make(map[string][]string, len(g.endorsements))
	for fg, set := range g.endorsements {
		for tg := range set {
			endorsements[fg] = append(endorsements[fg], tg)
		}
	}
	return groups, endorsements
}

// Import merges membership and endorsement edges into the graph (used on load).
func (g *Graph) Import(groups map[string][]string, endorsements map[string][]string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for gid, members := range groups {
		for _, p := range members {
			g.addMemberLocked(gid, p)
		}
	}
	for fg, tgs := range endorsements {
		set := g.endorsements[fg]
		if set == nil {
			set = make(map[string]bool)
			g.endorsements[fg] = set
		}
		for _, tg := range tgs {
			set[tg] = true
		}
	}
}

// IsMember reports whether peerID belongs to groupID.
func (g *Graph) IsMember(groupID, peerID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.groupMembers[groupID][peerID]
}

// GroupsOf returns the IDs of groups the peer belongs to.
func (g *Graph) GroupsOf(peerID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var out []string
	for groupID, members := range g.groupMembers {
		if members[peerID] {
			out = append(out, groupID)
		}
	}
	return out
}
