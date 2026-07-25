package coordinator

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/whatgate/whatgate/internal/authn"
	"github.com/whatgate/whatgate/internal/persist"
	"github.com/whatgate/whatgate/internal/trust"
)

// authMaxSkew is how far a signed request's timestamp may be from the
// coordinator's clock (anti-replay window).
const authMaxSkew = 2 * time.Minute

// maxBodyBytes caps a request body — the JSON messages here are tiny, so this
// just stops a client from streaming an unbounded body at the coordinator.
const maxBodyBytes = 64 << 10

// maxRegisterAddrs bounds how many multiaddrs a node may advertise, so a
// malicious registrant can't bloat the directory (which is broadcast to all).
const maxRegisterAddrs = 32

// decodeJSON reads a size-limited JSON body into v, returning false (and writing
// a 400) on any error.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return false
	}
	return true
}

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

type reportRequest struct {
	Subject string           `json:"subject"` // peer the outcome is about
	Outcome string           `json:"outcome"` // trust.Outcome wire value
	Auth    authn.SignedAuth `json:"auth"`    // the reporting exit signs
}

type reputationResponse struct {
	Score int `json:"score"`
}

type groupsResponse struct {
	Groups []string `json:"groups"`
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
	rep     *trust.Reputation
	now     func() time.Time

	mu           sync.Mutex
	groupSecrets map[string]string // groupID -> join secret
	statePath    string            // if set, durable state is snapshotted here

	sigMu    sync.Mutex
	seenSigs map[string]time.Time // signature -> expiry, for replay rejection
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
		rep:          trust.NewReputation(),
		now:          time.Now,
		groupSecrets: make(map[string]string),
		seenSigs:     make(map[string]time.Time),
	}
}

// Reputation exposes the coordinator's reputation store.
func (s *Server) Reputation() *trust.Reputation { return s.rep }

// DecayReputation decays all reputation scores toward zero by step and persists.
// Run periodically so punishments fade and stale scores don't linger forever.
func (s *Server) DecayReputation(step int) {
	s.rep.Decay(step)
	s.save()
}

// SetStatePath enables durable persistence: after each state-changing request,
// the full snapshot is written to path (atomically).
func (s *Server) SetStatePath(path string) { s.statePath = path }

// Snapshot gathers the coordinator's durable state.
func (s *Server) Snapshot() persist.Snapshot {
	inv, adm := s.invites.Export()
	groups, endorse := s.graph.Export()
	prep, grep := s.rep.Export()
	s.mu.Lock()
	secrets := make(map[string]string, len(s.groupSecrets))
	for k, v := range s.groupSecrets {
		secrets[k] = v
	}
	s.mu.Unlock()
	return persist.Snapshot{
		Invites:         inv,
		Admissions:      adm,
		Groups:          groups,
		Endorsements:    endorse,
		GroupSecrets:    secrets,
		PeerReputation:  prep,
		GroupReputation: grep,
	}
}

// LoadSnapshot restores durable state from snap.
func (s *Server) LoadSnapshot(snap persist.Snapshot) {
	s.invites.Import(snap.Invites, snap.Admissions)
	s.graph.Import(snap.Groups, snap.Endorsements)
	s.rep.Import(snap.PeerReputation, snap.GroupReputation)
	s.mu.Lock()
	for k, v := range snap.GroupSecrets {
		s.groupSecrets[k] = v
	}
	s.mu.Unlock()
}

// save persists the current snapshot if a state path is configured.
func (s *Server) save() {
	if s.statePath == "" {
		return
	}
	if err := persist.Save(s.statePath, s.Snapshot()); err != nil {
		log.Printf("[coordinator] persist state: %v", err)
	}
}

// checkAuth verifies that a proves ownership of claimedPeerID for action, and
// rejects a signature already seen within the replay window.
func (s *Server) checkAuth(a authn.SignedAuth, action, claimedPeerID string) error {
	if a.PeerID != claimedPeerID {
		return errors.New("auth: peer ID does not match request")
	}
	if err := authn.Verify(a, action, s.now(), authMaxSkew); err != nil {
		return err
	}
	if s.seenSignature(a.Signature) {
		return errors.New("auth: replayed signature")
	}
	return nil
}

// seenSignature records sig and reports whether it was already seen (and still
// within its replay window). Ed25519 signatures are deterministic, so a replayed
// request carries an identical signature. Expired entries are pruned lazily.
func (s *Server) seenSignature(sig string) bool {
	now := s.now()
	s.sigMu.Lock()
	defer s.sigMu.Unlock()
	if exp, ok := s.seenSigs[sig]; ok && now.Before(exp) {
		return true
	}
	// Keep an entry until the far edge of the acceptance window so a captured
	// request can't be replayed after its own timestamp check would still pass.
	s.seenSigs[sig] = now.Add(authMaxSkew)
	if len(s.seenSigs) > 1 {
		for k, exp := range s.seenSigs {
			if !now.Before(exp) {
				delete(s.seenSigs, k)
			}
		}
	}
	return false
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
	mux.HandleFunc("/report", s.handleReport)
	mux.HandleFunc("/reputation", s.handleReputation)
	mux.HandleFunc("/groups", s.handleGroups)
	return mux
}

// handleGroups returns the groups a peer belongs to.
func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	groups := s.graph.GroupsOf(r.URL.Query().Get("peer"))
	if groups == nil {
		groups = []string{}
	}
	writeJSON(w, http.StatusOK, groupsResponse{Groups: groups})
}

// handleReport applies a reputation change based on an exit's report about a
// requester's behavior. The reporter must be an admitted, authenticated member
// (reports are attributable — see backlog for rate-limiting/weighting).
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req reportRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	action := "report:" + req.Subject + ":" + req.Outcome
	if err := s.checkAuth(req.Auth, action, req.Auth.PeerID); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if _, admitted := s.invites.AdmissionOf(req.Auth.PeerID); !admitted {
		http.Error(w, "only admitted members may report", http.StatusForbidden)
		return
	}
	outcome, err := trust.ParseOutcome(req.Outcome)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	delta := outcome.Delta()
	s.rep.AdjustPeer(req.Subject, delta)
	for _, g := range s.graph.GroupsOf(req.Subject) {
		s.rep.AdjustGroup(g, delta)
	}
	s.save()
	w.WriteHeader(http.StatusOK)
}

// handleReputation returns a peer's current reputation score.
func (s *Server) handleReputation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	peerID := r.URL.Query().Get("peer")
	writeJSON(w, http.StatusOK, reputationResponse{Score: s.rep.PeerScore(peerID)})
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
	if !decodeJSON(w, r, &req) {
		return
	}
	// The signer may only add itself (signer == peerID).
	action := "group/join:" + req.GroupID + ":" + req.PeerID
	if err := s.checkAuth(req.Auth, action, req.PeerID); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	s.mu.Lock()
	secret, exists := s.groupSecrets[req.GroupID]
	if !exists {
		s.groupSecrets[req.GroupID] = req.Secret
	}
	s.mu.Unlock()

	switch {
	case !exists:
		s.graph.CreateGroup(req.GroupID, req.PeerID) // founder
	case secret == req.Secret:
		s.graph.AddMember(req.GroupID, req.PeerID)
	default:
		http.Error(w, "wrong group secret", http.StatusForbidden)
		return
	}
	s.save()
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
	if !decodeJSON(w, r, &req) {
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
	s.save()
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
	if !decodeJSON(w, r, &req) {
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
	s.save() // admission changed
	writeJSON(w, http.StatusOK, joinResponse{Issuer: issuer})
}

// handleRegister records a node's presence. The node must already be admitted.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req registerRequest
	if !decodeJSON(w, r, &req) {
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

	if len(req.Addrs) > maxRegisterAddrs {
		http.Error(w, "too many addresses", http.StatusBadRequest)
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
