package membership

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// exitFixture makes an exit identity, a root/issuer that authorized it for the
// given roles, and the member cert chain for the exit.
func exitFixture(t *testing.T, roles []Role) (exitPriv crypto.PrivKey, subject string, rootPub crypto.PubKey, memberCert, issuerCert []byte) {
	t.Helper()
	rootPriv, rootPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuerPriv, issuerPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	exitPriv, exitPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id, err := peer.IDFromPublicKey(exitPub)
	if err != nil {
		t.Fatal(err)
	}
	subject = id.String()
	issuerCert, err = SignIssuerCert(rootPriv, issuerPub, "iss", roles, time.Now(), time.Now().Add(365*24*time.Hour), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	memberCert, err = SignMemberCert(issuerPriv, subject, "iss", roles, time.Now(), time.Now().Add(30*24*time.Hour), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	return exitPriv, subject, rootPub, memberCert, issuerCert
}

// A record signed by the exit, whose member cert authorizes the exit role,
// verifies and yields its addrs/region/generation.
func TestNodeRecordRoundTrip(t *testing.T) {
	exitPriv, subject, rootPub, memberCert, issuerCert := exitFixture(t, []Role{RoleMember, RoleExit})
	rec := NodeRecord{V: 1, Subject: subject, Roles: []Role{RoleExit}, Addrs: []string{"/ip4/1.2.3.4/tcp/443/ws"}, Region: "JP"}
	signed, err := SignNodeRecord(exitPriv, rec, time.Now(), time.Now().Add(5*time.Minute), 7)
	if err != nil {
		t.Fatalf("SignNodeRecord: %v", err)
	}
	got, err := VerifyNodeRecord(rootPub, subject, signed, memberCert, issuerCert, time.Now(), VerifyOpts{}, 0)
	if err != nil {
		t.Fatalf("VerifyNodeRecord: %v", err)
	}
	if got.Region != "JP" || got.Generation != 7 || len(got.Addrs) != 1 || len(got.Roles) != 1 || got.Roles[0] != RoleExit {
		t.Fatalf("record = %+v", got)
	}
}

// A record whose generation is below the caller's floor is rejected as a
// rollback (an attacker can't replay a stale address advertisement).
func TestNodeRecordRejectsRolledBackGeneration(t *testing.T) {
	exitPriv, subject, rootPub, memberCert, issuerCert := exitFixture(t, []Role{RoleMember, RoleExit})
	rec := NodeRecord{V: 1, Subject: subject, Roles: []Role{RoleExit}, Addrs: []string{"/ip4/1.2.3.4/tcp/4001"}, Region: "JP"}
	signed, _ := SignNodeRecord(exitPriv, rec, time.Now(), time.Now().Add(5*time.Minute), 3)
	if _, err := VerifyNodeRecord(rootPub, subject, signed, memberCert, issuerCert, time.Now(), VerifyOpts{}, 5); err == nil {
		t.Fatal("expected generation 3 below floor 5 to be rejected")
	}
}

// A record advertising a role its member cert does not grant is rejected (an
// authorized member cannot advertise itself as an exit without an exit cert).
func TestNodeRecordRejectsUnauthorizedRole(t *testing.T) {
	exitPriv, subject, rootPub, memberCert, issuerCert := exitFixture(t, []Role{RoleMember}) // member only, no exit
	rec := NodeRecord{V: 1, Subject: subject, Roles: []Role{RoleExit}, Addrs: []string{"/ip4/1.2.3.4/tcp/4001"}, Region: "JP"}
	signed, _ := SignNodeRecord(exitPriv, rec, time.Now(), time.Now().Add(5*time.Minute), 1)
	if _, err := VerifyNodeRecord(rootPub, subject, signed, memberCert, issuerCert, time.Now(), VerifyOpts{}, 0); err == nil {
		t.Fatal("expected a record advertising an unauthorized exit role to be rejected")
	}
}

// A record signed by a key other than the subject's is rejected.
func TestNodeRecordRejectsWrongSigner(t *testing.T) {
	_, subject, rootPub, memberCert, issuerCert := exitFixture(t, []Role{RoleMember, RoleExit})
	otherPriv, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	rec := NodeRecord{V: 1, Subject: subject, Roles: []Role{RoleExit}, Addrs: []string{"/ip4/1.2.3.4/tcp/4001"}, Region: "JP"}
	signed, _ := SignNodeRecord(otherPriv, rec, time.Now(), time.Now().Add(5*time.Minute), 1) // signed by the wrong key
	if _, err := VerifyNodeRecord(rootPub, subject, signed, memberCert, issuerCert, time.Now(), VerifyOpts{}, 0); err == nil {
		t.Fatal("expected a record not signed by the subject to be rejected")
	}
}
