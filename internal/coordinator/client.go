package coordinator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/whatgate/whatgate/internal/authn"
	"github.com/whatgate/whatgate/internal/discovery"
	"github.com/whatgate/whatgate/internal/trust"
)

// Client is a node's HTTP client for the coordinator control plane: it admits
// the node (Join), advertises its presence (Register), and discovers exits
// (Directory).
type Client struct {
	http *http.Client

	epMu      sync.Mutex
	endpoints []string // one or more coordinator base URLs (failover order)
	current   int      // index of the last-known-reachable endpoint

	// Signer produces the signed authentication attached to identity-proving
	// requests (join/register). It must sign with the private key of the peer ID
	// used in those requests. If nil, such requests go unsigned and the
	// coordinator will reject them.
	Signer func(action string) (authn.SignedAuth, error)

	// pinnedKey, when set, is the control-plane public key that discovery
	// responses must be signed by. A pinned client verifies every directory and
	// refuses any that is unsigned or signed by a different key — so a
	// reachable-but-rogue endpoint cannot inject a poisoned directory.
	pinnedKey crypto.PubKey
	dirMu     sync.Mutex
	dirFloor  uint64 // highest directory serial accepted so far (anti-rollback)
}

// SetPinnedKey pins the control-plane key that signed discovery responses must
// verify against. The key is distributed out of band (embedded seed / operator
// config). With no pinned key the client accepts the legacy unsigned directory.
func (c *Client) SetPinnedKey(pub crypto.PubKey) { c.pinnedKey = pub }

// NewClient creates a coordinator client targeting a single baseURL (e.g.
// "https://coordinator.example:8080").
func NewClient(baseURL string) *Client {
	return NewClientEndpoints([]string{baseURL})
}

// NewClientEndpoints creates a client that fails over across several coordinator
// base URLs. A request is tried against the last-known-reachable endpoint first;
// on a connection-level failure (not an HTTP error response) it rotates to the
// next. This survives the blocking of any single coordinator address. For real
// independence the endpoints should span different fault domains (ASN / DNS host
// / CDN / operator) — that is an operational property, not enforced here.
func NewClientEndpoints(baseURLs []string) *Client {
	eps := make([]string, 0, len(baseURLs))
	for _, u := range baseURLs {
		if u != "" {
			eps = append(eps, u)
		}
	}
	return &Client{http: http.DefaultClient, endpoints: eps}
}

// attempt runs do against each endpoint, starting from the last-known-reachable
// one, rotating only when do reports a connection-level error (nil response). A
// received HTTP response — even an error status — means the endpoint is
// reachable, so the caller's business error is surfaced rather than masked by
// failover. On success the reachable endpoint is remembered.
func (c *Client) attempt(op string, do func(base string) (*http.Response, error)) (*http.Response, error) {
	c.epMu.Lock()
	start, eps := c.current, append([]string(nil), c.endpoints...)
	c.epMu.Unlock()
	if len(eps) == 0 {
		return nil, fmt.Errorf("%s: no coordinator endpoint configured", op)
	}

	var lastErr error
	for i := 0; i < len(eps); i++ {
		idx := (start + i) % len(eps)
		resp, err := do(eps[idx])
		if err != nil {
			lastErr = err
			continue
		}
		c.epMu.Lock()
		c.current = idx
		c.epMu.Unlock()
		return resp, nil
	}
	return nil, fmt.Errorf("%s: all %d coordinator endpoints unreachable: %w", op, len(eps), lastErr)
}

// get performs a failover-aware GET of pathAndQuery (e.g. "/directory?from=x").
func (c *Client) get(op, pathAndQuery string) (*http.Response, error) {
	return c.attempt(op, func(base string) (*http.Response, error) {
		return c.http.Get(base + pathAndQuery)
	})
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
	path := "/directory"
	if from != "" {
		path += "?from=" + url.QueryEscape(from)
	}
	resp, err := c.get("GET /directory", path)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, statusError("GET /directory", resp)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	// A pinned client requires a verified, non-rolled-back signed envelope and
	// uses its payload as the entries; an unpinned client reads the bare array.
	entriesJSON := body
	if c.pinnedKey != nil {
		payload, err := c.verifyDirectory(body)
		if err != nil {
			return nil, nil, err
		}
		entriesJSON = payload
	}

	var entries []directoryEntry
	if err := json.Unmarshal(entriesJSON, &entries); err != nil {
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

// verifyDirectory checks a signed directory envelope against the pinned key and
// the client's rollback floor, advancing the floor on success and returning the
// verified entries payload.
func (c *Client) verifyDirectory(body []byte) ([]byte, error) {
	var signed discovery.Signed
	if err := json.Unmarshal(body, &signed); err != nil {
		return nil, fmt.Errorf("directory: not a signed envelope: %w", err)
	}
	c.dirMu.Lock()
	floor := c.dirFloor
	c.dirMu.Unlock()

	payload, err := signed.Verify(c.pinnedKey, discovery.TypeDirectory, time.Now(), floor)
	if err != nil {
		return nil, fmt.Errorf("directory: %w", err)
	}

	c.dirMu.Lock()
	if signed.Serial > c.dirFloor {
		c.dirFloor = signed.Serial
	}
	c.dirMu.Unlock()
	return payload, nil
}

// ReportOutcome reports (signed) an outcome about a requester's behavior, which
// the coordinator turns into a reputation change.
func (c *Client) ReportOutcome(subject string, outcome trust.Outcome) error {
	auth, err := c.sign("report:" + subject + ":" + string(outcome))
	if err != nil {
		return err
	}
	return c.postJSON("/report", reportRequest{
		Subject: subject,
		Outcome: string(outcome),
		Auth:    auth,
	}, nil)
}

// ReputationOf returns a peer's current reputation score.
func (c *Client) ReputationOf(peerID string) (int, error) {
	resp, err := c.get("GET /reputation", "/reputation?peer="+url.QueryEscape(peerID))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, statusError("GET /reputation", resp)
	}
	var out reputationResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.Score, nil
}

// TrustBetween returns the trust tier from one peer toward another, as computed
// by the coordinator's authoritative graph.
func (c *Client) TrustBetween(from, to string) (trust.Tier, error) {
	resp, err := c.get("GET /trust", "/trust?from="+url.QueryEscape(from)+"&to="+url.QueryEscape(to))
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

// GroupsOf returns the groups a peer belongs to.
func (c *Client) GroupsOf(peerID string) ([]string, error) {
	resp, err := c.get("GET /groups", "/groups?peer="+url.QueryEscape(peerID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("GET /groups", resp)
	}
	var out groupsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Groups, nil
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
	resp, err := c.get("GET /relay", "/relay")
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
	// Failover only happens on connection-level errors, i.e. before the request
	// reaches a coordinator — so a write is never re-applied by an endpoint that
	// already received it. A fresh reader is built per attempt.
	resp, err := c.attempt("POST "+path, func(base string) (*http.Response, error) {
		return c.http.Post(base+path, "application/json", bytes.NewReader(buf))
	})
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
