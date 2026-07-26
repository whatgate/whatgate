package membership

import (
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/whatgate/whatgate/internal/discovery"
)

func sampleCheckpoint(version, revEpoch uint64, now time.Time, subjects, issuers []string) RevocationCheckpoint {
	return RevocationCheckpoint{
		V:               1,
		Version:         version,
		RevEpoch:        revEpoch,
		ThisUpdate:      now.Unix(),
		NextUpdate:      now.Add(6 * time.Hour).Unix(),
		MaxStalenessSec: int64((24 * time.Hour).Seconds()),
		RevokedSubjects: subjects,
		RevokedIssuers:  issuers,
	}
}

// A root-signed checkpoint verifies against the pinned root and round-trips its
// fields.
func TestCheckpointRoundTrip(t *testing.T) {
	rootPriv, rootPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	cp := sampleCheckpoint(3, 2, now, []string{"12D3KooWBad"}, nil)
	signed, err := SignRevocationCheckpoint(rootPriv, cp, now.Add(365*24*time.Hour))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	got, err := VerifyRevocationCheckpoint(rootPub, signed, now, 0)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Version != 3 || got.RevEpoch != 2 || len(got.RevokedSubjects) != 1 || got.RevokedSubjects[0] != "12D3KooWBad" {
		t.Fatalf("checkpoint = %+v", got)
	}
}

// A checkpoint signed by a different key is rejected.
func TestCheckpointWrongRoot(t *testing.T) {
	rootPriv, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	_, otherPub, _ := crypto.GenerateEd25519Key(rand.Reader)
	now := time.Now()
	signed, _ := SignRevocationCheckpoint(rootPriv, sampleCheckpoint(1, 0, now, nil, nil), now.Add(time.Hour))
	if _, err := VerifyRevocationCheckpoint(otherPub, signed, now, 0); err == nil {
		t.Fatal("expected checkpoint from wrong root to be rejected")
	}
}

// A lower-version checkpoint is rejected against a higher version floor
// (anti-rollback: an attacker can't replay a stale checkpoint that omits a
// later revocation).
func TestCheckpointRollback(t *testing.T) {
	rootPriv, rootPub, _ := crypto.GenerateEd25519Key(rand.Reader)
	now := time.Now()
	old := sampleCheckpoint(3, 0, now, nil, nil)
	signedOld, _ := SignRevocationCheckpoint(rootPriv, old, now.Add(365*24*time.Hour))
	if _, err := VerifyRevocationCheckpoint(rootPub, signedOld, now, 5); err == nil {
		t.Fatal("expected version 3 below floor 5 to be rejected as rollback")
	}
}

// Past the envelope's hard expiry the checkpoint no longer verifies at all.
func TestCheckpointHardExpiry(t *testing.T) {
	rootPriv, rootPub, _ := crypto.GenerateEd25519Key(rand.Reader)
	now := time.Now()
	signed, _ := SignRevocationCheckpoint(rootPriv, sampleCheckpoint(1, 0, now, nil, nil), now.Add(-time.Minute))
	if _, err := VerifyRevocationCheckpoint(rootPub, signed, now, 0); err == nil {
		t.Fatal("expected past-hard-expiry checkpoint to be rejected")
	}
}

// Revokes reports a cert revoked when its subject is listed, and not otherwise.
func TestRevokesSubject(t *testing.T) {
	now := time.Now()
	cp := sampleCheckpoint(1, 0, now, []string{"12D3KooWRevoked"}, nil)
	if !cp.Revokes(Cert{Subject: "12D3KooWRevoked", IssuerID: "issuer-A"}) {
		t.Fatal("expected listed subject to be revoked")
	}
	if cp.Revokes(Cert{Subject: "12D3KooWGood", IssuerID: "issuer-A"}) {
		t.Fatal("did not expect unlisted subject to be revoked")
	}
}

// Revokes reports a cert revoked when its issuer is listed (mass revocation of
// everything a compromised issuer signed).
func TestRevokesIssuer(t *testing.T) {
	now := time.Now()
	cp := sampleCheckpoint(1, 0, now, nil, []string{"issuer-bad"})
	if !cp.Revokes(Cert{Subject: "12D3KooWAny", IssuerID: "issuer-bad"}) {
		t.Fatal("expected cert from a revoked issuer to be revoked")
	}
	if cp.Revokes(Cert{Subject: "12D3KooWAny", IssuerID: "issuer-ok"}) {
		t.Fatal("did not expect cert from a good issuer to be revoked")
	}
}

// Stale flips once now passes thisUpdate + maxStaleness — the signal a consumer
// must degrade to emergency-only scope.
func TestCheckpointStale(t *testing.T) {
	now := time.Now()
	cp := sampleCheckpoint(1, 0, now, nil, nil) // maxStaleness = 24h
	if cp.Stale(now.Add(23 * time.Hour)) {
		t.Fatal("checkpoint should be fresh within the staleness window")
	}
	if !cp.Stale(now.Add(25 * time.Hour)) {
		t.Fatal("checkpoint should be stale past the staleness window")
	}
}

// A tampered payload no longer verifies.
func TestCheckpointTampered(t *testing.T) {
	rootPriv, rootPub, _ := crypto.GenerateEd25519Key(rand.Reader)
	now := time.Now()
	signed, _ := SignRevocationCheckpoint(rootPriv, sampleCheckpoint(1, 0, now, []string{"a"}, nil), now.Add(time.Hour))
	var env discovery.Signed
	if err := json.Unmarshal(signed, &env); err != nil {
		t.Fatal(err)
	}
	env.Payload = json.RawMessage(`{"v":1,"version":1,"revokedSubjects":[]}`) // drop the revocation
	tampered, _ := json.Marshal(env)
	if _, err := VerifyRevocationCheckpoint(rootPub, tampered, now, 0); err == nil {
		t.Fatal("expected tampered checkpoint to be rejected")
	}
}
