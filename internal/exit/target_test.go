package exit

import (
	"net"
	"testing"
)

func TestDisallowedTargetIP(t *testing.T) {
	cases := []struct {
		ip         string
		disallowed bool
	}{
		// Public — allowed.
		{"1.1.1.1", false},
		{"8.8.8.8", false},
		{"93.184.216.34", false}, // example.com
		{"2606:2800:220:1:248:1893:25c8:1946", false},
		// Loopback.
		{"127.0.0.1", true},
		{"127.0.0.53", true},
		{"::1", true},
		// Private RFC1918.
		{"10.0.0.5", true},
		{"172.16.5.4", true},
		{"192.168.1.1", true},
		// IPv6 ULA.
		{"fd00::1", true},
		// Link-local incl. cloud metadata.
		{"169.254.169.254", true},
		{"169.254.1.1", true},
		{"fe80::1", true},
		// Unspecified / multicast.
		{"0.0.0.0", true},
		{"::", true},
		{"224.0.0.1", true},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("bad test IP %q", c.ip)
		}
		if got := DisallowedTargetIP(ip); got != c.disallowed {
			t.Errorf("DisallowedTargetIP(%s) = %v, want %v", c.ip, got, c.disallowed)
		}
	}
	if !DisallowedTargetIP(nil) {
		t.Error("nil IP should be disallowed")
	}
}

func TestCheckDialAddress(t *testing.T) {
	// Blocked when allowPrivate is false.
	if err := checkDialAddress("127.0.0.1:8080", false); err != ErrBlockedPrivateTarget {
		t.Errorf("loopback: err = %v, want ErrBlockedPrivateTarget", err)
	}
	if err := checkDialAddress("169.254.169.254:80", false); err != ErrBlockedPrivateTarget {
		t.Errorf("metadata: err = %v, want ErrBlockedPrivateTarget", err)
	}
	// Public passes.
	if err := checkDialAddress("8.8.8.8:53", false); err != nil {
		t.Errorf("public: err = %v, want nil", err)
	}
	// allowPrivate bypasses.
	if err := checkDialAddress("127.0.0.1:8080", true); err != nil {
		t.Errorf("allowPrivate: err = %v, want nil", err)
	}
}
