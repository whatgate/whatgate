package tun

import (
	"strings"
	"testing"
)

// joinAll renders commands as "name arg arg\n..." for easy substring assertions.
func joinAll(cmds []Command) string {
	var b strings.Builder
	for _, c := range cmds {
		b.WriteString(c.Name)
		for _, a := range c.Args {
			b.WriteByte(' ')
			b.WriteString(a)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func mustContain(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("commands missing %q\n--- got ---\n%s", want, got)
	}
}

func baseCfg(os string) RouteConfig {
	return RouteConfig{
		OS:       os,
		Device:   "whatgate0",
		TUNAddr:  "10.6.7.1",
		Prefix:   24,
		Gateway:  "192.168.1.1",
		Excludes: []string{"203.0.113.9", "198.51.100.7"},
	}
}

func TestPlanUpWindows(t *testing.T) {
	cmds, err := PlanUp(baseCfg("windows"))
	if err != nil {
		t.Fatalf("PlanUp: %v", err)
	}
	got := joinAll(cmds)
	// Assigns the TUN IP.
	mustContain(t, got, "10.6.7.1")
	// Default route captured via two /1 halves (overrides without deleting OS default).
	mustContain(t, got, "0.0.0.0 mask 128.0.0.0 10.6.7.1")
	mustContain(t, got, "128.0.0.0 mask 128.0.0.0 10.6.7.1")
	// Self traffic (coordinator + exit peers) pinned to the physical gateway.
	mustContain(t, got, "203.0.113.9 mask 255.255.255.255 192.168.1.1")
	mustContain(t, got, "198.51.100.7 mask 255.255.255.255 192.168.1.1")
}

func TestPlanUpLinux(t *testing.T) {
	cmds, err := PlanUp(baseCfg("linux"))
	if err != nil {
		t.Fatalf("PlanUp: %v", err)
	}
	got := joinAll(cmds)
	mustContain(t, got, "addr add 10.6.7.1/24 dev whatgate0")
	mustContain(t, got, "link set whatgate0 up")
	mustContain(t, got, "route add 0.0.0.0/1 dev whatgate0")
	mustContain(t, got, "route add 128.0.0.0/1 dev whatgate0")
	mustContain(t, got, "route add 203.0.113.9/32 via 192.168.1.1")
	mustContain(t, got, "route add 198.51.100.7/32 via 192.168.1.1")
}

func TestPlanUpDarwin(t *testing.T) {
	cmds, err := PlanUp(baseCfg("darwin"))
	if err != nil {
		t.Fatalf("PlanUp: %v", err)
	}
	got := joinAll(cmds)
	mustContain(t, got, "whatgate0 10.6.7.1")
	mustContain(t, got, "-net 0.0.0.0/1 10.6.7.1")
	mustContain(t, got, "-net 128.0.0.0/1 10.6.7.1")
	mustContain(t, got, "-host 203.0.113.9 192.168.1.1")
	mustContain(t, got, "-host 198.51.100.7 192.168.1.1")
}

// PlanDown reverses the exclusions and the captured default so the box is left
// as it was found.
func TestPlanDownWindowsRemovesRoutes(t *testing.T) {
	cmds, err := PlanDown(baseCfg("windows"))
	if err != nil {
		t.Fatalf("PlanDown: %v", err)
	}
	got := joinAll(cmds)
	mustContain(t, got, "delete 0.0.0.0 mask 128.0.0.0")
	mustContain(t, got, "delete 128.0.0.0 mask 128.0.0.0")
	mustContain(t, got, "delete 203.0.113.9")
	mustContain(t, got, "delete 198.51.100.7")
}

func TestPlanDownLinuxRemovesRoutes(t *testing.T) {
	cmds, err := PlanDown(baseCfg("linux"))
	if err != nil {
		t.Fatalf("PlanDown: %v", err)
	}
	got := joinAll(cmds)
	mustContain(t, got, "route del 0.0.0.0/1")
	mustContain(t, got, "route del 128.0.0.0/1")
	mustContain(t, got, "route del 203.0.113.9/32")
	mustContain(t, got, "route del 198.51.100.7/32")
}

// A missing gateway is only a problem when there are excludes to pin to it.
func TestPlanUpRequiresGatewayForExcludes(t *testing.T) {
	cfg := baseCfg("linux")
	cfg.Gateway = ""
	if _, err := PlanUp(cfg); err == nil {
		t.Fatal("expected error when excludes given without a gateway")
	}

	cfg.Excludes = nil
	if _, err := PlanUp(cfg); err != nil {
		t.Fatalf("no excludes should not need a gateway: %v", err)
	}
}

func TestPlanUpValidatesRequiredFields(t *testing.T) {
	cfg := baseCfg("linux")
	cfg.TUNAddr = ""
	if _, err := PlanUp(cfg); err == nil {
		t.Fatal("expected error when TUNAddr is empty")
	}

	cfg = baseCfg("linux")
	cfg.Device = ""
	if _, err := PlanUp(cfg); err == nil {
		t.Fatal("expected error when Device is empty")
	}
}

func TestPlanUpUnsupportedOS(t *testing.T) {
	if _, err := PlanUp(baseCfg("plan9")); err == nil {
		t.Fatal("expected error for unsupported OS")
	}
}

// Bad exclude IPs must be rejected, not blindly shelled out.
func TestPlanUpRejectsBadExcludeIP(t *testing.T) {
	cfg := baseCfg("linux")
	cfg.Excludes = []string{"not-an-ip"}
	if _, err := PlanUp(cfg); err == nil {
		t.Fatal("expected error for malformed exclude IP")
	}
}
