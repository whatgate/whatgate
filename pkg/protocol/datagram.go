package protocol

import (
	"encoding/binary"
	"errors"
	"io"
)

// maxDatagramLen bounds a single tunneled UDP payload.
const maxDatagramLen = 65535

// ErrDatagramTooLarge is returned for oversized payloads.
var ErrDatagramTooLarge = errors.New("protocol: datagram payload too large")

// WriteDatagram frames one tunneled UDP datagram on a stream: a uint16
// length-prefixed target ("host:port") followed by a uint32 length-prefixed
// payload. Multiple datagrams (potentially to different targets) share one
// stream.
func WriteDatagram(w io.Writer, target string, payload []byte) error {
	if len(target) == 0 || len(target) >= maxTargetLen {
		return ErrEmptyTarget
	}
	if len(payload) > maxDatagramLen {
		return ErrDatagramTooLarge
	}
	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(len(target)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if _, err := io.WriteString(w, target); err != nil {
		return err
	}
	var plen [4]byte
	binary.BigEndian.PutUint32(plen[:], uint32(len(payload)))
	if _, err := w.Write(plen[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// ReadDatagram reads one framed datagram written by WriteDatagram.
func ReadDatagram(r io.Reader) (target string, payload []byte, err error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return "", nil, err
	}
	tlen := binary.BigEndian.Uint16(hdr[:])
	if tlen == 0 {
		return "", nil, ErrEmptyTarget
	}
	tbuf := make([]byte, tlen)
	if _, err := io.ReadFull(r, tbuf); err != nil {
		return "", nil, err
	}
	var plen [4]byte
	if _, err := io.ReadFull(r, plen[:]); err != nil {
		return "", nil, err
	}
	n := binary.BigEndian.Uint32(plen[:])
	if n > maxDatagramLen {
		return "", nil, ErrDatagramTooLarge
	}
	pbuf := make([]byte, n)
	if _, err := io.ReadFull(r, pbuf); err != nil {
		return "", nil, err
	}
	return string(tbuf), pbuf, nil
}
