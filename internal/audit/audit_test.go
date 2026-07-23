package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileLoggerAppendsJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	l, err := NewFileLogger(path)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}

	entries := []Entry{
		{Time: time.Unix(1000, 0).UTC(), Requester: "peerA", Target: "example.com:443", Outcome: "served"},
		{Time: time.Unix(1001, 0).UTC(), Requester: "peerB", Target: "mail:25", Outcome: "denied: blocked port"},
	}
	for _, e := range entries {
		if err := l.Log(e); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	var got []Entry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("unmarshal line %q: %v", sc.Text(), err)
		}
		got = append(got, e)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].Requester != "peerA" || got[0].Outcome != "served" {
		t.Fatalf("entry 0 = %+v", got[0])
	}
	if got[1].Target != "mail:25" || got[1].Outcome != "denied: blocked port" {
		t.Fatalf("entry 1 = %+v", got[1])
	}
}

func TestFileLoggerAppendsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")

	l1, _ := NewFileLogger(path)
	_ = l1.Log(Entry{Requester: "first"})
	l1.Close()

	l2, _ := NewFileLogger(path) // reopen — must append, not truncate
	_ = l2.Log(Entry{Requester: "second"})
	l2.Close()

	data, _ := os.ReadFile(path)
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 2 {
		t.Fatalf("expected 2 lines after reopen+append, got %d", lines)
	}
}
