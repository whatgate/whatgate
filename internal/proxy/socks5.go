// Package proxy implements WhatGate's local ingress: a SOCKS5 server that hands
// each CONNECT request off to a pluggable Dialer. The Dialer abstraction keeps
// this package decoupled from the libp2p transport, so the ingress can be tested
// on its own and reused whether the target is dialed directly or tunneled to a
// remote exit node.
package proxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
)

// Dialer establishes a connection to addr (in "host:port" form) on behalf of a
// SOCKS5 client. In WhatGate the concrete Dialer tunnels to a remote exit node,
// but any implementation that yields a net.Conn to the target works.
type Dialer interface {
	Dial(ctx context.Context, addr string) (net.Conn, error)
}

// Server is a minimal SOCKS5 (no-auth) ingress. TCP CONNECT is forwarded through
// Dialer; UDP ASSOCIATE (if OpenUDPTunnel is set) is relayed through a UDP tunnel
// to the exit.
type Server struct {
	Dialer Dialer
	// OpenUDPTunnel, if set, opens a UDP tunnel to the exit for a UDP ASSOCIATE
	// association. Nil means UDP ASSOCIATE is unsupported.
	OpenUDPTunnel func() (UDPTunnel, error)
}

// SOCKS5 protocol constants (RFC 1928).
const (
	socksVersion = 0x05

	authNoAuth       = 0x00
	authNoAcceptable = 0xFF

	cmdConnect      = 0x01
	cmdUDPAssociate = 0x03

	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04

	repSucceeded           = 0x00
	repGeneralFailure      = 0x01
	repCommandNotSupported = 0x07
	repAddressNotSupported = 0x08
)

// Serve accepts connections on l until it is closed, handling each in its own
// goroutine.
func (s *Server) Serve(l net.Listener) error {
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go s.handleConn(c)
	}
}

func (s *Server) handleConn(c net.Conn) {
	defer c.Close()

	if err := s.negotiate(c); err != nil {
		return
	}

	cmd, addr, err := s.readRequest(c)
	if err != nil {
		// readRequest sends the appropriate error reply itself.
		return
	}

	switch cmd {
	case cmdConnect:
		s.handleConnect(c, addr)
	case cmdUDPAssociate:
		s.handleUDPAssociate(c)
	default:
		_ = sendReply(c, repCommandNotSupported)
	}
}

// handleConnect dials the target and pipes bytes both ways.
func (s *Server) handleConnect(c net.Conn, addr string) {
	remote, err := s.Dialer.Dial(context.Background(), addr)
	if err != nil {
		_ = sendReply(c, repGeneralFailure)
		return
	}
	defer remote.Close()

	if err := sendReply(c, repSucceeded); err != nil {
		return
	}
	pipe(c, remote)
}

// negotiate performs the SOCKS5 method-selection handshake, accepting only the
// no-authentication method.
func (s *Server) negotiate(c net.Conn) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(c, header); err != nil {
		return err
	}
	if header[0] != socksVersion {
		return fmt.Errorf("unsupported socks version %d", header[0])
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(c, methods); err != nil {
		return err
	}
	for _, m := range methods {
		if m == authNoAuth {
			_, err := c.Write([]byte{socksVersion, authNoAuth})
			return err
		}
	}
	_, _ = c.Write([]byte{socksVersion, authNoAcceptable})
	return fmt.Errorf("no acceptable auth method")
}

// readRequest parses a SOCKS5 request, returning its command and target
// ("host:port"). On a protocol or unsupported-feature error it writes the
// matching SOCKS5 error reply before returning.
func (s *Server) readRequest(c net.Conn) (cmd byte, addr string, err error) {
	header := make([]byte, 4) // ver, cmd, rsv, atyp
	if _, err := io.ReadFull(c, header); err != nil {
		return 0, "", err
	}
	if header[0] != socksVersion {
		_ = sendReply(c, repGeneralFailure)
		return 0, "", fmt.Errorf("unsupported socks version %d", header[0])
	}
	cmd = header[1]

	host, err := readAddr(c, header[3])
	if err != nil {
		_ = sendReply(c, repAddressNotSupported)
		return cmd, "", err
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(c, portBuf); err != nil {
		return cmd, "", err
	}
	port := binary.BigEndian.Uint16(portBuf)
	return cmd, net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

// readAddr reads a SOCKS5 address of the given type from r, returning the host.
func readAddr(r io.Reader, atyp byte) (string, error) {
	switch atyp {
	case atypIPv4:
		buf := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	case atypIPv6:
		buf := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	case atypDomain:
		lenByte := make([]byte, 1)
		if _, err := io.ReadFull(r, lenByte); err != nil {
			return "", err
		}
		buf := make([]byte, int(lenByte[0]))
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return string(buf), nil
	default:
		return "", fmt.Errorf("unsupported address type %d", atyp)
	}
}

// sendReply writes a SOCKS5 reply with the given status and a zero bound
// address (0.0.0.0:0), which clients accept for CONNECT replies.
func sendReply(c net.Conn, status byte) error {
	_, err := c.Write([]byte{socksVersion, status, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0})
	return err
}

// pipe copies bytes in both directions until either side closes.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(a, b); done <- struct{}{} }()
	go func() { _, _ = io.Copy(b, a); done <- struct{}{} }()
	<-done
}
