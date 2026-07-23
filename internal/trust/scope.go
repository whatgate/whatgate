package trust

import "fmt"

// Scope is a user's trust-range policy: how far outside their own circle they
// are willing to trust (both when choosing an exit and, later, when serving as
// one). WhatGate does not preset this — the first-run wizard asks the user to
// choose, explaining the risk.
type Scope int

const (
	// ScopeConservative trusts only same-group peers and endorsed neighbors.
	ScopeConservative Scope = iota
	// ScopeOpen trusts the whole network (with policy-based safeguards).
	ScopeOpen
)

func (s Scope) String() string {
	switch s {
	case ScopeOpen:
		return "open"
	default:
		return "conservative"
	}
}

// ParseScope maps a user-facing string to a Scope.
func ParseScope(s string) (Scope, error) {
	switch s {
	case "conservative":
		return ScopeConservative, nil
	case "open":
		return ScopeOpen, nil
	default:
		return 0, fmt.Errorf("trust: unknown scope %q (want conservative|open)", s)
	}
}

// Allows reports whether a peer at the given trust tier is within this scope.
func (s Scope) Allows(t Tier) bool {
	switch s {
	case ScopeOpen:
		return true
	default: // conservative: same-group or endorsed only
		return t >= TierEndorsed
	}
}
