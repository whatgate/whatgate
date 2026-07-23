// Package tun provides WhatGate's system-wide TUN mode: it captures all of the
// host's traffic through a virtual network interface and forwards it to the
// node's local SOCKS5 proxy, which tunnels it to the selected exit. This turns
// WhatGate from a per-app (SOCKS) proxy into a whole-system one — the VPN-like
// experience.
//
// TUN support is behind the "tun" build tag because it pulls in a userspace
// network stack (gvisor, via tun2socks). The default build stays lean and
// returns an error if TUN is requested.
//
//	go build -tags tun ./cmd/node
//
// Running TUN mode requires administrator/root privileges to create the virtual
// adapter, and on Windows the wintun.dll driver next to the binary. You must
// also route traffic into the adapter (assign it an IP and set the default
// route), which the OS does not do automatically.
package tun

// Config configures TUN mode.
type Config struct {
	// Device is the TUN adapter name or spec. A bare name like "whatgate0" is
	// treated as "tun://whatgate0" (wintun on Windows).
	Device string
	// SocksAddr is the local SOCKS5 address captured traffic is forwarded to.
	SocksAddr string
	// MTU is the interface MTU; 0 uses a sensible default.
	MTU int
}
