// Package authn proves that a request genuinely comes from the owner of a
// libp2p peer ID. A WhatGate peer signs a canonical message ("action + peerID +
// timestamp") with its private key; the coordinator verifies that (1) the peer
// ID is derived from the presented public key, (2) the signature is valid, and
// (3) the timestamp is fresh. This stops one node from registering or joining
// under another node's identity.
package authn

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// SignedAuth is the authentication envelope attached to a request.
type SignedAuth struct {
	PeerID    string `json:"peerID"`
	PubKey    string `json:"pubKey"` // base64 of the marshaled libp2p public key
	Timestamp int64  `json:"ts"`     // unix seconds
	Signature string `json:"sig"`    // base64 signature over the canonical message
}

// canonical builds the exact bytes that are signed and verified.
func canonical(action, peerID string, ts int64) []byte {
	return []byte(fmt.Sprintf("whatgate-authn/v1|%s|%s|%d", action, peerID, ts))
}

// Sign produces a SignedAuth for the given action at time ts, using priv.
func Sign(priv crypto.PrivKey, action string, ts time.Time) (SignedAuth, error) {
	pub := priv.GetPublic()
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return SignedAuth{}, err
	}
	pkBytes, err := crypto.MarshalPublicKey(pub)
	if err != nil {
		return SignedAuth{}, err
	}
	sig, err := priv.Sign(canonical(action, id.String(), ts.Unix()))
	if err != nil {
		return SignedAuth{}, err
	}
	return SignedAuth{
		PeerID:    id.String(),
		PubKey:    base64.StdEncoding.EncodeToString(pkBytes),
		Timestamp: ts.Unix(),
		Signature: base64.StdEncoding.EncodeToString(sig),
	}, nil
}

// Verify checks that a is a valid, fresh signature for action: the public key
// must derive to the claimed peer ID, the signature must be valid, and the
// timestamp must be within maxSkew of now.
func Verify(a SignedAuth, action string, now time.Time, maxSkew time.Duration) error {
	pkBytes, err := base64.StdEncoding.DecodeString(a.PubKey)
	if err != nil {
		return fmt.Errorf("authn: bad public key: %w", err)
	}
	pub, err := crypto.UnmarshalPublicKey(pkBytes)
	if err != nil {
		return fmt.Errorf("authn: unmarshal public key: %w", err)
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return err
	}
	if id.String() != a.PeerID {
		return errors.New("authn: peer ID does not match public key")
	}

	skew := now.Sub(time.Unix(a.Timestamp, 0))
	if skew < 0 {
		skew = -skew
	}
	if skew > maxSkew {
		return errors.New("authn: timestamp outside allowed window")
	}

	sig, err := base64.StdEncoding.DecodeString(a.Signature)
	if err != nil {
		return fmt.Errorf("authn: bad signature: %w", err)
	}
	ok, err := pub.Verify(canonical(action, a.PeerID, a.Timestamp), sig)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("authn: signature verification failed")
	}
	return nil
}
