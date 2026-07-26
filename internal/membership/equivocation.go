package membership

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/whatgate/whatgate/internal/discovery"
)

// EquivocationGuard detects Byzantine equivocation on the discovery plane: a
// subject publishing two *different* signed records at the *same* generation
// (§4, Codex P0). Because both records are validly signed by the subject, the
// conflict itself is the proof of misbehavior — a well-behaved node advances the
// generation for every new payload. On detecting a conflict the guard isolates
// the subject, refusing all of its later records too.
//
// It keeps, per subject, the payload hashes seen at recent generations (a bounded
// window), so memory stays proportional to active subjects, not history. It is
// safe for concurrent use.
type EquivocationGuard struct {
	window uint64
	mu     sync.Mutex
	seen   map[string]map[uint64][32]byte
	isolat map[string]struct{}
}

// NewEquivocationGuard returns a guard that remembers, per subject, the payload
// hashes of the most recent `window` generations.
func NewEquivocationGuard(window uint64) *EquivocationGuard {
	if window == 0 {
		window = 16
	}
	return &EquivocationGuard{
		window: window,
		seen:   make(map[string]map[uint64][32]byte),
		isolat: make(map[string]struct{}),
	}
}

// recordFingerprint extracts a signed record's generation (envelope serial) and
// the hash of its payload.
func recordFingerprint(recordSigned []byte) (generation uint64, hash [32]byte, err error) {
	var env discovery.Signed
	if err := json.Unmarshal(recordSigned, &env); err != nil {
		return 0, [32]byte{}, fmt.Errorf("equivocation: not a signed envelope: %w", err)
	}
	return env.Serial, sha256.Sum256(env.Payload), nil
}

// Observe records that subject published recordSigned. It returns an error if
// subject is already isolated, or if this conflicts with a different payload
// previously seen at the same generation (in which case subject becomes
// isolated). Re-observing an identical record is a no-op.
func (g *EquivocationGuard) Observe(subject string, recordSigned []byte) error {
	generation, hash, err := recordFingerprint(recordSigned)
	if err != nil {
		return err
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if _, bad := g.isolat[subject]; bad {
		return errors.New("equivocation: subject is isolated for prior equivocation")
	}

	gens := g.seen[subject]
	if gens == nil {
		gens = make(map[uint64][32]byte)
		g.seen[subject] = gens
	}
	if prev, ok := gens[generation]; ok && prev != hash {
		// Two different payloads at one generation: proof of equivocation.
		g.isolat[subject] = struct{}{}
		delete(g.seen, subject)
		return fmt.Errorf("equivocation: subject published conflicting records at generation %d", generation)
	}
	gens[generation] = hash

	// Prune generations far below the newest one seen for this subject.
	var newest uint64
	for ggen := range gens {
		if ggen > newest {
			newest = ggen
		}
	}
	if newest > g.window {
		floor := newest - g.window
		for ggen := range gens {
			if ggen < floor {
				delete(gens, ggen)
			}
		}
	}
	return nil
}
