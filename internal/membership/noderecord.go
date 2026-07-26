package membership

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/whatgate/whatgate/internal/discovery"
)

// NodeRecord is an exit/relay's short-lived, self-signed discovery record: where
// to reach it and in what capacity (C1.4). It is the payload a querier pulls from
// a candidate over the node-record protocol after finding it on the DHT. The
// record is signed by the node's own identity key; the node's authority to
// advertise its roles is proven by the member cert presented alongside it.
//
// Generation is the anti-rollback key: it is carried in the signed envelope
// (Serial), strictly increases across republications, and lets a querier reject a
// replayed stale advertisement (e.g. an address the node has since abandoned).
type NodeRecord struct {
	V          int      `json:"v"`
	Subject    string   `json:"subject"` // PeerID; must equal the signer and the presented cert's subject
	Roles      []Role   `json:"roles"`
	Addrs      []string `json:"addrs"`
	Region     string   `json:"region"`
	Generation uint64   `json:"-"` // from the signed envelope Serial (signed via the canonical message)
}

// SignNodeRecord signs rec with the node's identity key. generation must
// strictly increase across republications; expires should be short (minutes).
func SignNodeRecord(nodePriv crypto.PrivKey, rec NodeRecord, issued, expires time.Time, generation uint64) ([]byte, error) {
	payload, err := json.Marshal(rec)
	if err != nil {
		return nil, err
	}
	obj, err := discovery.Sign(nodePriv, discovery.Meta{
		Type:      discovery.TypeNodeRecord,
		Serial:    generation,
		IssuedAt:  issued,
		ExpiresAt: expires,
	}, payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(obj)
}

// VerifyNodeRecord verifies a node record for subject: the member cert chain must
// anchor to pinnedRoot and authorize subject; the record must be signed by
// subject's own key, unexpired, and at/above genFloor (anti-rollback); and the
// roles it advertises must be a subset of what its cert grants. On success it
// returns the record with Generation filled from the envelope.
func VerifyNodeRecord(pinnedRoot crypto.PubKey, subject string, recordSigned, memberCert, issuerCert []byte, now time.Time, opts VerifyOpts, genFloor uint64) (NodeRecord, Cert, error) {
	// 1. The presenter's membership + authorized roles.
	cert, err := VerifyMemberCert(pinnedRoot, subject, memberCert, issuerCert, now, opts)
	if err != nil {
		return NodeRecord{}, Cert{}, fmt.Errorf("node record: %w", err)
	}

	// 2. The record must be signed by the subject's own identity key.
	subjID, err := peer.Decode(subject)
	if err != nil {
		return NodeRecord{}, Cert{}, fmt.Errorf("node record: bad subject: %w", err)
	}
	subjPub, err := subjID.ExtractPublicKey()
	if err != nil {
		return NodeRecord{}, Cert{}, fmt.Errorf("node record: subject key: %w", err)
	}
	var env discovery.Signed
	if err := json.Unmarshal(recordSigned, &env); err != nil {
		return NodeRecord{}, Cert{}, fmt.Errorf("node record: not a signed envelope: %w", err)
	}
	payload, err := env.Verify(subjPub, discovery.TypeNodeRecord, now, genFloor)
	if err != nil {
		return NodeRecord{}, Cert{}, fmt.Errorf("node record: %w", err)
	}
	var rec NodeRecord
	if err := json.Unmarshal(payload, &rec); err != nil {
		return NodeRecord{}, Cert{}, fmt.Errorf("node record: bad payload: %w", err)
	}
	rec.Generation = env.Serial

	// 3. Bindings and authorization.
	if rec.Subject != subject {
		return NodeRecord{}, Cert{}, fmt.Errorf("node record: subject %q does not match %q", rec.Subject, subject)
	}
	if !rolesSubsetOf(rec.Roles, cert.Roles) {
		return NodeRecord{}, Cert{}, fmt.Errorf("node record: advertises roles %v beyond cert %v", rec.Roles, cert.Roles)
	}
	return rec, cert, nil
}
