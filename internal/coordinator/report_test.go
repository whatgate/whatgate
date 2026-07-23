package coordinator

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/whatgate/whatgate/internal/trust"
)

func TestReportAdjustsPeerAndGroupReputation(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 100)
	srv := NewServer(dir, inv)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reporterC, _ := admitAndRegister(t, ts.URL, "welcome")
	subjC, subj := admitAndRegister(t, ts.URL, "welcome")
	if err := subjC.JoinGroup("g1", subj, "s1"); err != nil {
		t.Fatalf("subject joins g1: %v", err)
	}

	// A blocked report costs the subject -10.
	if err := reporterC.ReportOutcome(subj, trust.OutcomeBlocked); err != nil {
		t.Fatalf("report: %v", err)
	}

	score, err := reporterC.ReputationOf(subj)
	if err != nil {
		t.Fatalf("reputation: %v", err)
	}
	if score != -10 {
		t.Fatalf("subject peer score = %d, want -10", score)
	}
	if g := srv.Reputation().GroupScore("g1"); g != -10 {
		t.Fatalf("group g1 score = %d, want -10 (subject's abuse reflects on the group)", g)
	}

	// A served report nudges it back up by +1.
	if err := reporterC.ReportOutcome(subj, trust.OutcomeServed); err != nil {
		t.Fatalf("report served: %v", err)
	}
	if score, _ := reporterC.ReputationOf(subj); score != -9 {
		t.Fatalf("subject peer score = %d, want -9", score)
	}
}

func TestReportRequiresAdmittedReporter(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 100)
	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	defer ts.Close()

	// Signed, but never admitted → cannot report.
	strangerC, _ := newSignedClient(t, ts.URL)
	err := strangerC.ReportOutcome("victim", trust.OutcomeBlocked)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("non-admitted report should be 403, got %v", err)
	}
}
