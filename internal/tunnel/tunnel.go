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

// ServeExit handles one inbound tunnel stream on the exit node: it reads the
// requested target, dials it via dial, and relays bytes both ways until either
// side closes.
func ServeExit(stream net.Conn, dial DialFunc) error {
	defer stream.Close()

	addr, err := protocol.ReadTarget(stream)
	if err != nil {
		return err
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
