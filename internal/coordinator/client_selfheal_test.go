package coordinator

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// When every configured endpoint is unreachable and no cache exists, a client
// with a bootstrap URL self-heals (C2) on the DIRECTORY path too — not only on
// cold-start join — so a returning node whose coordinators are all blocked can
// recover its endpoints and fetch a verified directory.
func TestDirectorySelfHealsViaBootstrap(t *testing.T) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// A live coordinator that signs its directory with the pinned key.
	dir := NewDirectory(time.Minute, nil)
	dir.Register(NodeInfo{PeerID: "exit1", Region: "JP", WantExit: true})
	live := NewServer(dir, NewInviteStore(nil))
	live.SetSigningKey(priv)
	liveTS := httptest.NewServer(live.Handler())
	defer liveTS.Close()

	// An out-of-band CDN serving a signed bootstrap list that points at the live
	// coordinator.
	bootBody := signBootstrap(t, priv, 1, []string{liveTS.URL})
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bootBody)
	}))
	defer cdn.Close()

	// Client points only at a dead endpoint, but has the pinned key and bootstrap
	// URL configured.
	c := NewClient("http://127.0.0.1:1") // connection refused
	c.SetPinnedKey(pub)
	c.SetBootstrapURL(cdn.URL)

	nodes, _, err := c.DirectoryFor("")
	if err != nil {
		t.Fatalf("DirectoryFor should have self-healed: %v", err)
	}
	if len(nodes) != 1 || nodes[0].PeerID != "exit1" {
		t.Fatalf("directory = %v, want the live coordinator's exit1", nodes)
	}
}
