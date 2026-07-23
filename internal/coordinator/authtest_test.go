package coordinator

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/whatgate/whatgate/internal/authn"
)

// newSignedClient returns a coordinator client that signs join/register requests
// as a freshly generated identity, along with that identity's peer ID.
func newSignedClient(t *testing.T, baseURL string) (*Client, string) {
	t.Helper()
	priv, pub, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("peer id: %v", err)
	}
	c := NewClient(baseURL)
	c.Signer = func(action string) (authn.SignedAuth, error) {
		return authn.Sign(priv, action, time.Now())
	}
	return c, id.String()
}

// admitAndRegister creates a fresh signed identity, admits it with code, and
// registers it as a JP exit. Returns its signing client and peer ID.
func admitAndRegister(t *testing.T, baseURL, code string) (*Client, string) {
	t.Helper()
	c, id := newSignedClient(t, baseURL)
	if _, err := c.Join(code, id); err != nil {
		t.Fatalf("join %s: %v", id, err)
	}
	if err := c.Register(NodeInfo{PeerID: id, Region: "JP", WantExit: true}); err != nil {
		t.Fatalf("register %s: %v", id, err)
	}
	return c, id
}
