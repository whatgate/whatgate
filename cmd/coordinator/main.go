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
	"time"

	"github.com/whatgate/whatgate/internal/coordinator"
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
	relayListen := flag.String("relay-listen", "/ip4/0.0.0.0/tcp/0", "libp2p listen multiaddr for the co-located relay (empty to disable)")
	statePath := flag.String("state", "", "path to a JSON state file for durable admissions/groups/reputation (empty = in-memory only)")
	repDecay := flag.Int("reputation-decay", 1, "reputation points to decay toward zero each interval (0 = disabled)")
	repDecayInterval := flag.Duration("reputation-decay-interval", time.Hour, "how often to apply reputation decay")
	flag.Parse()

	if *showVersion {
		fmt.Printf("whatgate coordinator %s\n", version)
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
