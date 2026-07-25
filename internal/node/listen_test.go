package node

import (
	"context"
	"strings"
	"testing"
)

// ParseListenAddrs splits a comma-separated list of listen multiaddrs, trimming
// whitespace and preserving order.
func TestParseListenAddrsMultiple(t *testing.T) {
	got, err := ParseListenAddrs("/ip4/0.0.0.0/tcp/0 , /ip4/0.0.0.0/tcp/443/ws")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []string{"/ip4/0.0.0.0/tcp/0", "/ip4/0.0.0.0/tcp/443/ws"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// A malformed multiaddr is rejected with an error naming the offending value,
// rather than being silently dropped or passed to libp2p.
func TestParseListenAddrsRejectsMalformed(t *testing.T) {
	_, err := ParseListenAddrs("/ip4/0.0.0.0/tcp/0,not-a-multiaddr")
	if err == nil {
		t.Fatal("expected an error for a malformed multiaddr")
	}
	if !strings.Contains(err.Error(), "not-a-multiaddr") {
		t.Fatalf("error should name the bad value, got: %v", err)
	}
}

// Empty input yields no addresses (the caller falls back to the node default).
func TestParseListenAddrsEmpty(t *testing.T) {
	got, err := ParseListenAddrs("  ,  ")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

// A node configured with a WebSocket listen address actually brings up a
// WebSocket listener — the transport that lets data-plane traffic ride :443 like
// web traffic.
func TestNodeListensOnWebSocket(t *testing.T) {
	addrs, err := ParseListenAddrs("/ip4/127.0.0.1/tcp/0/ws")
	if err != nil {
		t.Fatal(err)
	}
	n, err := New(context.Background(), WithListenAddrs(addrs...))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer n.Close()

	var found bool
	for _, a := range n.AddrStrings() {
		if strings.Contains(a, "/ws") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no /ws listen address; got %v", n.AddrStrings())
	}
}
