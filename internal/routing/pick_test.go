package routing

import (
	"testing"

	"github.com/whatgate/whatgate/internal/coordinator"
)

func TestPickExitSelectsMatchingRegionExit(t *testing.T) {
	nodes := []coordinator.NodeInfo{
		{PeerID: "self", Region: "JP", WantExit: true},    // skip: is self
		{PeerID: "noexit", Region: "JP", WantExit: false}, // skip: not an exit
		{PeerID: "us", Region: "US", WantExit: true},      // skip: wrong region
		{PeerID: "jp1", Region: "JP", WantExit: true},     // eligible
	}

	got, ok := PickExit(nodes, "JP", "self")
	if !ok {
		t.Fatal("expected to find a JP exit")
	}
	if got.PeerID != "jp1" {
		t.Fatalf("picked %q, want jp1", got.PeerID)
	}
}

func TestPickExitNoneReturnsFalse(t *testing.T) {
	nodes := []coordinator.NodeInfo{
		{PeerID: "us", Region: "US", WantExit: true},
	}
	if _, ok := PickExit(nodes, "JP", "self"); ok {
		t.Fatal("expected no JP exit")
	}
}
