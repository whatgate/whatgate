package coordinator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Client is a node's HTTP client for the coordinator control plane: it admits
// the node (Join), advertises its presence (Register), and discovers exits
// (Directory).
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient creates a coordinator client targeting baseURL (e.g.
// "http://coordinator.example:8080").
func NewClient(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: http.DefaultClient}
}

// Join redeems an invite code to admit peerID, returning the vouching issuer.
func (c *Client) Join(code, peerID string) (issuer string, err error) {
	var resp joinResponse
	if err := c.postJSON("/join", joinRequest{Code: code, PeerID: peerID}, &resp); err != nil {
		return "", err
	}
	return resp.Issuer, nil
}

// Register advertises the node's presence in the directory.
func (c *Client) Register(info NodeInfo) error {
	return c.postJSON("/register", registerRequest{
		PeerID:   info.PeerID,
		Addrs:    info.Addrs,
		Region:   info.Region,
		WantExit: info.WantExit,
	}, nil)
}

// Directory fetches the current live node directory.
func (c *Client) Directory() ([]NodeInfo, error) {
	resp, err := c.http.Get(c.baseURL + "/directory")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("GET /directory", resp)
	}

	var entries []directoryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}
	nodes := make([]NodeInfo, 0, len(entries))
	for _, e := range entries {
		nodes = append(nodes, NodeInfo{
			PeerID:   e.PeerID,
			Addrs:    e.Addrs,
			Region:   e.Region,
			WantExit: e.WantExit,
		})
	}
	return nodes, nil
}

// ErrNoRelay is returned by Relay when the coordinator advertises none.
var ErrNoRelay = errors.New("coordinator: no relay configured")

// Relay fetches the coordinator's advertised relay, or ErrNoRelay if absent.
func (c *Client) Relay() (RelayInfo, error) {
	resp, err := c.http.Get(c.baseURL + "/relay")
	if err != nil {
		return RelayInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return RelayInfo{}, ErrNoRelay
	}
	if resp.StatusCode != http.StatusOK {
		return RelayInfo{}, statusError("GET /relay", resp)
	}
	var info RelayInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return RelayInfo{}, err
	}
	return info, nil
}

// postJSON POSTs body as JSON to path and, if out is non-nil, decodes the
// response into it. Non-2xx responses become errors.
func (c *Client) postJSON(path string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := c.http.Post(c.baseURL+path, "application/json", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return statusError("POST "+path, resp)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func statusError(op string, resp *http.Response) error {
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("%s: %s: %s", op, resp.Status, bytes.TrimSpace(msg))
}
