package coordinator

import (
	"testing"
	"time"
)

// clock is a mutable time source for exercising expiry.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func TestRegisterThenListReturnsEntry(t *testing.T) {
	d := NewDirectory(time.Minute, func() time.Time { return time.Unix(1000, 0) })

	d.Register(NodeInfo{
		PeerID:   "12D3KooExample",
		Addrs:    []string{"/ip4/1.2.3.4/tcp/4001"},
		Region:   "JP",
		WantExit: true,
	})

	got := d.List()
	if len(got) != 1 {
		t.Fatalf("List returned %d entries, want 1", len(got))
	}
	if got[0].PeerID != "12D3KooExample" {
		t.Fatalf("PeerID = %q, want %q", got[0].PeerID, "12D3KooExample")
	}
	if got[0].Region != "JP" || !got[0].WantExit {
		t.Fatalf("entry fields not preserved: %+v", got[0])
	}
}

func TestReRegisterUpsertsWithoutDuplicating(t *testing.T) {
	d := NewDirectory(time.Minute, func() time.Time { return time.Unix(1000, 0) })

	d.Register(NodeInfo{PeerID: "peerA", Region: "JP"})
	d.Register(NodeInfo{PeerID: "peerA", Region: "US"})

	got := d.List()
	if len(got) != 1 {
		t.Fatalf("List returned %d entries, want 1 (upsert)", len(got))
	}
	if got[0].Region != "US" {
		t.Fatalf("Region = %q, want %q (latest wins)", got[0].Region, "US")
	}
}

func TestStaleEntriesExpireFromList(t *testing.T) {
	c := &clock{t: time.Unix(1000, 0)}
	d := NewDirectory(30*time.Second, c.now)

	d.Register(NodeInfo{PeerID: "peerA"})

	c.advance(20 * time.Second)
	if len(d.List()) != 1 {
		t.Fatal("entry should still be live before TTL")
	}

	c.advance(20 * time.Second) // now 40s since register, past 30s TTL
	if len(d.List()) != 0 {
		t.Fatal("entry should have expired past TTL")
	}
}

func TestRefreshKeepsEntryAlive(t *testing.T) {
	c := &clock{t: time.Unix(1000, 0)}
	d := NewDirectory(30*time.Second, c.now)

	d.Register(NodeInfo{PeerID: "peerA"})
	c.advance(20 * time.Second)
	d.Register(NodeInfo{PeerID: "peerA"}) // refresh
	c.advance(20 * time.Second)           // 20s since refresh, still < TTL

	if len(d.List()) != 1 {
		t.Fatal("refreshed entry should stay live")
	}
}

func TestUnregisterRemovesEntry(t *testing.T) {
	d := NewDirectory(time.Minute, func() time.Time { return time.Unix(1000, 0) })

	d.Register(NodeInfo{PeerID: "peerA"})
	d.Unregister("peerA")

	if len(d.List()) != 0 {
		t.Fatal("entry should be gone after Unregister")
	}
}
