package authn

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func newKey(t *testing.T) (crypto.PrivKey, string) {
	t.Helper()
	priv, pub, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("peer id: %v", err)
	}
	return priv, id.String()
}

func TestSignVerifyRoundTrip(t *testing.T) {
	priv, _ := newKey(t)
	now := time.Unix(1000, 0)

	a, err := Sign(priv, "register", now)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := Verify(a, "register", now, time.Minute); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerifyRejectsWrongAction(t *testing.T) {
	priv, _ := newKey(t)
	now := time.Unix(1000, 0)
	a, _ := Sign(priv, "register", now)

	if err := Verify(a, "join", now, time.Minute); err == nil {
		t.Fatal("verify should fail for a different action")
	}
}

func TestVerifyRejectsPeerIDMismatch(t *testing.T) {
	priv, _ := newKey(t)
	now := time.Unix(1000, 0)
	a, _ := Sign(priv, "register", now)

	// Claim a different peer ID than the public key derives to.
	_, otherID := newKey(t)
	a.PeerID = otherID

	if err := Verify(a, "register", now, time.Minute); err == nil {
		t.Fatal("verify should fail when peerID does not match the public key")
	}
}

func TestVerifyRejectsStaleTimestamp(t *testing.T) {
	priv, _ := newKey(t)
	signed := time.Unix(1000, 0)
	a, _ := Sign(priv, "register", signed)

	// Verify 10 minutes later with a 1-minute window.
	later := signed.Add(10 * time.Minute)
	if err := Verify(a, "register", later, time.Minute); err == nil {
		t.Fatal("verify should fail for a stale timestamp")
	}
}

func TestVerifyRejectsTamperedSignature(t *testing.T) {
	priv, _ := newKey(t)
	now := time.Unix(1000, 0)
	a, _ := Sign(priv, "register", now)

	raw, _ := base64.StdEncoding.DecodeString(a.Signature)
	raw[0] ^= 0xFF // flip a bit
	a.Signature = base64.StdEncoding.EncodeToString(raw)

	if err := Verify(a, "register", now, time.Minute); err == nil {
		t.Fatal("verify should fail for a tampered signature")
	}
}
