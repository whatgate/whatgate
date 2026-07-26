package node

import (
	"net"

	"github.com/multiformats/go-multiaddr"

	"github.com/whatgate/whatgate/internal/exit"
)

// SafeDialAddrs filters a set of advertised multiaddrs down to those safe to
// dial: it drops any whose IP literal is loopback, private (RFC1918 / ULA),
// link-local (including the 169.254.169.254 cloud-metadata endpoint), or
// otherwise non-public, and drops malformed addrs. Addresses with no IP literal
// (dns*, relayed /p2p-circuit) are kept — they are resolved/relayed by libp2p and
// the peer's identity is still verified by the secure handshake. This stops a
// record advertised on the DHT from steering a dial at the operator's LAN,
// localhost, or a VPS metadata service (SSRF; reuses the exit-side F1 guard).
func SafeDialAddrs(addrs []string) []string {
	out := make([]string, 0, len(addrs))
	for _, s := range addrs {
		a, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			continue // malformed: drop
		}
		if ipStr, err := a.ValueForProtocol(multiaddr.P_IP4); err == nil {
			if exit.DisallowedTargetIP(net.ParseIP(ipStr)) {
				continue
			}
		} else if ipStr, err := a.ValueForProtocol(multiaddr.P_IP6); err == nil {
			if exit.DisallowedTargetIP(net.ParseIP(ipStr)) {
				continue
			}
		}
		out = append(out, s)
	}
	return out
}
