package ratelimit

import (
	"testing"
	"time"
)

// A fresh key may burst up to the configured burst, then is denied until tokens
// refill.
func TestAllowsBurstThenDenies(t *testing.T) {
	now := time.Unix(0, 0)
	l := New(1, 3) // 1 token/sec, burst 3
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if !l.Allow("ip") {
			t.Fatalf("burst request %d should be allowed", i+1)
		}
	}
	if l.Allow("ip") {
		t.Fatal("4th immediate request should be denied (burst exhausted)")
	}
}

// Tokens refill over time at the configured rate.
func TestRefillsOverTime(t *testing.T) {
	now := time.Unix(0, 0)
	l := New(2, 2) // 2 tokens/sec, burst 2
	l.now = func() time.Time { return now }

	l.Allow("ip")
	l.Allow("ip")
	if l.Allow("ip") {
		t.Fatal("should be denied after exhausting burst")
	}
	now = now.Add(time.Second) // +1s → +2 tokens (capped at burst 2)
	if !l.Allow("ip") || !l.Allow("ip") {
		t.Fatal("should be allowed again after refill")
	}
	if l.Allow("ip") {
		t.Fatal("refill is capped at burst")
	}
}

// Keys are rate-limited independently.
func TestPerKeyIndependent(t *testing.T) {
	now := time.Unix(0, 0)
	l := New(1, 1)
	l.now = func() time.Time { return now }

	if !l.Allow("a") {
		t.Fatal("a first request allowed")
	}
	if l.Allow("a") {
		t.Fatal("a second request denied")
	}
	if !l.Allow("b") {
		t.Fatal("b's limit is independent of a's")
	}
}
