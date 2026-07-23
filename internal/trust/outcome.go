package trust

import "fmt"

// Outcome is the result an exit reports about a requester's behavior. It drives
// reputation: good conduct nudges reputation up, abuse signals push it down
// (harder), so repeat offenders quickly fall below any serving threshold.
type Outcome string

const (
	// OutcomeServed: the request was within policy and served.
	OutcomeServed Outcome = "served"
	// OutcomeBlocked: the request targeted a policy-blocked destination — an
	// abuse signal.
	OutcomeBlocked Outcome = "blocked"
)

// Delta is the reputation change this outcome implies.
func (o Outcome) Delta() int {
	switch o {
	case OutcomeServed:
		return 1
	case OutcomeBlocked:
		return -10
	default:
		return 0
	}
}

// ParseOutcome maps a wire string to an Outcome.
func ParseOutcome(s string) (Outcome, error) {
	switch Outcome(s) {
	case OutcomeServed:
		return OutcomeServed, nil
	case OutcomeBlocked:
		return OutcomeBlocked, nil
	default:
		return "", fmt.Errorf("trust: unknown outcome %q", s)
	}
}
