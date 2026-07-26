package relay

import (
	"testing"
	"time"

	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
)

// A zero Limits changes nothing: every field keeps the libp2p default, so an
// operator who sets no relay limits gets stock behavior.
func TestBuildResourcesZeroKeepsDefaults(t *testing.T) {
	def := relayv2.DefaultResources()
	got := buildResources(Limits{})

	if got.ReservationTTL != def.ReservationTTL ||
		got.MaxReservations != def.MaxReservations ||
		got.MaxCircuits != def.MaxCircuits ||
		got.MaxReservationsPerIP != def.MaxReservationsPerIP {
		t.Fatalf("scalar defaults changed: got %+v want %+v", got, def)
	}
	if got.Limit.Duration != def.Limit.Duration || got.Limit.Data != def.Limit.Data {
		t.Fatalf("limit defaults changed: got %+v want %+v", *got.Limit, *def.Limit)
	}
}

// Non-zero fields override the matching resource; the rest stay at default.
func TestBuildResourcesOverrides(t *testing.T) {
	got := buildResources(Limits{
		CircuitDuration:      90 * time.Second,
		CircuitDataBytes:     1 << 20,
		MaxReservations:      64,
		MaxCircuitsPerPeer:   4,
		MaxReservationsPerIP: 2,
		ReservationTTL:       30 * time.Minute,
	})
	if got.Limit.Duration != 90*time.Second || got.Limit.Data != 1<<20 {
		t.Fatalf("limit = %+v, want 90s/1MB", *got.Limit)
	}
	if got.MaxReservations != 64 || got.MaxCircuits != 4 ||
		got.MaxReservationsPerIP != 2 || got.ReservationTTL != 30*time.Minute {
		t.Fatalf("scalars not overridden: %+v", got)
	}
}

// Overriding only the duration must not zero the data cap (or vice versa): the
// unset half keeps its default rather than dropping to an unlimited 0.
func TestBuildResourcesPartialLimit(t *testing.T) {
	def := relayv2.DefaultResources()
	got := buildResources(Limits{CircuitDuration: 45 * time.Second})
	if got.Limit.Duration != 45*time.Second {
		t.Fatalf("duration = %v, want 45s", got.Limit.Duration)
	}
	if got.Limit.Data != def.Limit.Data {
		t.Fatalf("data = %d, want default %d (unset half preserved)", got.Limit.Data, def.Limit.Data)
	}
}
