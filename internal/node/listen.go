package node

import (
	"fmt"
	"strings"

	"github.com/multiformats/go-multiaddr"
)

// ParseListenAddrs splits a comma-separated list of libp2p listen multiaddrs and
// validates each, so a typo is caught early with a clear message instead of
// libp2p silently failing to bring up a listener.
//
// Beyond the default TCP address, a node can listen on a WebSocket address (e.g.
// "/ip4/0.0.0.0/tcp/443/ws"), which makes its data-plane connections ride port
// 443 and resemble ordinary web traffic — the cheapest layer of censorship
// resistance (A4). Note this is look-alike port camouflage, NOT probe resistance
// (see Tier B), and to truly resemble HTTPS the WebSocket must sit behind TLS
// (a "/tls/ws" address with a certificate, or a TLS-terminating reverse proxy).
// Most residential nodes cannot bind an inbound privileged port or lack a public
// IPv4 at all; those simply stay dial-only and reach exits via a relay.
func ParseListenAddrs(s string) ([]string, error) {
	var out []string
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if _, err := multiaddr.NewMultiaddr(p); err != nil {
			return nil, fmt.Errorf("invalid listen address %q: %w", p, err)
		}
		out = append(out, p)
	}
	return out, nil
}
