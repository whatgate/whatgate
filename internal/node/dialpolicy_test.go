package node

import "testing"

// SafeDialAddrs keeps public-IP and non-IP (dns / relayed) multiaddrs and drops
// any pointing at loopback, private, link-local, or cloud-metadata addresses — so
// a record advertised on the DHT cannot steer a dial at our LAN, localhost, or a
// VPS metadata endpoint (SSRF, Codex P0).
func TestSafeDialAddrs(t *testing.T) {
	in := []string{
		"/ip4/1.2.3.4/tcp/443/ws",           // public: keep
		"/ip4/127.0.0.1/tcp/4001",           // loopback: drop
		"/ip4/10.0.0.5/tcp/4001",            // private: drop
		"/ip4/192.168.1.9/tcp/4001",         // private: drop
		"/ip4/169.254.169.254/tcp/80",       // metadata: drop
		"/ip6/2606:4700:4700::1111/tcp/443", // public v6: keep
		"/ip6/::1/tcp/4001",                 // loopback v6: drop
		"/dns4/exit.example.com/tcp/443/ws", // dns (no IP literal): keep
		"/ip4/9.9.9.9/tcp/4001/p2p-circuit", // relayed via public: keep
		"not-a-multiaddr",                   // malformed: drop
	}
	got := SafeDialAddrs(in)

	want := map[string]bool{
		"/ip4/1.2.3.4/tcp/443/ws":           true,
		"/ip6/2606:4700:4700::1111/tcp/443": true,
		"/dns4/exit.example.com/tcp/443/ws": true,
		"/ip4/9.9.9.9/tcp/4001/p2p-circuit": true,
	}
	if len(got) != len(want) {
		t.Fatalf("SafeDialAddrs = %v, want %d entries", got, len(want))
	}
	for _, a := range got {
		if !want[a] {
			t.Fatalf("unexpectedly kept unsafe/dropped addr %q (got %v)", a, got)
		}
	}
}
