package node

import (
	"testing"

	"github.com/whatgate/whatgate/internal/exit"
	"github.com/whatgate/whatgate/internal/trust"
)

// meterFor returns nil when bandwidth limiting is off (so the relay keeps its
// zero-overhead io.Copy fast path), and a working meter when it is on: charging
// past the budget trips it and fires the reputation penalty + metric exactly once.
func TestGuardedExitBandwidthMeter(t *testing.T) {
	// Disabled: no meter.
	off := GuardedExit{Guard: exit.NewGuard(exit.Policy{Scope: trust.ScopeOpen})}
	if off.meterFor("x") != nil {
		t.Fatal("meterFor should be nil when bandwidth limiting is disabled")
	}

	var reports []trust.Outcome
	var events []string
	cfg := GuardedExit{
		Guard: exit.NewGuard(exit.Policy{
			Scope: trust.ScopeOpen, RequesterBytesPerSec: 1000, RequesterByteBurst: 1000,
		}),
		Report:  func(_ string, o trust.Outcome) { reports = append(reports, o) },
		Metrics: func(e string) { events = append(events, e) },
	}
	meter := cfg.meterFor("hog")
	if meter == nil {
		t.Fatal("meterFor should return a meter when bandwidth limiting is enabled")
	}
	if meter(500) {
		t.Fatal("500 within budget should not trip")
	}
	if !meter(2000) {
		t.Fatal("2500 total over 1000 budget should trip")
	}
	// Repeated over-budget charges must not fire the penalty again.
	meter(2000)

	if len(reports) != 1 || reports[0] != trust.OutcomeBlocked {
		t.Fatalf("reports = %v, want one OutcomeBlocked", reports)
	}
	if len(events) != 1 || events[0] != "bandwidth-tripped" {
		t.Fatalf("events = %v, want one bandwidth-tripped", events)
	}
}

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
