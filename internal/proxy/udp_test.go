package proxy

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

// echoTunnel simulates an exit whose target echoes datagrams straight back.
type echoTunnel struct{ ch chan dgram }

type dgram struct {
	target  string
	payload []byte
}

func (e *echoTunnel) Send(target string, payload []byte) error {
	e.ch <- dgram{target, append([]byte(nil), payload...)}
	return nil
}
func (e *echoTunnel) Receive() (string, []byte, error) {
	d, ok := <-e.ch
	if !ok {
		return "", nil, io.EOF
	}
	return d.target, d.payload, nil
}
func (e *echoTunnel) Close() error { return nil }

func TestSOCKS5UDPAssociate(t *testing.T) {
	srv := &Server{OpenUDPTunnel: func() (UDPTunnel, error) {
		return &echoTunnel{ch: make(chan dgram, 8)}, nil
	}}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() { _ = srv.Serve(l) }()

	c, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatalf("dial control: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	// No-auth negotiation.
	if _, err := c.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatalf("negotiate: %v", err)
	}
	nego := make([]byte, 2)
	if _, err := io.ReadFull(c, nego); err != nil || nego[1] != 0x00 {
		t.Fatalf("negotiation reply = %v, %v", nego, err)
	}

	// UDP ASSOCIATE (client advertises 0.0.0.0:0).
	if _, err := c.Write([]byte{0x05, cmdUDPAssociate, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatalf("associate: %v", err)
	}
	rep := make([]byte, 10) // ver rep rsv atyp ip(4) port(2)
	if _, err := io.ReadFull(c, rep); err != nil {
		t.Fatalf("associate reply: %v", err)
	}
	if rep[1] != repSucceeded {
		t.Fatalf("associate rep = %d", rep[1])
	}
	relay := &net.UDPAddr{IP: net.IP(rep[4:8]), Port: int(binary.BigEndian.Uint16(rep[8:10]))}

	// Send a UDP datagram for 1.2.3.4:53 through the relay.
	uc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("client udp: %v", err)
	}
	t.Cleanup(func() { _ = uc.Close() })

	pkt := append([]byte{0, 0, 0, atypIPv4, 1, 2, 3, 4, 0, 53}, []byte("ping")...)
	if _, err := uc.WriteToUDP(pkt, relay); err != nil {
		t.Fatalf("send udp: %v", err)
	}

	_ = uc.SetReadDeadline(time.Now().Add(3 * time.Second))
	rbuf := make([]byte, 1024)
	n, _, err := uc.ReadFromUDP(rbuf)
	if err != nil {
		t.Fatalf("read udp reply: %v", err)
	}
	// Reply header for an IPv4 source: RSV(2)+FRAG(1)+ATYP(1)+IP(4)+PORT(2) = 10.
	if n < 10 {
		t.Fatalf("reply too short: %d", n)
	}
	if got := string(rbuf[10:n]); got != "ping" {
		t.Fatalf("reply data = %q, want ping", got)
	}
}
