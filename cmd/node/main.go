// Command node runs a WhatGate participant. A single node can act as a client
// (exposing a local SOCKS5 proxy that tunnels to a chosen exit) and/or as an
// exit (serving other nodes' traffic) — reflecting WhatGate's premise that every
// online user is both.
//
// Two ways to reach an exit:
//
//	# Manual (M1): connect to a known exit multiaddr.
//	node -exit
//	node -connect <exit-multiaddr> -socks 127.0.0.1:1080
//
//	# Via coordinator (M2): join, register, and discover an exit by region.
//	coordinator -addr :8080 -invite welcome
//	node -coordinator http://host:8080 -invite welcome -exit -region JP
//	node -coordinator http://host:8080 -invite welcome -to JP -socks 127.0.0.1:1080
//
// Prove egress happens at the exit:
//
//	curl --socks5-hostname 127.0.0.1:1080 https://api.ipify.org
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
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/whatgate/whatgate/internal/coordinator"
	"github.com/whatgate/whatgate/internal/node"
	"github.com/whatgate/whatgate/internal/proxy"
	"github.com/whatgate/whatgate/internal/routing"
)

func main() {
	listen := flag.String("listen", "/ip4/0.0.0.0/tcp/0", "libp2p listen multiaddr")
	asExit := flag.Bool("exit", false, "act as an exit node for other peers (serves their traffic)")
	connect := flag.String("connect", "", "exit peer multiaddr to tunnel through (manual client mode)")
	socksAddr := flag.String("socks", "127.0.0.1:1080", "local SOCKS5 listen address (client mode)")
	coordURL := flag.String("coordinator", "", "coordinator base URL, e.g. http://host:8080")
	invite := flag.String("invite", "", "invite code to redeem when joining via coordinator")
	region := flag.String("region", "", "this node's exit region tag when acting as exit, e.g. JP")
	toRegion := flag.String("to", "", "desired exit region to discover via coordinator (client mode)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	nodeOpts := []node.Option{node.WithListenAddrs(*listen)}

	var coord *coordinator.Client
	if *coordURL != "" {
		coord = coordinator.NewClient(*coordURL)
		// Configure the coordinator's relay as a fallback path, if it advertises
		// one, so this node stays reachable when hole punching fails.
		if info, err := coord.Relay(); err == nil {
			if ai, err := node.AddrInfoFromStrings(info.PeerID, info.Addrs); err == nil {
				nodeOpts = append(nodeOpts, node.WithStaticRelays(ai))
				fmt.Printf("relay fallback: %s\n", ai.ID)
			}
		}
	}

	n, err := node.New(ctx, nodeOpts...)
	if err != nil {
		log.Fatalf("start node: %v", err)
	}
	defer n.Close()
	selfID := n.ID().String()

	fmt.Printf("peer id: %s\n", selfID)
	fmt.Println("listening on:")
	for _, a := range n.Addrs() {
		fmt.Printf("  %s/p2p/%s\n", a, selfID)
	}

	if *asExit {
		// WhatGate safety note: becoming an exit is opt-in. In M2 these flags are
		// the explicit consent; later milestones add trust-scope and policy.
		n.EnableExit(func(ctx context.Context, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", addr)
		})
		fmt.Println("exit: ENABLED — serving other peers' traffic through this node")
	}

	// Coordinator-based flow: join, register presence, discover exits.
	if coord != nil {
		if *invite != "" {
			issuer, err := coord.Join(*invite, selfID)
			if err != nil {
				log.Fatalf("join via coordinator: %v", err)
			}
			fmt.Printf("joined network (vouched by %s)\n", issuer)
		}

		registerOnce(coord, selfID, n.AddrStrings(), *region, *asExit)
		go keepRegistered(ctx, coord, selfID, n, *region, *asExit)

		if *toRegion != "" {
			ai, err := discoverExit(coord, *toRegion, selfID)
			if err != nil {
				log.Fatalf("discover exit: %v", err)
			}
			startClient(ctx, n, ai, *socksAddr)
		}
	}

	// Manual flow: connect to an explicit exit multiaddr.
	if *connect != "" {
		ai, err := parsePeer(*connect)
		if err != nil {
			log.Fatalf("parse -connect: %v", err)
		}
		startClient(ctx, n, ai, *socksAddr)
	}

	fmt.Println("running; press Ctrl+C to stop")
	<-ctx.Done()
	fmt.Println("shutting down")
}

// startClient connects to the exit and serves a local SOCKS5 proxy tunneling to it.
func startClient(ctx context.Context, n *node.Node, exit peer.AddrInfo, socksAddr string) {
	if err := n.Connect(ctx, exit); err != nil {
		log.Fatalf("connect to exit %s: %v", exit.ID, err)
	}
	fmt.Printf("connected to exit %s\n", exit.ID)

	srv := &proxy.Server{Dialer: n.NewClientDialer(exit.ID)}
	l, err := net.Listen("tcp", socksAddr)
	if err != nil {
		log.Fatalf("listen socks: %v", err)
	}
	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()
	go func() {
		if err := srv.Serve(l); err != nil && ctx.Err() == nil {
			log.Printf("socks server stopped: %v", err)
		}
	}()
	fmt.Printf("SOCKS5 proxy on %s → tunneling to exit %s\n", socksAddr, exit.ID)
}

// discoverExit queries the directory and picks an exit in the desired region.
func discoverExit(c *coordinator.Client, region, selfID string) (peer.AddrInfo, error) {
	nodes, err := c.Directory()
	if err != nil {
		return peer.AddrInfo{}, err
	}
	pick, ok := routing.PickExit(nodes, region, selfID)
	if !ok {
		return peer.AddrInfo{}, fmt.Errorf("no exit available in region %q", region)
	}
	fmt.Printf("selected exit %s in region %s\n", pick.PeerID, pick.Region)
	return node.AddrInfoFromStrings(pick.PeerID, pick.Addrs)
}

func registerOnce(c *coordinator.Client, selfID string, addrs []string, region string, wantExit bool) {
	err := c.Register(coordinator.NodeInfo{
		PeerID:   selfID,
		Addrs:    addrs,
		Region:   region,
		WantExit: wantExit,
	})
	if err != nil {
		log.Printf("register with coordinator: %v", err)
	}
}

// keepRegistered periodically refreshes the node's directory entry so it does
// not expire, until the context is cancelled.
func keepRegistered(ctx context.Context, c *coordinator.Client, selfID string, n *node.Node, region string, wantExit bool) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			registerOnce(c, selfID, n.AddrStrings(), region, wantExit)
		}
	}
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
