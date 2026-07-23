package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/proxy"
)

// recordingDialer records the addr it was asked to dial and redirects every
// request to a fixed target (the echo server), so the test can assert the
// SOCKS5 server parsed and forwarded the requested destination.
type recordingDialer struct {
	target string

	mu       sync.Mutex
	lastAddr string
}

func (r *recordingDialer) Dial(ctx context.Context, addr string) (net.Conn, error) {
	r.mu.Lock()
	r.lastAddr = addr
	r.mu.Unlock()
	return net.Dial("tcp", r.target)
}

func (r *recordingDialer) LastAddr() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastAddr
}

// startEchoServer starts a TCP server that echoes back whatever it receives.
func startEchoServer(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen echo: %v", err)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c) }()
		}
	}()
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func TestServerTunnelsConnectToDialerTarget(t *testing.T) {
	echo := startEchoServer(t)

	rec := &recordingDialer{target: echo.Addr().String()}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen socks: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	srv := &Server{Dialer: rec}
	go func() { _ = srv.Serve(l) }()

	// Run the whole client exchange under a hard timeout so a server that never
	// completes the SOCKS handshake fails the test fast instead of hanging until
	// the go-test deadline.
	done := make(chan error, 1)
	go func() { done <- roundTrip(l.Addr().String()) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("socks round trip: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for socks round trip")
	}

	if got := rec.LastAddr(); got != "example.com:1234" {
		t.Fatalf("dialer target: got %q want %q", got, "example.com:1234")
	}
}

// roundTrip dials a fake destination through the SOCKS5 server at socksAddr
// using an independent client implementation, then verifies a byte echo.
func roundTrip(socksAddr string) error {
	d, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		return fmt.Errorf("build socks client: %w", err)
	}
	conn, err := d.Dial("tcp", "example.com:1234")
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte("ping")); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if string(buf) != "ping" {
		return fmt.Errorf("echo mismatch: got %q want %q", buf, "ping")
	}
	return nil
}
