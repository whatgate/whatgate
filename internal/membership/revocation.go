package membership

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/whatgate/whatgate/internal/discovery"
)

// RevocationCheckpoint is a root-signed snapshot of what has been revoked, with
// a freshness window (C1.2, see docs/c1-decentralized-discovery.md §4/§15). It
// closes the revocation loop: a consumer learns which subjects/issuers are
// revoked, and — crucially — whether its own copy is too old to be trusted.
//
// Two freshness tiers keep an offline consumer safe:
//   - Stale (now > ThisUpdate + MaxStalenessSec): the consumer can no longer be
//     sure it would have seen a recent revocation, so it must degrade to an
//     emergency-restricted scope rather than build normal exit tunnels.
//   - Hard expiry (the signed envelope's ExpiresAt): past this the checkpoint is
//     too old to verify at all and is rejected outright.
//
// Version is monotonic: a consumer tracks the highest version it has accepted
// and rejects any lower one, so an attacker cannot replay a stale checkpoint
// that omits a later revocation.
type RevocationCheckpoint struct {
	V               int      `json:"v"`
	Version         uint64   `json:"version"`
	RevEpoch        uint64   `json:"revEpoch"` // current coarse revocation epoch; consumers pass as VerifyOpts.MinRevEpoch
	ThisUpdate      int64    `json:"thisUpdate"`
	NextUpdate      int64    `json:"nextUpdate"`
	MaxStalenessSec int64    `json:"maxStalenessSec"`
	RevokedSubjects []string `json:"revokedSubjects,omitempty"`
	RevokedIssuers  []string `json:"revokedIssuers,omitempty"`
}

// SignRevocationCheckpoint signs cp with the offline root. hardExpiry is the
// envelope's absolute expiry — past it the checkpoint no longer verifies (set it
// generously beyond MaxStalenessSec so a merely-stale checkpoint still verifies
// and can trigger graceful degradation instead of a hard failure).
func SignRevocationCheckpoint(root crypto.PrivKey, cp RevocationCheckpoint, hardExpiry time.Time) ([]byte, error) {
	payload, err := json.Marshal(cp)
	if err != nil {
		return nil, err
	}
	obj, err := discovery.Sign(root, discovery.Meta{
		Type:            discovery.TypeRevocationCheckpoint,
		Serial:          cp.Version,
		IssuedAt:        time.Unix(cp.ThisUpdate, 0),
		ExpiresAt:       hardExpiry,
		RevocationEpoch: cp.RevEpoch,
	}, payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(obj)
}

// VerifyRevocationCheckpoint verifies a checkpoint against the pinned root,
// rejecting a version below floor (anti-rollback) and one past its hard expiry.
func VerifyRevocationCheckpoint(pinnedRoot crypto.PubKey, signed []byte, now time.Time, versionFloor uint64) (RevocationCheckpoint, error) {
	var env discovery.Signed
	if err := json.Unmarshal(signed, &env); err != nil {
		return RevocationCheckpoint{}, fmt.Errorf("checkpoint: not a signed envelope: %w", err)
	}
	payload, err := env.Verify(pinnedRoot, discovery.TypeRevocationCheckpoint, now, versionFloor)
	if err != nil {
		return RevocationCheckpoint{}, fmt.Errorf("checkpoint: %w", err)
	}
	var cp RevocationCheckpoint
	if err := json.Unmarshal(payload, &cp); err != nil {
		return RevocationCheckpoint{}, fmt.Errorf("checkpoint: bad payload: %w", err)
	}
	return cp, nil
}

// Revokes reports whether cert is revoked by this checkpoint — either its
// subject is listed, or its whole issuer has been revoked.
func (cp RevocationCheckpoint) Revokes(cert Cert) bool {
	for _, s := range cp.RevokedSubjects {
		if s == cert.Subject {
			return true
		}
	}
	for _, i := range cp.RevokedIssuers {
		if i == cert.IssuerID {
			return true
		}
	}
	return false
}

// Stale reports whether now is past ThisUpdate + MaxStalenessSec — the point at
// which a consumer must stop trusting this checkpoint as current and degrade to
// an emergency-restricted scope.
func (cp RevocationCheckpoint) Stale(now time.Time) bool {
	return now.Unix() > cp.ThisUpdate+cp.MaxStalenessSec
}
