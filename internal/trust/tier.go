package trust

// Tier is how much one peer trusts another, derived from the group graph.
// Higher means more trusted.
type Tier int

const (
	// TierStranger: no group relationship.
	TierStranger Tier = iota
	// TierEndorsed: the truster's group endorses the trustee's group.
	TierEndorsed
	// TierSameGroup: both peers share a group.
	TierSameGroup
)

func (t Tier) String() string {
	switch t {
	case TierSameGroup:
		return "same-group"
	case TierEndorsed:
		return "endorsed"
	default:
		return "stranger"
	}
}

// Endorse records that fromGroup vouches for toGroup (directional).
func (g *Graph) Endorse(fromGroup, toGroup string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	set, ok := g.endorsements[fromGroup]
	if !ok {
		set = make(map[string]bool)
		g.endorsements[fromGroup] = set
	}
	set[toGroup] = true
}

// Trust returns how much fromPeer trusts toPeer based on group membership and
// endorsements: same group outranks an endorsed neighbor, which outranks a
// stranger.
func (g *Graph) Trust(fromPeer, toPeer string) Tier {
	g.mu.RLock()
	defer g.mu.RUnlock()

	fromGroups := g.groupSetLocked(fromPeer)
	toGroups := g.groupSetLocked(toPeer)

	for gid := range fromGroups {
		if toGroups[gid] {
			return TierSameGroup
		}
	}
	for fg := range fromGroups {
		endorsed := g.endorsements[fg]
		for tg := range toGroups {
			if endorsed[tg] {
				return TierEndorsed
			}
		}
	}
	return TierStranger
}

// groupSetLocked returns the set of groups a peer belongs to. Caller holds the
// lock.
func (g *Graph) groupSetLocked(peerID string) map[string]bool {
	out := make(map[string]bool)
	for groupID, members := range g.groupMembers {
		if members[peerID] {
			out[groupID] = true
		}
	}
	return out
}
