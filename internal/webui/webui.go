// Package webui serves a small local dashboard for a running WhatGate node: a
// live status page plus runtime controls — switch exit region, toggle serving as
// an exit, the first-run trust wizard, and small-network management — exposed as
// JSON endpoints driven by injected Controls callbacks.
//
// The page is self-contained (no external assets) and meant to be bound to
// localhost only.
package webui

import (
	"encoding/json"
	"net/http"
)

// Status is the live node state shown by the dashboard.
type Status struct {
	PeerID        string   `json:"peerID"`
	Role          string   `json:"role"`        // "exit", "client", "client+exit", or "idle"
	Coordinator   string   `json:"coordinator"` // coordinator URL, or "" (manual/none)
	ExitEnabled   bool     `json:"exitEnabled"`
	ExitRegion    string   `json:"exitRegion"`
	ExitLoad      int      `json:"exitLoad"`
	ToRegion      string   `json:"toRegion"`      // desired exit region (client)
	TrustScope    string   `json:"trustScope"`    // client trust scope
	ConnectedExit string   `json:"connectedExit"` // exit peer ID the client tunnels through
	SocksAddr     string   `json:"socksAddr"`     // local SOCKS listen address (client)
	CanSwitch     bool     `json:"canSwitch"`     // whether runtime region switching is available
	CanToggleExit bool     `json:"canToggleExit"` // whether runtime exit on/off is available
	NeedsSetup    bool     `json:"needsSetup"`    // first-run: user must choose a trust scope
	Groups        []string `json:"groups"`        // small-networks this node belongs to
	CanManage     bool     `json:"canManage"`     // whether group management is available
	Uptime        string   `json:"uptime"`
}

// Controls are runtime actions the dashboard can trigger. Nil callbacks are
// treated as unsupported.
type Controls struct {
	// SwitchRegion re-selects an exit in the given region and re-points the local
	// SOCKS proxy at it, returning the new exit's peer ID.
	SwitchRegion func(region string) (newExitID string, err error)
	// ToggleExit turns this node's exit service on or off at runtime.
	ToggleExit func(enabled bool) error
	// Setup completes the first-run trust wizard: it applies the chosen scope
	// ("conservative"/"open"), selects an exit, and returns its peer ID.
	Setup func(scope string) (exitID string, err error)
	// JoinGroup joins (or founds) a small-network with the given secret.
	JoinGroup func(groupID, secret string) error
	// Endorse makes fromGroup vouch for toGroup (caller must be a fromGroup member).
	Endorse func(fromGroup, toGroup string) error
}

// Server serves the dashboard over HTTP.
type Server struct {
	status   func() Status
	controls Controls
}

// NewServer creates a dashboard server backed by a live status provider and
// optional runtime controls.
func NewServer(status func() Status, controls Controls) *Server {
	return &Server{status: status, controls: controls}
}

// Handler returns the HTTP handler for the dashboard.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.status())
	})
	mux.HandleFunc("/api/switch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.controls.SwitchRegion == nil {
			http.Error(w, "region switching not available on this node", http.StatusBadRequest)
			return
		}
		var req struct {
			Region string `json:"region"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		exitID, err := s.controls.SwitchRegion(req.Region)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"exit": exitID})
	})
	mux.HandleFunc("/api/group/join", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.controls.JoinGroup == nil {
			http.Error(w, "group management not available", http.StatusBadRequest)
			return
		}
		var req struct {
			GroupID string `json:"groupID"`
			Secret  string `json:"secret"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := s.controls.JoinGroup(req.GroupID, req.Secret); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/group/endorse", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.controls.Endorse == nil {
			http.Error(w, "group management not available", http.StatusBadRequest)
			return
		}
		var req struct {
			FromGroup string `json:"fromGroup"`
			ToGroup   string `json:"toGroup"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := s.controls.Endorse(req.FromGroup, req.ToGroup); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/setup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.controls.Setup == nil {
			http.Error(w, "setup not available on this node", http.StatusBadRequest)
			return
		}
		var req struct {
			Scope string `json:"scope"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		exitID, err := s.controls.Setup(req.Scope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"exit": exitID})
	})
	mux.HandleFunc("/api/exit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.controls.ToggleExit == nil {
			http.Error(w, "exit toggle not available on this node", http.StatusBadRequest)
			return
		}
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := s.controls.ToggleExit(req.Enabled); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"exitEnabled": req.Enabled})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(dashboardHTML))
	})
	return mux
}
