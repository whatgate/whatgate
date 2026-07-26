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

	"github.com/whatgate/whatgate/internal/bandwidth"
	"github.com/whatgate/whatgate/internal/ratelimit"
	"github.com/whatgate/whatgate/internal/trust"
)

// Guard decisions.
var (
	ErrUntrustedRequester    = errors.New("exit: requester outside trust scope")
	ErrLowReputation         = errors.New("exit: requester reputation below threshold")
	ErrBlockedPort           = errors.New("exit: destination port blocked by policy")
	ErrBlockedDomain         = errors.New("exit: destination domain blocked by policy")
	ErrBlockedPrivateTarget  = errors.New("exit: destination is a private/loopback/link-local address")
	ErrTooManyConns          = errors.New("exit: connection limit reached")
	ErrTooManyRequesterConns = errors.New("exit: per-requester connection limit reached")
	ErrRequesterRateLimited  = errors.New("exit: per-requester connection rate limit exceeded")
	ErrBandwidthExceeded     = errors.New("exit: per-requester bandwidth budget exceeded")
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
	MaxConnsPerRequester   int             // max concurrent connections per requester (0 = unlimited)
	// RequesterRatePerSec limits how fast a single requester may open new
	// connections (0 = unlimited). Unlike MaxConnsPerRequester, it throttles
	// churn — rapid open/close bursts that never occupy a concurrency slot.
	// RequesterBurst is the momentary allowance; it defaults to RequesterRatePerSec
	// when unset (must be >= 1 to admit any request).
	RequesterRatePerSec float64
	RequesterBurst      float64
	// RequesterBytesPerSec caps a single requester's sustained throughput
	// (0 = unlimited), with RequesterByteBurst as the momentary allowance
	// (defaults to RequesterBytesPerSec). When a requester exceeds it the breaker
	// trips: its in-flight transfer is cut and new connections are refused until
	// the byte budget refills. Unlike the connection limits, this bounds volume.
	RequesterBytesPerSec float64
	RequesterByteBurst   float64
	// AllowPrivateTargets, when true, permits dialing private/loopback/link-local
	// destinations. Default false blocks them (SSRF protection) — see
	// DisallowedTargetIP and DialControl.
	AllowPrivateTargets bool
}

// Request describes one exit attempt to authorize.
type Request struct {
	RequesterID         string // peer ID of the requester (for per-requester limits)
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
	perReq map[string]int // active connections per requester ID

	reqLimiter *ratelimit.Limiter // per-requester connection-rate limiter (nil = disabled)
	bwLimiter  *bandwidth.Limiter // per-requester bandwidth limiter (nil = disabled)

	domMu          sync.RWMutex
	blockedDomains map[string]bool // normalized domains (static policy + threat feed)
	blockedNets    []*net.IPNet    // IP/CIDR entries from the block set
}

// NewGuard creates a Guard for the given policy.
func NewGuard(p Policy) *Guard {
	g := &Guard{policy: p, perReq: make(map[string]int)}
	g.blockedDomains, g.blockedNets = parseBlockSet(p.BlockedDomains)
	if p.RequesterRatePerSec > 0 {
		burst := p.RequesterBurst
		if burst <= 0 {
			burst = p.RequesterRatePerSec
		}
		g.reqLimiter = ratelimit.New(p.RequesterRatePerSec, burst)
	}
	if p.RequesterBytesPerSec > 0 {
		burst := p.RequesterByteBurst
		if burst <= 0 {
			burst = p.RequesterBytesPerSec
		}
		g.bwLimiter = bandwidth.New(p.RequesterBytesPerSec, burst)
	}
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

// BandwidthLimited reports whether per-requester bandwidth limiting is enabled,
// so the data path can skip per-chunk accounting when it is off.
func (g *Guard) BandwidthLimited() bool { return g.bwLimiter != nil }

// Charge records n bytes relayed on behalf of requesterID and reports whether the
// requester is now over its bandwidth budget (breaker tripped). It is a no-op
// returning false when bandwidth limiting is disabled.
func (g *Guard) Charge(requesterID string, n int) bool {
	if g.bwLimiter == nil {
		return false
	}
	return g.bwLimiter.Charge(requesterID, int64(n))
}

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

	// Throttle a single requester's connection-establishment rate (churn), a
	// distinct control from the concurrency cap below.
	if g.reqLimiter != nil && !g.reqLimiter.Allow(req.RequesterID) {
		return nil, ErrRequesterRateLimited
	}
	// Breaker: a requester that blew its bandwidth budget is refused new
	// connections until the budget refills.
	if g.bwLimiter != nil && g.bwLimiter.Over(req.RequesterID) {
		return nil, ErrBandwidthExceeded
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.policy.MaxConns > 0 && g.active >= g.policy.MaxConns {
		return nil, ErrTooManyConns
	}
	if g.policy.MaxConnsPerRequester > 0 && g.perReq[req.RequesterID] >= g.policy.MaxConnsPerRequester {
		return nil, ErrTooManyRequesterConns
	}
	g.active++
	g.perReq[req.RequesterID]++

	id := req.RequesterID
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			g.active--
			if g.perReq[id]--; g.perReq[id] <= 0 {
				delete(g.perReq, id)
			}
			g.mu.Unlock()
		})
	}, nil
}
