package coordinator

import (
	"crypto/rand"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/whatgate/whatgate/internal/membership"
)

// When an online issuer is configured, a successful join returns a member cert
// (plus the root-signed issuer cert) that verifies as a {member} credential for
// the joining peer against the pinned offline root.
func TestJoinIssuesVerifiableMemberCert(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 10)
	srv := NewServer(dir, inv)

	// Offline root authorizes the coordinator's online issuer key for {member}.
	rootPriv, rootPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuerPriv, issuerPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuerCert, err := membership.SignIssuerCert(rootPriv, issuerPub, "coord-issuer",
		[]membership.Role{membership.RoleMember}, time.Now(), time.Now().Add(365*24*time.Hour), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetIssuer(issuerPriv, issuerCert, "coord-issuer")

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	c, id := newSignedClient(t, ts.URL)
	adm, err := c.Join("welcome", id)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if len(adm.MemberCert) == 0 || len(adm.IssuerCert) == 0 {
		t.Fatalf("join did not return a cert chain: %+v", adm)
	}
	cert, err := membership.VerifyMemberCert(rootPub, id, adm.MemberCert, adm.IssuerCert, time.Now(), membership.VerifyOpts{})
	if err != nil {
		t.Fatalf("issued cert does not verify: %v", err)
	}
	if cert.Subject != id || len(cert.Roles) != 1 || cert.Roles[0] != membership.RoleMember {
		t.Fatalf("cert = %+v (want subject=%s role=member)", cert, id)
	}
}

// The coordinator will not mint an exit cert on join — auto-join grants only
// {member}; exit/relay authority must come from the offline root out of band.
func TestJoinCertGrantsOnlyMember(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 10)
	srv := NewServer(dir, inv)

	rootPriv, rootPub, _ := crypto.GenerateEd25519Key(rand.Reader)
	issuerPriv, issuerPub, _ := crypto.GenerateEd25519Key(rand.Reader)
	// Even though the offline root trusts this issuer for exit too...
	issuerCert, _ := membership.SignIssuerCert(rootPriv, issuerPub, "coord-issuer",
		[]membership.Role{membership.RoleMember, membership.RoleExit}, time.Now(), time.Now().Add(365*24*time.Hour), 1, 0)
	srv.SetIssuer(issuerPriv, issuerCert, "coord-issuer")

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	c, id := newSignedClient(t, ts.URL)
	adm, err := c.Join("welcome", id)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := membership.VerifyMemberCert(rootPub, id, adm.MemberCert, adm.IssuerCert, time.Now(), membership.VerifyOpts{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range cert.Roles {
		if r == membership.RoleExit || r == membership.RoleRelay {
			t.Fatalf("auto-join cert unexpectedly granted %q", r)
		}
	}
}

// With no issuer configured, join still works and simply returns no cert
// (backward compatible with unsigned deployments).
func TestJoinWithoutIssuerReturnsNoCert(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 10)
	srv := NewServer(dir, inv)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	c, id := newSignedClient(t, ts.URL)
	adm, err := c.Join("welcome", id)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if adm.Issuer != "founder" {
		t.Fatalf("issuer = %q, want founder", adm.Issuer)
	}
	if len(adm.MemberCert) != 0 {
		t.Fatalf("expected no member cert when issuer unconfigured")
	}
}
