package proxy

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
)

// UDPTunnel carries UDP datagrams to/from an exit. node.UDPSession satisfies it.
type UDPTunnel interface {
	Send(target string, payload []byte) error
	Receive() (target string, payload []byte, err error)
	Close() error
}

var (
	errShortUDP = errors.New("proxy: short SOCKS5 UDP datagram")
	errFragUDP  = errors.New("proxy: SOCKS5 UDP fragmentation unsupported")
)

// handleUDPAssociate implements SOCKS5 UDP ASSOCIATE: it binds a local UDP relay,
// tells the client its address, and shuttles datagrams between the client and a
// UDP tunnel to the exit until the control connection closes.
func (s *Server) handleUDPAssociate(ctrl net.Conn) {
	if s.OpenUDPTunnel == nil {
		_ = sendReply(ctrl, repCommandNotSupported)
		return
	}
	relay, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		_ = sendReply(ctrl, repGeneralFailure)
		return
	}
	defer relay.Close()
	if err := sendReplyAddr(ctrl, relay.LocalAddr().(*net.UDPAddr)); err != nil {
		return
	}

	tun, err := s.OpenUDPTunnel()
	if err != nil {
		return
	}
	defer tun.Close()

	var client *net.UDPAddr // learned from the first datagram
	clientCh := make(chan *net.UDPAddr, 1)

	// Tunnel replies -> client app.
	go func() {
		var ca *net.UDPAddr
		for {
			target, payload, err := tun.Receive()
			if err != nil {
				return
			}
			if ca == nil {
				select {
				case ca = <-clientCh:
				default:
					continue // no client addr yet; drop
				}
			}
			if pkt := buildUDPPacket(target, payload); pkt != nil {
				_, _ = relay.WriteToUDP(pkt, ca)
			}
		}
	}()

	// Closing the control TCP tears down the association.
	go func() { _, _ = io.Copy(io.Discard, ctrl); relay.Close(); tun.Close() }()

	buf := make([]byte, 64*1024)
	for {
		n, addr, err := relay.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if client == nil {
			client = addr
			clientCh <- addr
		}
		target, data, err := parseUDPPacket(buf[:n])
		if err != nil {
			continue
		}
		_ = tun.Send(target, data)
	}
}

// sendReplyAddr writes a SOCKS5 success reply carrying a bound UDP address.
func sendReplyAddr(w io.Writer, addr *net.UDPAddr) error {
	reply := []byte{socksVersion, repSucceeded, 0x00}
	if v4 := addr.IP.To4(); v4 != nil {
		reply = append(reply, atypIPv4)
		reply = append(reply, v4...)
	} else {
		reply = append(reply, atypIPv6)
		reply = append(reply, addr.IP.To16()...)
	}
	var port [2]byte
	binary.BigEndian.PutUint16(port[:], uint16(addr.Port))
	reply = append(reply, port[:]...)
	_, err := w.Write(reply)
	return err
}

// parseUDPPacket parses a SOCKS5 UDP request: RSV(2) FRAG(1) ATYP DST.ADDR
// DST.PORT DATA, returning the target and payload.
func parseUDPPacket(pkt []byte) (target string, data []byte, err error) {
	if len(pkt) < 4 {
		return "", nil, errShortUDP
	}
	if pkt[2] != 0 {
		return "", nil, errFragUDP
	}
	rest := pkt[4:]
	var host string
	var n int
	switch pkt[3] {
	case atypIPv4:
		if len(rest) < net.IPv4len+2 {
			return "", nil, errShortUDP
		}
		host, n = net.IP(rest[:net.IPv4len]).String(), net.IPv4len
	case atypIPv6:
		if len(rest) < net.IPv6len+2 {
			return "", nil, errShortUDP
		}
		host, n = net.IP(rest[:net.IPv6len]).String(), net.IPv6len
	case atypDomain:
		if len(rest) < 1 {
			return "", nil, errShortUDP
		}
		dlen := int(rest[0])
		if len(rest) < 1+dlen+2 {
			return "", nil, errShortUDP
		}
		host, n = string(rest[1:1+dlen]), 1+dlen
	default:
		return "", nil, errShortUDP
	}
	port := binary.BigEndian.Uint16(rest[n : n+2])
	return net.JoinHostPort(host, strconv.Itoa(int(port))), rest[n+2:], nil
}

// buildUDPPacket wraps data in a SOCKS5 UDP reply datagram for source target.
func buildUDPPacket(target string, data []byte) []byte {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return nil
	}
	port, _ := strconv.Atoi(portStr)
	pkt := []byte{0, 0, 0} // RSV RSV FRAG
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			pkt = append(pkt, atypIPv4)
			pkt = append(pkt, v4...)
		} else {
			pkt = append(pkt, atypIPv6)
			pkt = append(pkt, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return nil
		}
		pkt = append(pkt, atypDomain, byte(len(host)))
		pkt = append(pkt, host...)
	}
	var pb [2]byte
	binary.BigEndian.PutUint16(pb[:], uint16(port))
	pkt = append(pkt, pb[:]...)
	return append(pkt, data...)
}
