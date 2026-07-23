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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/whatgate/whatgate/internal/coordinator"
	"github.com/whatgate/whatgate/internal/exit"
	"github.com/whatgate/whatgate/internal/node"
	"github.com/whatgate/whatgate/internal/proxy"
	"github.com/whatgate/whatgate/internal/routing"
	"github.com/whatgate/whatgate/internal/trust"
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
	group := flag.String("group", "", "join (or create) this small-network group id")
	endorse := flag.String("endorse", "", "endorse fromGroup:toGroup (your group vouches for another)")
	trustScope := flag.String("trust-scope", "", "trust range for exit selection: conservative|open (required with -to; no default by design)")
	exitScope := flag.String("exit-scope", "open", "ExitGuard: whose traffic to serve as exit: conservative|open")
	blockPorts := flag.String("block-ports", "", "ExitGuard: extra destination ports to block, comma-separated (SMTP ports blocked by default)")
	blockDomains := flag.String("block-domains", "", "ExitGuard: destination domains to block, comma-separated")
	maxConns := flag.Int("max-conns", 0, "ExitGuard: max concurrent connections to serve as exit (0 = unlimited)")
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
		// Becoming an exit is opt-in (this flag is the consent). ExitGuard then
		// protects the operator: it only serves requesters within exit-scope and
		// refuses blocked destinations / excess load.
		dial := func(ctx context.Context, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", addr)
		}
		policy, err := buildExitPolicy(*exitScope, *blockPorts, *blockDomains, *maxConns)
		if err != nil {
			log.Fatalf("exit policy: %v", err)
		}
		guard := exit.NewGuard(policy)
		tierOf := func(requesterID string) trust.Tier {
			if coord == nil {
				return trust.TierStranger // manual mode: no trust graph available
			}
			t, err := coord.TrustBetween(selfID, requesterID)
			if err != nil {
				return trust.TierStranger
			}
			return t
		}
		n.EnableGuardedExit(dial, guard, tierOf)
		fmt.Printf("exit: ENABLED (exit-scope=%s, blocked-ports=%d, blocked-domains=%d, max-conns=%d)\n",
			policy.Scope, len(policy.BlockedPorts), len(policy.BlockedDomains), policy.MaxConns)
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

		if *group != "" {
			if err := coord.JoinGroup(*group, selfID); err != nil {
				log.Printf("join group %s: %v", *group, err)
			} else {
				fmt.Printf("joined small-network group %s\n", *group)
			}
		}
		if *endorse != "" {
			if from, to, ok := strings.Cut(*endorse, ":"); ok {
				if err := coord.EndorseGroup(from, to); err != nil {
					log.Printf("endorse %s->%s: %v", from, to, err)
				} else {
					fmt.Printf("endorsed group %s -> %s\n", from, to)
				}
			} else {
				log.Printf("bad -endorse %q; want fromGroup:toGroup", *endorse)
			}
		}

		registerOnce(coord, selfID, n.AddrStrings(), *region, *asExit, n.ExitLoad())
		go keepRegistered(ctx, coord, selfID, n, *region, *asExit)

		if *toRegion != "" {
			scope, err := trust.ParseScope(*trustScope)
			if err != nil {
				log.Fatalf("%v", err)
			}
			ai, err := discoverExit(ctx, n, coord, *toRegion, selfID, scope)
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

// discoverExit queries the directory and picks the best exit in the desired
// region within the user's trust scope, ranked by trust, then measured latency,
// then reported load.
func discoverExit(ctx context.Context, n *node.Node, c *coordinator.Client, region, selfID string, scope trust.Scope) (peer.AddrInfo, error) {
	nodes, tiers, err := c.DirectoryFor(selfID)
	if err != nil {
		return peer.AddrInfo{}, err
	}
	tierOf := func(p string) trust.Tier { return tiers[p] }

	// Probe latency to each in-scope candidate; load comes from the directory.
	load := make(map[string]int, len(nodes))
	latency := make(map[string]int, len(nodes))
	for _, nd := range nodes {
		load[nd.PeerID] = nd.Load
		if nd.PeerID == selfID || !nd.WantExit || nd.Region != region || !scope.Allows(tiers[nd.PeerID]) {
			continue
		}
		ai, err := node.AddrInfoFromStrings(nd.PeerID, nd.Addrs)
		if err != nil {
			continue
		}
		pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		if rtt, err := n.Ping(pctx, ai); err == nil {
			latency[nd.PeerID] = int(rtt.Milliseconds())
		} else {
			latency[nd.PeerID] = 1 << 30 // unreachable: rank last
		}
		cancel()
	}

	ranked := routing.RankExits(nodes, region, selfID, scope, tierOf, func(p string) routing.Metrics {
		return routing.Metrics{LatencyMs: latency[p], Load: load[p]}
	})
	if len(ranked) == 0 {
		return peer.AddrInfo{}, fmt.Errorf("no exit in region %q within %s trust scope", region, scope)
	}
	best := ranked[0]
	fmt.Printf("selected exit %s in region %s (trust: %s, latency: %dms, load: %d)\n",
		best.PeerID, best.Region, tiers[best.PeerID], latency[best.PeerID], load[best.PeerID])
	return node.AddrInfoFromStrings(best.PeerID, best.Addrs)
}

func registerOnce(c *coordinator.Client, selfID string, addrs []string, region string, wantExit bool, load int) {
	err := c.Register(coordinator.NodeInfo{
		PeerID:   selfID,
		Addrs:    addrs,
		Region:   region,
		WantExit: wantExit,
		Load:     load,
	})
	if err != nil {
		log.Printf("register with coordinator: %v", err)
	}
}

// keepRegistered periodically refreshes the node's directory entry (including
// its current exit load) so it does not expire, until the context is cancelled.
func keepRegistered(ctx context.Context, c *coordinator.Client, selfID string, n *node.Node, region string, wantExit bool) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			registerOnce(c, selfID, n.AddrStrings(), region, wantExit, n.ExitLoad())
		}
	}
}

// buildExitPolicy assembles an ExitGuard policy from CLI flags. SMTP ports are
// always blocked by default; -block-ports adds to them.
func buildExitPolicy(scopeStr, portsCSV, domainsCSV string, maxConns int) (exit.Policy, error) {
	scope, err := trust.ParseScope(scopeStr)
	if err != nil {
		return exit.Policy{}, err
	}
	ports := exit.DefaultBlockedPorts()
	for _, p := range splitCSV(portsCSV) {
		if v, err := strconv.Atoi(p); err == nil {
			ports[v] = true
		}
	}
	domains := make(map[string]bool)
	for _, d := range splitCSV(domainsCSV) {
		domains[d] = true
	}
	return exit.Policy{Scope: scope, BlockedPorts: ports, BlockedDomains: domains, MaxConns: maxConns}, nil
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
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
