//go:build !tun

package tun

import "errors"

// ErrNotBuilt is returned when TUN mode is requested from a binary compiled
// without the "tun" build tag.
var ErrNotBuilt = errors.New("tun: this binary was built without TUN support (rebuild with: go build -tags tun ./cmd/node)")

// Start reports that TUN support is not compiled in.
func Start(Config) error { return ErrNotBuilt }

// Stop is a no-op in builds without TUN support.
func Stop() {}
