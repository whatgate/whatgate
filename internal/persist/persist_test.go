package persist

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	want := Snapshot{
		Invites:         map[string]InviteRecord{"welcome": {Issuer: "founder", MaxUses: 100, Uses: 3}},
		Admissions:      map[string]AdmissionRecord{"peerA": {PeerID: "peerA", Code: "welcome", Issuer: "founder", At: 1000}},
		Groups:          map[string][]string{"g1": {"peerA"}},
		Endorsements:    map[string][]string{"g1": {"g2"}},
		GroupSecrets:    map[string]string{"g1": "s1"},
		PeerReputation:  map[string]int{"peerA": 5},
		GroupReputation: map[string]int{"g1": 10},
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got  %+v\n want %+v", got, want)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if got.Invites != nil || got.Groups != nil {
		t.Fatalf("expected empty snapshot, got %+v", got)
	}
}
