package coordinator

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Wire types for the coordinator HTTP/JSON API.
type joinRequest struct {
	Code   string `json:"code"`
	PeerID string `json:"peerID"`
}

type joinResponse struct {
	Issuer string `json:"issuer"`
}

type registerRequest struct {
	PeerID   string   `json:"peerID"`
	Addrs    []string `json:"addrs"`
	Region   string   `json:"region"`
	WantExit bool     `json:"wantExit"`
}

type directoryEntry struct {
	PeerID   string   `json:"peerID"`
	Addrs    []string `json:"addrs"`
	Region   string   `json:"region"`
	WantExit bool     `json:"wantExit"`
}

// Server exposes the coordinator's directory and admission over HTTP/JSON. It is
// the control plane only: nodes register presence and discover exits here, but
// proxied traffic never touches it.
type Server struct {
	dir     *Directory
	invites *InviteStore
}

// NewServer builds a coordinator HTTP server over the given directory and
// invite store.
func NewServer(dir *Directory, invites *InviteStore) *Server {
	return &Server{dir: dir, invites: invites}
}

// Handler returns the HTTP handler exposing the coordinator API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/join", s.handleJoin)
	mux.HandleFunc("/register", s.handleRegister)
	mux.HandleFunc("/directory", s.handleDirectory)
	return mux
}

// handleJoin admits a peer by redeeming an invite code.
func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req joinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	issuer, err := s.invites.Redeem(req.Code, req.PeerID)
	switch {
	case errors.Is(err, ErrUnknownInvite):
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	case errors.Is(err, ErrInviteExhausted):
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, joinResponse{Issuer: issuer})
}

// handleRegister records a node's presence. The node must already be admitted.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if _, admitted := s.invites.AdmissionOf(req.PeerID); !admitted {
		http.Error(w, "not admitted; redeem an invite first", http.StatusForbidden)
		return
	}

	s.dir.Register(NodeInfo{
		PeerID:   req.PeerID,
		Addrs:    req.Addrs,
		Region:   req.Region,
		WantExit: req.WantExit,
	})
	w.WriteHeader(http.StatusOK)
}

// handleDirectory returns the live node directory.
func (s *Server) handleDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	nodes := s.dir.List()
	entries := make([]directoryEntry, 0, len(nodes))
	for _, n := range nodes {
		entries = append(entries, directoryEntry{
			PeerID:   n.PeerID,
			Addrs:    n.Addrs,
			Region:   n.Region,
			WantExit: n.WantExit,
		})
	}
	writeJSON(w, http.StatusOK, entries)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
