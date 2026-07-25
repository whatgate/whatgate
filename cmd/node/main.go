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
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/whatgate/whatgate/internal/audit"
	"github.com/whatgate/whatgate/internal/coordinator"
	"github.com/whatgate/whatgate/internal/exit"
	"github.com/whatgate/whatgate/internal/node"
	"github.com/whatgate/whatgate/internal/proxy"
	"github.com/whatgate/whatgate/internal/routing"
	"github.com/whatgate/whatgate/internal/threatfeed"
	"github.com/whatgate/whatgate/internal/trust"
	"github.com/whatgate/whatgate/internal/tun"
	"github.com/whatgate/whatgate/internal/webui"
)

// version is the build version, injected via -ldflags "-X main.version=..." at
// release time; "dev" for local builds.
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	listen := flag.String("listen", "/ip4/0.0.0.0/tcp/0", "libp2p listen multiaddr")
	asExit := flag.Bool("exit", false, "act as an exit node for other peers (serves their traffic)")
	connect := flag.String("connect", "", "exit peer multiaddr to tunnel through (manual client mode)")
	socksAddr := flag.String("socks", "127.0.0.1:1080", "local SOCKS5 listen address (client mode)")
	coordURL := flag.String("coordinator", "", "coordinator base URL, e.g. http://host:8080")
	invite := flag.String("invite", "", "invite code to redeem when joining via coordinator")
	region := flag.String("region", "", "this node's exit region tag when acting as exit, e.g. JP")
	toRegion := flag.String("to", "", "desired exit region to discover via coordinator (client mode)")
	group := flag.String("group", "", "join (or create) this small-network group id")
	groupSecret := flag.String("group-secret", "", "secret required to join -group (the first joiner sets it)")
	endorse := flag.String("endorse", "", "endorse fromGroup:toGroup (your group vouches for another)")
	trustScope := flag.String("trust-scope", "", "trust range for exit selection: conservative|open (required with -to; no default by design)")
	exitScope := flag.String("exit-scope", "open", "ExitGuard: whose traffic to serve as exit: conservative|open")
	blockPorts := flag.String("block-ports", "", "ExitGuard: extra destination ports to block, comma-separated (SMTP ports blocked by default)")
	blockDomains := flag.String("block-domains", "", "ExitGuard: destination domains to block, comma-separated")
	maxConns := flag.Int("max-conns", 0, "ExitGuard: max concurrent connections to serve as exit (0 = unlimited)")
	minReputation := flag.Int("min-reputation", -1_000_000_000, "ExitGuard: refuse requesters whose reputation is below this (default effectively disabled)")
	allowPrivateTargets := flag.Bool("allow-private-targets", false, "ExitGuard: allow serving requests to private/loopback/link-local addresses (default false blocks SSRF to your LAN, localhost, and cloud metadata)")
	auditLog := flag.String("audit-log", "", "ExitGuard: append a JSON-lines record of served/denied requests to this file (accountability)")
	threatFeed := flag.String("threat-feed", "", "ExitGuard: URL or file of known-malicious domains to block (merged with -block-domains)")
	threatFeedInterval := flag.Duration("threat-feed-interval", time.Hour, "how often to refresh -threat-feed (0 = fetch once at startup)")
	webAddr := flag.String("web", "", "serve a local status dashboard at this address, e.g. 127.0.0.1:7070")
	tunMode := flag.Bool("tun", false, "route ALL system traffic through the tunnel via a TUN device (needs -tags tun build, admin, and wintun.dll on Windows)")
	tunDevice := flag.String("tun-device", "whatgate0", "TUN adapter name (TUN mode)")
	tunMTU := flag.Int("tun-mtu", 1500, "TUN interface MTU (TUN mode)")
	tunAutoRoute := flag.Bool("tun-auto-route", false, "TUN mode: automatically assign the TUN IP, redirect the default route, and exclude the node's own coordinator/exit traffic (needs admin/root; restored on exit)")
	tunAddr := flag.String("tun-addr", "10.6.7.1", "TUN mode: IP to assign the TUN interface when -tun-auto-route is set")
	tunGateway := flag.String("tun-gateway", "", "TUN mode: physical default gateway for -tun-auto-route exclusions (auto-detected if empty)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("whatgate node %s\n", version)
		return
	}

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

	// Live client state for the dashboard (updated on connect / region switch).
	var (
		stMu     sync.Mutex
		stExit   string
		stRegion = *toRegion
	)
	setState := func(exit, region string) { stMu.Lock(); stExit, stRegion = exit, region; stMu.Unlock() }
	getState := func() (string, string) { stMu.Lock(); defer stMu.Unlock(); return stExit, stRegion }
	// switchTo re-selects an exit in a region at runtime; setupFn completes the
	// first-run trust wizard (choose scope, then discover+connect); needsSetup
	// reports whether that wizard is still pending.
	var (
		switchTo func(region string) (string, error)
		setupFn  func(scope string) (string, error)
	)
	needsSetup := func() bool { return false }

	// Sign identity-proving requests (join/register) with the node's key.
	if coord != nil {
		coord.Signer = n.SignAuth
	}

	fmt.Printf("peer id: %s\n", selfID)
	fmt.Println("listening on:")
	for _, a := range n.Addrs() {
		fmt.Printf("  %s/p2p/%s\n", a, selfID)
	}

	// Build the exit configuration unconditionally so serving as an exit can be
	// toggled on/off at runtime (via the dashboard). It stays off until -exit or a
	// toggle. ExitGuard protects the operator whenever it is on.
	var (
		setExit  func(on bool)
		isExitOn func() bool
	)
	{
		// SSRF guard: block dialing private/loopback/metadata targets at connect
		// time (after DNS resolution, so hostname→internal rebinding is caught too),
		// unless the operator opts in with -allow-private-targets.
		dial := func(ctx context.Context, addr string) (net.Conn, error) {
			d := net.Dialer{Control: exit.DialControl(*allowPrivateTargets)}
			return d.DialContext(ctx, "tcp", addr)
		}
		policy, err := buildExitPolicy(*exitScope, *blockPorts, *blockDomains, *maxConns, *minReputation, *allowPrivateTargets)
		if err != nil {
			log.Fatalf("exit policy: %v", err)
		}
		guard := exit.NewGuard(policy)

		// Threat intelligence: merge a malicious-domain feed into the guard, and
		// refresh it periodically.
		if *threatFeed != "" {
			applyFeed := func() {
				feed, err := threatfeed.Fetch(*threatFeed)
				if err != nil {
					log.Printf("threat feed fetch: %v", err)
					return
				}
				merged := guard.StaticBlockedDomains()
				for d := range feed {
					merged[d] = true
				}
				guard.SetBlockedDomains(merged)
				fmt.Printf("threat feed: %d domains loaded from %s\n", len(feed), *threatFeed)
			}
			applyFeed()
			if *threatFeedInterval > 0 {
				go func() {
					t := time.NewTicker(*threatFeedInterval)
					defer t.Stop()
					for {
						select {
						case <-ctx.Done():
							return
						case <-t.C:
							applyFeed()
						}
					}
				}()
			}
		}

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
		repOf := func(requesterID string) int {
			if coord == nil {
				return 0
			}
			s, err := coord.ReputationOf(requesterID)
			if err != nil {
				return 0
			}
			return s
		}
		var report func(requesterID string, outcome trust.Outcome)
		if coord != nil {
			report = func(requesterID string, outcome trust.Outcome) {
				go func() { _ = coord.ReportOutcome(requesterID, outcome) }()
			}
		}
		var auditFn func(requesterID, target, outcome string)
		if *auditLog != "" {
			al, err := audit.NewFileLogger(*auditLog)
			if err != nil {
				log.Fatalf("audit log: %v", err)
			}
			defer al.Close()
			auditFn = func(requesterID, target, outcome string) {
				_ = al.Log(audit.Entry{Time: time.Now(), Requester: requesterID, Target: target, Outcome: outcome})
			}
			fmt.Printf("audit log: %s\n", *auditLog)
		}
		exitCfg := node.GuardedExit{
			Dial:   dial,
			Guard:  guard,
			TierOf: tierOf,
			RepOf:  repOf,
			Report: report,
			Audit:  auditFn,
		}
		var (
			exitMu sync.Mutex
			exitOn bool
		)
		setExit = func(on bool) {
			exitMu.Lock()
			defer exitMu.Unlock()
			if on {
				n.EnableGuardedExit(exitCfg)
				n.EnableGuardedUDPExit(exitCfg) // UDP forwarding under the same ExitGuard
			} else {
				n.DisableExit()
				n.DisableUDPExit()
			}
			exitOn = on
		}
		isExitOn = func() bool { exitMu.Lock(); defer exitMu.Unlock(); return exitOn }

		setExit(*asExit)
		if *asExit {
			fmt.Printf("exit: ENABLED (exit-scope=%s, blocked-ports=%d, blocked-domains=%d, max-conns=%d)\n",
				policy.Scope, len(policy.BlockedPorts), len(policy.BlockedDomains), policy.MaxConns)
		}
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
			if err := coord.JoinGroup(*group, selfID, *groupSecret); err != nil {
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

		registerOnce(coord, selfID, n.AddrStrings(), *region, isExitOn(), n.ExitLoad())
		go keepRegistered(ctx, coord, selfID, n, *region, isExitOn)

		if *toRegion != "" {
			// A switchable dialer lets the dashboard re-point the SOCKS proxy at a
			// different exit/region at runtime. It starts empty; dials fail until an
			// exit is selected (immediately if -trust-scope is set, otherwise after
			// the first-run wizard).
			sw := &proxy.SwitchableDialer{}
			var (
				scopeMu    sync.Mutex
				curScope   trust.Scope
				curExitID  peer.ID
				configured bool
			)
			// openUDP opens a UDP tunnel to the current exit for SOCKS5 UDP ASSOCIATE.
			openUDP := func() (proxy.UDPTunnel, error) {
				scopeMu.Lock()
				id, ok := curExitID, configured
				scopeMu.Unlock()
				if !ok {
					return nil, fmt.Errorf("no exit selected yet")
				}
				return n.OpenUDPSession(ctx, id)
			}
			serveSOCKS(ctx, *socksAddr, sw, openUDP)

			connectIn := func(sc trust.Scope, region string) (string, error) {
				ai, err := discoverExit(ctx, n, coord, region, selfID, sc)
				if err != nil {
					return "", err
				}
				if err := n.Connect(ctx, ai); err != nil {
					return "", err
				}
				sw.Set(n.NewClientDialer(ai.ID))
				setState(ai.ID.String(), region)
				scopeMu.Lock()
				curScope, curExitID, configured = sc, ai.ID, true
				scopeMu.Unlock()
				return ai.ID.String(), nil
			}

			needsSetup = func() bool { scopeMu.Lock(); defer scopeMu.Unlock(); return !configured }
			setupFn = func(scopeStr string) (string, error) {
				sc, err := trust.ParseScope(scopeStr)
				if err != nil {
					return "", err
				}
				fmt.Printf("first-run wizard: trust scope = %s\n", sc)
				return connectIn(sc, *toRegion)
			}
			switchTo = func(region string) (string, error) {
				scopeMu.Lock()
				sc, ok := curScope, configured
				scopeMu.Unlock()
				if !ok {
					return "", fmt.Errorf("choose a trust scope first")
				}
				id, err := connectIn(sc, region)
				if err == nil {
					fmt.Printf("switched exit to %s in region %s\n", id, region)
				}
				return id, err
			}

			if *trustScope != "" {
				sc, err := trust.ParseScope(*trustScope)
				if err != nil {
					log.Fatalf("%v", err)
				}
				if _, err := connectIn(sc, *toRegion); err != nil {
					log.Fatalf("discover exit: %v", err)
				}
			} else {
				if *webAddr == "" {
					log.Fatalf("client mode needs a trust scope: set -trust-scope conservative|open, or -web to choose it in the first-run wizard")
				}
				fmt.Printf("no -trust-scope set: open the web console to choose (first-run wizard)\n")
			}
		}
	}

	// Manual flow: connect to an explicit exit multiaddr.
	if *connect != "" {
		ai, err := parsePeer(*connect)
		if err != nil {
			log.Fatalf("parse -connect: %v", err)
		}
		setState(ai.ID.String(), "")
		startClient(ctx, n, ai, *socksAddr, n.NewClientDialer(ai.ID), func() (proxy.UDPTunnel, error) {
			return n.OpenUDPSession(ctx, ai.ID)
		})
	}

	// TUN mode: route the whole system's traffic through the local SOCKS proxy
	// (which tunnels to the exit). Requires an active client-mode SOCKS proxy.
	if *tunMode {
		if *toRegion == "" && *connect == "" {
			log.Fatalf("-tun requires client mode: set -to <region> or -connect <exit>")
		}
		if err := tun.Start(tun.Config{Device: *tunDevice, SocksAddr: *socksAddr, MTU: *tunMTU}); err != nil {
			log.Fatalf("tun mode: %v", err)
		}
		defer tun.Stop()
		fmt.Printf("TUN mode: ENABLED on %s → SOCKS %s (all system traffic routed through the tunnel)\n", *tunDevice, *socksAddr)

		// Auto-routing: assign the TUN IP, redirect the default route into it,
		// and pin the node's own control/data-plane connections to the physical
		// gateway so the tunnel doesn't capture its own packets and loop.
		if *tunAutoRoute {
			gw := *tunGateway
			if gw == "" {
				detected, err := tun.DefaultGateway()
				if err != nil {
					log.Fatalf("tun auto-route: %v (pass -tun-gateway to set it manually)", err)
				}
				gw = detected
			}
			// Exclude the coordinator (control plane) and the manual exit peer
			// (data plane). Relayed/discovered peers still rely on tun2socks
			// binding to the physical interface to avoid the loop.
			var selfRefs []string
			if *coordURL != "" {
				if u, err := url.Parse(*coordURL); err == nil {
					selfRefs = append(selfRefs, u.Host)
				}
			}
			selfRefs = append(selfRefs, connectHostIPs(*connect)...)
			excludes := tun.HostIPs(selfRefs...)

			routeCfg := tun.RouteConfig{
				Device:   *tunDevice,
				TUNAddr:  *tunAddr,
				Gateway:  gw,
				Excludes: excludes,
			}
			if err := tun.ApplyUp(routeCfg); err != nil {
				_ = tun.ApplyDown(routeCfg)
				log.Fatalf("tun auto-route: %v", err)
			}
			defer func() {
				if err := tun.ApplyDown(routeCfg); err != nil {
					log.Printf("tun auto-route teardown: %v", err)
				}
			}()
			fmt.Printf("TUN auto-route: default route → %s, gateway %s, %d self-exclusion(s)\n", *tunAddr, gw, len(excludes))
		}
	}

	// Local status dashboard.
	if *webAddr != "" {
		isClient := *toRegion != "" || *connect != ""
		socks := ""
		if isClient {
			socks = *socksAddr
		}
		started := time.Now()
		statusFn := func() webui.Status {
			curExit, curRegion := getState()
			exitOn := isExitOn()
			var groups []string
			if coord != nil {
				groups, _ = coord.GroupsOf(selfID)
			}
			role := "idle"
			switch {
			case exitOn && isClient:
				role = "client+exit"
			case exitOn:
				role = "exit"
			case isClient:
				role = "client"
			}
			return webui.Status{
				PeerID:        selfID,
				Role:          role,
				Coordinator:   *coordURL,
				ExitEnabled:   exitOn,
				ExitRegion:    *region,
				ExitLoad:      n.ExitLoad(),
				ToRegion:      curRegion,
				TrustScope:    *trustScope,
				ConnectedExit: curExit,
				SocksAddr:     socks,
				CanSwitch:     switchTo != nil,
				CanToggleExit: true,
				NeedsSetup:    needsSetup(),
				Groups:        groups,
				CanManage:     coord != nil,
				Uptime:        time.Since(started).Round(time.Second).String(),
			}
		}
		l, err := net.Listen("tcp", *webAddr)
		if err != nil {
			log.Fatalf("web console: %v", err)
		}
		var joinGroupFn func(string, string) error
		var endorseFn func(string, string) error
		if coord != nil {
			joinGroupFn = func(gid, secret string) error { return coord.JoinGroup(gid, selfID, secret) }
			endorseFn = func(from, to string) error { return coord.EndorseGroup(from, to) }
		}
		webSrv := webui.NewServer(statusFn, webui.Controls{
			SwitchRegion: switchTo,
			Setup:        setupFn,
			JoinGroup:    joinGroupFn,
			Endorse:      endorseFn,
			ToggleExit: func(on bool) error {
				setExit(on)
				// Re-register immediately so the change is visible to others now
				// (not only at the next refresh tick).
				if coord != nil {
					registerOnce(coord, selfID, n.AddrStrings(), *region, on, n.ExitLoad())
				}
				return nil
			},
		})
		go func() { _ = http.Serve(l, webSrv.Handler()) }()
		go func() { <-ctx.Done(); _ = l.Close() }()
		fmt.Printf("web console: http://%s\n", *webAddr)
	}

	fmt.Println("running; press Ctrl+C to stop")
	<-ctx.Done()
	fmt.Println("shutting down")
}

// startClient connects to the exit and serves a local SOCKS5 proxy that tunnels
// through dialer (which may be a SwitchableDialer for runtime exit switching).
func startClient(ctx context.Context, n *node.Node, exit peer.AddrInfo, socksAddr string, dialer proxy.Dialer, openUDP func() (proxy.UDPTunnel, error)) {
	if err := n.Connect(ctx, exit); err != nil {
		log.Fatalf("connect to exit %s: %v", exit.ID, err)
	}
	fmt.Printf("connected to exit %s\n", exit.ID)
	serveSOCKS(ctx, socksAddr, dialer, openUDP)
}

// serveSOCKS starts the local SOCKS5 ingress on socksAddr: TCP CONNECT via
// dialer, and UDP ASSOCIATE via openUDP (a UDP tunnel factory). The dialer may
// be a SwitchableDialer that is (re)pointed at an exit later.
func serveSOCKS(ctx context.Context, socksAddr string, dialer proxy.Dialer, openUDP func() (proxy.UDPTunnel, error)) {
	srv := &proxy.Server{Dialer: dialer, OpenUDPTunnel: openUDP}
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
	fmt.Printf("SOCKS5 proxy on %s\n", socksAddr)
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
func keepRegistered(ctx context.Context, c *coordinator.Client, selfID string, n *node.Node, region string, wantExit func() bool) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			registerOnce(c, selfID, n.AddrStrings(), region, wantExit(), n.ExitLoad())
		}
	}
}

// buildExitPolicy assembles an ExitGuard policy from CLI flags. SMTP ports are
// always blocked by default; -block-ports adds to them.
func buildExitPolicy(scopeStr, portsCSV, domainsCSV string, maxConns, minReputation int, allowPrivate bool) (exit.Policy, error) {
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
	return exit.Policy{
		Scope:                  scope,
		MinRequesterReputation: minReputation,
		BlockedPorts:           ports,
		BlockedDomains:         domains,
		MaxConns:               maxConns,
		AllowPrivateTargets:    allowPrivate,
	}, nil
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

// connectHostIPs pulls the host references (ip4/ip6/dns) out of a manual
// -connect multiaddr so TUN auto-routing can exclude the exit peer's traffic.
func connectHostIPs(connect string) []string {
	if connect == "" {
		return nil
	}
	maddr, err := multiaddr.NewMultiaddr(connect)
	if err != nil {
		return nil
	}
	var refs []string
	for _, p := range []int{multiaddr.P_IP4, multiaddr.P_IP6, multiaddr.P_DNS, multiaddr.P_DNS4, multiaddr.P_DNS6} {
		if v, err := maddr.ValueForProtocol(p); err == nil && v != "" {
			refs = append(refs, v)
		}
	}
	return refs
}
