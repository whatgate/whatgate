package membership

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// signedRecordAt builds a signed node record for a fresh exit identity at the
// given generation with the given region (region varies the payload, so two
// records at the same generation with different regions have different hashes).
func signedRecordAt(t *testing.T, generation uint64, region string) (subject string, signed []byte) {
	t.Helper()
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := peer.IDFromPublicKey(pub)
	subject = id.String()
	rec := NodeRecord{V: 1, Subject: subject, Roles: []Role{RoleExit}, Addrs: []string{"/ip4/1.2.3.4/tcp/443/ws"}, Region: region}
	signed, err = SignNodeRecord(priv, rec, time.Now(), time.Now().Add(5*time.Minute), generation)
	if err != nil {
		t.Fatal(err)
	}
	return subject, signed
}

// A record's fingerprint (generation + payload hash) is stable and detects a
// changed payload at the same generation.
func TestEquivocationGuardAcceptsConsistent(t *testing.T) {
	g := NewEquivocationGuard(16)
	subject, rec := signedRecordAt(t, 5, "JP")
	if err := g.Observe(subject, rec); err != nil {
		t.Fatalf("first observe: %v", err)
	}
	if err := g.Observe(subject, rec); err != nil {
		t.Fatalf("re-observing the same record should be fine: %v", err)
	}
}

// Two different records at the same generation from one subject is equivocation:
// the second is rejected and the subject is isolated thereafter.
func TestEquivocationGuardIsolatesConflict(t *testing.T) {
	g := NewEquivocationGuard(16)
	// Same subject, same generation, different payloads. We must reuse one key to
	// keep the subject identical, so sign both here.
	priv, pub, _ := crypto.GenerateEd25519Key(rand.Reader)
	id, _ := peer.IDFromPublicKey(pub)
	subject := id.String()
	mk := func(region string) []byte {
		rec := NodeRecord{V: 1, Subject: subject, Roles: []Role{RoleExit}, Addrs: []string{"/ip4/1.2.3.4/tcp/443/ws"}, Region: region}
		b, _ := SignNodeRecord(priv, rec, time.Now(), time.Now().Add(5*time.Minute), 7)
		return b
	}
	recJP := mk("JP")
	recUS := mk("US") // same generation 7, different payload

	if err := g.Observe(subject, recJP); err != nil {
		t.Fatalf("first observe: %v", err)
	}
	if err := g.Observe(subject, recUS); err == nil {
		t.Fatal("expected equivocation (same generation, different payload) to be rejected")
	}
	// Subject is now isolated: even a fresh, higher-generation record is refused.
	recNext := func() []byte {
		rec := NodeRecord{V: 1, Subject: subject, Roles: []Role{RoleExit}, Addrs: []string{"/ip4/1.2.3.4/tcp/443/ws"}, Region: "JP"}
		b, _ := SignNodeRecord(priv, rec, time.Now(), time.Now().Add(5*time.Minute), 8)
		return b
	}()
	if err := g.Observe(subject, recNext); err == nil {
		t.Fatal("expected an isolated subject's later records to be refused")
	}
}

// A normal republish at a higher generation with different content is fine.
func TestEquivocationGuardAllowsHigherGeneration(t *testing.T) {
	g := NewEquivocationGuard(16)
	priv, pub, _ := crypto.GenerateEd25519Key(rand.Reader)
	id, _ := peer.IDFromPublicKey(pub)
	subject := id.String()
	mk := func(gen uint64, region string) []byte {
		rec := NodeRecord{V: 1, Subject: subject, Roles: []Role{RoleExit}, Addrs: []string{"/ip4/1.2.3.4/tcp/443/ws"}, Region: region}
		b, _ := SignNodeRecord(priv, rec, time.Now(), time.Now().Add(5*time.Minute), gen)
		return b
	}
	if err := g.Observe(subject, mk(5, "JP")); err != nil {
		t.Fatal(err)
	}
	if err := g.Observe(subject, mk(6, "US")); err != nil {
		t.Fatalf("higher-generation republish should be accepted: %v", err)
	}
}

// One subject's equivocation does not affect another subject.
func TestEquivocationGuardIsolatesPerSubject(t *testing.T) {
	g := NewEquivocationGuard(16)
	s1, r1 := signedRecordAt(t, 3, "JP")
	s2, r2 := signedRecordAt(t, 3, "JP")
	if err := g.Observe(s1, r1); err != nil {
		t.Fatal(err)
	}
	if err := g.Observe(s2, r2); err != nil {
		t.Fatalf("a different subject at the same generation is not equivocation: %v", err)
	}
}
