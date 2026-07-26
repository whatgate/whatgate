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

// resolveFixture stands up a client + an exit as mutual members on a private DHT,
// issues the exit an exit-role credential, has it serve+advertise a node record,
// and returns the pieces a resolve test needs.
func resolveFixture(t *testing.T, ctx context.Context) (client *Node, dhtClient *PrivateDHT, rootPriv crypto.PrivKey, rootPub crypto.PubKey, exitSubject string) {
	t.Helper()
	rootPriv, rootPub, _ = crypto.GenerateEd25519Key(rand.Reader)
	issuerPriv, issuerPub, _ := crypto.GenerateEd25519Key(rand.Reader)
	issuerCert, _ := membership.SignIssuerCert(rootPriv, issuerPub, "iss",
		[]membership.Role{membership.RoleMember, membership.RoleExit}, time.Now(), time.Now().Add(365*24*time.Hour), 1, 0)

	msC, msE := NewMemberSet(), NewMemberSet()
	client, err := New(ctx, WithMemberGater(msC))
	if err != nil {
		t.Fatal(err)
	}
	exit, err := New(ctx, WithMemberGater(msE))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { exit.Close() })
	msC.Set([]peer.ID{exit.ID()})
	msE.Set([]peer.ID{client.ID()})

	exitSubject = exit.ID().String()
	memberCert, _ := membership.SignMemberCert(issuerPriv, exitSubject, "iss",
		[]membership.Role{membership.RoleMember, membership.RoleExit}, time.Now(), time.Now().Add(30*24*time.Hour), 1, 0)
	rec := membership.NodeRecord{V: 1, Subject: exitSubject, Roles: []membership.Role{membership.RoleExit},
		Addrs: []string{"/ip4/1.2.3.4/tcp/443/ws"}, Region: "JP"}
	signedRec, _ := membership.SignNodeRecord(exit.hostKey(), rec, time.Now(), time.Now().Add(5*time.Minute), 4)
	exit.SetMemberCredential(memberCert, issuerCert)
	exit.SetNodeRecord(signedRec)

	dhtClient, err = client.StartPrivateDHT(ctx, msC, []peer.AddrInfo{{ID: exit.ID(), Addrs: exit.h.Addrs()}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dhtClient.Close() })
	dhtExit, err := exit.StartPrivateDHT(ctx, msE, []peer.AddrInfo{{ID: client.ID(), Addrs: client.h.Addrs()}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dhtExit.Close() })

	for i := 0; i < 50 && (dhtClient.RoutingTableSize() == 0 || dhtExit.RoutingTableSize() == 0); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if err := dhtExit.Advertise(ctx, "exit", "JP", 1); err != nil {
		t.Fatal(err)
	}
	return client, dhtClient, rootPriv, rootPub, exitSubject
}

// ResolveExits composes discovery: find candidates, fetch+verify each record, and
// return them tagged. A DHT-resolved exit is authorized/eligible/reachable but
// never recommended (that only comes from authoritative reputation).
func TestResolveExitsReturnsVerified(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	client, dhtClient, _, rootPub, exitSubject := resolveFixture(t, ctx)
	defer client.Close()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		exits := client.ResolveExits(ctx, dhtClient, "exit", "JP", 1, rootPub, membership.VerifyOpts{}, nil, nil, 0, 8)
		if len(exits) == 1 {
			e := exits[0]
			if e.Record.Subject != exitSubject || e.Record.Region != "JP" {
				t.Fatalf("resolved exit = %+v", e.Record)
			}
			if !e.Authorized || !e.Eligible || !e.Reachable || e.Recommended {
				t.Fatalf("states = %+v (want authorized/eligible/reachable, not recommended)", e)
			}
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatal("ResolveExits did not return the advertised exit")
}

// A revoked exit is dropped from the resolve results.
func TestResolveExitsDropsRevoked(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	client, dhtClient, rootPriv, rootPub, exitSubject := resolveFixture(t, ctx)
	defer client.Close()

	// A checkpoint — signed by the SAME root that anchors the exit's cert — that
	// revokes the exit's subject.
	now := time.Now()
	cp := membership.RevocationCheckpoint{V: 1, Version: 1, ThisUpdate: now.Unix(), NextUpdate: now.Add(time.Hour).Unix(),
		MaxStalenessSec: int64((24 * time.Hour).Seconds()), RevokedSubjects: []string{exitSubject}}
	signed, _ := membership.SignRevocationCheckpoint(rootPriv, cp, now.Add(24*time.Hour))
	checkpoint, err := membership.VerifyRevocationCheckpoint(rootPub, signed, now, 0)
	if err != nil {
		t.Fatal(err)
	}

	// First confirm the exit IS discoverable without the checkpoint (so the
	// absence below is due to revocation, not slow propagation), then confirm the
	// revoking checkpoint drops it.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if len(client.ResolveExits(ctx, dhtClient, "exit", "JP", 1, rootPub, membership.VerifyOpts{}, nil, nil, 0, 8)) == 1 {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if got := client.ResolveExits(ctx, dhtClient, "exit", "JP", 1, rootPub, membership.VerifyOpts{}, &checkpoint, nil, 0, 8); len(got) != 0 {
		t.Fatalf("expected revoked exit to be dropped, got %d", len(got))
	}
}
