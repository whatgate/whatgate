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
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/whatgate/whatgate/internal/coordinator"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	invite := flag.String("invite", "welcome", "invite code to seed for admission")
	issuer := flag.String("issuer", "founder", "issuer attributed to the seeded invite")
	uses := flag.Int("uses", 100, "how many members the seeded invite may admit")
	ttl := flag.Duration("ttl", 60*time.Second, "directory entry time-to-live")
	flag.Parse()

	dir := coordinator.NewDirectory(*ttl, nil)
	invites := coordinator.NewInviteStore(nil)
	invites.Create(*invite, *issuer, *uses)

	srv := coordinator.NewServer(dir, invites)

	fmt.Printf("WhatGate coordinator listening on %s\n", *addr)
	fmt.Printf("seeded invite: %q (issuer=%s, uses=%d)\n", *invite, *issuer, *uses)
	fmt.Println("directory TTL:", *ttl)

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(httpSrv.ListenAndServe())
}
