// Package exit implements ExitGuard: the protection an exit node applies before
// serving another peer's traffic. It is the concrete answer to "don't let
// strangers use my IP to do harm" — enforcing the operator's trust scope,
// destination policy, and concurrency limits. All checks are pure and injected
// with the requester's already-computed trust tier, so the guard is fully
// testable without any network.
package exit

import (
	"errors"
	"net"
	"strings"
	"sync"

	"github.com/whatgate/whatgate/internal/trust"
)

// Guard decisions.
var (
	ErrUntrustedRequester   = errors.New("exit: requester outside trust scope")
	ErrLowReputation        = errors.New("exit: requester reputation below threshold")
	ErrBlockedPort          = errors.New("exit: destination port blocked by policy")
	ErrBlockedDomain        = errors.New("exit: destination domain blocked by policy")
	ErrBlockedPrivateTarget = errors.New("exit: destination is a private/loopback/link-local address")
	ErrTooManyConns         = errors.New("exit: connection limit reached")
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
	// AllowPrivateTargets, when true, permits dialing private/loopback/link-local
	// destinations. Default false blocks them (SSRF protection) — see
	// DisallowedTargetIP and DialControl.
	AllowPrivateTargets bool
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

	domMu          sync.RWMutex
	blockedDomains map[string]bool // normalized domains (static policy + threat feed)
	blockedNets    []*net.IPNet    // IP/CIDR entries from the block set
}

// NewGuard creates a Guard for the given policy.
func NewGuard(p Policy) *Guard {
	g := &Guard{policy: p}
	g.blockedDomains, g.blockedNets = parseBlockSet(p.BlockedDomains)
	return g
}

// parseBlockSet splits a raw block set into normalized domain names and IP/CIDR
// networks. Domain entries are lower-cased and stripped of a trailing dot; an
// IP entry becomes a /32 or /128 net; a CIDR entry is parsed as-is.
func parseBlockSet(raw map[string]bool) (map[string]bool, []*net.IPNet) {
	domains := make(map[string]bool, len(raw))
	var nets []*net.IPNet
	for entry := range raw {
		if _, n, err := net.ParseCIDR(entry); err == nil {
			nets = append(nets, n)
			continue
		}
		if ip := net.ParseIP(entry); ip != nil {
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		domains[normalizeHost(entry)] = true
	}
	return domains, nets
}

// normalizeHost lower-cases a host and removes a single trailing dot so
// blocklist matching is case- and FQDN-insensitive.
func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

// AllowsPrivateTargets reports whether the policy permits dialing
// private/loopback targets (used by the UDP path to decide post-resolution
// filtering).
func (g *Guard) AllowsPrivateTargets() bool { return g.policy.AllowPrivateTargets }

// StaticBlockedDomains returns a copy of the operator-configured blocked domains
// (the baseline to merge a threat feed onto).
func (g *Guard) StaticBlockedDomains() map[string]bool {
	out := make(map[string]bool, len(g.policy.BlockedDomains))
	for d := range g.policy.BlockedDomains {
		out[d] = true
	}
	return out
}

// SetBlockedDomains atomically replaces the blocked set, e.g. after a
// threat-feed refresh (pass the union of static + feed entries). Entries may be
// domains, IPs, or CIDRs.
func (g *Guard) SetBlockedDomains(domains map[string]bool) {
	d, n := parseBlockSet(domains)
	g.domMu.Lock()
	g.blockedDomains = d
	g.blockedNets = n
	g.domMu.Unlock()
}

// isHostBlocked reports whether host (a hostname or IP literal) is blocked. A
// domain match covers the exact name and any subdomain of it; an IP literal is
// matched against the blocked IP/CIDR set.
func (g *Guard) isHostBlocked(host string) bool {
	g.domMu.RLock()
	defer g.domMu.RUnlock()

	if ip := net.ParseIP(host); ip != nil {
		for _, n := range g.blockedNets {
			if n.Contains(ip) {
				return true
			}
		}
		return false
	}

	h := normalizeHost(host)
	// Match h and each of its parent domains against the blocked set, so a
	// blocked "evil.example" also blocks "sub.evil.example" but not
	// "notevil.example".
	for {
		if g.blockedDomains[h] {
			return true
		}
		dot := strings.IndexByte(h, '.')
		if dot < 0 {
			return false
		}
		h = h[dot+1:]
	}
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
	if g.isHostBlocked(req.Host) {
		return nil, ErrBlockedDomain
	}
	// SSRF guard for IP-literal targets. Hostname targets are additionally
	// checked at dial time against their resolved IP (see DialControl), which
	// also defeats DNS rebinding.
	if !g.policy.AllowPrivateTargets {
		if ip := net.ParseIP(req.Host); ip != nil && DisallowedTargetIP(ip) {
			return nil, ErrBlockedPrivateTarget
		}
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
