package node

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

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

// TestTwoNodesTunnelThroughExit spins up two real libp2p nodes: an exit that
// dials a local echo server, and a client that tunnels to it. Bytes written by
// the client must come back echoed, proving traffic traversed the exit.
func TestTwoNodesTunnelThroughExit(t *testing.T) {
	echo := startEchoServer(t)
	ctx := context.Background()

	exit, err := New(ctx, WithListenAddrs("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("new exit node: %v", err)
	}
	t.Cleanup(func() { _ = exit.Close() })

	// The exit ignores the requested target and dials the echo server, keeping
	// the test hermetic (no real DNS/egress).
	exit.EnableExit(func(ctx context.Context, addr string) (net.Conn, error) {
		return net.Dial("tcp", echo.Addr().String())
	})

	client, err := New(ctx, WithListenAddrs("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("new client node: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if err := client.Connect(ctx, exit.AddrInfo()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	d := client.NewClientDialer(exit.ID())

	done := make(chan error, 1)
	go func() {
		conn, err := d.Dial(ctx, "example.com:80")
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err := conn.Write([]byte("ping")); err != nil {
			done <- err
			return
		}
		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			done <- err
			return
		}
		if string(buf) != "ping" {
			done <- io.ErrUnexpectedEOF
			return
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("tunnel round trip: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for tunnel round trip")
	}
}
