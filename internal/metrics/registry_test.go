package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestRegistryIncAndSnapshot(t *testing.T) {
	r := NewRegistry()
	r.Inc("a")
	r.Inc("a")
	r.Inc("b")
	snap := r.Snapshot()
	if snap["a"] != 2 || snap["b"] != 1 {
		t.Fatalf("snapshot = %v, want a:2 b:1", snap)
	}
}

func TestRegistryAdd(t *testing.T) {
	r := NewRegistry()
	r.Add("bytes", 100)
	r.Add("bytes", 50)
	if got := r.Snapshot()["bytes"]; got != 150 {
		t.Fatalf("bytes = %d, want 150", got)
	}
}

// A snapshot is a copy: mutating it must not corrupt the live registry.
func TestSnapshotIsCopy(t *testing.T) {
	r := NewRegistry()
	r.Inc("a")
	snap := r.Snapshot()
	snap["a"] = 999
	if got := r.Snapshot()["a"]; got != 1 {
		t.Fatalf("live value = %d, want 1 (snapshot must be a copy)", got)
	}
}

func TestHandlerServesJSON(t *testing.T) {
	r := NewRegistry()
	r.Inc("served")
	r.Add("denied", 3)

	ts := httptest.NewServer(r.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got map[string]int64
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["served"] != 1 || got["denied"] != 3 {
		t.Fatalf("body = %v, want served:1 denied:3", got)
	}
}

// Concurrent increments are race-safe and total correctly (run with -race).
func TestRegistryConcurrent(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				r.Inc("hits")
			}
		}()
	}
	wg.Wait()
	if got := r.Snapshot()["hits"]; got != 10000 {
		t.Fatalf("hits = %d, want 10000", got)
	}
}
