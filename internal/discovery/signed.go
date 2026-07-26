// Package discovery authenticates the coordinator's *responses*, not just
// clients' requests. A node's request signature (see internal/authn) proves who
// is asking; it says nothing about whether the directory, relay, or bootstrap
// data a node receives actually came from the trusted control plane. Without
// response authentication, any reachable-but-rogue endpoint (a MITM, a
// selectively-unblocked malicious mirror) can feed a node a poisoned directory
// and steer it onto a monitored or hostile exit.
//
// A Signed object is a domain-separated, pinned-key envelope: the control plane
// signs each response with its long-term key, and a node verifies against a key
// it pins out of band. The envelope carries the fields needed to freeze the
// wire format now (type, monotonic serial, issue/expiry, revocation epoch) so
// later work — anti-rollback caching (A3), revocation, out-of-band bootstrap
// (C2) — reuses the same signed schema instead of inventing new ones.
package discovery

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// Object types. The type is part of the signed canonical message, so an object
// signed for one purpose can never be replayed as another (domain separation).
const (
	TypeDirectory  = "directory"
	TypeRelay      = "relay"
	TypeBootstrap  = "bootstrap"
	TypeRevocation = "revocation"
	// C1 membership credential chain (see docs/c1-decentralized-discovery.md §15):
	// an offline root signs an issuer cert authorizing an online issuer key to
	// grant member certs of certain roles; that issuer signs member certs.
	TypeIssuerCert = "issuer-cert"
	TypeMemberCert = "member-cert"
	// TypeRevocationCheckpoint is a root-signed, monotonically-versioned snapshot
	// of revoked subjects/issuers with a freshness window (C1.2). It supersedes
	// the placeholder TypeRevocation.
	TypeRevocationCheckpoint = "revocation-checkpoint"
)

// Meta is the signed metadata wrapping a payload. All fields are covered by the
// signature.
type Meta struct {
	Type            string
	Serial          uint64 // monotonic per issuer; enables anti-rollback
	IssuedAt        time.Time
	ExpiresAt       time.Time
	RevocationEpoch uint64 // carried now; enforced when revocation lands
}

// Signed is the wire envelope: metadata + opaque payload + the signer's public
// key and signature.
type Signed struct {
	Type            string          `json:"type"`
	Serial          uint64          `json:"serial"`
	IssuedAt        int64           `json:"issuedAt"`  // unix seconds
	ExpiresAt       int64           `json:"expiresAt"` // unix seconds
	RevocationEpoch uint64          `json:"revocationEpoch"`
	Payload         json.RawMessage `json:"payload"`
	PubKey          string          `json:"pubKey"` // base64 of the marshaled libp2p public key
	Signature       string          `json:"sig"`    // base64 signature over the canonical message
}

// canonical builds the exact bytes that are signed and verified. The payload is
// bound by its SHA-256 so the canonical string stays fixed-size regardless of
// payload length.
func canonical(typ string, serial uint64, issued, expires int64, epoch uint64, payload []byte) []byte {
	h := sha256.Sum256(payload)
	return []byte(fmt.Sprintf("whatgate-disco/v1|%s|%d|%d|%d|%d|%s",
		typ, serial, issued, expires, epoch, base64.StdEncoding.EncodeToString(h[:])))
}

// Sign wraps payload into a Signed object signed by priv.
func Sign(priv crypto.PrivKey, m Meta, payload []byte) (Signed, error) {
	pkBytes, err := crypto.MarshalPublicKey(priv.GetPublic())
	if err != nil {
		return Signed{}, err
	}
	issued := m.IssuedAt.Unix()
	expires := m.ExpiresAt.Unix()
	sig, err := priv.Sign(canonical(m.Type, m.Serial, issued, expires, m.RevocationEpoch, payload))
	if err != nil {
		return Signed{}, err
	}
	return Signed{
		Type:            m.Type,
		Serial:          m.Serial,
		IssuedAt:        issued,
		ExpiresAt:       expires,
		RevocationEpoch: m.RevocationEpoch,
		Payload:         append(json.RawMessage(nil), payload...),
		PubKey:          base64.StdEncoding.EncodeToString(pkBytes),
		Signature:       base64.StdEncoding.EncodeToString(sig),
	}, nil
}

// Verify checks that s was signed by the pinned public key expected, is of the
// given type, has not expired at now, and carries a serial at or above floor
// (anti-rollback: floor is the highest serial the caller has already accepted).
// On success it returns the verified payload.
func (s Signed) Verify(expected crypto.PubKey, typ string, now time.Time, floor uint64) ([]byte, error) {
	if s.Type != typ {
		return nil, fmt.Errorf("discovery: type %q, want %q", s.Type, typ)
	}
	if s.Serial < floor {
		return nil, errors.New("discovery: serial below rollback floor")
	}
	if now.After(time.Unix(s.ExpiresAt, 0)) {
		return nil, errors.New("discovery: object expired")
	}

	// Pinning: the embedded key must be the one we trust out of band, and the
	// signature must verify under it. Verifying against the pinned key (not the
	// embedded one alone) is what stops a forged object from a different key.
	pkBytes, err := base64.StdEncoding.DecodeString(s.PubKey)
	if err != nil {
		return nil, fmt.Errorf("discovery: bad public key: %w", err)
	}
	pub, err := crypto.UnmarshalPublicKey(pkBytes)
	if err != nil {
		return nil, fmt.Errorf("discovery: unmarshal public key: %w", err)
	}
	if !pub.Equals(expected) {
		return nil, errors.New("discovery: object not signed by the pinned key")
	}

	sig, err := base64.StdEncoding.DecodeString(s.Signature)
	if err != nil {
		return nil, fmt.Errorf("discovery: bad signature: %w", err)
	}
	ok, err := expected.Verify(canonical(s.Type, s.Serial, s.IssuedAt, s.ExpiresAt, s.RevocationEpoch, s.Payload), sig)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("discovery: signature verification failed")
	}
	return s.Payload, nil
}
