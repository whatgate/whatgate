package exit

import (
	"net"
	"syscall"
)

// DisallowedTargetIP reports whether ip must not be dialed by an exit serving
// others' traffic: loopback, private (RFC1918 / ULA), link-local (which includes
// the 169.254.169.254 cloud-metadata endpoint), unspecified, or multicast.
// Only public unicast addresses are allowed. This is the core SSRF guard — it
// stops a requester from using someone's exit to reach the operator's LAN,
// localhost services, or a VPS's metadata credentials.
func DisallowedTargetIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() || // 10/8, 172.16/12, 192.168/16, fc00::/7
		ip.IsLinkLocalUnicast() || // 169.254/16 (incl. metadata), fe80::/10
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

// checkDialAddress rejects a post-resolution "ip:port" address pointing at a
// disallowed range, unless allowPrivate is set. A non-IP address (should not
// happen after resolution) is allowed through so we never break on a form we
// don't understand — the IP-literal path in Authorize is the primary gate.
func checkDialAddress(address string, allowPrivate bool) error {
	if allowPrivate {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	if ip := net.ParseIP(host); ip != nil && DisallowedTargetIP(ip) {
		return ErrBlockedPrivateTarget
	}
	return nil
}

// DialControl returns a net.Dialer.Control hook that blocks connections to
// disallowed (private/loopback/metadata) addresses. Because Control runs after
// DNS resolution on the concrete IP about to be dialed, it also defeats
// hostnames that resolve to internal IPs (DNS rebinding) — the case the
// Authorize-time literal check cannot see.
func DialControl(allowPrivate bool) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		return checkDialAddress(address, allowPrivate)
	}
}
