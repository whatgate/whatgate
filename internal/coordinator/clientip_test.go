package coordinator

import (
	"net/http"
	"testing"
)

func reqWith(remoteAddr, xff string) *http.Request {
	r := &http.Request{RemoteAddr: remoteAddr, Header: make(http.Header)}
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	return r
}

// With no trusted proxies configured, the socket peer IP is the key and any
// X-Forwarded-For header is ignored (it is attacker-spoofable).
func TestClientIPNoTrustedProxyIgnoresXFF(t *testing.T) {
	s := &Server{}
	if got := s.clientIP(reqWith("198.51.100.7:443", "1.2.3.4")); got != "198.51.100.7" {
		t.Fatalf("clientIP = %q, want 198.51.100.7 (XFF must be ignored)", got)
	}
}

// A request arriving directly from a non-trusted peer keeps that peer's IP even
// if it forges an X-Forwarded-For.
func TestClientIPUntrustedDirectPeerIgnoresXFF(t *testing.T) {
	s := &Server{}
	if err := s.SetTrustedProxies([]string{"10.0.0.0/8"}); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}
	if got := s.clientIP(reqWith("198.51.100.7:443", "1.2.3.4")); got != "198.51.100.7" {
		t.Fatalf("clientIP = %q, want 198.51.100.7 (spoofed XFF from untrusted peer ignored)", got)
	}
}

// When the direct peer is a trusted proxy, the real client is taken from XFF.
func TestClientIPTrustedProxyUsesXFF(t *testing.T) {
	s := &Server{}
	if err := s.SetTrustedProxies([]string{"10.0.0.1"}); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}
	if got := s.clientIP(reqWith("10.0.0.1:443", "203.0.113.9")); got != "203.0.113.9" {
		t.Fatalf("clientIP = %q, want 203.0.113.9", got)
	}
}

// The real client is the rightmost XFF entry that is not itself a trusted proxy,
// so an attacker prepending a spoofed IP cannot impersonate another client.
func TestClientIPStripsTrustedHopsAndIgnoresSpoof(t *testing.T) {
	s := &Server{}
	if err := s.SetTrustedProxies([]string{"10.0.0.0/8"}); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}
	// Direct peer 10.0.0.2 (trusted). The trusted proxy appended the real client
	// (203.0.113.9); the attacker prepended a spoofed 1.1.1.1 and an internal hop.
	got := s.clientIP(reqWith("10.0.0.2:443", "1.1.1.1, 203.0.113.9, 10.0.0.5"))
	if got != "203.0.113.9" {
		t.Fatalf("clientIP = %q, want 203.0.113.9 (rightmost non-trusted, spoof ignored)", got)
	}
}

// If XFF holds only trusted-proxy IPs (misconfig / no client hop), fall back to
// the direct peer rather than returning a proxy as the client.
func TestClientIPAllTrustedFallsBackToPeer(t *testing.T) {
	s := &Server{}
	if err := s.SetTrustedProxies([]string{"10.0.0.0/8"}); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}
	if got := s.clientIP(reqWith("10.0.0.2:443", "10.0.0.9")); got != "10.0.0.2" {
		t.Fatalf("clientIP = %q, want 10.0.0.2 (fallback to peer)", got)
	}
}

func TestSetTrustedProxiesRejectsBadEntry(t *testing.T) {
	s := &Server{}
	if err := s.SetTrustedProxies([]string{"not-an-ip"}); err == nil {
		t.Fatal("expected error for invalid trusted-proxy entry")
	}
}
