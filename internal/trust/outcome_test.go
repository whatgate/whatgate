package trust

import "testing"

func TestOutcomeDeltas(t *testing.T) {
	if d := OutcomeServed.Delta(); d != 1 {
		t.Errorf("served delta = %d, want 1", d)
	}
	if d := OutcomeBlocked.Delta(); d != -10 {
		t.Errorf("blocked delta = %d, want -10", d)
	}
	// Abuse must cost more than good conduct earns.
	if OutcomeBlocked.Delta() >= 0 || -OutcomeBlocked.Delta() <= OutcomeServed.Delta() {
		t.Error("blocked penalty should outweigh served reward")
	}
}

func TestParseOutcome(t *testing.T) {
	if o, err := ParseOutcome("blocked"); err != nil || o != OutcomeBlocked {
		t.Fatalf("ParseOutcome(blocked) = %v, %v", o, err)
	}
	if _, err := ParseOutcome("nope"); err == nil {
		t.Fatal("ParseOutcome(nope) should error")
	}
}
