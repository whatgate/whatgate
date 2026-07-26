// Package dnsx builds a DNS resolver an exit can pin, instead of relying on the
// exit host's system resolver. This matters for WhatGate's two DNS goals:
// geo-unblocking (resolve where the exit is, not where the client is — already
// achieved by resolving hostname targets at the exit) and censorship/leak
// resistance (an operator whose ISP resolver is poisoned or filtered can point at
// a trusted server). Resolution still happens at the exit; this only changes
// which server answers.
package dnsx

import (
	"context"
	"errors"
	"net"
)

// Resolver returns a *net.Resolver that sends queries to server (a "host" or
// "host:port"; :53 is assumed when no port is given), or nil when server is empty
// — nil means the caller's dialer uses the system resolver. Attach the result to
// a net.Dialer.Resolver so the exit's hostname dials use it.
func Resolver(server string) (*net.Resolver, error) {
	if server == "" {
		return nil, nil
	}
	addr, err := normalizeServer(server)
	if err != nil {
		return nil, err
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			// Ignore the address the resolver would pick; always query the
			// configured server.
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}, nil
}

// normalizeServer returns server with a port, defaulting to :53, and brackets a
// bare IPv6 literal.
func normalizeServer(server string) (string, error) {
	if server == "" {
		return "", errors.New("dnsx: empty server")
	}
	if _, _, err := net.SplitHostPort(server); err == nil {
		return server, nil // already host:port (or [ipv6]:port)
	}
	// No port present: append the default. JoinHostPort brackets IPv6 literals.
	return net.JoinHostPort(server, "53"), nil
}
