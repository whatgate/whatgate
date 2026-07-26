package node

import (
	"testing"

	"github.com/whatgate/whatgate/internal/exit"
	"github.com/whatgate/whatgate/internal/trust"
)

// The Metrics hook receives "served" on an authorized request and a stable
// "denied:<reason>" label on a policy rejection.
func TestGuardedExitMetricsHook(t *testing.T) {
	var events []string
	cfg := GuardedExit{
		Guard: exit.NewGuard(exit.Policy{
			Scope:        trust.ScopeOpen,
			BlockedPorts: map[int]bool{25: true},
		}),
		TierOf:  func(string) trust.Tier { return trust.TierStranger },
		Metrics: func(e string) { events = append(events, e) },
	}
	authorize := cfg.authorizer()

	// A public target on an allowed port is served.
	release, err := authorize("peerX", "1.1.1.1:443")
	if err != nil {
		t.Fatalf("expected served: %v", err)
	}
	release()

	// A blocked port is denied with the matching reason label.
	if _, err := authorize("peerX", "1.1.1.1:25"); err == nil {
		t.Fatal("expected blocked-port denial")
	}

	if len(events) != 2 || events[0] != "served" || events[1] != "denied:blocked-port" {
		t.Fatalf("events = %v, want [served denied:blocked-port]", events)
	}
}
