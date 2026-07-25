package coordinator

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/whatgate/whatgate/internal/discovery"
)

// signedDirServer serves the given entries as a discovery.Signed envelope signed
// by priv, using whatever serial serialOf returns for each request.
func signedDirServer(t *testing.T, priv crypto.PrivKey, entries []directoryEntry, serialOf func() uint64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := json.Marshal(entries)
		if err != nil {
			t.Error(err)
			return
		}
		obj, err := discovery.Sign(priv, discovery.Meta{
			Type:      discovery.TypeDirectory,
			Serial:    serialOf(),
			IssuedAt:  time.Now(),
			ExpiresAt: time.Now().Add(time.Minute),
		}, payload)
		if err != nil {
			t.Error(err)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	}))
}

// A client with a pinned key accepts a directory signed by the matching key.
func TestClientAcceptsSignedDirectory(t *testing.T) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	entries := []directoryEntry{{PeerID: "exit1", Region: "JP", WantExit: true}}
	ts := signedDirServer(t, priv, entries, func() uint64 { return 1 })
	defer ts.Close()

	c := NewClient(ts.URL)
	c.SetPinnedKey(pub)

	nodes, _, err := c.DirectoryFor("")
	if err != nil {
		t.Fatalf("DirectoryFor: %v", err)
	}
	if len(nodes) != 1 || nodes[0].PeerID != "exit1" {
		t.Fatalf("nodes = %+v, want one exit1", nodes)
	}
}

// A client with a pinned key rejects a directory signed by a different key — the
// rogue-endpoint / MITM case.
func TestClientRejectsWrongKeyDirectory(t *testing.T) {
	roguePriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, pinnedPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ts := signedDirServer(t, roguePriv, []directoryEntry{{PeerID: "evil"}}, func() uint64 { return 1 })
	defer ts.Close()

	c := NewClient(ts.URL)
	c.SetPinnedKey(pinnedPub)

	if _, _, err := c.DirectoryFor(""); err == nil {
		t.Fatal("expected DirectoryFor to reject a directory signed by an unpinned key")
	}
}

// A pinned client rejects a rollback: after accepting serial 5 it must reject a
// later response bearing a lower serial.
func TestClientRejectsRolledBackDirectory(t *testing.T) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	serials := []uint64{5, 3}
	i := 0
	next := func() uint64 {
		mu.Lock()
		defer mu.Unlock()
		s := serials[i]
		if i < len(serials)-1 {
			i++
		}
		return s
	}
	ts := signedDirServer(t, priv, []directoryEntry{{PeerID: "exit1"}}, next)
	defer ts.Close()

	c := NewClient(ts.URL)
	c.SetPinnedKey(pub)

	if _, _, err := c.DirectoryFor(""); err != nil {
		t.Fatalf("first (serial 5) should succeed: %v", err)
	}
	if _, _, err := c.DirectoryFor(""); err == nil {
		t.Fatal("expected second (serial 3) to be rejected as a rollback")
	}
}

// A pinned client refuses an unsigned (legacy bare-array) directory, rather than
// silently downgrading to unauthenticated data.
func TestPinnedClientRejectsUnsignedDirectory(t *testing.T) {
	_, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []directoryEntry{{PeerID: "exit1"}})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	c.SetPinnedKey(pub)

	if _, _, err := c.DirectoryFor(""); err == nil {
		t.Fatal("expected pinned client to reject an unsigned directory")
	}
}
