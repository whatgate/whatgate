// Command coordinator runs WhatGate's central coordination server: the node
// directory and invite-based admission. It handles metadata only — proxied
// traffic flows peer-to-peer and never passes through here.
//
// M2 usage:
//
//	coordinator -addr :8080 -invite welcome -uses 100
//
// Distribute the printed invite code to people you want to admit; they pass it
// to `node -coordinator http://<host>:8080 -invite welcome ...`.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/whatgate/whatgate/internal/coordinator"
	"github.com/whatgate/whatgate/internal/discovery"
	"github.com/whatgate/whatgate/internal/membership"
	"github.com/whatgate/whatgate/internal/persist"
	"github.com/whatgate/whatgate/internal/relay"
)

// version is the build version, injected via -ldflags "-X main.version=..." at
// release time; "dev" for local builds.
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	invite := flag.String("invite", "welcome", "invite code to seed for admission")
	issuer := flag.String("issuer", "founder", "issuer attributed to the seeded invite")
	uses := flag.Int("uses", 100, "how many members the seeded invite may admit")
	ttl := flag.Duration("ttl", 60*time.Second, "directory entry time-to-live")
	tlsCert := flag.String("tls-cert", "", "path to a TLS certificate (with -tls-key, serve HTTPS so invite codes/group secrets aren't sent in cleartext)")
	tlsKey := flag.String("tls-key", "", "path to the TLS private key (pairs with -tls-cert)")
	signingKey := flag.String("signing-key", "", "path to the control-plane signing key (created if missing); signs the directory so nodes pinning the printed public key reject a forged/MITM directory")
	relayListen := flag.String("relay-listen", "/ip4/0.0.0.0/tcp/0", "libp2p listen multiaddr for the co-located relay (empty to disable)")
	statePath := flag.String("state", "", "path to a JSON state file for durable admissions/groups/reputation (empty = in-memory only)")
	repDecay := flag.Int("reputation-decay", 1, "reputation points to decay toward zero each interval (0 = disabled)")
	repDecayInterval := flag.Duration("reputation-decay-interval", time.Hour, "how often to apply reputation decay")
	rateLimit := flag.Float64("rate-limit", 0, "per-client-IP requests/sec on mutating endpoints (join/register/group/report); 0 = disabled. Slows mass join/register from a leaked invite")
	rateBurst := flag.Float64("rate-burst", 20, "per-client-IP burst allowance for -rate-limit")
	sybilWindow := flag.Duration("sybil-window", time.Hour, "anomaly detection: sliding window over which distinct join identities per IP are counted")
	sybilMaxIdentities := flag.Int("sybil-max-identities", 0, "anomaly detection: reject join once an IP mints this many distinct PeerIDs within -sybil-window (0 = disabled). Catches patient Sybil growth that stays under -rate-limit. Keyed by IP; set generously for CGNAT/proxy sharing")
	emitBootstrap := flag.String("emit-bootstrap", "", "sign a bootstrap list of the given comma-separated coordinator URLs (needs -signing-key), print it to stdout, and exit; host the output on an out-of-band channel (CDN/GitHub raw) as a node -bootstrap-url")
	bootstrapSerial := flag.Uint64("bootstrap-serial", 0, "monotonic serial for -emit-bootstrap; must strictly increase across publications (older ones are rejected as rollbacks)")
	bootstrapTTL := flag.Duration("bootstrap-ttl", 30*24*time.Hour, "how long a -emit-bootstrap list stays acceptable to nodes")
	// C1 membership issuance.
	emitIssuerCert := flag.Bool("emit-issuer-cert", false, "OFFLINE-ROOT mode: sign an issuer cert authorizing -issuer-pubkey for -issuer-roles (needs -root-key), print it to stdout, and exit; give the output to a coordinator as -issuer-cert")
	rootKey := flag.String("root-key", "", "path to the OFFLINE root signing key (created if missing) used by -emit-issuer-cert; keep this key offline — it is the trust anchor")
	issuerPubkey := flag.String("issuer-pubkey", "", "base64 public key of the online issuer to authorize (for -emit-issuer-cert; a coordinator prints its issuer public key on serve with -issuer-key)")
	issuerRoles := flag.String("issuer-roles", "member", "comma-separated roles the issuer cert may grant: member,exit,relay (for -emit-issuer-cert; grant exit/relay only to trusted/dual-approved issuers)")
	issuerID := flag.String("issuer-id", "coord-issuer", "issuer identifier bound into the emitted issuer cert")
	issuerCertTTL := flag.Duration("issuer-cert-ttl", 365*24*time.Hour, "validity of the emitted issuer cert")
	issuerCertSerial := flag.Uint64("issuer-cert-serial", 1, "monotonic serial for -emit-issuer-cert")
	issuerKeyPath := flag.String("issuer-key", "", "path to the ONLINE issuer signing key (created if missing); with -issuer-cert, a successful join issues the peer a {member} cert")
	issuerCertPath := flag.String("issuer-cert", "", "path to the root-signed issuer cert authorizing -issuer-key (produced by -emit-issuer-cert)")
	// C1.2 revocation checkpoint (offline-root producer).
	emitRevocation := flag.Bool("emit-revocation", false, "OFFLINE-ROOT mode: sign a revocation checkpoint (needs -root-key), print it to stdout, and exit; distribute via a hard-to-block channel so nodes learn what is revoked")
	revokeSubjects := flag.String("revoke-subjects", "", "comma-separated peer IDs to revoke (for -emit-revocation)")
	revokeIssuers := flag.String("revoke-issuers", "", "comma-separated issuer ids to revoke wholesale (for -emit-revocation; revokes everything they signed)")
	revocationVersion := flag.Uint64("revocation-version", 1, "monotonic version for -emit-revocation; must strictly increase (older ones are rejected as rollbacks)")
	revocationEpoch := flag.Uint64("revocation-epoch", 0, "current coarse revocation epoch for -emit-revocation (nodes reject certs below it)")
	revocationNextUpdate := flag.Duration("revocation-next-update", 6*time.Hour, "when the next checkpoint is expected (advisory freshness)")
	revocationMaxStaleness := flag.Duration("revocation-max-staleness", 24*time.Hour, "how stale a checkpoint may get before nodes must degrade to emergency-only scope")
	revocationHardTTL := flag.Duration("revocation-hard-ttl", 30*24*time.Hour, "absolute expiry of the checkpoint envelope (past it, it no longer verifies)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("whatgate coordinator %s\n", version)
		return
	}

	// Producer mode: sign a bootstrap list and exit. This is an offline operator
	// tool, not part of serving — a node self-heals by fetching this output from a
	// hard-to-block channel when its coordinators are all blocked (Tier C2).
	if *emitBootstrap != "" {
		if *signingKey == "" {
			log.Fatal("coordinator: -emit-bootstrap requires -signing-key (the key nodes pin)")
		}
		priv, err := discovery.LoadOrCreateSigningKey(*signingKey)
		if err != nil {
			log.Fatalf("signing key: %v", err)
		}
		out, err := coordinator.SignBootstrap(priv, *bootstrapSerial, splitEndpoints(*emitBootstrap), *bootstrapTTL)
		if err != nil {
			log.Fatalf("emit bootstrap: %v", err)
		}
		fmt.Println(string(out))
		return
	}

	// Offline-root mode: sign an issuer cert and exit. Run this on an OFFLINE
	// machine holding the root key; hand the printed cert to a coordinator as
	// -issuer-cert. The root is the trust anchor nodes ultimately pin (C1 §15).
	if *emitIssuerCert {
		if *rootKey == "" {
			log.Fatal("coordinator: -emit-issuer-cert requires -root-key (keep it offline)")
		}
		if *issuerPubkey == "" {
			log.Fatal("coordinator: -emit-issuer-cert requires -issuer-pubkey (a coordinator prints it on serve with -issuer-key)")
		}
		root, err := discovery.LoadOrCreateSigningKey(*rootKey)
		if err != nil {
			log.Fatalf("root key: %v", err)
		}
		issuerPub, err := discovery.DecodePublicKey(*issuerPubkey)
		if err != nil {
			log.Fatalf("bad -issuer-pubkey: %v", err)
		}
		now := time.Now()
		out, err := membership.SignIssuerCert(root, issuerPub, *issuerID, parseRoles(*issuerRoles),
			now, now.Add(*issuerCertTTL), *issuerCertSerial, 0)
		if err != nil {
			log.Fatalf("emit issuer cert: %v", err)
		}
		// Root public key to stderr so `> issuer-cert.json` captures only the cert;
		// nodes ultimately pin this root to verify the whole credential chain.
		fmt.Fprintln(os.Stderr, "root public key (pin on nodes to anchor the credential chain):", discovery.EncodePublicKey(root.GetPublic()))
		fmt.Println(string(out))
		return
	}

	// Offline-root mode: sign a revocation checkpoint and exit.
	if *emitRevocation {
		if *rootKey == "" {
			log.Fatal("coordinator: -emit-revocation requires -root-key (keep it offline)")
		}
		root, err := discovery.LoadOrCreateSigningKey(*rootKey)
		if err != nil {
			log.Fatalf("root key: %v", err)
		}
		now := time.Now()
		cp := membership.RevocationCheckpoint{
			V:               1,
			Version:         *revocationVersion,
			RevEpoch:        *revocationEpoch,
			ThisUpdate:      now.Unix(),
			NextUpdate:      now.Add(*revocationNextUpdate).Unix(),
			MaxStalenessSec: int64(revocationMaxStaleness.Seconds()),
			RevokedSubjects: splitEndpoints(*revokeSubjects),
			RevokedIssuers:  splitEndpoints(*revokeIssuers),
		}
		out, err := membership.SignRevocationCheckpoint(root, cp, now.Add(*revocationHardTTL))
		if err != nil {
			log.Fatalf("emit revocation: %v", err)
		}
		fmt.Fprintf(os.Stderr, "revocation checkpoint v%d: %d subject(s), %d issuer(s), max-staleness %s\n",
			cp.Version, len(cp.RevokedSubjects), len(cp.RevokedIssuers), *revocationMaxStaleness)
		fmt.Println(string(out))
		return
	}

	dir := coordinator.NewDirectory(*ttl, nil)
	invites := coordinator.NewInviteStore(nil)
	srv := coordinator.NewServer(dir, invites)

	// Restore durable state, then seed the invite only if it is not already
	// known, and enable save-on-change.
	if *statePath != "" {
		snap, err := persist.Load(*statePath)
		if err != nil {
			log.Fatalf("load state %s: %v", *statePath, err)
		}
		srv.LoadSnapshot(snap)
		srv.SetStatePath(*statePath)
		fmt.Printf("state file: %s\n", *statePath)
	}
	if !invites.Exists(*invite) {
		invites.Create(*invite, *issuer, *uses)
	}

	// Sign discovery responses so nodes can authenticate the directory (defeats a
	// rogue/MITM coordinator). The public key is distributed out of band and
	// pinned by nodes via -coordinator-key.
	if *signingKey != "" {
		priv, err := discovery.LoadOrCreateSigningKey(*signingKey)
		if err != nil {
			log.Fatalf("signing key: %v", err)
		}
		srv.SetSigningKey(priv)
		fmt.Printf("directory signing: enabled\n")
		fmt.Printf("coordinator public key (share as node -coordinator-key): %s\n", discovery.EncodePublicKey(priv.GetPublic()))
	} else {
		fmt.Println("directory signing: DISABLED — nodes cannot authenticate the directory; set -signing-key in production")
	}

	// C1 membership issuance: as an online, root-authorized issuer, mint a
	// {member} cert for each joining peer. Requires both the online issuer key and
	// the root-signed issuer cert authorizing it.
	if *issuerKeyPath != "" {
		issuerPriv, err := discovery.LoadOrCreateSigningKey(*issuerKeyPath)
		if err != nil {
			log.Fatalf("issuer key: %v", err)
		}
		fmt.Printf("issuer public key (authorize offline: coordinator -emit-issuer-cert -root-key <root> -issuer-pubkey %s): %s\n",
			discovery.EncodePublicKey(issuerPriv.GetPublic()), discovery.EncodePublicKey(issuerPriv.GetPublic()))
		if *issuerCertPath == "" {
			fmt.Println("WARNING: -issuer-key set without -issuer-cert; member-cert issuance DISABLED until a root-signed issuer cert is provided")
		} else {
			certBytes, err := os.ReadFile(*issuerCertPath)
			if err != nil {
				log.Fatalf("read issuer cert %s: %v", *issuerCertPath, err)
			}
			id, err := membership.IssuerCertID(certBytes)
			if err != nil {
				log.Fatalf("issuer cert: %v", err)
			}
			srv.SetIssuer(issuerPriv, certBytes, id)
			fmt.Printf("member cert issuance: enabled (issuer=%s)\n", id)
		}
	}

	// Co-locate a Circuit Relay v2 so nodes that cannot hole-punch still connect.
	if *relayListen != "" {
		rl, err := relay.New(context.Background(), *relayListen)
		if err != nil {
			log.Fatalf("start relay: %v", err)
		}
		defer rl.Close()
		addrs := make([]string, 0, len(rl.Addrs()))
		for _, a := range rl.Addrs() {
			addrs = append(addrs, a.String())
		}
		srv.SetRelayInfo(rl.ID().String(), addrs)
		fmt.Printf("relay peer id: %s\n", rl.ID())
		for _, a := range addrs {
			fmt.Printf("  relay addr: %s/p2p/%s\n", a, rl.ID())
		}
	}

	// Per-client-IP rate limiting on mutating endpoints (anti-Sybil / anti-abuse).
	if *rateLimit > 0 {
		srv.SetRateLimit(*rateLimit, *rateBurst)
		fmt.Printf("rate limit: %.3g req/s per IP (burst %.3g) on join/register/group/report\n", *rateLimit, *rateBurst)
	}
	// Sybil-pattern isolation: flag IPs minting too many distinct identities.
	if *sybilMaxIdentities > 0 {
		srv.SetAnomalyDetection(*sybilWindow, *sybilMaxIdentities)
		fmt.Printf("anomaly detection: isolate IP after %d distinct join identities within %s\n", *sybilMaxIdentities, *sybilWindow)
	}

	// Periodically decay reputation so punishments fade and scores don't linger.
	if *repDecay > 0 && *repDecayInterval > 0 {
		go func() {
			t := time.NewTicker(*repDecayInterval)
			defer t.Stop()
			for range t.C {
				srv.DecayReputation(*repDecay)
			}
		}()
		fmt.Printf("reputation decay: %d per %s\n", *repDecay, *repDecayInterval)
	}

	fmt.Printf("WhatGate coordinator listening on %s\n", *addr)
	fmt.Printf("seeded invite: %q (issuer=%s, uses=%d)\n", *invite, *issuer, *uses)
	fmt.Println("directory TTL:", *ttl)

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Serve HTTPS when a cert/key pair is given. The control plane carries
	// secrets (invite codes, group secrets) that must not travel in cleartext;
	// operators without a cert should terminate TLS at a reverse proxy instead.
	if (*tlsCert == "") != (*tlsKey == "") {
		log.Fatal("coordinator: -tls-cert and -tls-key must be set together")
	}
	if *tlsCert != "" {
		fmt.Println("TLS: enabled")
		log.Fatal(httpSrv.ListenAndServeTLS(*tlsCert, *tlsKey))
	}
	fmt.Println("TLS: DISABLED — control plane is plaintext; use -tls-cert/-tls-key or a TLS-terminating proxy in production")
	log.Fatal(httpSrv.ListenAndServe())
}

// splitEndpoints parses a comma-separated list of URLs, trimming spaces and
// dropping empties.
func splitEndpoints(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseRoles parses a comma-separated role list into membership roles.
func parseRoles(s string) []membership.Role {
	parts := strings.Split(s, ",")
	out := make([]membership.Role, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, membership.Role(p))
		}
	}
	return out
}
