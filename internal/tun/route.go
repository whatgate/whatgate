package tun

import (
	"fmt"
	"net"
	"runtime"
	"strconv"
)

// Command is a single OS command the router runs to change the routing table.
// Keeping the plan as data (rather than shelling out inline) lets the
// route-planning logic be unit-tested without touching the real system.
type Command struct {
	Name string
	Args []string
}

// RouteConfig describes the desired routing for TUN mode.
type RouteConfig struct {
	// OS selects the command syntax ("windows"/"linux"/"darwin"); empty uses
	// the current runtime.GOOS.
	OS string
	// Device is the TUN interface name (e.g. "whatgate0").
	Device string
	// TUNAddr is the IP assigned to the TUN interface (e.g. "10.6.7.1").
	TUNAddr string
	// Prefix is the TUN subnet prefix length; 0 defaults to 24.
	Prefix int
	// Gateway is the host's physical default gateway. Required only when
	// Excludes is non-empty (self traffic is pinned to it).
	Gateway string
	// Excludes are IPs whose traffic must bypass the TUN and keep using the
	// physical gateway — the node's own control-plane (coordinator) and
	// data-plane (exit/relay peer) connections. Without these, TUN would
	// capture the tunnel's own packets and loop.
	Excludes []string
}

func (c RouteConfig) osName() string {
	if c.OS != "" {
		return c.OS
	}
	return runtime.GOOS
}

func (c RouteConfig) prefix() int {
	if c.Prefix == 0 {
		return 24
	}
	return c.Prefix
}

// validate checks the fields common to every platform.
func (c RouteConfig) validate() error {
	if c.Device == "" {
		return fmt.Errorf("tun route: Device is required")
	}
	if c.TUNAddr == "" {
		return fmt.Errorf("tun route: TUNAddr is required")
	}
	if net.ParseIP(c.TUNAddr) == nil {
		return fmt.Errorf("tun route: TUNAddr %q is not a valid IP", c.TUNAddr)
	}
	for _, ex := range c.Excludes {
		if net.ParseIP(ex) == nil {
			return fmt.Errorf("tun route: exclude %q is not a valid IP", ex)
		}
	}
	if len(c.Excludes) > 0 && c.Gateway == "" {
		return fmt.Errorf("tun route: Gateway is required to exclude self traffic")
	}
	return nil
}

// PlanUp returns the ordered commands that assign the TUN address, redirect the
// default route into the TUN, and pin self traffic to the physical gateway.
// It runs nothing — the caller executes the plan.
func PlanUp(cfg RouteConfig) ([]Command, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	switch cfg.osName() {
	case "windows":
		return planUpWindows(cfg), nil
	case "linux":
		return planUpLinux(cfg), nil
	case "darwin":
		return planUpDarwin(cfg), nil
	default:
		return nil, fmt.Errorf("tun route: unsupported OS %q", cfg.osName())
	}
}

// PlanDown returns the commands that undo PlanUp, restoring the host's routing.
func PlanDown(cfg RouteConfig) ([]Command, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	switch cfg.osName() {
	case "windows":
		return planDownWindows(cfg), nil
	case "linux":
		return planDownLinux(cfg), nil
	case "darwin":
		return planDownDarwin(cfg), nil
	default:
		return nil, fmt.Errorf("tun route: unsupported OS %q", cfg.osName())
	}
}

// The default route is captured as two /1 halves (0.0.0.0/1 and 128.0.0.0/1)
// rather than by replacing 0.0.0.0/0. The halves are more specific than the OS
// default, so they win without deleting it — which makes teardown a clean
// delete of exactly what we added.

func planUpWindows(cfg RouteConfig) []Command {
	cmds := []Command{
		{"netsh", []string{"interface", "ip", "set", "address",
			"name=" + cfg.Device, "static", cfg.TUNAddr, maskFor(cfg.prefix())}},
		{"route", []string{"add", "0.0.0.0", "mask", "128.0.0.0", cfg.TUNAddr, "metric", "1"}},
		{"route", []string{"add", "128.0.0.0", "mask", "128.0.0.0", cfg.TUNAddr, "metric", "1"}},
	}
	for _, ex := range cfg.Excludes {
		cmds = append(cmds, Command{"route", []string{"add", ex,
			"mask", "255.255.255.255", cfg.Gateway, "metric", "1"}})
	}
	return cmds
}

func planDownWindows(cfg RouteConfig) []Command {
	cmds := []Command{
		{"route", []string{"delete", "0.0.0.0", "mask", "128.0.0.0"}},
		{"route", []string{"delete", "128.0.0.0", "mask", "128.0.0.0"}},
	}
	for _, ex := range cfg.Excludes {
		cmds = append(cmds, Command{"route", []string{"delete", ex}})
	}
	return cmds
}

func planUpLinux(cfg RouteConfig) []Command {
	cidr := cfg.TUNAddr + "/" + strconv.Itoa(cfg.prefix())
	cmds := []Command{
		{"ip", []string{"addr", "add", cidr, "dev", cfg.Device}},
		{"ip", []string{"link", "set", cfg.Device, "up"}},
		{"ip", []string{"route", "add", "0.0.0.0/1", "dev", cfg.Device}},
		{"ip", []string{"route", "add", "128.0.0.0/1", "dev", cfg.Device}},
	}
	for _, ex := range cfg.Excludes {
		cmds = append(cmds, Command{"ip", []string{"route", "add",
			ex + "/32", "via", cfg.Gateway}})
	}
	return cmds
}

func planDownLinux(cfg RouteConfig) []Command {
	cmds := []Command{
		{"ip", []string{"route", "del", "0.0.0.0/1"}},
		{"ip", []string{"route", "del", "128.0.0.0/1"}},
	}
	for _, ex := range cfg.Excludes {
		cmds = append(cmds, Command{"ip", []string{"route", "del", ex + "/32"}})
	}
	return cmds
}

func planUpDarwin(cfg RouteConfig) []Command {
	cmds := []Command{
		{"ifconfig", []string{cfg.Device, cfg.TUNAddr, cfg.TUNAddr, "up"}},
		{"route", []string{"add", "-net", "0.0.0.0/1", cfg.TUNAddr}},
		{"route", []string{"add", "-net", "128.0.0.0/1", cfg.TUNAddr}},
	}
	for _, ex := range cfg.Excludes {
		cmds = append(cmds, Command{"route", []string{"add", "-host", ex, cfg.Gateway}})
	}
	return cmds
}

func planDownDarwin(cfg RouteConfig) []Command {
	cmds := []Command{
		{"route", []string{"delete", "-net", "0.0.0.0/1", cfg.TUNAddr}},
		{"route", []string{"delete", "-net", "128.0.0.0/1", cfg.TUNAddr}},
	}
	for _, ex := range cfg.Excludes {
		cmds = append(cmds, Command{"route", []string{"delete", "-host", ex}})
	}
	return cmds
}

// maskFor renders a prefix length as a dotted IPv4 netmask (Windows `route`
// wants the mask form, not a prefix).
func maskFor(prefix int) string {
	m := net.CIDRMask(prefix, 32)
	return fmt.Sprintf("%d.%d.%d.%d", m[0], m[1], m[2], m[3])
}
