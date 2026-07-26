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

// An exit that serves a signed node record is discovered by a peer via the
// node-record protocol: the record verifies (signer, chain-to-root, role,
// generation) and the exit proves liveness by signing the challenge nonce.
func TestFetchNodeRecordVerifies(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Root + issuer authorizing the exit role.
	rootPriv, rootPub, _ := crypto.GenerateEd25519Key(rand.Reader)
	issuerPriv, issuerPub, _ := crypto.GenerateEd25519Key(rand.Reader)
	issuerCert, _ := membership.SignIssuerCert(rootPriv, issuerPub, "iss",
		[]membership.Role{membership.RoleMember, membership.RoleExit}, time.Now(), time.Now().Add(365*24*time.Hour), 1, 0)

	client, err := New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	exit, err := New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer exit.Close()

	subject := exit.ID().String()
	memberCert, _ := membership.SignMemberCert(issuerPriv, subject, "iss",
		[]membership.Role{membership.RoleMember, membership.RoleExit}, time.Now(), time.Now().Add(30*24*time.Hour), 1, 0)
	rec := membership.NodeRecord{V: 1, Subject: subject, Roles: []membership.Role{membership.RoleExit}, Addrs: []string{"/ip4/1.2.3.4/tcp/443/ws"}, Region: "JP"}
	signedRec, err := membership.SignNodeRecord(exit.hostKey(), rec, time.Now(), time.Now().Add(5*time.Minute), 4)
	if err != nil {
		t.Fatal(err)
	}
	exit.SetMemberCredential(memberCert, issuerCert)
	exit.SetNodeRecord(signedRec)

	if err := client.h.Connect(ctx, peer.AddrInfo{ID: exit.ID(), Addrs: exit.h.Addrs()}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	got, err := client.FetchNodeRecord(ctx, exit.ID(), rootPub, membership.VerifyOpts{}, 0)
	if err != nil {
		t.Fatalf("FetchNodeRecord: %v", err)
	}
	if got.Region != "JP" || got.Generation != 4 || len(got.Roles) != 1 || got.Roles[0] != membership.RoleExit {
		t.Fatalf("record = %+v", got)
	}
}

// A rolled-back record (generation below the caller's floor) is rejected on
// fetch.
func TestFetchNodeRecordRejectsRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	rootPriv, rootPub, _ := crypto.GenerateEd25519Key(rand.Reader)
	issuerPriv, issuerPub, _ := crypto.GenerateEd25519Key(rand.Reader)
	issuerCert, _ := membership.SignIssuerCert(rootPriv, issuerPub, "iss",
		[]membership.Role{membership.RoleMember, membership.RoleExit}, time.Now(), time.Now().Add(365*24*time.Hour), 1, 0)

	client, _ := New(ctx)
	defer client.Close()
	exit, _ := New(ctx)
	defer exit.Close()

	subject := exit.ID().String()
	memberCert, _ := membership.SignMemberCert(issuerPriv, subject, "iss",
		[]membership.Role{membership.RoleMember, membership.RoleExit}, time.Now(), time.Now().Add(30*24*time.Hour), 1, 0)
	rec := membership.NodeRecord{V: 1, Subject: subject, Roles: []membership.Role{membership.RoleExit}, Addrs: []string{"/ip4/1.2.3.4/tcp/4001"}, Region: "JP"}
	signedRec, _ := membership.SignNodeRecord(exit.hostKey(), rec, time.Now(), time.Now().Add(5*time.Minute), 2)
	exit.SetMemberCredential(memberCert, issuerCert)
	exit.SetNodeRecord(signedRec)

	_ = client.h.Connect(ctx, peer.AddrInfo{ID: exit.ID(), Addrs: exit.h.Addrs()})
	if _, err := client.FetchNodeRecord(ctx, exit.ID(), rootPub, membership.VerifyOpts{}, 5); err == nil {
		t.Fatal("expected generation 2 below floor 5 to be rejected")
	}
}

// hostKey exposes the node's identity private key for tests that sign records.
func (n *Node) hostKey() crypto.PrivKey {
	return n.h.Peerstore().PrivKey(n.h.ID())
}
