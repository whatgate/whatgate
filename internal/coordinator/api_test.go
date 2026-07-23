package coordinator

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func TestJoinRegisterDirectoryFlow(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)
	inv.Create("welcome", "founder", 5)

	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	defer ts.Close()

	join := postJSON(t, ts.URL+"/join", map[string]any{"code": "welcome", "peerID": "peerA"})
	if join.StatusCode != http.StatusOK {
		t.Fatalf("join status = %d, want 200", join.StatusCode)
	}
	join.Body.Close()

	reg := postJSON(t, ts.URL+"/register", map[string]any{
		"peerID":   "peerA",
		"addrs":    []string{"/ip4/1.2.3.4/tcp/4001"},
		"region":   "JP",
		"wantExit": true,
	})
	if reg.StatusCode != http.StatusOK {
		t.Fatalf("register status = %d, want 200", reg.StatusCode)
	}
	reg.Body.Close()

	resp, err := http.Get(ts.URL + "/directory")
	if err != nil {
		t.Fatalf("GET /directory: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("directory status = %d, want 200", resp.StatusCode)
	}

	var entries []directoryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("decode directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("directory has %d entries, want 1", len(entries))
	}
	if entries[0].PeerID != "peerA" || entries[0].Region != "JP" || !entries[0].WantExit {
		t.Fatalf("directory entry = %+v", entries[0])
	}
}

func TestRegisterWithoutAdmissionForbidden(t *testing.T) {
	dir := NewDirectory(time.Minute, nil)
	inv := NewInviteStore(nil)

	ts := httptest.NewServer(NewServer(dir, inv).Handler())
	defer ts.Close()

	reg := postJSON(t, ts.URL+"/register", map[string]any{
		"peerID": "stranger",
		"addrs":  []string{"/ip4/9.9.9.9/tcp/4001"},
	})
	defer reg.Body.Close()
	if reg.StatusCode != http.StatusForbidden {
		t.Fatalf("register-without-admission status = %d, want 403", reg.StatusCode)
	}
}
