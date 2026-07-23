package coordinator

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/whatgate/whatgate/internal/authn"
	"github.com/whatgate/whatgate/internal/trust"
)

// authMaxSkew is how far a signed request's timestamp may be from the
// coordinator's clock (anti-replay window).
const authMaxSkew = 2 * time.Minute

// Wire types for the coordinator HTTP/JSON API.
type joinRequest struct {
	Code   string           `json:"code"`
	PeerID string           `json:"peerID"`
	Auth   authn.SignedAuth `json:"auth"`
}

type joinResponse struct {
	Issuer string `json:"issuer"`
}

type registerRequest struct {
	PeerID   string           `json:"peerID"`
	Addrs    []string         `json:"addrs"`
	Region   string           `json:"region"`
	WantExit bool             `json:"wantExit"`
	Load     int              `json:"load"`
	Auth     authn.SignedAuth `json:"auth"`
}

type directoryEntry struct {
	PeerID   string     `json:"peerID"`
	Addrs    []string   `json:"addrs"`
	Region   string     `json:"region"`
	WantExit bool       `json:"wantExit"`
	Load     int        `json:"load"`
	Tier     trust.Tier `json:"tier"` // trust tier relative to the ?from peer
}

type joinGroupRequest struct {
	GroupID string           `json:"groupID"`
	PeerID  string           `json:"peerID"`
	Secret  string           `json:"secret"`
	Auth    authn.SignedAuth `json:"auth"`
}

type endorseRequest struct {
	FromGroup string           `json:"fromGroup"`
	ToGroup   string           `json:"toGroup"`
	Auth      authn.SignedAuth `json:"auth"`
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
	now     func() time.Time

	mu           sync.Mutex
	groupSecrets map[string]string // groupID -> join secret
}

// SetRelayInfo advertises a relay through the /relay endpoint.
func (s *Server) SetRelayInfo(peerID string, addrs []string) {
	s.relay = &RelayInfo{PeerID: peerID, Addrs: addrs}
}

// NewServer builds a coordinator HTTP server over the given directory and
// invite store.
func NewServer(dir *Directory, invites *InviteStore) *Server {
	return &Server{
		dir:          dir,
		invites:      invites,
		graph:        trust.NewGraph(),
		now:          time.Now,
		groupSecrets: make(map[string]string),
	}
}

// checkAuth verifies that a proves ownership of claimedPeerID for action.
func (s *Server) checkAuth(a authn.SignedAuth, action, claimedPeerID string) error {
	if a.PeerID != claimedPeerID {
		return errors.New("auth: peer ID does not match request")
	}
	return authn.Verify(a, action, s.now(), authMaxSkew)
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

// handleGroupJoin adds the (authenticated) peer to a small-network group. The
// first join creates the group and sets its secret (the joiner becomes founder);
// subsequent joins must present the matching secret. This authenticates who
// joins (no impersonation) and gates which group (strangers without the secret
// cannot slip into your trust circle).
func (s *Server) handleGroupJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req joinGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// The signer may only add itself (signer == peerID).
	action := "group/join:" + req.GroupID + ":" + req.PeerID
	if err := s.checkAuth(req.Auth, action, req.PeerID); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	secret, exists := s.groupSecrets[req.GroupID]
	switch {
	case !exists:
		s.groupSecrets[req.GroupID] = req.Secret
		s.graph.CreateGroup(req.GroupID, req.PeerID) // founder
	case secret == req.Secret:
		s.graph.AddMember(req.GroupID, req.PeerID)
	default:
		http.Error(w, "wrong group secret", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleGroupEndorse records that fromGroup vouches for toGroup. Only a member
// of fromGroup may make it endorse another group.
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
	action := "group/endorse:" + req.FromGroup + ":" + req.ToGroup
	if err := s.checkAuth(req.Auth, action, req.Auth.PeerID); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if !s.graph.IsMember(req.FromGroup, req.Auth.PeerID) {
		http.Error(w, "endorser must be a member of the endorsing group", http.StatusForbidden)
		return
	}
	s.graph.Endorse(req.FromGroup, req.ToGroup)
	w.WriteHeader(http.StatusOK)
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
	if err := s.checkAuth(req.Auth, "join", req.PeerID); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
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
	if err := s.checkAuth(req.Auth, "register", req.PeerID); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
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
