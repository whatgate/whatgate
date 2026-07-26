// Package metrics is a tiny, dependency-free counter registry for operational
// visibility — how many requests an exit served vs denied (and why), routing
// decisions, and the like. It intentionally avoids a Prometheus dependency: a
// flat name→count map served as JSON keeps the binary small (distribution
// simplicity) while still being scrapeable and human-readable. Callers compose
// descriptive names (e.g. "exit_denied:untrusted") since there are no labels.
package metrics

import (
	"encoding/json"
	"net/http"
	"sync"
)

// Registry is a concurrency-safe set of named monotonic counters.
type Registry struct {
	mu       sync.Mutex
	counters map[string]int64
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{counters: make(map[string]int64)}
}

// Inc adds one to the named counter.
func (r *Registry) Inc(name string) { r.Add(name, 1) }

// Add adds n to the named counter.
func (r *Registry) Add(name string, n int64) {
	r.mu.Lock()
	r.counters[name] += n
	r.mu.Unlock()
}

// Snapshot returns a copy of all counters, safe for the caller to mutate.
func (r *Registry) Snapshot() map[string]int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int64, len(r.counters))
	for k, v := range r.counters {
		out[k] = v
	}
	return out
}

// Handler serves the current counters as a JSON object. encoding/json emits map
// keys sorted, so the output is stable across scrapes.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(r.Snapshot())
	})
}
