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
	"strings"
	"time"

	"github.com/whatgate/whatgate/internal/coordinator"
	"github.com/whatgate/whatgate/internal/discovery"
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
	emitBootstrap := flag.String("emit-bootstrap", "", "sign a bootstrap list of the given comma-separated coordinator URLs (needs -signing-key), print it to stdout, and exit; host the output on an out-of-band channel (CDN/GitHub raw) as a node -bootstrap-url")
	bootstrapSerial := flag.Uint64("bootstrap-serial", 0, "monotonic serial for -emit-bootstrap; must strictly increase across publications (older ones are rejected as rollbacks)")
	bootstrapTTL := flag.Duration("bootstrap-ttl", 30*24*time.Hour, "how long a -emit-bootstrap list stays acceptable to nodes")
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
