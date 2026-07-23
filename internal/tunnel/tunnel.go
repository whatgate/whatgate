// Package tunnel carries proxied traffic between a WhatGate client and an exit
// node over a single stream. It is transport-agnostic: it operates on net.Conn
// (in production a libp2p stream, in tests a net.Pipe), so the tunnel logic can
// be exercised without any networking stack.
//
// Stream layout: the client first sends the target address (see pkg/protocol),
// then the two ends relay raw bytes in both directions.
package tunnel

import (
	"context"
	"io"
	"net"

	"github.com/whatgate/whatgate/pkg/protocol"
)

// DialFunc dials a target address ("host:port") on the exit side.
type DialFunc func(ctx context.Context, addr string) (net.Conn, error)

// Authorizer decides whether the exit may serve a request for the given target
// ("host:port"). On approval it returns a release func to be called when the
// connection ends. A nil Authorizer means allow everything (no ExitGuard).
type Authorizer func(target string) (release func(), err error)

// ServeExit handles one inbound tunnel stream on the exit node: it reads the
// requested target, applies the authorizer (ExitGuard), dials the target, and
// relays bytes both ways until either side closes. A denied request closes the
// stream without dialing.
func ServeExit(stream net.Conn, authorize Authorizer, dial DialFunc) error {
	defer stream.Close()

	addr, err := protocol.ReadTarget(stream)
	if err != nil {
		return err
	}

	if authorize != nil {
		release, err := authorize(addr)
		if err != nil {
			return err
		}
		defer release()
	}

	remote, err := dial(context.Background(), addr)
	if err != nil {
		return err
	}
	defer remote.Close()

	pipe(stream, remote)
	return nil
}

// pipe copies bytes in both directions until either side closes.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(a, b); done <- struct{}{} }()
	go func() { _, _ = io.Copy(b, a); done <- struct{}{} }()
	<-done
}
