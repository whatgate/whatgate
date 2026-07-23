// Package audit records what an exit node served, for after-the-fact
// accountability. Each request (served or denied) becomes one JSON line
// appended to a local file, so an operator — or an investigation, via the
// coordinator's traceable invite chain — can see who sent what through their IP.
//
// Append-only JSON Lines keeps writes cheap and crash-resilient (no rewrite).
package audit

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Entry is one audited exit request.
type Entry struct {
	Time      time.Time `json:"time"`
	Requester string    `json:"requester"` // requester peer ID
	Target    string    `json:"target"`    // host:port
	Outcome   string    `json:"outcome"`   // "served" or "denied: <reason>"
}

// FileLogger appends audit entries as JSON Lines to a file.
type FileLogger struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

// NewFileLogger opens (creating/appending) the audit log at path.
func NewFileLogger(path string) (*FileLogger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &FileLogger{f: f, enc: json.NewEncoder(f)}, nil
}

// Log appends one entry as a JSON line (json.Encoder.Encode adds the newline).
func (l *FileLogger) Log(e Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.enc.Encode(e)
}

// Close closes the underlying file.
func (l *FileLogger) Close() error {
	if l.f == nil {
		return nil
	}
	return l.f.Close()
}
