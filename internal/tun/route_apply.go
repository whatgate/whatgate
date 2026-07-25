package tun

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
)

// Run executes one planned command, returning its combined output on failure.
func (c Command) Run() error {
	out, err := exec.Command(c.Name, c.Args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v: %s",
			c.Name, strings.Join(c.Args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ApplyUp plans and runs the routing setup for cfg. On the first failure it
// stops and returns the error; the caller should ApplyDown to clean up whatever
// did apply. Requires administrator/root privileges.
func ApplyUp(cfg RouteConfig) error {
	cmds, err := PlanUp(cfg)
	if err != nil {
		return err
	}
	for _, c := range cmds {
		if err := c.Run(); err != nil {
			return err
		}
	}
	return nil
}

// ApplyDown plans and runs the teardown, continuing past individual failures so
// a partial setup is cleaned up as far as possible. It returns the first error
// seen (if any) for logging.
func ApplyDown(cfg RouteConfig) error {
	cmds, err := PlanDown(cfg)
	if err != nil {
		return err
	}
	var firstErr error
	for _, c := range cmds {
		if err := c.Run(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// DefaultGateway discovers the host's physical default gateway so self traffic
// can be pinned to it. Best-effort and OS-specific; returns an error if it
// can't be parsed.
func DefaultGateway() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return gatewayFromOutput(run("route", "print", "0.0.0.0"), "windows")
	case "linux":
		return gatewayFromOutput(run("ip", "route", "show", "default"), "linux")
	case "darwin":
		return gatewayFromOutput(run("route", "-n", "get", "default"), "darwin")
	default:
		return "", fmt.Errorf("tun route: default gateway lookup unsupported on %s", runtime.GOOS)
	}
}

func run(name string, args ...string) string {
	out, _ := exec.Command(name, args...).CombinedOutput()
	return string(out)
}

// gatewayFromOutput extracts the gateway IP from the platform's route dump. It
// is split out from DefaultGateway so the parsing can be unit-tested with
// captured fixtures.
func gatewayFromOutput(out, os string) (string, error) {
	switch os {
	case "windows":
		// "          0.0.0.0          0.0.0.0      192.168.1.1     192.168.1.5     25"
		for _, line := range strings.Split(out, "\n") {
			f := strings.Fields(line)
			if len(f) >= 3 && f[0] == "0.0.0.0" && f[1] == "0.0.0.0" {
				if ip := net.ParseIP(f[2]); ip != nil {
					return f[2], nil
				}
			}
		}
	case "linux":
		// "default via 192.168.1.1 dev eth0 proto dhcp metric 100"
		f := strings.Fields(out)
		for i := 0; i+1 < len(f); i++ {
			if f[i] == "via" {
				if ip := net.ParseIP(f[i+1]); ip != nil {
					return f[i+1], nil
				}
			}
		}
	case "darwin":
		// "    gateway: 192.168.1.1"
		for _, line := range strings.Split(out, "\n") {
			f := strings.Fields(line)
			if len(f) == 2 && f[0] == "gateway:" {
				if ip := net.ParseIP(f[1]); ip != nil {
					return f[1], nil
				}
			}
		}
	}
	return "", fmt.Errorf("tun route: could not parse default gateway from %s route output", os)
}

// HostIPs resolves each "host" or "host:port" reference to its IP strings,
// skipping anything that doesn't resolve. Used to turn the coordinator URL and
// exit/relay peer addresses into concrete exclude IPs.
func HostIPs(refs ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, ref := range refs {
		host := ref
		if h, _, err := net.SplitHostPort(ref); err == nil {
			host = h
		}
		if host == "" {
			continue
		}
		if ip := net.ParseIP(host); ip != nil {
			if !seen[host] {
				seen[host] = true
				out = append(out, host)
			}
			continue
		}
		addrs, err := net.LookupHost(host)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if net.ParseIP(a) != nil && !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
		}
	}
	return out
}
