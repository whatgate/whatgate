// Command node runs a WhatGate participant. A single node can act as a client
// (exposing a local SOCKS5 proxy that tunnels to a chosen exit) and/or as an
// exit (serving other nodes' traffic) — reflecting WhatGate's premise that every
// online user is both.
//
// M1 usage (manual, same machine or two hosts):
//
//	# Terminal 1 — run an exit node; copy a printed /p2p/ multiaddr.
//	node -exit
//
//	# Terminal 2 — run a client that tunnels through that exit.
//	node -connect <exit-multiaddr> -socks 127.0.0.1:1080
//
//	# Then prove traffic egresses via the exit:
//	curl --socks5-hostname 127.0.0.1:1080 https://ifconfig.me
//
// The IP returned should be the exit node's, not the client's.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/whatgate/whatgate/internal/node"
	"github.com/whatgate/whatgate/internal/proxy"
)

func main() {
	listen := flag.String("listen", "/ip4/0.0.0.0/tcp/0", "libp2p listen multiaddr")
	asExit := flag.Bool("exit", false, "act as an exit node for other peers (serves their traffic)")
	connect := flag.String("connect", "", "exit peer multiaddr to tunnel through (client mode)")
	socksAddr := flag.String("socks", "127.0.0.1:1080", "local SOCKS5 listen address (client mode)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	n, err := node.New(ctx, *listen)
	if err != nil {
		log.Fatalf("start node: %v", err)
	}
	defer n.Close()

	fmt.Printf("peer id: %s\n", n.ID())
	fmt.Println("listening on:")
	for _, a := range n.Addrs() {
		fmt.Printf("  %s/p2p/%s\n", a, n.ID())
	}

	if *asExit {
		// WhatGate safety note: becoming an exit is opt-in. In M1 this flag is
		// the explicit consent; later milestones add trust-scope and policy.
		n.EnableExit(func(ctx context.Context, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", addr)
		})
		fmt.Println("exit: ENABLED — serving other peers' traffic through this node")
	}

	if *connect != "" {
		exit, err := parsePeer(*connect)
		if err != nil {
			log.Fatalf("parse -connect: %v", err)
		}
		if err := n.Connect(ctx, exit); err != nil {
			log.Fatalf("connect to exit: %v", err)
		}
		fmt.Printf("connected to exit %s\n", exit.ID)

		srv := &proxy.Server{Dialer: n.NewClientDialer(exit.ID)}
		l, err := net.Listen("tcp", *socksAddr)
		if err != nil {
			log.Fatalf("listen socks: %v", err)
		}
		defer l.Close()
		go func() {
			if err := srv.Serve(l); err != nil && ctx.Err() == nil {
				log.Printf("socks server stopped: %v", err)
			}
		}()
		fmt.Printf("SOCKS5 proxy on %s → tunneling to exit %s\n", *socksAddr, exit.ID)
	}

	fmt.Println("running; press Ctrl+C to stop")
	<-ctx.Done()
	fmt.Println("shutting down")
}

// parsePeer turns a /p2p/ multiaddr string into an AddrInfo.
func parsePeer(s string) (peer.AddrInfo, error) {
	maddr, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		return peer.AddrInfo{}, err
	}
	ai, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return peer.AddrInfo{}, err
	}
	return *ai, nil
}
