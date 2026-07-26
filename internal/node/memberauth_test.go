package node

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/whatgate/whatgate/internal/membership"
)

// authFixture builds a root+issuer and a Node holding a valid member cert for
// its own peer ID, plus a second Node acting as the verifier, already connected.
func authFixture(t *testing.T, ctx context.Context) (verifier, responder *Node, rootPub crypto.PubKey, issuerPriv crypto.PrivKey, issuerCert []byte) {
	t.Helper()
	rootPriv, rootPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var issuerPub crypto.PubKey
	issuerPriv, issuerPub, err = crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuerCert, err = membership.SignIssuerCert(rootPriv, issuerPub, "iss",
		[]membership.Role{membership.RoleMember}, time.Now(), time.Now().Add(365*24*time.Hour), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err = New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	responder, err = New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.h.Connect(ctx, peer.AddrInfo{ID: responder.ID(), Addrs: responder.h.Addrs()}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return verifier, responder, rootPub, issuerPriv, issuerCert
}

// A node serving a valid member credential is verified by a peer via the
// member-auth protocol: the chain checks against the pinned root and the cert's
// subject matches the connection's authenticated peer ID.
func TestMemberAuthVerifiesPeer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	verifier, responder, rootPub, issuerPriv, issuerCert := authFixture(t, ctx)
	defer verifier.Close()
	defer responder.Close()

	mc, err := membership.SignMemberCert(issuerPriv, responder.ID().String(), "iss",
		[]membership.Role{membership.RoleMember}, time.Now(), time.Now().Add(30*24*time.Hour), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	responder.SetMemberCredential(mc, issuerCert)

	cert, err := verifier.VerifyPeerMembership(ctx, responder.ID(), rootPub, membership.VerifyOpts{})
	if err != nil {
		t.Fatalf("VerifyPeerMembership: %v", err)
	}
	if cert.Subject != responder.ID().String() || len(cert.Roles) != 1 || cert.Roles[0] != membership.RoleMember {
		t.Fatalf("cert = %+v", cert)
	}
}

// A credential whose subject is a different peer than the one connected is
// rejected — a member cannot present someone else's cert.
func TestMemberAuthRejectsSubjectMismatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	verifier, responder, rootPub, issuerPriv, issuerCert := authFixture(t, ctx)
	defer verifier.Close()
	defer responder.Close()

	// A cert for some OTHER peer id, not the responder's.
	mc, _ := membership.SignMemberCert(issuerPriv, "12D3KooWSomeoneElse", "iss",
		[]membership.Role{membership.RoleMember}, time.Now(), time.Now().Add(30*24*time.Hour), 1, 0)
	responder.SetMemberCredential(mc, issuerCert)

	if _, err := verifier.VerifyPeerMembership(ctx, responder.ID(), rootPub, membership.VerifyOpts{}); err == nil {
		t.Fatal("expected a cert for a different subject to be rejected")
	}
}

// A credential chain that does not verify against the pinned root is rejected.
func TestMemberAuthRejectsWrongRoot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	verifier, responder, _, issuerPriv, issuerCert := authFixture(t, ctx)
	defer verifier.Close()
	defer responder.Close()

	mc, _ := membership.SignMemberCert(issuerPriv, responder.ID().String(), "iss",
		[]membership.Role{membership.RoleMember}, time.Now(), time.Now().Add(30*24*time.Hour), 1, 0)
	responder.SetMemberCredential(mc, issuerCert)

	_, otherRoot, _ := crypto.GenerateEd25519Key(rand.Reader)
	if _, err := verifier.VerifyPeerMembership(ctx, responder.ID(), otherRoot, membership.VerifyOpts{}); err == nil {
		t.Fatal("expected a chain not anchored to the pinned root to be rejected")
	}
}
