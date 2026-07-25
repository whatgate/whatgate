package discovery

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
)

func newKey(t *testing.T) (crypto.PrivKey, crypto.PubKey) {
	t.Helper()
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub
}

// A legitimately signed object round-trips: the pinned verifier recovers the
// exact payload.
func TestSignVerifyRoundTrip(t *testing.T) {
	priv, pub := newKey(t)
	now := time.Unix(1000, 0)
	payload := []byte(`{"hello":"world"}`)

	obj, err := Sign(priv, Meta{
		Type:      TypeDirectory,
		Serial:    5,
		IssuedAt:  now,
		ExpiresAt: now.Add(time.Hour),
	}, payload)
	if err != nil {
		t.Fatal(err)
	}

	got, err := obj.Verify(pub, TypeDirectory, now.Add(time.Minute), 0)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
}

// Verify must reject an object signed by anyone other than the pinned key — this
// is the whole point of response authentication (a rogue endpoint can't forge a
// directory).
func TestVerifyRejectsWrongPinnedKey(t *testing.T) {
	priv, _ := newKey(t)
	_, attackerPinned := newKey(t)
	now := time.Unix(1000, 0)

	obj, err := Sign(priv, Meta{Type: TypeDirectory, Serial: 1, IssuedAt: now, ExpiresAt: now.Add(time.Hour)}, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := obj.Verify(attackerPinned, TypeDirectory, now, 0); err == nil {
		t.Fatal("expected verify to reject a mismatched pinned key")
	}
}

// Verify must reject an expired object.
func TestVerifyRejectsExpired(t *testing.T) {
	priv, pub := newKey(t)
	now := time.Unix(1000, 0)

	obj, err := Sign(priv, Meta{Type: TypeDirectory, Serial: 1, IssuedAt: now, ExpiresAt: now.Add(time.Hour)}, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := obj.Verify(pub, TypeDirectory, now.Add(2*time.Hour), 0); err == nil {
		t.Fatal("expected verify to reject an expired object")
	}
}

// Domain separation: an object signed as one type must not verify as another,
// so a bootstrap record can never be replayed as a directory.
func TestVerifyRejectsWrongType(t *testing.T) {
	priv, pub := newKey(t)
	now := time.Unix(1000, 0)

	obj, err := Sign(priv, Meta{Type: TypeBootstrap, Serial: 1, IssuedAt: now, ExpiresAt: now.Add(time.Hour)}, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := obj.Verify(pub, TypeDirectory, now, 0); err == nil {
		t.Fatal("expected verify to reject a type mismatch")
	}
}

// Anti-rollback: an object whose serial is below the caller's floor (the highest
// serial already accepted) must be rejected, so a stale-but-validly-signed
// directory can't be replayed to hide a revocation.
func TestVerifyRejectsRollback(t *testing.T) {
	priv, pub := newKey(t)
	now := time.Unix(1000, 0)

	obj, err := Sign(priv, Meta{Type: TypeDirectory, Serial: 5, IssuedAt: now, ExpiresAt: now.Add(time.Hour)}, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := obj.Verify(pub, TypeDirectory, now, 10); err == nil {
		t.Fatal("expected verify to reject serial below the floor")
	}
	// At or above the floor is fine.
	if _, err := obj.Verify(pub, TypeDirectory, now, 5); err != nil {
		t.Fatalf("serial == floor should verify: %v", err)
	}
}

// A tampered payload must fail verification even though every other field is
// intact.
func TestVerifyRejectsTamperedPayload(t *testing.T) {
	priv, pub := newKey(t)
	now := time.Unix(1000, 0)

	obj, err := Sign(priv, Meta{Type: TypeDirectory, Serial: 1, IssuedAt: now, ExpiresAt: now.Add(time.Hour)}, []byte(`{"exit":"good"}`))
	if err != nil {
		t.Fatal(err)
	}
	obj.Payload = []byte(`{"exit":"evil"}`)
	if _, err := obj.Verify(pub, TypeDirectory, now, 0); err == nil {
		t.Fatal("expected verify to reject a tampered payload")
	}
}
