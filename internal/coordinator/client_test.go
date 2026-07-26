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

	c, id := newSignedClient(t, ts.URL)

	adm, err := c.Join("welcome", id)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if adm.Issuer != "founder" {
		t.Fatalf("issuer = %q, want founder", adm.Issuer)
	}

	err = c.Register(NodeInfo{
		PeerID:   id,
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
	if list[0].PeerID != id || list[0].Region != "JP" || !list[0].WantExit {
		t.Fatalf("entry = %+v", list[0])
	}
}

func TestClientRelayReturnsAdvertisedRelay(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	srv := NewServer(dir, inv)
	srv.SetRelayInfo("RID", []string{"/ip4/1.2.3.4/tcp/4001"})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	c := NewClient(ts.URL)
	info, err := c.Relay()
	if err != nil {
		t.Fatalf("Relay: %v", err)
	}
	if info.PeerID != "RID" || len(info.Addrs) != 1 || info.Addrs[0] != "/ip4/1.2.3.4/tcp/4001" {
		t.Fatalf("relay info = %+v", info)
	}
}

func TestClientRelayAbsentReturnsErrNoRelay(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	defer ts.Close()

	c := NewClient(ts.URL)
	if _, err := c.Relay(); err != ErrNoRelay {
		t.Fatalf("Relay err = %v, want ErrNoRelay", err)
	}
}

func TestClientJoinUnknownCodeErrors(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	defer ts.Close()

	c, id := newSignedClient(t, ts.URL)
	if _, err := c.Join("nope", id); err == nil {
		t.Fatal("Join with unknown code should error")
	}
}
