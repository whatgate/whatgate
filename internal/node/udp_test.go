package node

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/whatgate/whatgate/internal/exit"
	"github.com/whatgate/whatgate/internal/trust"
)

// startUDPEcho starts a UDP server that echoes each datagram back to its sender.
func startUDPEcho(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(buf[:n], addr)
		}
	}()
	t.Cleanup(func() { _ = pc.Close() })
	return pc.LocalAddr().String()
}

// TestUDPTunnelRoundTrip sends a datagram from a client node through an exit
// node to a UDP echo server and verifies the reply comes back over the tunnel.
func TestUDPTunnelRoundTrip(t *testing.T) {
	echo := startUDPEcho(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	exit, err := New(ctx, WithListenAddrs("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("new exit: %v", err)
	}
	t.Cleanup(func() { _ = exit.Close() })
	exit.EnableUDPExit()

	client, err := New(ctx, WithListenAddrs("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Connect(ctx, exit.AddrInfo()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	sess, err := client.OpenUDPSession(ctx, exit.ID())
	if err != nil {
		t.Fatalf("open udp session: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	if err := sess.Send(echo, []byte("ping")); err != nil {
		t.Fatalf("send: %v", err)
	}

	type reply struct {
		target  string
		payload []byte
		err     error
	}
	done := make(chan reply, 1)
	go func() {
		tg, p, err := sess.Receive()
		done <- reply{tg, p, err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("receive: %v", r.err)
		}
		if string(r.payload) != "ping" {
			t.Fatalf("payload = %q, want ping", r.payload)
		}
		if r.target != echo {
			t.Fatalf("reply target = %q, want %q", r.target, echo)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for UDP tunnel reply")
	}
}

// receiveTimeout reads one reply, reporting whether it timed out.
func receiveTimeout(s *UDPSession, d time.Duration) (payload []byte, timedOut bool) {
	ch := make(chan []byte, 1)
	go func() {
		_, p, err := s.Receive()
		if err == nil {
			ch <- p
		}
	}()
	select {
	case p := <-ch:
		return p, false
	case <-time.After(d):
		return nil, true
	}
}

// TestGuardedUDPExitEnforcesPolicy checks the UDP exit drops datagrams to a
// policy-blocked target while still serving an allowed one.
func TestGuardedUDPExitEnforcesPolicy(t *testing.T) {
	echoA := startUDPEcho(t) // allowed
	echoB := startUDPEcho(t) // blocked by port

	_, portBStr, _ := net.SplitHostPort(echoB)
	portB, _ := strconv.Atoi(portBStr)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	exitNode, err := New(ctx, WithListenAddrs("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("new exit: %v", err)
	}
	t.Cleanup(func() { _ = exitNode.Close() })
	guard := exit.NewGuard(exit.Policy{Scope: trust.ScopeOpen, BlockedPorts: map[int]bool{portB: true}})
	exitNode.EnableGuardedUDPExit(GuardedExit{
		Guard:  guard,
		TierOf: func(string) trust.Tier { return trust.TierStranger },
	})

	client, err := New(ctx, WithListenAddrs("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Connect(ctx, exitNode.AddrInfo()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	sess, err := client.OpenUDPSession(ctx, exitNode.ID())
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	// Allowed target replies.
	if err := sess.Send(echoA, []byte("a")); err != nil {
		t.Fatalf("send a: %v", err)
	}
	if p, timedOut := receiveTimeout(sess, 4*time.Second); timedOut || string(p) != "a" {
		t.Fatalf("allowed target: payload=%q timedOut=%v", p, timedOut)
	}

	// Blocked target is dropped (no reply).
	if err := sess.Send(echoB, []byte("b")); err != nil {
		t.Fatalf("send b: %v", err)
	}
	if _, timedOut := receiveTimeout(sess, 2*time.Second); !timedOut {
		t.Fatal("blocked target should have produced no reply")
	}
}
