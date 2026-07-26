// Package membership implements WhatGate's C1 credential chain: an offline root
// signs an issuer cert authorizing an online, short-lived, role-limited issuer;
// that issuer signs member certs. A node proves it may act (as a plain member,
// an exit, or a relay) by presenting a member cert plus the issuer cert that
// authorizes it, verified against a pinned root public key.
//
// The security property is enforced at *verification*, not signing: a member
// cert's roles must be a subset of the roles its issuer was authorized to grant.
// So an online issuer authorized only for {member} cannot mint an exit/relay
// cert — even if it tries, the chain fails to verify. Exit/relay authority
// therefore requires an issuer cert the offline root (or a dual-approved
// process) granted those roles. See docs/c1-decentralized-discovery.md §15.
package membership

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/whatgate/whatgate/internal/discovery"
)

// Role is a capability a credential may grant.
type Role string

const (
	RoleMember Role = "member" // baseline network membership
	RoleExit   Role = "exit"   // may serve as an exit
	RoleRelay  Role = "relay"  // may serve as a relay
)

// IssuerCert is the payload of a TypeIssuerCert object: the offline root
// authorizing an online issuer key to grant member certs of the listed roles.
// Validity window, serial and revocation epoch live in the signed envelope.
type IssuerCert struct {
	V         int    `json:"v"`
	IssuerKey string `json:"issuerKey"` // base64 marshaled issuer public key
	IssuerID  string `json:"issuerID"`
	Roles     []Role `json:"roles"` // roles this issuer may grant
}

// Cert is the payload of a TypeMemberCert object: an issuer authorizing a
// subject peer to hold the listed roles. Validity window, serial and revocation
// epoch live in the signed envelope.
type Cert struct {
	V        int    `json:"v"`
	Subject  string `json:"subject"` // PeerID the cert authorizes
	IssuerID string `json:"issuerID"`
	Roles    []Role `json:"roles"`
}

// VerifyOpts tunes verification.
type VerifyOpts struct {
	// MinRevEpoch rejects any cert (issuer or member) whose revocation epoch is
	// below this floor — a coarse, network-wide revocation cutoff. The full
	// signed-checkpoint revocation mechanism is C1.2.
	MinRevEpoch uint64
}

// SignIssuerCert produces a root-signed issuer cert authorizing issuerPub to
// grant the given roles. serial must increase across reissues; revEpoch is the
// revocation epoch at issuance.
func SignIssuerCert(root crypto.PrivKey, issuerPub crypto.PubKey, issuerID string, roles []Role, issued, expires time.Time, serial, revEpoch uint64) ([]byte, error) {
	payload, err := json.Marshal(IssuerCert{
		V:         1,
		IssuerKey: discovery.EncodePublicKey(issuerPub),
		IssuerID:  issuerID,
		Roles:     roles,
	})
	if err != nil {
		return nil, err
	}
	obj, err := discovery.Sign(root, discovery.Meta{
		Type:            discovery.TypeIssuerCert,
		Serial:          serial,
		IssuedAt:        issued,
		ExpiresAt:       expires,
		RevocationEpoch: revEpoch,
	}, payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(obj)
}

// SignMemberCert produces an issuer-signed member cert authorizing subject to
// hold the given roles. It does not enforce that the issuer may grant those
// roles — that is checked at verification against the issuer cert.
func SignMemberCert(issuer crypto.PrivKey, subject, issuerID string, roles []Role, issued, expires time.Time, serial, revEpoch uint64) ([]byte, error) {
	payload, err := json.Marshal(Cert{
		V:        1,
		Subject:  subject,
		IssuerID: issuerID,
		Roles:    roles,
	})
	if err != nil {
		return nil, err
	}
	obj, err := discovery.Sign(issuer, discovery.Meta{
		Type:            discovery.TypeMemberCert,
		Serial:          serial,
		IssuedAt:        issued,
		ExpiresAt:       expires,
		RevocationEpoch: revEpoch,
	}, payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(obj)
}

// VerifyMemberCert verifies the full chain: the issuer cert must be signed by
// the pinned root, the member cert by the key that issuer cert authorizes, the
// member cert's subject must equal subject, and the member cert's roles must be
// a subset of the issuer's authorized roles (no privilege escalation). Both
// certs must be unexpired at now and at/above the revocation-epoch floor. On
// success it returns the verified member cert.
func VerifyMemberCert(pinnedRoot crypto.PubKey, subject string, memberSigned, issuerSigned []byte, now time.Time, opts VerifyOpts) (Cert, error) {
	// 1. Issuer cert against the pinned root.
	var issuerEnv discovery.Signed
	if err := json.Unmarshal(issuerSigned, &issuerEnv); err != nil {
		return Cert{}, fmt.Errorf("issuer cert: not a signed envelope: %w", err)
	}
	icPayload, err := issuerEnv.Verify(pinnedRoot, discovery.TypeIssuerCert, now, 0)
	if err != nil {
		return Cert{}, fmt.Errorf("issuer cert: %w", err)
	}
	if issuerEnv.RevocationEpoch < opts.MinRevEpoch {
		return Cert{}, errors.New("issuer cert: below revocation-epoch floor")
	}
	var ic IssuerCert
	if err := json.Unmarshal(icPayload, &ic); err != nil {
		return Cert{}, fmt.Errorf("issuer cert: bad payload: %w", err)
	}
	issuerPub, err := discovery.DecodePublicKey(ic.IssuerKey)
	if err != nil {
		return Cert{}, fmt.Errorf("issuer cert: bad issuer key: %w", err)
	}

	// 2. Member cert against the issuer's key.
	var memberEnv discovery.Signed
	if err := json.Unmarshal(memberSigned, &memberEnv); err != nil {
		return Cert{}, fmt.Errorf("member cert: not a signed envelope: %w", err)
	}
	mcPayload, err := memberEnv.Verify(issuerPub, discovery.TypeMemberCert, now, 0)
	if err != nil {
		return Cert{}, fmt.Errorf("member cert: %w", err)
	}
	if memberEnv.RevocationEpoch < opts.MinRevEpoch {
		return Cert{}, errors.New("member cert: below revocation-epoch floor")
	}
	var mc Cert
	if err := json.Unmarshal(mcPayload, &mc); err != nil {
		return Cert{}, fmt.Errorf("member cert: bad payload: %w", err)
	}

	// 3. Bindings and privilege check.
	if mc.Subject != subject {
		return Cert{}, fmt.Errorf("member cert: subject %q does not match expected %q", mc.Subject, subject)
	}
	if mc.IssuerID != ic.IssuerID {
		return Cert{}, fmt.Errorf("member cert: issuerID %q does not match issuer cert %q", mc.IssuerID, ic.IssuerID)
	}
	if !rolesSubsetOf(mc.Roles, ic.Roles) {
		return Cert{}, fmt.Errorf("member cert: roles %v exceed issuer's authorized roles %v", mc.Roles, ic.Roles)
	}
	return mc, nil
}

// IssuerCertID reads the IssuerID out of a signed issuer cert without verifying
// it — for a serving coordinator to bind the matching id into the member certs
// it mints. Verification of the chain happens at the consuming node.
func IssuerCertID(signed []byte) (string, error) {
	var env discovery.Signed
	if err := json.Unmarshal(signed, &env); err != nil {
		return "", fmt.Errorf("issuer cert: not a signed envelope: %w", err)
	}
	var ic IssuerCert
	if err := json.Unmarshal(env.Payload, &ic); err != nil {
		return "", fmt.Errorf("issuer cert: bad payload: %w", err)
	}
	return ic.IssuerID, nil
}

// rolesSubsetOf reports whether every role in want is present in allowed.
func rolesSubsetOf(want, allowed []Role) bool {
	set := make(map[Role]struct{}, len(allowed))
	for _, r := range allowed {
		set[r] = struct{}{}
	}
	for _, r := range want {
		if _, ok := set[r]; !ok {
			return false
		}
	}
	return true
}
