package tunnel

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/whatgate/whatgate/pkg/protocol"
)

func TestClientDialerFramesTargetThenTunnels(t *testing.T) {
	clientEnd, serverEnd := net.Pipe()
	t.Cleanup(func() { _ = serverEnd.Close() })

	d := &ClientDialer{
		Open: func(ctx context.Context) (net.Conn, error) { return clientEnd, nil },
	}

	// The far end reads the framed target, then echoes any further bytes.
	gotAddr := make(chan string, 1)
	go func() {
		addr, err := protocol.ReadTarget(serverEnd)
		if err != nil {
			gotAddr <- "read error: " + err.Error()
			return
		}
		gotAddr <- addr
		_, _ = io.Copy(serverEnd, serverEnd)
	}()

	done := make(chan error, 1)
	go func() {
		conn, err := d.Dial(context.Background(), "target.example:8443")
		if err != nil {
			done <- err
			return
		}
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
			done <- errString("echo mismatch: " + string(buf))
			return
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("client dial/tunnel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for client tunnel")
	}

	if addr := <-gotAddr; addr != "target.example:8443" {
		t.Fatalf("exit received target %q want %q", addr, "target.example:8443")
	}
}
