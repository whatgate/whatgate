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
