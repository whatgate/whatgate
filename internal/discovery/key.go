package discovery

import (
	"encoding/base64"
	"fmt"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// EncodePublicKey serializes a public key to the base64 string used to pin a
// control-plane key (e.g. a coordinator's -coordinator-key flag value).
func EncodePublicKey(pub crypto.PubKey) string {
	b, err := crypto.MarshalPublicKey(pub)
	if err != nil {
		// MarshalPublicKey only fails on an unsupported key type; the keys we
		// generate (Ed25519) always marshal.
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

// DecodePublicKey parses a base64 string produced by EncodePublicKey back into a
// public key, to be pinned as the trusted control-plane key.
func DecodePublicKey(s string) (crypto.PubKey, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("discovery: bad public key encoding: %w", err)
	}
	pub, err := crypto.UnmarshalPublicKey(b)
	if err != nil {
		return nil, fmt.Errorf("discovery: unmarshal public key: %w", err)
	}
	return pub, nil
}
