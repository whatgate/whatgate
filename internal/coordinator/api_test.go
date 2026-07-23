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
