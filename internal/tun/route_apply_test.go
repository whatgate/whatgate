package tun

import (
	"strings"
	"testing"
)

func TestGatewayFromOutputWindows(t *testing.T) {
	out := `
===========================================================================
Active Routes:
Network Destination        Netmask          Gateway       Interface  Metric
          0.0.0.0          0.0.0.0      192.168.1.1     192.168.1.5     25
        127.0.0.0        255.0.0.0         On-link       127.0.0.1    331
===========================================================================
`
	gw, err := gatewayFromOutput(out, "windows")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if gw != "192.168.1.1" {
		t.Fatalf("gateway = %q, want 192.168.1.1", gw)
	}
}

func TestGatewayFromOutputLinux(t *testing.T) {
	out := "default via 10.0.0.1 dev eth0 proto dhcp metric 100\n"
	gw, err := gatewayFromOutput(out, "linux")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if gw != "10.0.0.1" {
		t.Fatalf("gateway = %q, want 10.0.0.1", gw)
	}
}

func TestGatewayFromOutputDarwin(t *testing.T) {
	out := "   route to: default\ndestination: default\n   gateway: 172.16.0.1\n interface: en0\n"
	gw, err := gatewayFromOutput(out, "darwin")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if gw != "172.16.0.1" {
		t.Fatalf("gateway = %q, want 172.16.0.1", gw)
	}
}

func TestGatewayFromOutputNoDefault(t *testing.T) {
	if _, err := gatewayFromOutput("nothing useful here", "linux"); err == nil {
		t.Fatal("expected error when no default route present")
	}
}

func TestHostIPsPassesThroughLiteralsAndDedups(t *testing.T) {
	got := HostIPs("203.0.113.9:4001", "203.0.113.9", "198.51.100.7")
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 unique IPs", got)
	}
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "203.0.113.9") || !strings.Contains(joined, "198.51.100.7") {
		t.Fatalf("missing expected IPs: %v", got)
	}
}

func TestHostIPsSkipsUnresolvable(t *testing.T) {
	// A syntactically-hostname-like string that won't resolve is dropped, not
	// returned as a bogus exclude.
	got := HostIPs("this-host-does-not-exist.invalid:80")
	if len(got) != 0 {
		t.Fatalf("expected no IPs for unresolvable host, got %v", got)
	}
}
