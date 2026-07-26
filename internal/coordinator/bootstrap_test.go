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

// signBootstrap produces a signed bootstrap-list envelope (as it would be served
// from an out-of-band channel) carrying the given endpoints and serial.
func signBootstrap(t *testing.T, priv crypto.PrivKey, serial uint64, eps []string) []byte {
	t.Helper()
	payload, err := json.Marshal(BootstrapList{Endpoints: eps})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	obj, err := discovery.Sign(priv, discovery.Meta{
		Type:      discovery.TypeBootstrap,
		Serial:    serial,
		IssuedAt:  now,
		ExpiresAt: now.Add(24 * time.Hour),
	}, payload)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// A pinned client applies a validly signed bootstrap list by replacing its
// coordinator endpoints with the list's — this is the C2 self-heal path when the
// known coordinators are all blocked.
func TestApplyBootstrapReplacesEndpoints(t *testing.T) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient("https://old.example:8080")
	c.SetPinnedKey(pub)

	body := signBootstrap(t, priv, 1, []string{"https://a.example:8080", "https://b.example:8080"})
	if err := c.ApplyBootstrap(body); err != nil {
		t.Fatalf("ApplyBootstrap: %v", err)
	}
	got := c.Endpoints()
	if len(got) != 2 || got[0] != "https://a.example:8080" || got[1] != "https://b.example:8080" {
		t.Fatalf("endpoints = %v, want the two from the list", got)
	}
}

// A bootstrap list signed by a different key is rejected and leaves the existing
// endpoints untouched — a poisoned list can't redirect a node onto adversary
// coordinators.
func TestApplyBootstrapRejectsWrongKey(t *testing.T) {
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, otherPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient("https://old.example:8080")
	c.SetPinnedKey(otherPub)

	body := signBootstrap(t, priv, 1, []string{"https://evil.example:8080"})
	if err := c.ApplyBootstrap(body); err == nil {
		t.Fatal("expected a list signed by a different key to be rejected")
	}
	if got := c.Endpoints(); len(got) != 1 || got[0] != "https://old.example:8080" {
		t.Fatalf("endpoints changed on rejection: %v", got)
	}
}

// An older-serial bootstrap list is rejected as a rollback, keeping the freshest
// applied list — an attacker can't replay a stale list to steer a node onto
// retired/seized coordinators.
func TestApplyBootstrapRejectsRollback(t *testing.T) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient("https://old.example:8080")
	c.SetPinnedKey(pub)

	if err := c.ApplyBootstrap(signBootstrap(t, priv, 5, []string{"https://fresh.example:8080"})); err != nil {
		t.Fatalf("apply serial 5: %v", err)
	}
	if err := c.ApplyBootstrap(signBootstrap(t, priv, 3, []string{"https://stale.example:8080"})); err == nil {
		t.Fatal("expected older-serial list to be rejected as rollback")
	}
	if got := c.Endpoints(); len(got) != 1 || got[0] != "https://fresh.example:8080" {
		t.Fatalf("endpoints = %v, want the serial-5 list retained", got)
	}
}

// A tampered payload (signature no longer matches) is rejected.
func TestApplyBootstrapRejectsTampered(t *testing.T) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient("https://old.example:8080")
	c.SetPinnedKey(pub)

	body := signBootstrap(t, priv, 1, []string{"https://a.example:8080"})
	var env discovery.Signed
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	env.Payload = json.RawMessage(`{"endpoints":["https://evil.example:8080"]}`) // swap payload, keep old sig
	tampered, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyBootstrap(tampered); err == nil {
		t.Fatal("expected tampered payload to be rejected")
	}
}

// SignBootstrap produces a signed list that a pinned client accepts (producer /
// consumer round trip), so operators can publish one from their signing key.
func TestSignBootstrapRoundTrip(t *testing.T) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body, err := SignBootstrap(priv, 7, []string{"https://a.example:8080", "https://b.example:8080"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("SignBootstrap: %v", err)
	}
	c := NewClient("https://old.example:8080")
	c.SetPinnedKey(pub)
	if err := c.ApplyBootstrap(body); err != nil {
		t.Fatalf("ApplyBootstrap: %v", err)
	}
	if got := c.Endpoints(); len(got) != 2 || got[0] != "https://a.example:8080" {
		t.Fatalf("endpoints = %v", got)
	}
}

// RefreshFromBootstrap fetches a signed list from an out-of-band URL (here a
// stand-in for a CDN / GitHub raw file) and applies it, replacing the endpoints.
func TestRefreshFromBootstrapFetchesAndApplies(t *testing.T) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body := signBootstrap(t, priv, 1, []string{"https://healed.example:8080"})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	c := NewClient("https://blocked.example:8080")
	c.SetPinnedKey(pub)
	if err := c.RefreshFromBootstrap(ts.URL); err != nil {
		t.Fatalf("RefreshFromBootstrap: %v", err)
	}
	if got := c.Endpoints(); len(got) != 1 || got[0] != "https://healed.example:8080" {
		t.Fatalf("endpoints = %v, want the fetched list", got)
	}
}

// A non-200 from the out-of-band channel surfaces as an error and leaves the
// endpoints untouched.
func TestRefreshFromBootstrapNon200(t *testing.T) {
	_, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer ts.Close()

	c := NewClient("https://blocked.example:8080")
	c.SetPinnedKey(pub)
	if err := c.RefreshFromBootstrap(ts.URL); err == nil {
		t.Fatal("expected non-200 fetch to error")
	}
	if got := c.Endpoints(); len(got) != 1 || got[0] != "https://blocked.example:8080" {
		t.Fatalf("endpoints changed on failed fetch: %v", got)
	}
}

// Fail-closed: without a pinned key the client refuses to apply any bootstrap
// list, since an unauthenticated list is itself a poisoning vector.
func TestApplyBootstrapRefusedWithoutPinnedKey(t *testing.T) {
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient("https://old.example:8080") // no SetPinnedKey

	body := signBootstrap(t, priv, 1, []string{"https://a.example:8080"})
	if err := c.ApplyBootstrap(body); err == nil {
		t.Fatal("expected apply without a pinned key to be refused")
	}
	if got := c.Endpoints(); len(got) != 1 || got[0] != "https://old.example:8080" {
		t.Fatalf("endpoints changed while unpinned: %v", got)
	}
}
