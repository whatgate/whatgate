package coordinator

import (
	"testing"
	"time"
)

func fixedClock() func() time.Time {
	return func() time.Time { return time.Unix(1000, 0) }
}

func TestRedeemUnknownCodeFails(t *testing.T) {
	s := NewInviteStore(fixedClock())

	if _, err := s.Redeem("nope", "peerA"); err == nil {
		t.Fatal("redeeming an unknown code should fail")
	}
}

func TestRedeemValidCodeReturnsIssuer(t *testing.T) {
	s := NewInviteStore(fixedClock())
	s.Create("welcome", "founder", 1)

	issuer, err := s.Redeem("welcome", "peerA")
	if err != nil {
		t.Fatalf("redeem valid code: %v", err)
	}
	if issuer != "founder" {
		t.Fatalf("issuer = %q, want %q", issuer, "founder")
	}
}

func TestRedeemBeyondMaxUsesFails(t *testing.T) {
	s := NewInviteStore(fixedClock())
	s.Create("once", "founder", 1)

	if _, err := s.Redeem("once", "peerA"); err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	if _, err := s.Redeem("once", "peerB"); err != ErrInviteExhausted {
		t.Fatalf("second redeem err = %v, want ErrInviteExhausted", err)
	}
}

func TestAdmissionIsTraceable(t *testing.T) {
	s := NewInviteStore(fixedClock())
	s.Create("welcome", "founder", 5)
	if _, err := s.Redeem("welcome", "peerA"); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	a, ok := s.AdmissionOf("peerA")
	if !ok {
		t.Fatal("admission of peerA should be recorded")
	}
	if a.Issuer != "founder" || a.Code != "welcome" {
		t.Fatalf("admission = %+v, want issuer=founder code=welcome", a)
	}

	if _, ok := s.AdmissionOf("stranger"); ok {
		t.Fatal("unknown peer should have no admission record")
	}
}
