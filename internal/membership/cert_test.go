package membership

import (
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/whatgate/whatgate/internal/discovery"
)

// chain builds a root -> issuer -> member credential chain for tests.
type chain struct {
	rootPub      crypto.PubKey
	issuerSigned []byte
	memberSigned []byte
	subject      string
}

type chainCfg struct {
	issuerRoles   []Role
	memberRoles   []Role
	issuerExpires time.Time
	memberExpires time.Time
	memberRevEp   uint64
	issuerRevEp   uint64
	subject       string
}

func buildChain(t *testing.T, cfg chainCfg) chain {
	t.Helper()
	now := time.Now()
	if cfg.issuerExpires.IsZero() {
		cfg.issuerExpires = now.Add(365 * 24 * time.Hour)
	}
	if cfg.memberExpires.IsZero() {
		cfg.memberExpires = now.Add(24 * time.Hour)
	}
	if cfg.subject == "" {
		cfg.subject = "12D3KooWSubjectPeerIDPlaceholder"
	}
	rootPriv, rootPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuerPriv, issuerPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuerSigned, err := SignIssuerCert(rootPriv, issuerPub, "issuer-A", cfg.issuerRoles,
		now, cfg.issuerExpires, 1, cfg.issuerRevEp)
	if err != nil {
		t.Fatalf("SignIssuerCert: %v", err)
	}
	memberSigned, err := SignMemberCert(issuerPriv, cfg.subject, "issuer-A", cfg.memberRoles,
		now, cfg.memberExpires, 1, cfg.memberRevEp)
	if err != nil {
		t.Fatalf("SignMemberCert: %v", err)
	}
	return chain{rootPub: rootPub, issuerSigned: issuerSigned, memberSigned: memberSigned, subject: cfg.subject}
}

// A well-formed root -> issuer(member) -> member(member) chain verifies against
// the pinned root and returns the member cert.
func TestValidChainVerifies(t *testing.T) {
	c := buildChain(t, chainCfg{issuerRoles: []Role{RoleMember}, memberRoles: []Role{RoleMember}})
	cert, err := VerifyMemberCert(c.rootPub, c.subject, c.memberSigned, c.issuerSigned, time.Now(), VerifyOpts{})
	if err != nil {
		t.Fatalf("VerifyMemberCert: %v", err)
	}
	if cert.Subject != c.subject || len(cert.Roles) != 1 || cert.Roles[0] != RoleMember {
		t.Fatalf("cert = %+v", cert)
	}
}

// A chain pinned to the wrong root is rejected.
func TestRejectsWrongRoot(t *testing.T) {
	c := buildChain(t, chainCfg{issuerRoles: []Role{RoleMember}, memberRoles: []Role{RoleMember}})
	_, otherRoot, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyMemberCert(otherRoot, c.subject, c.memberSigned, c.issuerSigned, time.Now(), VerifyOpts{}); err == nil {
		t.Fatal("expected chain pinned to wrong root to be rejected")
	}
}

// KEY PROPERTY: an online issuer authorized only for {member} cannot mint a cert
// granting {exit} — verification rejects the role escalation.
func TestRejectsRoleEscalation(t *testing.T) {
	c := buildChain(t, chainCfg{issuerRoles: []Role{RoleMember}, memberRoles: []Role{RoleExit}})
	if _, err := VerifyMemberCert(c.rootPub, c.subject, c.memberSigned, c.issuerSigned, time.Now(), VerifyOpts{}); err == nil {
		t.Fatal("expected role escalation (issuer{member} -> member{exit}) to be rejected")
	}
}

// An exit cert IS accepted when the issuer was authorized (by the offline root)
// for the exit role.
func TestAllowsExitWhenIssuerAuthorized(t *testing.T) {
	c := buildChain(t, chainCfg{issuerRoles: []Role{RoleMember, RoleExit}, memberRoles: []Role{RoleExit}})
	cert, err := VerifyMemberCert(c.rootPub, c.subject, c.memberSigned, c.issuerSigned, time.Now(), VerifyOpts{})
	if err != nil {
		t.Fatalf("VerifyMemberCert: %v", err)
	}
	if len(cert.Roles) != 1 || cert.Roles[0] != RoleExit {
		t.Fatalf("cert roles = %v", cert.Roles)
	}
}

func TestRejectsExpiredMemberCert(t *testing.T) {
	c := buildChain(t, chainCfg{
		issuerRoles:   []Role{RoleMember},
		memberRoles:   []Role{RoleMember},
		memberExpires: time.Now().Add(-time.Minute),
	})
	if _, err := VerifyMemberCert(c.rootPub, c.subject, c.memberSigned, c.issuerSigned, time.Now(), VerifyOpts{}); err == nil {
		t.Fatal("expected expired member cert to be rejected")
	}
}

func TestRejectsExpiredIssuerCert(t *testing.T) {
	c := buildChain(t, chainCfg{
		issuerRoles:   []Role{RoleMember},
		memberRoles:   []Role{RoleMember},
		issuerExpires: time.Now().Add(-time.Minute),
	})
	if _, err := VerifyMemberCert(c.rootPub, c.subject, c.memberSigned, c.issuerSigned, time.Now(), VerifyOpts{}); err == nil {
		t.Fatal("expected expired issuer cert to be rejected")
	}
}

// A cert issued for one subject cannot be presented as authorizing a different
// peer.
func TestRejectsSubjectMismatch(t *testing.T) {
	c := buildChain(t, chainCfg{issuerRoles: []Role{RoleMember}, memberRoles: []Role{RoleMember}, subject: "12D3KooWPeerA"})
	if _, err := VerifyMemberCert(c.rootPub, "12D3KooWPeerB", c.memberSigned, c.issuerSigned, time.Now(), VerifyOpts{}); err == nil {
		t.Fatal("expected subject mismatch to be rejected")
	}
}

// A cert whose revocation epoch is below the caller's floor is rejected (coarse
// revocation; the full checkpoint mechanism is C1.2).
func TestRejectsRevokedByEpoch(t *testing.T) {
	c := buildChain(t, chainCfg{issuerRoles: []Role{RoleMember}, memberRoles: []Role{RoleMember}, memberRevEp: 1})
	if _, err := VerifyMemberCert(c.rootPub, c.subject, c.memberSigned, c.issuerSigned, time.Now(), VerifyOpts{MinRevEpoch: 2}); err == nil {
		t.Fatal("expected cert below the revocation-epoch floor to be rejected")
	}
}

// A tampered member payload (signature no longer matches) is rejected.
func TestRejectsTamperedMemberPayload(t *testing.T) {
	c := buildChain(t, chainCfg{issuerRoles: []Role{RoleMember, RoleExit}, memberRoles: []Role{RoleMember}})
	var env discovery.Signed
	if err := json.Unmarshal(c.memberSigned, &env); err != nil {
		t.Fatal(err)
	}
	env.Payload = json.RawMessage(`{"v":1,"subject":"12D3KooWSubjectPeerIDPlaceholder","issuerID":"issuer-A","roles":["exit"]}`)
	tampered, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyMemberCert(c.rootPub, c.subject, tampered, c.issuerSigned, time.Now(), VerifyOpts{}); err == nil {
		t.Fatal("expected tampered member payload to be rejected")
	}
}
