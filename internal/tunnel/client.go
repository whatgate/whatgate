package tunnel

import (
	"context"
	"net"

	"github.com/whatgate/whatgate/pkg/protocol"
)

// StreamOpener opens a fresh stream to the chosen exit node. In production this
// opens a libp2p stream; in tests it can return one end of a net.Pipe.
type StreamOpener func(ctx context.Context) (net.Conn, error)

// ClientDialer is the client half of the tunnel. It satisfies the Dialer used by
// the SOCKS5 ingress: each Dial opens a stream to the exit, frames the target
// address on it, and returns the stream as the connection to the target.
type ClientDialer struct {
	Open StreamOpener
}

// Dial opens a tunnel stream and sends addr as its target, returning the stream
// as the connection the caller can read/write the target through.
func (d *ClientDialer) Dial(ctx context.Context, addr string) (net.Conn, error) {
	stream, err := d.Open(ctx)
	if err != nil {
		return nil, err
	}
	if err := protocol.WriteTarget(stream, addr); err != nil {
		_ = stream.Close()
		return nil, err
	}
	return stream, nil
}
