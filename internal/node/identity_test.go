package node

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
)

func TestWithIdentityKeepsPeerIDAcrossRestarts(t *testing.T) {
	identity, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	first, err := New(context.Background(), WithIdentity(identity))
	if err != nil {
		t.Fatal(err)
	}
	firstID := first.ID()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := New(context.Background(), WithIdentity(identity))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	if second.ID() != firstID {
		t.Fatalf("peer ID changed across restart: %s != %s", firstID, second.ID())
	}
}
