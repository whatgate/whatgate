package tunnel

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/whatgate/whatgate/pkg/protocol"
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

func TestServeExitDialsTargetAndPipes(t *testing.T) {
	echo := startEchoServer(t)

	var dialed string
	dial := func(ctx context.Context, addr string) (net.Conn, error) {
		dialed = addr
		return net.Dial("tcp", echo.Addr().String())
	}

	clientEnd, exitEnd := net.Pipe()
	t.Cleanup(func() { _ = clientEnd.Close() })

	go func() { _ = ServeExit(exitEnd, nil, dial, Options{}) }()

	done := make(chan error, 1)
	go func() {
		// Client speaks the tunnel protocol: target first, then payload.
		if err := protocol.WriteTarget(clientEnd, "example.com:80"); err != nil {
			done <- err
			return
		}
		if _, err := clientEnd.Write([]byte("ping")); err != nil {
			done <- err
			return
		}
		buf := make([]byte, 4)
		if _, err := io.ReadFull(clientEnd, buf); err != nil {
			done <- err
			return
		}
		if string(buf) != "ping" {
			done <- errString("echo mismatch: " + string(buf))
			return
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("tunnel exchange: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for tunnel exchange")
	}

	if dialed != "example.com:80" {
		t.Fatalf("exit dialed %q want %q", dialed, "example.com:80")
	}
}

// TestServeExitTargetReadTimeout: a peer that opens the stream but never sends a
// full target must not hold the exit — ServeExit returns once the deadline hits.
func TestServeExitTargetReadTimeout(t *testing.T) {
	dial := func(ctx context.Context, addr string) (net.Conn, error) {
		t.Error("dial should not be reached when the target never arrives")
		return nil, nil
	}
	clientEnd, exitEnd := net.Pipe()
	t.Cleanup(func() { _ = clientEnd.Close() })

	done := make(chan error, 1)
	go func() { done <- ServeExit(exitEnd, nil, dial, Options{TargetReadTimeout: 200 * time.Millisecond}) }()

	// Client sends nothing.
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a timeout error, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ServeExit did not return after the target-read timeout")
	}
}

// TestServeExitDialTimeout: a target that never connects must not hang the exit.
func TestServeExitDialTimeout(t *testing.T) {
	dial := func(ctx context.Context, addr string) (net.Conn, error) {
		<-ctx.Done() // never connects; wait for the dial timeout
		return nil, ctx.Err()
	}
	clientEnd, exitEnd := net.Pipe()
	t.Cleanup(func() { _ = clientEnd.Close() })

	done := make(chan error, 1)
	go func() { done <- ServeExit(exitEnd, nil, dial, Options{DialTimeout: 200 * time.Millisecond}) }()

	go func() { _ = protocol.WriteTarget(clientEnd, "example.com:80") }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a dial timeout error, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ServeExit did not return after the dial timeout")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
