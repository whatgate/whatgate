package trust

import "testing"

func TestConservativeScopeExcludesStrangers(t *testing.T) {
	s := ScopeConservative
	if !s.Allows(TierSameGroup) {
		t.Error("conservative should allow same-group")
	}
	if !s.Allows(TierEndorsed) {
		t.Error("conservative should allow endorsed")
	}
	if s.Allows(TierStranger) {
		t.Error("conservative should NOT allow strangers")
	}
}

func TestOpenScopeAllowsEveryone(t *testing.T) {
	s := ScopeOpen
	for _, tier := range []Tier{TierStranger, TierEndorsed, TierSameGroup} {
		if !s.Allows(tier) {
			t.Errorf("open should allow tier %v", tier)
		}
	}
}

func TestParseScope(t *testing.T) {
	if s, err := ParseScope("open"); err != nil || s != ScopeOpen {
		t.Fatalf("ParseScope(open) = %v, %v", s, err)
	}
	if _, err := ParseScope("bogus"); err == nil {
		t.Fatal("ParseScope(bogus) should error")
	}
}
