// Package persist snapshots the coordinator's durable state to a single JSON
// file so it survives restarts. The directory is intentionally excluded — it is
// short-lived (nodes re-register), so it rebuilds itself. What must persist is
// admissions, the trust graph, group secrets, and reputation; losing those
// would lock out admitted peers or erase small-networks.
//
// A JSON snapshot (atomic temp+rename write) keeps the zero-dependency,
// single-binary ethos. If state ever grows large or high-frequency, swap this
// for an embedded key-value store (e.g. bbolt) behind the same Load/Save API.
package persist

import (
	"encoding/json"
	"errors"
	"os"
)

// InviteRecord is a persisted invite code.
type InviteRecord struct {
	Issuer  string `json:"issuer"`
	MaxUses int    `json:"maxUses"`
	Uses    int    `json:"uses"`
}

// AdmissionRecord is a persisted admission (who joined via which invite).
type AdmissionRecord struct {
	PeerID string `json:"peerID"`
	Code   string `json:"code"`
	Issuer string `json:"issuer"`
	At     int64  `json:"at"` // unix seconds
}

// Snapshot is the coordinator's complete durable state.
type Snapshot struct {
	Invites         map[string]InviteRecord    `json:"invites"`
	Admissions      map[string]AdmissionRecord `json:"admissions"`
	Groups          map[string][]string        `json:"groups"`       // groupID -> members
	Endorsements    map[string][]string        `json:"endorsements"` // fromGroup -> toGroups
	GroupSecrets    map[string]string          `json:"groupSecrets"`
	PeerReputation  map[string]int             `json:"peerReputation"`
	GroupReputation map[string]int             `json:"groupReputation"`
}

// Load reads a snapshot from path. A missing file yields an empty snapshot and
// no error (first run).
func Load(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, nil
	}
	if err != nil {
		return Snapshot{}, err
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return Snapshot{}, err
	}
	return s, nil
}

// Save writes s to path atomically (write temp file, then rename).
func Save(path string, s Snapshot) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
