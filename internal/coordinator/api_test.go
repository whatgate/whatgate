package coordinator

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJoinRegisterDirectoryFlow(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 5)

	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	defer ts.Close()

	c, id := newSignedClient(t, ts.URL)
	if _, err := c.Join("welcome", id); err != nil {
		t.Fatalf("join: %v", err)
	}
	if err := c.Register(NodeInfo{
		PeerID:   id,
		Addrs:    []string{"/ip4/1.2.3.4/tcp/4001"},
		Region:   "JP",
		WantExit: true,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	nodes, err := c.Directory()
	if err != nil {
		t.Fatalf("directory: %v", err)
	}
	if len(nodes) != 1 || nodes[0].PeerID != id || nodes[0].Region != "JP" || !nodes[0].WantExit {
		t.Fatalf("unexpected directory: %+v", nodes)
	}
}

func TestJoinFlagsSybilIdentityBurstFromOneIP(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 100) // ample uses; the IP cap must bite first

	srv := NewServer(dir, inv)
	srv.SetAnomalyDetection(time.Hour, 2) // >= 2 distinct identities per IP → isolate
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// First distinct identity from this IP joins fine.
	c1, id1 := newSignedClient(t, ts.URL)
	if _, err := c1.Join("welcome", id1); err != nil {
		t.Fatalf("first join should succeed: %v", err)
	}
	// A second distinct identity from the same IP trips the Sybil detector.
	c2, id2 := newSignedClient(t, ts.URL)
	if _, err := c2.Join("welcome", id2); err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("second distinct identity should be flagged (429), got %v", err)
	}
}

func TestRegisterWithoutAdmissionForbidden(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	defer ts.Close()

	// Signed, but never admitted → 403.
	c, id := newSignedClient(t, ts.URL)
	err := c.Register(NodeInfo{PeerID: id, Addrs: []string{"/ip4/9.9.9.9/tcp/4001"}})
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected 403 forbidden, got %v", err)
	}
}

func TestRegisterRejectsUnsignedRequest(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 5)
	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	defer ts.Close()

	c, id := newSignedClient(t, ts.URL)
	if _, err := c.Join("welcome", id); err != nil {
		t.Fatalf("join: %v", err)
	}

	// Same peer ID but no signer → coordinator rejects with 401.
	unsigned := NewClient(ts.URL)
	err := unsigned.Register(NodeInfo{PeerID: id})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 unauthorized, got %v", err)
	}
}

func TestRegisterRejectsPeerIDMismatch(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 5)
	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	defer ts.Close()

	c, id := newSignedClient(t, ts.URL)
	if _, err := c.Join("welcome", id); err != nil {
		t.Fatalf("join: %v", err)
	}

	// c signs as `id` but claims a different peer ID → mismatch → 401.
	_, otherID := newSignedClient(t, ts.URL)
	err := c.Register(NodeInfo{PeerID: otherID})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 for peer ID mismatch, got %v", err)
	}
}
