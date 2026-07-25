package discovery

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// LoadOrCreateSigningKey loads the coordinator's control-plane signing key from
// path, generating and persisting a new Ed25519 key the first time. The key is
// stored base64-encoded with owner-only permissions.
func LoadOrCreateSigningKey(path string) (crypto.PrivKey, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if err != nil {
			return nil, fmt.Errorf("discovery: decode signing key %s: %w", path, err)
		}
		priv, err := crypto.UnmarshalPrivateKey(raw)
		if err != nil {
			return nil, fmt.Errorf("discovery: unmarshal signing key %s: %w", path, err)
		}
		return priv, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("discovery: read signing key %s: %w", path, err)
	}

	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	raw, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	enc := base64.StdEncoding.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(enc+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("discovery: write signing key %s: %w", path, err)
	}
	return priv, nil
}
