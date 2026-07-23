package coordinator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/whatgate/whatgate/internal/authn"
	"github.com/whatgate/whatgate/internal/trust"
)

// Client is a node's HTTP client for the coordinator control plane: it admits
// the node (Join), advertises its presence (Register), and discovers exits
// (Directory).
type Client struct {
	baseURL string
	http    *http.Client

	// Signer produces the signed authentication attached to identity-proving
	// requests (join/register). It must sign with the private key of the peer ID
	// used in those requests. If nil, such requests go unsigned and the
	// coordinator will reject them.
	Signer func(action string) (authn.SignedAuth, error)
}

// NewClient creates a coordinator client targeting baseURL (e.g.
// "http://coordinator.example:8080").
func NewClient(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: http.DefaultClient}
}

func (c *Client) sign(action string) (authn.SignedAuth, error) {
	if c.Signer == nil {
		return authn.SignedAuth{}, nil
	}
	return c.Signer(action)
}

// Join redeems an invite code to admit peerID, returning the vouching issuer.
func (c *Client) Join(code, peerID string) (issuer string, err error) {
	auth, err := c.sign("join")
	if err != nil {
		return "", err
	}
	var resp joinResponse
	if err := c.postJSON("/join", joinRequest{Code: code, PeerID: peerID, Auth: auth}, &resp); err != nil {
		return "", err
	}
	return resp.Issuer, nil
}

// Register advertises the node's presence in the directory.
func (c *Client) Register(info NodeInfo) error {
	auth, err := c.sign("register")
	if err != nil {
		return err
	}
	return c.postJSON("/register", registerRequest{
		PeerID:   info.PeerID,
		Addrs:    info.Addrs,
		Region:   info.Region,
		WantExit: info.WantExit,
		Load:     info.Load,
		Auth:     auth,
	}, nil)
}

// Directory fetches the current live node directory.
func (c *Client) Directory() ([]NodeInfo, error) {
	nodes, _, err := c.DirectoryFor("")
	return nodes, err
}

// DirectoryFor fetches the directory annotated with each node's trust tier
// relative to the given peer (from). The returned map is keyed by peer ID; use
// it as the tierOf lookup for trust-scoped exit selection. Pass from="" to skip
// annotation (all tiers default to stranger).
func (c *Client) DirectoryFor(from string) ([]NodeInfo, map[string]trust.Tier, error) {
	u := c.baseURL + "/directory"
	if from != "" {
		u += "?from=" + url.QueryEscape(from)
	}
	resp, err := c.http.Get(u)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, statusError("GET /directory", resp)
	}

	var entries []directoryEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, nil, err
	}
	nodes := make([]NodeInfo, 0, len(entries))
	tiers := make(map[string]trust.Tier, len(entries))
	for _, e := range entries {
		nodes = append(nodes, NodeInfo{
			PeerID:   e.PeerID,
			Addrs:    e.Addrs,
			Region:   e.Region,
			WantExit: e.WantExit,
			Load:     e.Load,
		})
		tiers[e.PeerID] = e.Tier
	}
	return nodes, tiers, nil
}

// TrustBetween returns the trust tier from one peer toward another, as computed
// by the coordinator's authoritative graph.
func (c *Client) TrustBetween(from, to string) (trust.Tier, error) {
	u := c.baseURL + "/trust?from=" + url.QueryEscape(from) + "&to=" + url.QueryEscape(to)
	resp, err := c.http.Get(u)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, statusError("GET /trust", resp)
	}
	var out trustResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.Tier, nil
}

// JoinGroup joins (or, for the first member, creates) a small-network group.
// The caller must be the peer identified by peerID (its Signer signs the join),
// and must present the group's secret; the first join sets the secret.
func (c *Client) JoinGroup(groupID, peerID, secret string) error {
	auth, err := c.sign("group/join:" + groupID + ":" + peerID)
	if err != nil {
		return err
	}
	return c.postJSON("/group/join", joinGroupRequest{
		GroupID: groupID,
		PeerID:  peerID,
		Secret:  secret,
		Auth:    auth,
	}, nil)
}

// EndorseGroup records that fromGroup vouches for toGroup. The caller must sign
// as a member of fromGroup.
func (c *Client) EndorseGroup(fromGroup, toGroup string) error {
	auth, err := c.sign("group/endorse:" + fromGroup + ":" + toGroup)
	if err != nil {
		return err
	}
	return c.postJSON("/group/endorse", endorseRequest{
		FromGroup: fromGroup,
		ToGroup:   toGroup,
		Auth:      auth,
	}, nil)
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
