// Package webui serves a small local dashboard for a running WhatGate node: a
// status page and a JSON endpoint showing live state (identity, role, exit
// status, connected exit, region, trust scope). It is read-only for now;
// runtime controls (toggle exit, switch region) are a planned follow-up.
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
	PeerID        string `json:"peerID"`
	Role          string `json:"role"`        // "exit", "client", "client+exit", or "idle"
	Coordinator   string `json:"coordinator"` // coordinator URL, or "" (manual/none)
	ExitEnabled   bool   `json:"exitEnabled"`
	ExitRegion    string `json:"exitRegion"`
	ExitLoad      int    `json:"exitLoad"`
	ToRegion      string `json:"toRegion"`      // desired exit region (client)
	TrustScope    string `json:"trustScope"`    // client trust scope
	ConnectedExit string `json:"connectedExit"` // exit peer ID the client tunnels through
	SocksAddr     string `json:"socksAddr"`     // local SOCKS listen address (client)
	Uptime        string `json:"uptime"`
}

// Server serves the dashboard over HTTP.
type Server struct {
	status func() Status
}

// NewServer creates a dashboard server backed by a live status provider.
func NewServer(status func() Status) *Server {
	return &Server{status: status}
}

// Handler returns the HTTP handler for the dashboard.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.status())
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
