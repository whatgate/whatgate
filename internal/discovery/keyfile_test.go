package discovery

import (
	"path/filepath"
	"testing"
)

// LoadOrCreateSigningKey generates and persists a key on first call, then loads
// the same key on subsequent calls — so a coordinator's signing identity is
// stable across restarts.
func TestLoadOrCreateSigningKeyIsStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signing.key")

	k1, err := LoadOrCreateSigningKey(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	k2, err := LoadOrCreateSigningKey(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !k1.GetPublic().Equals(k2.GetPublic()) {
		t.Fatal("second load produced a different key")
	}
}
