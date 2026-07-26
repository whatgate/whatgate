package node

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/whatgate/whatgate/internal/membership"
)

// VerifiedExit is a candidate exit discovered on the private DHT and verified. It
// carries the four-state classification of the trust-merge model (§4): a
// DHT-resolved exit can be Authorized, Eligible, and Reachable, but is NEVER
// Recommended — recommendation only comes from authoritative reputation (the
// coordinator), which the DHT plane cannot supply. A caller with no authoritative
// reputation must therefore treat these conservatively (user-pinned / emergency).
type VerifiedExit struct {
	Record      membership.NodeRecord
	Cert        membership.Cert
	Authorized  bool // presented a member cert chain anchored to the pinned root
	Eligible    bool // authorized, role-authorized, not revoked, and revocation info fresh
	Recommended bool // always false from DHT resolve
	Reachable   bool // proven by the live, nonce-challenged record fetch
}

// ResolveExits performs two-layer discovery for a role/region/epoch and returns
// the verified candidates. For each candidate PeerID found on the DHT it fetches
// and verifies the node record (chain-to-root, subject binding, role, generation
// floor, dial-safe addresses), drops any revoked by checkpoint, and isolates any
// equivocating subject via equiv. If checkpoint is stale, results are returned
// but marked not-Eligible (the caller should degrade to emergency-only). A nil
// checkpoint or equiv skips that step.
func (n *Node) ResolveExits(ctx context.Context, dht *PrivateDHT, role, region string, epoch uint64, pinnedRoot crypto.PubKey, opts membership.VerifyOpts, checkpoint *membership.RevocationCheckpoint, equiv *membership.EquivocationGuard, genFloor uint64, limit int) []VerifiedExit {
	now := time.Now()
	stale := checkpoint != nil && checkpoint.Stale(now)

	var out []VerifiedExit
	for _, id := range dht.FindCandidates(ctx, role, region, epoch, limit) {
		if id == n.ID() {
			continue // don't resolve ourselves
		}
		fr, err := n.FetchNodeRecord(ctx, id, pinnedRoot, opts, genFloor)
		if err != nil {
			continue // unverifiable / unreachable / no dial-safe addr
		}
		if checkpoint != nil && checkpoint.Revokes(fr.Cert) {
			continue // explicitly revoked
		}
		if equiv != nil {
			if err := equiv.Observe(fr.Cert.Subject, fr.Signed); err != nil {
				continue // equivocation (or already isolated)
			}
		}
		out = append(out, VerifiedExit{
			Record:      fr.Record,
			Cert:        fr.Cert,
			Authorized:  true,
			Eligible:    !stale,
			Recommended: false,
			Reachable:   true,
		})
	}
	return out
}
