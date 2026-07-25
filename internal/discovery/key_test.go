package discovery

import (
	"crypto/rand"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// A public key round-trips through Encode/DecodePublicKey.
func TestPublicKeyRoundTrip(t *testing.T) {
	_, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s := EncodePublicKey(pub)
	got, err := DecodePublicKey(s)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Equals(pub) {
		t.Fatal("decoded key does not equal the original")
	}
}

// DecodePublicKey rejects a malformed string rather than returning a nil key.
func TestDecodePublicKeyRejectsGarbage(t *testing.T) {
	if _, err := DecodePublicKey("not-base64-!!!"); err == nil {
		t.Fatal("expected error for malformed key string")
	}
}
