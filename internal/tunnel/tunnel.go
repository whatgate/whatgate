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
	"time"

	"github.com/whatgate/whatgate/pkg/protocol"
)

// DialFunc dials a target address ("host:port") on the exit side.
type DialFunc func(ctx context.Context, addr string) (net.Conn, error)

// Authorizer decides whether the exit may serve a request for the given target
// ("host:port"). On approval it returns a release func to be called when the
// connection ends. A nil Authorizer means allow everything (no ExitGuard).
type Authorizer func(target string) (release func(), err error)

// Options bounds an exit-served connection's lifecycle, protecting the exit from
// slow/hung/idle peers holding resources. A zero value disables that limit.
type Options struct {
	// TargetReadTimeout caps how long the exit waits to receive the target
	// address before giving up (defeats slowloris pre-target stalls).
	TargetReadTimeout time.Duration
	// DialTimeout caps how long dialing the target may take.
	DialTimeout time.Duration
	// IdleTimeout closes the relay if no bytes flow in either direction for this
	// long (0 = never; long active transfers keep resetting it).
	IdleTimeout time.Duration
}

// ServeExit handles one inbound tunnel stream on the exit node: it reads the
// requested target, applies the authorizer (ExitGuard), dials the target, and
// relays bytes both ways until either side closes. A denied request closes the
// stream without dialing. opts bounds the read/dial/idle phases.
func ServeExit(stream net.Conn, authorize Authorizer, dial DialFunc, opts Options) error {
	defer stream.Close()

	// Bound the wait for the target so a peer can't open a stream and hold an
	// exit slot indefinitely by sending nothing (or a partial header).
	if opts.TargetReadTimeout > 0 {
		_ = stream.SetReadDeadline(time.Now().Add(opts.TargetReadTimeout))
	}
	addr, err := protocol.ReadTarget(stream)
	if err != nil {
		return err
	}
	if opts.TargetReadTimeout > 0 {
		_ = stream.SetReadDeadline(time.Time{}) // clear; the relay manages its own
	}

	if authorize != nil {
		release, err := authorize(addr)
		if err != nil {
			return err
		}
		defer release()
	}

	ctx := context.Background()
	if opts.DialTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.DialTimeout)
		defer cancel()
	}
	remote, err := dial(ctx, addr)
	if err != nil {
		return err
	}
	defer remote.Close()

	pipe(stream, remote, opts.IdleTimeout)
	return nil
}

// pipe copies bytes in both directions until either side closes. With idle > 0,
// a direction that sees no bytes for idle is torn down.
func pipe(a, b net.Conn, idle time.Duration) {
	done := make(chan struct{}, 2)
	go func() { copyIdle(a, b, idle); done <- struct{}{} }()
	go func() { copyIdle(b, a, idle); done <- struct{}{} }()
	<-done
}

// copyIdle copies src→dst, resetting src's read deadline on each read so an idle
// connection is closed after idle. idle <= 0 falls back to a plain copy.
func copyIdle(dst, src net.Conn, idle time.Duration) {
	if idle <= 0 {
		_, _ = io.Copy(dst, src)
		return
	}
	buf := make([]byte, 32*1024)
	for {
		_ = src.SetReadDeadline(time.Now().Add(idle))
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}
