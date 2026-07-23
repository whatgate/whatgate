//go:build tun

package tun

import "github.com/xjasonlyu/tun2socks/v2/engine"

// Start brings up the TUN device and starts forwarding all captured traffic to
// the configured SOCKS5 proxy. The netstack runs in background goroutines, so
// Start returns once setup completes. Note: the underlying engine terminates the
// process on a fatal setup error (e.g. missing privileges or wintun.dll).
func Start(cfg Config) error {
	mtu := cfg.MTU
	if mtu == 0 {
		mtu = 1500
	}
	engine.Insert(&engine.Key{
		Device:   cfg.Device,
		Proxy:    "socks5://" + cfg.SocksAddr,
		MTU:      mtu,
		LogLevel: "warn",
	})
	engine.Start()
	return nil
}

// Stop tears down the TUN device and netstack.
func Stop() {
	engine.Stop()
}
