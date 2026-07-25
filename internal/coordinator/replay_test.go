package coordinator

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/whatgate/whatgate/internal/authn"
)

// TestReplayedSignatureRejected: posting the exact same signed join twice — a
// captured-and-replayed request — is accepted once and rejected the second time.
func TestReplayedSignatureRejected(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 5)
	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	defer ts.Close()

	priv, pub, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	id, _ := peer.IDFromPublicKey(pub)
	auth, err := authn.Sign(priv, "join", time.Now())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	body, _ := json.Marshal(joinRequest{Code: "welcome", PeerID: id.String(), Auth: auth})

	post := func() int {
		resp, err := http.Post(ts.URL+"/join", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if code := post(); code != http.StatusOK {
		t.Fatalf("first join status = %d, want 200", code)
	}
	if code := post(); code != http.StatusUnauthorized {
		t.Fatalf("replayed join status = %d, want 401", code)
	}
}

// TestOversizedBodyRejected: a body beyond the size cap is refused, not buffered.
func TestOversizedBodyRejected(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	defer ts.Close()

	huge := `{"code":"` + strings.Repeat("A", (64<<10)+1) + `"}`
	resp, err := http.Post(ts.URL+"/join", "application/json", strings.NewReader(huge))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized body status = %d, want 400", resp.StatusCode)
	}
}
