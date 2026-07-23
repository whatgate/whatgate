package coordinator

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientJoinRegisterDirectory(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 5)

	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	defer ts.Close()

	c := NewClient(ts.URL)

	issuer, err := c.Join("welcome", "peerA")
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if issuer != "founder" {
		t.Fatalf("issuer = %q, want founder", issuer)
	}

	err = c.Register(NodeInfo{
		PeerID:   "peerA",
		Addrs:    []string{"/ip4/1.2.3.4/tcp/4001"},
		Region:   "JP",
		WantExit: true,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	list, err := c.Directory()
	if err != nil {
		t.Fatalf("Directory: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("directory has %d entries, want 1", len(list))
	}
	if list[0].PeerID != "peerA" || list[0].Region != "JP" || !list[0].WantExit {
		t.Fatalf("entry = %+v", list[0])
	}
}

func TestClientJoinUnknownCodeErrors(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	defer ts.Close()

	c := NewClient(ts.URL)
	if _, err := c.Join("nope", "peerA"); err == nil {
		t.Fatal("Join with unknown code should error")
	}
}
