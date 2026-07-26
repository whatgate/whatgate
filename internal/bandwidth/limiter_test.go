package bandwidth

import (
	"testing"
	"time"
)

// Bytes within the burst budget do not trip the breaker.
func TestChargeWithinBudget(t *testing.T) {
	l := New(1000, 1000)
	if l.Charge("a", 500) {
		t.Fatal("500 within 1000 budget should not trip")
	}
	if l.Charge("a", 400) {
		t.Fatal("900 total within budget should not trip")
	}
	if l.Over("a") {
		t.Fatal("a should not be over budget")
	}
}

// Exceeding the budget trips the breaker and Over reflects it.
func TestChargeOverBudgetTrips(t *testing.T) {
	l := New(1000, 1000)
	if !l.Charge("a", 1500) {
		t.Fatal("1500 over 1000 budget should trip")
	}
	if !l.Over("a") {
		t.Fatal("Over should report a as over budget")
	}
}

// A tripped key recovers after enough time passes for the bucket to refill —
// the breaker auto-resets proportional to the overage.
func TestBudgetRefillsAndRecovers(t *testing.T) {
	l := New(1000, 1000) // 1000 bytes/sec, burst 1000
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }

	l.Charge("a", 1000)      // bucket to 0
	if !l.Charge("a", 500) { // -500: tripped
		t.Fatal("should trip after exceeding")
	}
	if !l.Over("a") {
		t.Fatal("should be over right after tripping")
	}
	now = now.Add(1 * time.Second) // refill 1000 → +500
	if l.Over("a") {
		t.Fatal("should have recovered after refill")
	}
}

// Unknown keys are never over budget.
func TestOverUnknownKey(t *testing.T) {
	l := New(1000, 1000)
	if l.Over("x") {
		t.Fatal("unknown key should not be over budget")
	}
}

// Budgets are per-key: one heavy requester does not trip another.
func TestIndependentKeys(t *testing.T) {
	l := New(1000, 1000)
	l.Charge("heavy", 5000) // heavy trips
	if l.Over("light") {
		t.Fatal("light key must be unaffected by heavy key")
	}
	if !l.Over("heavy") {
		t.Fatal("heavy key should be over")
	}
}
