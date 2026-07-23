package coordinator

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/whatgate/whatgate/internal/trust"
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
	Load     int      `json:"load"`
}

type directoryEntry struct {
	PeerID   string     `json:"peerID"`
	Addrs    []string   `json:"addrs"`
	Region   string     `json:"region"`
	WantExit bool       `json:"wantExit"`
	Load     int        `json:"load"`
	Tier     trust.Tier `json:"tier"` // trust tier relative to the ?from peer
}

type groupMemberRequest struct {
	GroupID string `json:"groupID"`
	PeerID  string `json:"peerID"`
}

type endorseRequest struct {
	FromGroup string `json:"fromGroup"`
	ToGroup   string `json:"toGroup"`
}

type trustResponse struct {
	Tier trust.Tier `json:"tier"`
}

// RelayInfo advertises the coordinator's co-located Circuit Relay so nodes can
// configure it as a fallback path.
type RelayInfo struct {
	PeerID string   `json:"peerID"`
	Addrs  []string `json:"addrs"`
}

// Server exposes the coordinator's directory and admission over HTTP/JSON. It is
// the control plane only: nodes register presence and discover exits here, but
// proxied traffic never touches it.
type Server struct {
	dir     *Directory
	invites *InviteStore
	graph   *trust.Graph
	relay   *RelayInfo // optional co-located relay
}

// SetRelayInfo advertises a relay through the /relay endpoint.
func (s *Server) SetRelayInfo(peerID string, addrs []string) {
	s.relay = &RelayInfo{PeerID: peerID, Addrs: addrs}
}

// NewServer builds a coordinator HTTP server over the given directory and
// invite store.
func NewServer(dir *Directory, invites *InviteStore) *Server {
	return &Server{dir: dir, invites: invites, graph: trust.NewGraph()}
}

// Graph exposes the coordinator's trust graph (authoritative store).
func (s *Server) Graph() *trust.Graph { return s.graph }

// Handler returns the HTTP handler exposing the coordinator API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/join", s.handleJoin)
	mux.HandleFunc("/register", s.handleRegister)
	mux.HandleFunc("/directory", s.handleDirectory)
	mux.HandleFunc("/relay", s.handleRelay)
	mux.HandleFunc("/group/create", s.handleGroupCreate)
	mux.HandleFunc("/group/join", s.handleGroupJoin)
	mux.HandleFunc("/group/endorse", s.handleGroupEndorse)
	mux.HandleFunc("/trust", s.handleTrust)
	return mux
}

// handleTrust returns the trust tier from one peer toward another.
func (s *Server) handleTrust(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	tier := s.graph.Trust(q.Get("from"), q.Get("to"))
	writeJSON(w, http.StatusOK, trustResponse{Tier: tier})
}

// handleGroupCreate creates a group (小网) with the given founder.
func (s *Server) handleGroupCreate(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeGroupMember(w, r)
	if !ok {
		return
	}
	s.graph.CreateGroup(req.GroupID, req.PeerID)
	w.WriteHeader(http.StatusOK)
}

// handleGroupJoin adds a peer to a group.
func (s *Server) handleGroupJoin(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeGroupMember(w, r)
	if !ok {
		return
	}
	s.graph.AddMember(req.GroupID, req.PeerID)
	w.WriteHeader(http.StatusOK)
}

// handleGroupEndorse records that one group vouches for another.
func (s *Server) handleGroupEndorse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req endorseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.graph.Endorse(req.FromGroup, req.ToGroup)
	w.WriteHeader(http.StatusOK)
}

func decodeGroupMember(w http.ResponseWriter, r *http.Request) (groupMemberRequest, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return groupMemberRequest{}, false
	}
	var req groupMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return groupMemberRequest{}, false
	}
	return req, true
}

// handleRelay returns the co-located relay's addressing, or 404 if none.
func (s *Server) handleRelay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.relay == nil {
		http.Error(w, "no relay configured", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, *s.relay)
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
		Load:     req.Load,
	})
	w.WriteHeader(http.StatusOK)
}

// handleDirectory returns the live node directory.
func (s *Server) handleDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	from := r.URL.Query().Get("from")
	nodes := s.dir.List()
	entries := make([]directoryEntry, 0, len(nodes))
	for _, n := range nodes {
		tier := trust.TierStranger
		if from != "" {
			tier = s.graph.Trust(from, n.PeerID)
		}
		entries = append(entries, directoryEntry{
			PeerID:   n.PeerID,
			Addrs:    n.Addrs,
			Region:   n.Region,
			WantExit: n.WantExit,
			Load:     n.Load,
			Tier:     tier,
		})
	}
	writeJSON(w, http.StatusOK, entries)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
