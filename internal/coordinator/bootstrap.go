package coordinator

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/whatgate/whatgate/internal/discovery"
)

// SignBootstrap produces a signed bootstrap object (type=bootstrap) listing the
// given coordinator endpoints, for an operator to publish on an out-of-band
// channel (CDN / GitHub raw / DoH TXT). serial must strictly increase across
// successive publications so a node rejects a rolled-back list; ttl bounds how
// long nodes will accept it. The signing key is the same control-plane key nodes
// pin via -coordinator-key.
func SignBootstrap(priv crypto.PrivKey, serial uint64, endpoints []string, ttl time.Duration) ([]byte, error) {
	payload, err := json.Marshal(BootstrapList{Endpoints: endpoints})
	if err != nil {
		return nil, err
	}
	now := time.Now()
	obj, err := discovery.Sign(priv, discovery.Meta{
		Type:      discovery.TypeBootstrap,
		Serial:    serial,
		IssuedAt:  now,
		ExpiresAt: now.Add(ttl),
	}, payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(obj)
}

// BootstrapList is the payload of a signed out-of-band bootstrap object: the
// operator's current authoritative set of coordinator endpoints. It is
// distributed through a hard-to-block channel (CDN static JSON / GitHub raw /
// DoH TXT) so a node whose known coordinators are all blocked can self-heal
// (Tier C2). Additional fields (relay / DHT bootstrap addresses) can be added
// later; the signed envelope schema (discovery.Signed, type=bootstrap) is frozen
// and shared with the directory and relay objects.
type BootstrapList struct {
	Endpoints []string `json:"endpoints"`
}

// RefreshFromBootstrap fetches a signed bootstrap object from an out-of-band URL
// — a hard-to-block channel such as a CDN static file or a GitHub raw file,
// deliberately on a different fault domain from the coordinators — and applies
// it via ApplyBootstrap. Use it to self-heal (Tier C2) when every known
// coordinator endpoint is blocked. The fetch itself is unauthenticated
// transport; trust comes only from the pinned-key signature check in
// ApplyBootstrap, so a tampering CDN/MITM cannot poison the endpoint set.
func (c *Client) RefreshFromBootstrap(url string) error {
	resp, err := c.http.Get(url)
	if err != nil {
		return fmt.Errorf("bootstrap fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError("GET bootstrap", resp)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("bootstrap fetch: %w", err)
	}
	return c.ApplyBootstrap(body)
}

// Endpoints returns the client's current coordinator endpoints (failover order).
func (c *Client) Endpoints() []string {
	c.epMu.Lock()
	defer c.epMu.Unlock()
	return append([]string(nil), c.endpoints...)
}

// ApplyBootstrap verifies a signed out-of-band bootstrap object against the
// pinned key and the client's bootstrap rollback floor, and on success REPLACES
// the client's coordinator endpoints with the list's. It replaces rather than
// merges on purpose: the list is the operator's current authoritative view, so a
// retired or seized endpoint must drop out instead of lingering. Fail-closed —
// without a pinned key an unauthenticated list is refused, since such a list is
// itself a poisoning vector. On rejection the existing endpoints are left
// untouched.
func (c *Client) ApplyBootstrap(body []byte) error {
	if c.pinnedKey == nil {
		return errors.New("bootstrap: refusing unauthenticated list (no pinned key)")
	}
	var signed discovery.Signed
	if err := json.Unmarshal(body, &signed); err != nil {
		return fmt.Errorf("bootstrap: not a signed envelope: %w", err)
	}
	c.dirMu.Lock()
	floor := c.bootstrapFloor
	c.dirMu.Unlock()

	payload, err := signed.Verify(c.pinnedKey, discovery.TypeBootstrap, time.Now(), floor)
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	var list BootstrapList
	if err := json.Unmarshal(payload, &list); err != nil {
		return fmt.Errorf("bootstrap: bad payload: %w", err)
	}
	eps := make([]string, 0, len(list.Endpoints))
	for _, u := range list.Endpoints {
		if u != "" {
			eps = append(eps, u)
		}
	}
	if len(eps) == 0 {
		return errors.New("bootstrap: list has no usable endpoints")
	}

	c.dirMu.Lock()
	if signed.Serial > c.bootstrapFloor {
		c.bootstrapFloor = signed.Serial
	}
	c.dirMu.Unlock()

	c.epMu.Lock()
	c.endpoints = eps
	c.current = 0
	c.epMu.Unlock()
	return nil
}
