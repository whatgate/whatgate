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

// With a signing key configured, /relay returns a discovery.Signed envelope of
// type relay whose payload is the RelayInfo and which verifies against the
// coordinator's public key. This lets a pinned node reject a forged relay
// address the same way it rejects a forged directory (A0).
func TestRelaySignedWhenKeySet(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	srv := NewServer(dir, NewInviteStore(nil))
	srv.SetRelayInfo("RID", []string{"/ip4/1.2.3.4/tcp/4001"})

	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetSigningKey(priv)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/relay")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var signed discovery.Signed
	if err := json.NewDecoder(resp.Body).Decode(&signed); err != nil {
		t.Fatalf("decode signed envelope: %v", err)
	}
	payload, err := signed.Verify(pub, discovery.TypeRelay, time.Now(), 0)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	var info RelayInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		t.Fatalf("decode relay info: %v", err)
	}
	if info.PeerID != "RID" || len(info.Addrs) != 1 || info.Addrs[0] != "/ip4/1.2.3.4/tcp/4001" {
		t.Fatalf("relay info = %+v", info)
	}
}

// Without a signing key the endpoint keeps its legacy bare RelayInfo shape, so
// existing unsigned deployments still work.
func TestRelayUnsignedWhenNoKey(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	srv := NewServer(dir, NewInviteStore(nil))
	srv.SetRelayInfo("RID", []string{"/ip4/1.2.3.4/tcp/4001"})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	c := NewClient(ts.URL)
	info, err := c.Relay()
	if err != nil {
		t.Fatalf("Relay: %v", err)
	}
	if info.PeerID != "RID" {
		t.Fatalf("relay info = %+v", info)
	}
}

// A pinned client transparently verifies the signed relay envelope and returns
// the RelayInfo it wraps.
func TestClientRelayVerifiesPinned(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	srv := NewServer(dir, NewInviteStore(nil))
	srv.SetRelayInfo("RID", []string{"/ip4/1.2.3.4/tcp/4001"})

	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetSigningKey(priv)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	c := NewClient(ts.URL)
	c.SetPinnedKey(pub)
	info, err := c.Relay()
	if err != nil {
		t.Fatalf("Relay: %v", err)
	}
	if info.PeerID != "RID" || len(info.Addrs) != 1 || info.Addrs[0] != "/ip4/1.2.3.4/tcp/4001" {
		t.Fatalf("relay info = %+v", info)
	}
}

// A pinned client rejects a relay signed by a different key — a rogue mirror or
// MITM coordinator cannot steer a node onto an adversary-controlled relay.
func TestClientRelayRejectsWrongKey(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	srv := NewServer(dir, NewInviteStore(nil))
	srv.SetRelayInfo("RID", []string{"/ip4/1.2.3.4/tcp/4001"})

	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetSigningKey(priv)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	_, otherPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	c := NewClient(ts.URL)
	c.SetPinnedKey(otherPub) // pin a key the coordinator did NOT sign with
	if _, err := c.Relay(); err == nil {
		t.Fatal("expected relay signed by a different key to be rejected, got nil error")
	}
}

// A pinned client rejects an unsigned (bare) relay: a MITM cannot strip the
// signature to downgrade a pinned node onto an unauthenticated relay address.
func TestClientRelayRejectsUnsignedWhenPinned(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	srv := NewServer(dir, NewInviteStore(nil)) // no signing key: serves bare RelayInfo
	srv.SetRelayInfo("RID", []string{"/ip4/1.2.3.4/tcp/4001"})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	_, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	c := NewClient(ts.URL)
	c.SetPinnedKey(pub)
	if _, err := c.Relay(); err == nil {
		t.Fatal("expected unsigned relay to be rejected by a pinned client, got nil error")
	}
}
