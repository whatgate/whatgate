package exit

import (
	"testing"

	"github.com/whatgate/whatgate/internal/trust"
)

func TestAuthorizeRejectsUntrustedRequester(t *testing.T) {
	g := NewGuard(Policy{Scope: trust.ScopeConservative})

	_, err := g.Authorize(Request{RequesterTier: trust.TierStranger, Host: "example.com", Port: 443})
	if err != ErrUntrustedRequester {
		t.Fatalf("err = %v, want ErrUntrustedRequester", err)
	}
}

func TestAuthorizeRejectsLowReputation(t *testing.T) {
	g := NewGuard(Policy{Scope: trust.ScopeOpen, MinRequesterReputation: 0})

	_, err := g.Authorize(Request{RequesterTier: trust.TierStranger, RequesterReputation: -10, Host: "example.com", Port: 443})
	if err != ErrLowReputation {
		t.Fatalf("err = %v, want ErrLowReputation", err)
	}
}

func TestAuthorizeAllowsReputationAtThreshold(t *testing.T) {
	g := NewGuard(Policy{Scope: trust.ScopeOpen, MinRequesterReputation: 0})

	release, err := g.Authorize(Request{RequesterTier: trust.TierStranger, RequesterReputation: 0, Host: "example.com", Port: 443})
	if err != nil {
		t.Fatalf("reputation at threshold should be allowed: %v", err)
	}
	release()
}

func TestAuthorizeRejectsBlockedPort(t *testing.T) {
	g := NewGuard(Policy{Scope: trust.ScopeOpen, BlockedPorts: DefaultBlockedPorts()})

	_, err := g.Authorize(Request{RequesterTier: trust.TierStranger, Host: "mail.example.com", Port: 25})
	if err != ErrBlockedPort {
		t.Fatalf("err = %v, want ErrBlockedPort", err)
	}
}

func TestAuthorizeRejectsBlockedDomain(t *testing.T) {
	g := NewGuard(Policy{Scope: trust.ScopeOpen, BlockedDomains: map[string]bool{"evil.example": true}})

	_, err := g.Authorize(Request{RequesterTier: trust.TierStranger, Host: "evil.example", Port: 443})
	if err != ErrBlockedDomain {
		t.Fatalf("err = %v, want ErrBlockedDomain", err)
	}
}

func TestBlockedDomainMatchingIsRobust(t *testing.T) {
	g := NewGuard(Policy{Scope: trust.ScopeOpen, BlockedDomains: map[string]bool{"evil.example": true}})

	blocked := []string{
		"evil.example",     // exact
		"EVIL.example",     // case-insensitive
		"evil.example.",    // trailing dot
		"sub.evil.example", // subdomain
		"a.b.evil.example", // deep subdomain
	}
	for _, h := range blocked {
		if _, err := g.Authorize(Request{RequesterTier: trust.TierStranger, Host: h, Port: 443}); err != ErrBlockedDomain {
			t.Errorf("host %q: err = %v, want ErrBlockedDomain", h, err)
		}
	}
	// A domain that merely ends with the same text but isn't a subdomain must
	// NOT be blocked.
	if release, err := g.Authorize(Request{RequesterTier: trust.TierStranger, Host: "notevil.example", Port: 443}); err != nil {
		t.Errorf("notevil.example should be allowed: %v", err)
	} else {
		release()
	}
}

func TestBlockedDomainSetSupportsIPAndCIDR(t *testing.T) {
	g := NewGuard(Policy{Scope: trust.ScopeOpen, AllowPrivateTargets: true, BlockedDomains: map[string]bool{
		"198.51.100.7":   true, // single IP
		"203.0.113.0/24": true, // CIDR
	}})
	for _, h := range []string{"198.51.100.7", "203.0.113.5", "203.0.113.200"} {
		if _, err := g.Authorize(Request{RequesterTier: trust.TierStranger, Host: h, Port: 443}); err != ErrBlockedDomain {
			t.Errorf("host %q: err = %v, want ErrBlockedDomain", h, err)
		}
	}
	// An IP outside the blocked ranges is allowed.
	if release, err := g.Authorize(Request{RequesterTier: trust.TierStranger, Host: "203.0.114.5", Port: 443}); err != nil {
		t.Errorf("203.0.114.5 should be allowed: %v", err)
	} else {
		release()
	}
}

func TestSetBlockedDomainsUpdatesDynamically(t *testing.T) {
	g := NewGuard(Policy{Scope: trust.ScopeOpen})

	req := Request{RequesterTier: trust.TierStranger, Host: "feed-bad.example", Port: 443}
	if release, err := g.Authorize(req); err != nil {
		t.Fatalf("initially should be allowed: %v", err)
	} else {
		release()
	}

	// A threat feed refresh adds the domain.
	g.SetBlockedDomains(map[string]bool{"feed-bad.example": true})
	if _, err := g.Authorize(req); err != ErrBlockedDomain {
		t.Fatalf("after feed update err = %v, want ErrBlockedDomain", err)
	}
}

func TestAuthorizeBlocksPrivateIPLiteralTarget(t *testing.T) {
	g := NewGuard(Policy{Scope: trust.ScopeOpen}) // AllowPrivateTargets defaults false
	for _, host := range []string{"127.0.0.1", "169.254.169.254", "10.0.0.1", "192.168.1.1", "::1"} {
		if _, err := g.Authorize(Request{RequesterTier: trust.TierStranger, Host: host, Port: 80}); err != ErrBlockedPrivateTarget {
			t.Errorf("host %s: err = %v, want ErrBlockedPrivateTarget", host, err)
		}
	}
	// A public IP literal is allowed.
	if release, err := g.Authorize(Request{RequesterTier: trust.TierStranger, Host: "1.1.1.1", Port: 443}); err != nil {
		t.Fatalf("public IP literal should be allowed: %v", err)
	} else {
		release()
	}
}

func TestAuthorizeAllowPrivateTargetsBypass(t *testing.T) {
	g := NewGuard(Policy{Scope: trust.ScopeOpen, AllowPrivateTargets: true})
	release, err := g.Authorize(Request{RequesterTier: trust.TierStranger, Host: "127.0.0.1", Port: 80})
	if err != nil {
		t.Fatalf("with AllowPrivateTargets, loopback should be allowed: %v", err)
	}
	release()
}

func TestAuthorizeEnforcesPerRequesterCap(t *testing.T) {
	g := NewGuard(Policy{Scope: trust.ScopeOpen, MaxConnsPerRequester: 1})
	reqA := Request{RequesterID: "peerA", RequesterTier: trust.TierStranger, Host: "example.com", Port: 443}
	reqB := Request{RequesterID: "peerB", RequesterTier: trust.TierStranger, Host: "example.com", Port: 443}

	relA, err := g.Authorize(reqA)
	if err != nil {
		t.Fatalf("A first: %v", err)
	}
	// Same requester's second concurrent conn is refused...
	if _, err := g.Authorize(reqA); err != ErrTooManyRequesterConns {
		t.Fatalf("A second err = %v, want ErrTooManyRequesterConns", err)
	}
	// ...but a different requester is unaffected.
	relB, err := g.Authorize(reqB)
	if err != nil {
		t.Fatalf("B first: %v", err)
	}
	relB()

	// After A releases, A can connect again.
	relA()
	if rel, err := g.Authorize(reqA); err != nil {
		t.Fatalf("A after release: %v", err)
	} else {
		rel()
	}
}

func TestAuthorizeRateLimitsRequesterChurn(t *testing.T) {
	// A requester that opens and immediately closes connections never trips the
	// concurrency cap, but the per-requester rate limit still throttles the churn.
	g := NewGuard(Policy{Scope: trust.ScopeOpen, RequesterRatePerSec: 1, RequesterBurst: 2})
	reqA := Request{RequesterID: "peerA", RequesterTier: trust.TierStranger, Host: "example.com", Port: 443}

	// Burst of 2 establishment attempts is allowed (released immediately each time).
	for i := 0; i < 2; i++ {
		rel, err := g.Authorize(reqA)
		if err != nil {
			t.Fatalf("attempt %d within burst: %v", i, err)
		}
		rel()
	}
	// The 3rd attempt in the same instant exceeds the burst and is refused.
	if _, err := g.Authorize(reqA); err != ErrRequesterRateLimited {
		t.Fatalf("over-burst err = %v, want ErrRequesterRateLimited", err)
	}
	// A different requester has an independent bucket and is unaffected.
	reqB := Request{RequesterID: "peerB", RequesterTier: trust.TierStranger, Host: "example.com", Port: 443}
	if rel, err := g.Authorize(reqB); err != nil {
		t.Fatalf("peerB should be unaffected: %v", err)
	} else {
		rel()
	}
}

func TestAuthorizeRateLimitDisabledByDefault(t *testing.T) {
	// With no rate configured, rapid churn from one requester is never throttled.
	g := NewGuard(Policy{Scope: trust.ScopeOpen})
	reqA := Request{RequesterID: "peerA", RequesterTier: trust.TierStranger, Host: "example.com", Port: 443}
	for i := 0; i < 50; i++ {
		rel, err := g.Authorize(reqA)
		if err != nil {
			t.Fatalf("attempt %d should not be rate limited when disabled: %v", i, err)
		}
		rel()
	}
}

func TestAuthorizeRejectsOverBandwidthBudget(t *testing.T) {
	// A requester that has blown its byte budget is refused new connections
	// (the breaker), until the budget refills.
	g := NewGuard(Policy{Scope: trust.ScopeOpen, RequesterBytesPerSec: 1000, RequesterByteBurst: 1000})
	req := Request{RequesterID: "hog", RequesterTier: trust.TierStranger, Host: "example.com", Port: 443}

	// First connection is allowed and its transfer blows the budget.
	rel, err := g.Authorize(req)
	if err != nil {
		t.Fatalf("first authorize: %v", err)
	}
	rel()
	if tripped := g.Charge("hog", 5000); !tripped {
		t.Fatal("5000 bytes over 1000 budget should trip")
	}
	// The next connection attempt is refused by the breaker.
	if _, err := g.Authorize(req); err != ErrBandwidthExceeded {
		t.Fatalf("over-budget authorize err = %v, want ErrBandwidthExceeded", err)
	}
}

func TestChargeNoopWhenBandwidthDisabled(t *testing.T) {
	g := NewGuard(Policy{Scope: trust.ScopeOpen})
	if g.BandwidthLimited() {
		t.Fatal("bandwidth limiting should be disabled by default")
	}
	if g.Charge("x", 1<<30) {
		t.Fatal("Charge must be a no-op (never trip) when bandwidth limiting is off")
	}
}

func TestAuthorizeAllowsAndReleases(t *testing.T) {
	g := NewGuard(Policy{Scope: trust.ScopeOpen})

	release, err := g.Authorize(Request{RequesterTier: trust.TierStranger, Host: "example.com", Port: 443})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if release == nil {
		t.Fatal("release should not be nil on success")
	}
	release()
}

func TestAuthorizeEnforcesMaxConns(t *testing.T) {
	g := NewGuard(Policy{Scope: trust.ScopeOpen, MaxConns: 1})
	req := Request{RequesterTier: trust.TierEndorsed, Host: "example.com", Port: 443}

	release, err := g.Authorize(req)
	if err != nil {
		t.Fatalf("first Authorize: %v", err)
	}
	if _, err := g.Authorize(req); err != ErrTooManyConns {
		t.Fatalf("second Authorize err = %v, want ErrTooManyConns", err)
	}

	release() // free the slot
	if _, err := g.Authorize(req); err != nil {
		t.Fatalf("after release Authorize: %v", err)
	}
}
