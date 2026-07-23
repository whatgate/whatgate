// Package protocol defines the wire messages exchanged over WhatGate streams.
//
// When a client opens a tunnel stream to an exit node, the first thing it sends
// is the target address it wants the exit to dial. This package encodes and
// decodes that target as a length-prefixed UTF-8 string so both ends agree on
// where the framing ends and the tunneled payload begins.
package protocol

import (
	"encoding/binary"
	"errors"
	"io"
)

// maxTargetLen bounds the encoded target address. A "host:port" destination is
// far smaller than this; the cap just protects the reader from a bogus length.
const maxTargetLen = 1 << 16 // exclusive upper bound (fits a uint16 prefix)

// ErrEmptyTarget is returned when a zero-length target is read or written.
var ErrEmptyTarget = errors.New("protocol: empty target address")

// WriteTarget writes addr to w as a uint16 length prefix (big-endian) followed
// by the UTF-8 bytes of addr.
func WriteTarget(w io.Writer, addr string) error {
	if len(addr) == 0 {
		return ErrEmptyTarget
	}
	if len(addr) >= maxTargetLen {
		return errors.New("protocol: target address too long")
	}
	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(len(addr)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := io.WriteString(w, addr)
	return err
}

// ReadTarget reads a length-prefixed target address written by WriteTarget.
func ReadTarget(r io.Reader) (string, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return "", err
	}
	n := binary.BigEndian.Uint16(hdr[:])
	if n == 0 {
		return "", ErrEmptyTarget
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}
