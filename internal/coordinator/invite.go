package coordinator

import (
	"errors"
	"sync"
	"time"

	"github.com/whatgate/whatgate/internal/persist"
)

// Errors returned by InviteStore.
var (
	ErrUnknownInvite   = errors.New("coordinator: unknown invite code")
	ErrInviteExhausted = errors.New("coordinator: invite code exhausted")
)

// invite is a redeemable admission code.
type invite struct {
	issuer  string
	maxUses int
	uses    int
}

// Admission records that a peer joined via a specific invite, forming the
// traceable chain of who vouched for whom.
type Admission struct {
	PeerID string
	Code   string
	Issuer string
	At     time.Time
}

// InviteStore governs admission to the half-open network. Every member is
// admitted by redeeming an invite created by an existing member, so admissions
// form a traceable chain — essential for WhatGate's abuse accountability.
type InviteStore struct {
	now func() time.Time

	mu         sync.Mutex
	invites    map[string]*invite
	admissions map[string]Admission // keyed by admitted PeerID
}

// NewInviteStore creates an empty InviteStore. now is injectable for tests; nil
// defaults to time.Now.
func NewInviteStore(now func() time.Time) *InviteStore {
	if now == nil {
		now = time.Now
	}
	return &InviteStore{
		now:        now,
		invites:    make(map[string]*invite),
		admissions: make(map[string]Admission),
	}
}

// Create registers an invite code that admits up to maxUses members, attributed
// to issuer.
func (s *InviteStore) Create(code, issuer string, maxUses int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invites[code] = &invite{issuer: issuer, maxUses: maxUses}
}

// Redeem consumes one use of code to admit peerID, returning the issuer that
// vouched for the new member and recording the admission for traceability.
func (s *InviteStore) Redeem(code, peerID string) (issuer string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	inv, ok := s.invites[code]
	if !ok {
		return "", ErrUnknownInvite
	}
	if inv.uses >= inv.maxUses {
		return "", ErrInviteExhausted
	}
	inv.uses++
	s.admissions[peerID] = Admission{
		PeerID: peerID,
		Code:   code,
		Issuer: inv.issuer,
		At:     s.now(),
	}
	return inv.issuer, nil
}

// AdmissionOf returns how peerID was admitted, if it was.
func (s *InviteStore) AdmissionOf(peerID string) (Admission, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.admissions[peerID]
	return a, ok
}

// Exists reports whether an invite code is present.
func (s *InviteStore) Exists(code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.invites[code]
	return ok
}

// Export returns the invites and admissions for persistence.
func (s *InviteStore) Export() (map[string]persist.InviteRecord, map[string]persist.AdmissionRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv := make(map[string]persist.InviteRecord, len(s.invites))
	for code, iv := range s.invites {
		inv[code] = persist.InviteRecord{Issuer: iv.issuer, MaxUses: iv.maxUses, Uses: iv.uses}
	}
	adm := make(map[string]persist.AdmissionRecord, len(s.admissions))
	for pid, a := range s.admissions {
		adm[pid] = persist.AdmissionRecord{PeerID: a.PeerID, Code: a.Code, Issuer: a.Issuer, At: a.At.Unix()}
	}
	return inv, adm
}

// Import merges persisted invites and admissions (used on load).
func (s *InviteStore) Import(inv map[string]persist.InviteRecord, adm map[string]persist.AdmissionRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for code, r := range inv {
		s.invites[code] = &invite{issuer: r.Issuer, maxUses: r.MaxUses, uses: r.Uses}
	}
	for pid, r := range adm {
		s.admissions[pid] = Admission{PeerID: r.PeerID, Code: r.Code, Issuer: r.Issuer, At: time.Unix(r.At, 0)}
	}
}
