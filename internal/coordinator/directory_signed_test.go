package coordinator

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/whatgate/whatgate/internal/discovery"
)

// With a signing key configured, /directory returns a discovery.Signed envelope
// that verifies against the coordinator's public key and whose payload is the
// directory entries.
func TestDirectorySignedWhenKeySet(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	dir.Register(NodeInfo{PeerID: "exit1", Region: "JP", WantExit: true})
	srv := NewServer(dir, NewInviteStore(nil))

	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetSigningKey(priv)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/directory")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var signed discovery.Signed
	if err := json.NewDecoder(resp.Body).Decode(&signed); err != nil {
		t.Fatalf("decode signed envelope: %v", err)
	}
	payload, err := signed.Verify(pub, discovery.TypeDirectory, time.Now(), 0)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	var entries []directoryEntry
	if err := json.Unmarshal(payload, &entries); err != nil {
		t.Fatalf("decode entries: %v", err)
	}
	if len(entries) != 1 || entries[0].PeerID != "exit1" {
		t.Fatalf("entries = %+v, want one exit1", entries)
	}
}

// Successive signed directories carry strictly increasing serials, so a stale
// one can be detected as a rollback.
func TestDirectorySerialIsMonotonic(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	srv := NewServer(dir, NewInviteStore(nil))
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetSigningKey(priv)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	get := func() discovery.Signed {
		resp, err := http.Get(ts.URL + "/directory")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var s discovery.Signed
		if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	first := get().Serial
	second := get().Serial
	if second <= first {
		t.Fatalf("serial not monotonic: first=%d second=%d", first, second)
	}
}

// Without a signing key the endpoint keeps its legacy bare-array shape, so
// existing unsigned deployments still work.
func TestDirectoryUnsignedWhenNoKey(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	dir.Register(NodeInfo{PeerID: "exit1", WantExit: true})
	srv := NewServer(dir, NewInviteStore(nil))

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/directory")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var entries []directoryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("decode bare entries: %v", err)
	}
	if len(entries) != 1 || entries[0].PeerID != "exit1" {
		t.Fatalf("entries = %+v, want one exit1", entries)
	}
}
