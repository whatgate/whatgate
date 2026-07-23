package coordinator

import (
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/whatgate/whatgate/internal/persist"
)

// TestSnapshotRestorePreservesAdmissionAndGroups simulates a coordinator restart
// via Snapshot/LoadSnapshot: an admitted peer stays admitted (can still
// register) and its group membership survives.
func TestSnapshotRestorePreservesAdmissionAndGroups(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 5)
	s1 := NewServer(dir, inv)
	ts1 := httptest.NewServer(s1.Handler())

	c, id := newSignedClient(t, ts1.URL)
	if _, err := c.Join("welcome", id); err != nil {
		t.Fatalf("join: %v", err)
	}
	if err := c.JoinGroup("fam", id, "sec"); err != nil {
		t.Fatalf("group: %v", err)
	}
	ts1.Close()

	snap := s1.Snapshot()

	// Fresh "restarted" coordinator restores the snapshot.
	s2 := NewServer(NewDirectory(time.Minute, nil), NewInviteStore(nil))
	s2.LoadSnapshot(snap)
	ts2 := httptest.NewServer(s2.Handler())
	defer ts2.Close()

	c2 := NewClient(ts2.URL)
	c2.Signer = c.Signer // same key/identity
	if err := c2.Register(NodeInfo{PeerID: id, Region: "JP", WantExit: true}); err != nil {
		t.Fatalf("register after restart (admission not restored?): %v", err)
	}
	if !s2.Graph().IsMember("fam", id) {
		t.Fatal("group membership not restored after restart")
	}
}

// TestStateFileSurvivesRestart exercises the on-disk save() path: mutations are
// written to a state file, and a fresh server loads that file.
func TestStateFileSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 5)
	s1 := NewServer(NewDirectory(time.Minute, nil), inv)
	s1.SetStatePath(path) // enable save-on-change
	ts1 := httptest.NewServer(s1.Handler())

	c, id := newSignedClient(t, ts1.URL)
	if _, err := c.Join("welcome", id); err != nil {
		t.Fatalf("join: %v", err)
	}
	if err := c.JoinGroup("fam", id, "sec"); err != nil {
		t.Fatalf("group: %v", err)
	}
	ts1.Close()

	// A fresh server loads the file written by save().
	snap, err := persist.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	inv2 := NewInviteStore(nil)
	s2 := NewServer(NewDirectory(time.Minute, nil), inv2)
	s2.LoadSnapshot(snap)

	if _, ok := inv2.AdmissionOf(id); !ok {
		t.Fatal("admission not restored from file")
	}
	if !s2.Graph().IsMember("fam", id) {
		t.Fatal("group membership not restored from file")
	}
}
