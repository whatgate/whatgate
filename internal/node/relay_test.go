package node

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/whatgate/whatgate/internal/relay"
)

// TestTunnelOverCircuitRelay proves the relay fallback: the exit reserves a slot
// on a relay, and the client — given ONLY the exit's /p2p-circuit address — still
// tunnels through to an echo server. This exercises the relayed data path that
// takes over when direct hole punching is impossible.
func TestTunnelOverCircuitRelay(t *testing.T) {
	echo := startEchoServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	r, err := relay.New(ctx, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	exit, err := New(ctx, WithListenAddrs("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("new exit: %v", err)
	}
	t.Cleanup(func() { _ = exit.Close() })
	exit.EnableExit(func(ctx context.Context, addr string) (net.Conn, error) {
		return net.Dial("tcp", echo.Addr().String())
	})

	// Exit reserves a relay slot so it is reachable via /p2p-circuit.
	if err := exit.ReserveRelay(ctx, r.AddrInfo()); err != nil {
		t.Fatalf("reserve relay: %v", err)
	}

	// Client is told to reach the exit ONLY through the relay circuit.
	circAddrs, err := CircuitAddrsVia(r.AddrInfo())
	if err != nil {
		t.Fatalf("circuit addrs: %v", err)
	}
	exitViaRelay := peer.AddrInfo{ID: exit.ID(), Addrs: circAddrs}

	client, err := New(ctx, WithListenAddrs("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if err := client.Connect(ctx, exitViaRelay); err != nil {
		t.Fatalf("relayed connect: %v", err)
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
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
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
			t.Fatalf("relayed tunnel round trip: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for relayed tunnel")
	}
}
