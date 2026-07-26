package node

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/whatgate/whatgate/internal/membership"
)

// NodeRecordProtocol is the authenticated record-fetch layer of C1's two-layer
// discovery (§15.2): after finding a candidate PeerID on the DHT, a querier pulls
// that candidate's signed node record directly over this protocol — never from a
// DHT provider payload or value. One round trip yields membership + record +
// liveness: the responder returns its record and credential chain and signs the
// querier's nonce, which also serves as the connectivity challenge.
const NodeRecordProtocol = protocol.ID("/whatgate/noderecord/1.0.0")

type nodeRecordResponse struct {
	Record     json.RawMessage `json:"record"`
	MemberCert json.RawMessage `json:"memberCert"`
	IssuerCert json.RawMessage `json:"issuerCert"`
	Sig        []byte          `json:"sig"` // signature over the challenge nonce by the responder's identity key
}

// FetchedRecord is a verified node record plus the cert that authorized it and
// the raw signed record bytes (needed for equivocation fingerprinting).
type FetchedRecord struct {
	Record membership.NodeRecord
	Cert   membership.Cert
	Signed []byte
}

// SetNodeRecord stores this node's signed node record and registers the
// record-fetch responder. The credential chain must already be set via
// SetMemberCredential — it is presented alongside the record to prove the node's
// authority to advertise its roles.
func (n *Node) SetNodeRecord(recordSigned []byte) {
	n.credMu.Lock()
	n.nodeRecord = append([]byte(nil), recordSigned...)
	n.credMu.Unlock()
	n.h.SetStreamHandler(NodeRecordProtocol, n.handleNodeRecord)
}

// SignSelfRecord signs a node record advertising this node's own identity for
// the given roles/region/addrs, valid for ttl, at the given generation.
func (n *Node) SignSelfRecord(roles []membership.Role, region string, addrs []string, ttl time.Duration, generation uint64) ([]byte, error) {
	priv := n.h.Peerstore().PrivKey(n.h.ID())
	if priv == nil {
		return nil, errors.New("noderecord: no identity key to sign with")
	}
	rec := membership.NodeRecord{V: 1, Subject: n.ID().String(), Roles: roles, Addrs: addrs, Region: region}
	return membership.SignNodeRecord(priv, rec, time.Now(), time.Now().Add(ttl), generation)
}

func (n *Node) handleNodeRecord(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(10 * time.Second))

	var ch memberAuthChallenge
	if err := json.NewDecoder(io.LimitReader(s, memberAuthMaxBytes)).Decode(&ch); err != nil {
		return
	}
	priv := n.h.Peerstore().PrivKey(n.h.ID())
	if priv == nil {
		return
	}
	sig, err := priv.Sign(ch.Nonce)
	if err != nil {
		return
	}
	n.credMu.Lock()
	rec, mc, ic := n.nodeRecord, n.memberCert, n.issuerCert
	n.credMu.Unlock()
	_ = json.NewEncoder(s).Encode(nodeRecordResponse{Record: rec, MemberCert: mc, IssuerCert: ic, Sig: sig})
}

// FetchNodeRecord pulls and verifies p's node record over the record-fetch
// protocol: p must sign the fresh nonce with its identity key (liveness +
// key-control), the credential chain must anchor to pinnedRoot and authorize p,
// the record must be signed by p, name p as subject, advertise only authorized
// roles, and carry a generation at/above genFloor. On success it returns the
// verified record.
func (n *Node) FetchNodeRecord(ctx context.Context, p peer.ID, pinnedRoot crypto.PubKey, opts membership.VerifyOpts, genFloor uint64) (FetchedRecord, error) {
	s, err := n.h.NewStream(ctx, p, NodeRecordProtocol)
	if err != nil {
		return FetchedRecord{}, fmt.Errorf("noderecord: open stream: %w", err)
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(10 * time.Second))

	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return FetchedRecord{}, err
	}
	if err := json.NewEncoder(s).Encode(memberAuthChallenge{Nonce: nonce}); err != nil {
		return FetchedRecord{}, fmt.Errorf("noderecord: send challenge: %w", err)
	}

	var resp nodeRecordResponse
	if err := json.NewDecoder(io.LimitReader(s, memberAuthMaxBytes)).Decode(&resp); err != nil {
		return FetchedRecord{}, fmt.Errorf("noderecord: read response: %w", err)
	}

	// Liveness + key control: the nonce signature must verify under p's key.
	pub, err := p.ExtractPublicKey()
	if err != nil {
		return FetchedRecord{}, fmt.Errorf("noderecord: extract peer key: %w", err)
	}
	ok, err := pub.Verify(nonce, resp.Sig)
	if err != nil {
		return FetchedRecord{}, err
	}
	if !ok {
		return FetchedRecord{}, errors.New("noderecord: nonce signature does not verify")
	}

	rec, cert, err := membership.VerifyNodeRecord(pinnedRoot, p.String(), resp.Record, resp.MemberCert, resp.IssuerCert, time.Now(), opts, genFloor)
	if err != nil {
		return FetchedRecord{}, fmt.Errorf("noderecord: %w", err)
	}

	// Drop any address that would steer a dial at private/loopback/metadata
	// ranges (SSRF). A record left with no dial-safe address is unusable.
	rec.Addrs = SafeDialAddrs(rec.Addrs)
	if len(rec.Addrs) == 0 {
		return FetchedRecord{}, errors.New("noderecord: no dial-safe addresses")
	}
	return FetchedRecord{Record: rec, Cert: cert, Signed: append([]byte(nil), resp.Record...)}, nil
}
