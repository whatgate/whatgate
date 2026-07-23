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
	}, Controls{})
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
	srv := NewServer(func() Status { return Status{} }, Controls{})
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

func TestSwitchEndpointInvokesControl(t *testing.T) {
	var gotRegion string
	srv := NewServer(func() Status { return Status{} }, Controls{
		SwitchRegion: func(region string) (string, error) {
			gotRegion = region
			return "newExitID", nil
		},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/switch", "application/json", strings.NewReader(`{"region":"US"}`))
	if err != nil {
		t.Fatalf("POST /api/switch: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if gotRegion != "US" {
		t.Fatalf("switched region = %q, want US", gotRegion)
	}
	if !strings.Contains(string(body), "newExitID") {
		t.Fatalf("response = %s", body)
	}
}

func TestSwitchUnavailableIsBadRequest(t *testing.T) {
	srv := NewServer(func() Status { return Status{} }, Controls{}) // no SwitchRegion
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/switch", "application/json", strings.NewReader(`{"region":"US"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestExitToggleInvokesControl(t *testing.T) {
	var got, called bool
	srv := NewServer(func() Status { return Status{} }, Controls{
		ToggleExit: func(enabled bool) error { got, called = enabled, true; return nil },
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/exit", "application/json", strings.NewReader(`{"enabled":true}`))
	if err != nil {
		t.Fatalf("POST /api/exit: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !called || !got {
		t.Fatalf("ToggleExit not invoked with true (called=%v got=%v)", called, got)
	}
}

func TestSetupInvokesControl(t *testing.T) {
	var gotScope string
	srv := NewServer(func() Status { return Status{} }, Controls{
		Setup: func(scope string) (string, error) { gotScope = scope; return "exitX", nil },
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/setup", "application/json", strings.NewReader(`{"scope":"open"}`))
	if err != nil {
		t.Fatalf("POST /api/setup: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if gotScope != "open" || !strings.Contains(string(body), "exitX") {
		t.Fatalf("setup not invoked properly (scope=%q body=%s)", gotScope, body)
	}
}

func TestUnknownPathIs404(t *testing.T) {
	srv := NewServer(func() Status { return Status{} }, Controls{})
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
