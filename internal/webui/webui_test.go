package webui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStatusEndpointReturnsProviderState(t *testing.T) {
	srv := NewServer(func() Status {
		return Status{PeerID: "12D3KooExample", Role: "client+exit", ExitEnabled: true, ExitLoad: 3, ToRegion: "JP"}
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatalf("GET /api/status: %v", err)
	}
	defer resp.Body.Close()

	var s Status
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.PeerID != "12D3KooExample" || !s.ExitEnabled || s.ExitLoad != 3 || s.ToRegion != "JP" {
		t.Fatalf("status = %+v", s)
	}
}

func TestDashboardServesHTML(t *testing.T) {
	srv := NewServer(func() Status { return Status{} })
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type = %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "WhatGate") {
		t.Fatal("dashboard should mention WhatGate")
	}
}

func TestUnknownPathIs404(t *testing.T) {
	srv := NewServer(func() Status { return Status{} })
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/nope")
	if err != nil {
		t.Fatalf("GET /nope: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
