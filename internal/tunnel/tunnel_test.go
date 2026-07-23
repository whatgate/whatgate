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

	go func() { _ = ServeExit(exitEnd, nil, dial) }()

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

type errString string

func (e errString) Error() string { return string(e) }
