// Package exit implements ExitGuard: the protection an exit node applies before
// serving another peer's traffic. It is the concrete answer to "don't let
// strangers use my IP to do harm" — enforcing the operator's trust scope,
// destination policy, and concurrency limits. All checks are pure and injected
// with the requester's already-computed trust tier, so the guard is fully
// testable without any network.
package exit

import (
	"errors"
	"sync"

	"github.com/whatgate/whatgate/internal/trust"
)

// Guard decisions.
var (
	ErrUntrustedRequester = errors.New("exit: requester outside trust scope")
	ErrLowReputation      = errors.New("exit: requester reputation below threshold")
	ErrBlockedPort        = errors.New("exit: destination port blocked by policy")
	ErrBlockedDomain      = errors.New("exit: destination domain blocked by policy")
	ErrTooManyConns       = errors.New("exit: connection limit reached")
)

// DefaultBlockedPorts returns ports an exit should refuse by default (e.g. SMTP,
// to avoid being used for spam).
func DefaultBlockedPorts() map[int]bool {
	return map[int]bool{
		25:  true, // SMTP
		465: true, // SMTPS
		587: true, // submission
	}
}

// Policy is an exit operator's ExitGuard configuration.
type Policy struct {
	Scope                  trust.Scope     // only serve requesters within this trust scope
	MinRequesterReputation int             // refuse requesters with reputation below this
	BlockedPorts           map[int]bool    // destination ports to refuse
	BlockedDomains         map[string]bool // destination hosts to refuse
	MaxConns               int             // max concurrent served connections (0 = unlimited)
}

// Request describes one exit attempt to authorize.
type Request struct {
	RequesterTier       trust.Tier
	RequesterReputation int
	Host                string
	Port                int
}

// Guard enforces a Policy and tracks concurrency.
type Guard struct {
	policy Policy

	mu     sync.Mutex
	active int
}

// NewGuard creates a Guard for the given policy.
func NewGuard(p Policy) *Guard {
	return &Guard{policy: p}
}

// Authorize decides whether to serve req. On success it returns a release func
// the caller must invoke when the connection ends (to free a concurrency slot).
func (g *Guard) Authorize(req Request) (release func(), err error) {
	if !g.policy.Scope.Allows(req.RequesterTier) {
		return nil, ErrUntrustedRequester
	}
	if req.RequesterReputation < g.policy.MinRequesterReputation {
		return nil, ErrLowReputation
	}
	if g.policy.BlockedPorts[req.Port] {
		return nil, ErrBlockedPort
	}
	if g.policy.BlockedDomains[req.Host] {
		return nil, ErrBlockedDomain
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.policy.MaxConns > 0 && g.active >= g.policy.MaxConns {
		return nil, ErrTooManyConns
	}
	g.active++

	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			g.active--
			g.mu.Unlock()
		})
	}, nil
}
