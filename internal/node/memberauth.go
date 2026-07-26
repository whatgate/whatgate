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

// MemberAuthProtocol is stage two of C1's inbound authentication (§15.3): once a
// connection has passed PeerID-level gating, a peer proves *membership* over this
// protocol by presenting its member credential chain and signing a fresh nonce
// with its identity key. Stage one (the connection gater / routing-table filter)
// is coarse; this is the certificate check.
const MemberAuthProtocol = protocol.ID("/whatgate/member-auth/1.0.0")

// memberAuthMaxBytes caps the credential exchange — certs are small.
const memberAuthMaxBytes = 64 << 10

type memberAuthChallenge struct {
	Nonce []byte `json:"nonce"`
}

type memberAuthResponse struct {
	MemberCert json.RawMessage `json:"memberCert"`
	IssuerCert json.RawMessage `json:"issuerCert"`
	Sig        []byte          `json:"sig"` // signature over the challenge nonce by the responder's identity key
}

// SetMemberCredential stores this node's credential chain (issued by the
// coordinator on join, C1.1) and registers the member-auth responder so peers
// can verify this node's membership.
func (n *Node) SetMemberCredential(memberCert, issuerCert []byte) {
	n.credMu.Lock()
	n.memberCert = append([]byte(nil), memberCert...)
	n.issuerCert = append([]byte(nil), issuerCert...)
	n.credMu.Unlock()
	n.h.SetStreamHandler(MemberAuthProtocol, n.handleMemberAuth)
}

// handleMemberAuth answers a peer's challenge: sign the nonce with our identity
// key and return our credential chain. The signature proves we control the key
// behind our peer ID — which the presented member cert is bound to.
func (n *Node) handleMemberAuth(s network.Stream) {
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
	mc, ic := n.memberCert, n.issuerCert
	n.credMu.Unlock()
	_ = json.NewEncoder(s).Encode(memberAuthResponse{MemberCert: mc, IssuerCert: ic, Sig: sig})
}

// VerifyPeerMembership opens the member-auth protocol to p, challenges it with a
// fresh nonce, and verifies the returned credential chain: the nonce signature
// must be valid under p's identity key, the chain must verify against pinnedRoot,
// and the member cert's subject must equal p. On success it returns p's cert.
func (n *Node) VerifyPeerMembership(ctx context.Context, p peer.ID, pinnedRoot crypto.PubKey, opts membership.VerifyOpts) (membership.Cert, error) {
	s, err := n.h.NewStream(ctx, p, MemberAuthProtocol)
	if err != nil {
		return membership.Cert{}, fmt.Errorf("member-auth: open stream: %w", err)
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(10 * time.Second))

	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return membership.Cert{}, err
	}
	if err := json.NewEncoder(s).Encode(memberAuthChallenge{Nonce: nonce}); err != nil {
		return membership.Cert{}, fmt.Errorf("member-auth: send challenge: %w", err)
	}

	var resp memberAuthResponse
	if err := json.NewDecoder(io.LimitReader(s, memberAuthMaxBytes)).Decode(&resp); err != nil {
		return membership.Cert{}, fmt.Errorf("member-auth: read response: %w", err)
	}

	// The nonce signature must verify under p's identity key — proof the peer
	// controls the key its peer ID (and thus its member cert's subject) is bound to.
	pub, err := p.ExtractPublicKey()
	if err != nil {
		return membership.Cert{}, fmt.Errorf("member-auth: extract peer key: %w", err)
	}
	ok, err := pub.Verify(nonce, resp.Sig)
	if err != nil {
		return membership.Cert{}, err
	}
	if !ok {
		return membership.Cert{}, errors.New("member-auth: nonce signature does not verify")
	}

	// The credential chain must anchor to the pinned root and name p as subject.
	cert, err := membership.VerifyMemberCert(pinnedRoot, p.String(), resp.MemberCert, resp.IssuerCert, time.Now(), opts)
	if err != nil {
		return membership.Cert{}, fmt.Errorf("member-auth: %w", err)
	}
	return cert, nil
}
