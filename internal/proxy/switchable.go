package proxy

import (
	"context"
	"errors"
	"net"
	"sync"
)

// SwitchableDialer is a Dialer whose underlying target can be swapped at runtime.
// The SOCKS ingress holds one of these so a client can change which exit it
// tunnels through (e.g. switch region) without restarting: new connections use
// whatever dialer was most recently Set.
type SwitchableDialer struct {
	mu  sync.RWMutex
	cur Dialer
}

// Set replaces the current dialer.
func (s *SwitchableDialer) Set(d Dialer) {
	s.mu.Lock()
	s.cur = d
	s.mu.Unlock()
}

// Dial delegates to the current dialer.
func (s *SwitchableDialer) Dial(ctx context.Context, addr string) (net.Conn, error) {
	s.mu.RLock()
	d := s.cur
	s.mu.RUnlock()
	if d == nil {
		return nil, errors.New("proxy: no dialer set")
	}
	return d.Dial(ctx, addr)
}

// compile-time check that SwitchableDialer satisfies Dialer.
var _ Dialer = (*SwitchableDialer)(nil)
