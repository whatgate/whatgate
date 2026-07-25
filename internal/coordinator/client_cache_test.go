package coordinator

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/whatgate/whatgate/internal/discovery"
)

// After one successful verified fetch, a client whose coordinator becomes
// unreachable serves the cached directory and flags it as stale.
func TestServesCachedDirectoryWhenOffline(t *testing.T) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	entries := []directoryEntry{{PeerID: "exit1", Region: "JP", WantExit: true}}
	ts := signedDirServer(t, priv, entries, func() uint64 { return 1 })

	cachePath := filepath.Join(t.TempDir(), "dir.cache")
	c := NewClient(ts.URL)
	c.SetPinnedKey(pub)
	c.SetDirectoryCache(cachePath)

	if _, _, err := c.DirectoryFor(""); err != nil {
		t.Fatalf("prime: %v", err)
	}
	if c.LastDirectoryStale() {
		t.Fatal("fresh fetch must not be flagged stale")
	}

	ts.Close() // coordinator now unreachable

	nodes, _, err := c.DirectoryFor("")
	if err != nil {
		t.Fatalf("expected cached directory to be served offline: %v", err)
	}
	if len(nodes) != 1 || nodes[0].PeerID != "exit1" {
		t.Fatalf("nodes = %+v, want cached exit1", nodes)
	}
	if !c.LastDirectoryStale() {
		t.Fatal("cached directory must be flagged stale")
	}
}

// With no cache primed, an offline client returns an error rather than inventing
// a directory.
func TestOfflineWithoutCacheErrors(t *testing.T) {
	_, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient("http://127.0.0.1:1")
	c.SetPinnedKey(pub)
	c.SetDirectoryCache(filepath.Join(t.TempDir(), "dir.cache"))

	if _, _, err := c.DirectoryFor(""); err == nil {
		t.Fatal("expected an error when offline with no cache")
	}
}

// An expired cache entry is refused offline, rather than used past its acceptance
// window.
func TestExpiredCacheRefusedOffline(t *testing.T) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal([]directoryEntry{{PeerID: "exit1"}})
	past := time.Now().Add(-2 * time.Hour)
	obj, err := discovery.Sign(priv, discovery.Meta{
		Type:      discovery.TypeDirectory,
		Serial:    1,
		IssuedAt:  past,
		ExpiresAt: past.Add(time.Hour), // expired an hour ago
	}, payload)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(obj)
	cachePath := filepath.Join(t.TempDir(), "dir.cache")
	if err := os.WriteFile(cachePath, body, 0o600); err != nil {
		t.Fatal(err)
	}

	c := NewClient("http://127.0.0.1:1") // offline
	c.SetPinnedKey(pub)
	c.SetDirectoryCache(cachePath)

	if _, _, err := c.DirectoryFor(""); err == nil {
		t.Fatal("expected expired cache to be refused")
	}
}
