// Command whatgate runs a WhatGate participant. A single node can act as a client
// (exposing a local SOCKS5 proxy that tunnels to a chosen exit) and/or as an
// exit (serving other nodes' traffic) — reflecting WhatGate's premise that every
// online user is both.
//
// Two ways to reach an exit:
//
//	# Manual (M1): connect to a known exit multiaddr.
//	whatgate -exit
//	whatgate -connect <exit-multiaddr> -socks 127.0.0.1:1080
//
//	# Via coordinator (M2): join, register, and discover an exit by region.
//	coordinator -addr :8080 -invite welcome
//	whatgate -coordinator http://host:8080 -invite welcome -exit -region JP
//	whatgate -coordinator http://host:8080 -invite welcome -to JP -socks 127.0.0.1:1080
//
// Prove egress happens at the exit:
//
//	curl --socks5-hostname 127.0.0.1:1080 https://api.ipify.org
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
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

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/whatgate/whatgate/internal/audit"
	"github.com/whatgate/whatgate/internal/config"
	"github.com/whatgate/whatgate/internal/coordinator"
	"github.com/whatgate/whatgate/internal/discovery"
	"github.com/whatgate/whatgate/internal/dnsx"
	"github.com/whatgate/whatgate/internal/exit"
	"github.com/whatgate/whatgate/internal/logging"
	"github.com/whatgate/whatgate/internal/membership"
	"github.com/whatgate/whatgate/internal/metrics"
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
	configPath := flag.String("config", "", "JSON config file whose keys are flag names; command-line flags override it")
	identityPath := flag.String("identity", "", "path to a persistent node private key (created on first run, owner-only)")
	logFormat := flag.String("log-format", "text", "log output format: text (human-readable) or json (one object per line, for log collectors)")
	listen := flag.String("listen", "/ip4/0.0.0.0/tcp/0", "libp2p listen multiaddr(s), comma-separated; add e.g. /ip4/0.0.0.0/tcp/443/ws to also ride :443 like web traffic (A4)")
	asExit := flag.Bool("exit", false, "act as an exit node for other peers (serves their traffic)")
	connect := flag.String("connect", "", "exit peer multiaddr to tunnel through (manual client mode)")
	socksAddr := flag.String("socks", "127.0.0.1:1080", "local SOCKS5 listen address (client mode)")
	coordURL := flag.String("coordinator", "", "coordinator base URL(s), comma-separated for failover, e.g. https://a:8080,https://b:8080")
	coordKey := flag.String("coordinator-key", "", "pinned coordinator public key (base64, printed by the coordinator): the directory must be signed by it, else it is rejected — defeats a rogue/MITM coordinator")
	coordCache := flag.String("coordinator-cache", "", "path to cache the last verified directory (requires -coordinator-key); served when every coordinator endpoint is unreachable, so a blocked coordinator doesn't instantly disconnect you")
	bootstrapURL := flag.String("bootstrap-url", "", "out-of-band URL (CDN/GitHub raw) serving a signed bootstrap list (requires -coordinator-key); when every known coordinator is blocked at cold start, fetch it to self-heal onto fresh endpoints")
	memberCertPath := flag.String("member-cert", "", "path to persist the member credential chain the coordinator issues on join (for Tier C decentralized discovery); owner-only")
	dhtEnable := flag.Bool("dht", false, "EXPERIMENTAL Tier C: also discover exits over a private authenticated DHT (needs -root-key; unverified on real infrastructure)")
	rootKeyStr := flag.String("root-key", "", "pinned OFFLINE root public key (base64) that anchors the member credential chain; required with -dht")
	dhtEpoch := flag.Uint64("dht-epoch", 1, "Tier C discovery capability epoch (advertiser and querier must agree; lets the operator rotate the namespace)")
	invite := flag.String("invite", "", "invite code to redeem when joining via coordinator")
	bootstrapFounder := flag.Bool("bootstrap-founder", false, "become the first member when the coordinator explicitly permits one-time first-member bootstrap")
	region := flag.String("region", "", "this node's exit region tag when acting as exit, e.g. JP")
	toRegion := flag.String("to", "", "desired exit region to discover via coordinator (client mode)")
	group := flag.String("group", "", "join (or create) this small-network group id")
	groupSecret := flag.String("group-secret", "", "secret required to join -group (the first joiner sets it)")
	endorse := flag.String("endorse", "", "endorse fromGroup:toGroup (your group vouches for another)")
	trustScope := flag.String("trust-scope", "", "trust range for exit selection: conservative|open (required with -to; no default by design)")
	rankTrustW := flag.Float64("rank-trust-weight", 0, "weighted exit ranking: trust weight (set any -rank-*-weight to use weighted composite scoring instead of the default trust→latency→load order)")
	rankLatencyW := flag.Float64("rank-latency-weight", 0, "weighted exit ranking: latency weight (lower latency preferred)")
	rankLoadW := flag.Float64("rank-load-weight", 0, "weighted exit ranking: load weight (lower load preferred)")
	latencyAlpha := flag.Float64("latency-ewma-alpha", 0.3, "exit-latency smoothing: EWMA weight of each new probe (0<a<=1; 1 disables smoothing so the latest probe is used as-is). Damps single slow probes across re-discovery rounds.")
	exitScope := flag.String("exit-scope", "open", "ExitGuard: whose traffic to serve as exit: conservative|open")
	blockPorts := flag.String("block-ports", "", "ExitGuard: extra destination ports to block, comma-separated (SMTP ports blocked by default)")
	blockDomains := flag.String("block-domains", "", "ExitGuard: destination domains to block, comma-separated")
	maxConns := flag.Int("max-conns", 0, "ExitGuard: max concurrent connections to serve as exit (0 = unlimited)")
	maxConnsPerReq := flag.Int("max-conns-per-requester", 0, "ExitGuard: max concurrent connections per requester peer (0 = unlimited; caps single-peer resource exhaustion)")
	reqRate := flag.Float64("requester-rate", 0, "ExitGuard: max new connections per second a single requester may open (0 = unlimited; throttles open/close churn that evades the concurrency cap)")
	reqBurst := flag.Float64("requester-burst", 0, "ExitGuard: momentary burst allowance for -requester-rate (0 = defaults to -requester-rate)")
	reqBandwidth := flag.Float64("requester-bandwidth", 0, "ExitGuard: max sustained bytes/sec a single requester may push through your link (0 = unlimited); over budget cuts the transfer, refuses new connections, and lowers the requester's reputation (circuit breaker)")
	reqBandwidthBurst := flag.Float64("requester-bandwidth-burst", 0, "ExitGuard: momentary byte burst allowance for -requester-bandwidth (0 = defaults to -requester-bandwidth)")
	minReputation := flag.Int("min-reputation", -1_000_000_000, "ExitGuard: refuse requesters whose reputation is below this (default effectively disabled)")
	allowPrivateTargets := flag.Bool("allow-private-targets", false, "ExitGuard: allow serving requests to private/loopback/link-local addresses (default false blocks SSRF to your LAN, localhost, and cloud metadata)")
	dnsServer := flag.String("dns-server", "", "exit: resolve hostname targets via this DNS server (host or host:port, :53 assumed) instead of the exit host's system resolver — use a trusted/uncensored resolver so a poisoned local resolver can't misdirect exits (empty = system resolver)")
	auditLog := flag.String("audit-log", "", "ExitGuard: append a JSON-lines record of served/denied requests to this file (accountability)")
	metricsAddr := flag.String("metrics-addr", "", "if set, serve operational counters as JSON at http://<addr>/metrics (e.g. 127.0.0.1:9090): exit served/denied-by-reason, etc.")
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
		fmt.Printf("whatgate %s\n", version)
		return
	}

	// A config file fills in any flag not set on the command line.
	if *configPath != "" {
		if err := config.ApplyFile(flag.CommandLine, *configPath); err != nil {
			log.Fatal(err)
		}
	}

	// Structured logging for operational diagnostics (the human-readable startup
	// banner below stays on stdout regardless).
	slog.SetDefault(logging.New(os.Stderr, *logFormat))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listenAddrs, err := node.ParseListenAddrs(*listen)
	if err != nil {
		log.Fatalf("bad -listen: %v", err)
	}
	nodeOpts := []node.Option{node.WithListenAddrs(listenAddrs...)}
	if *identityPath != "" {
		identity, err := discovery.LoadOrCreateSigningKey(*identityPath)
		if err != nil {
			log.Fatalf("identity: %v", err)
		}
		nodeOpts = append(nodeOpts, node.WithIdentity(identity))
	}

	var coord *coordinator.Client
	if *coordURL != "" {
		endpoints := splitEndpoints(*coordURL)
		coord = coordinator.NewClientEndpoints(endpoints)
		if len(endpoints) > 1 {
			fmt.Printf("coordinator endpoints: %d (failover)\n", len(endpoints))
		}
		// Pin the coordinator's signing key so the directory AND relay it returns
		// must be signed by that key — a reachable-but-rogue endpoint can't inject
		// a poisoned directory or steer this node onto an adversary-controlled
		// relay. Distributed out of band (printed by the coordinator).
		if *coordKey != "" {
			pub, err := discovery.DecodePublicKey(*coordKey)
			if err != nil {
				log.Fatalf("bad -coordinator-key: %v", err)
			}
			coord.SetPinnedKey(pub)
			fmt.Println("coordinator key: pinned (directory and relay responses are verified)")
			// The cache only holds verified (pinned) directories, so it is only
			// meaningful alongside a pinned key.
			if *coordCache != "" {
				coord.SetDirectoryCache(*coordCache)
				fmt.Printf("directory cache: %s\n", *coordCache)
			}
			if *bootstrapURL != "" {
				coord.SetBootstrapURL(*bootstrapURL)
				fmt.Printf("bootstrap URL: %s (self-heals endpoints when coordinators are unreachable)\n", *bootstrapURL)
			}
		} else {
			if *coordCache != "" {
				fmt.Println("WARNING: -coordinator-cache ignored without -coordinator-key (only verified directories are cached)")
			}
			if *bootstrapURL != "" {
				fmt.Println("WARNING: -bootstrap-url ignored without -coordinator-key (an unauthenticated bootstrap list is a poisoning vector and is refused)")
			}
			fmt.Println("WARNING: no -coordinator-key pinned; the directory is unauthenticated — a rogue/MITM coordinator can steer you onto a hostile exit")
		}
		// TLS carries the control plane's confidentiality; without it, which exits
		// you ask about is observable and the endpoint is easy to SNI/DNS-block.
		for _, ep := range endpoints {
			if strings.HasPrefix(ep, "http://") {
				fmt.Println("WARNING: a coordinator URL is plaintext http://; use https:// (or a TLS-terminating proxy) so control-plane metadata isn't exposed")
				break
			}
		}
		// Configure the coordinator's relay as a fallback path, if it advertises
		// one, so this node stays reachable when hole punching fails. With a pinned
		// key the relay is signature-verified; a verification failure surfaces
		// (possible MITM) rather than silently dropping the fallback, while a plain
		// "no relay configured" stays quiet.
		info, err := coord.Relay()
		switch {
		case err == nil:
			if ai, err := node.AddrInfoFromStrings(info.PeerID, info.Addrs); err == nil {
				nodeOpts = append(nodeOpts, node.WithStaticRelays(ai))
				fmt.Printf("relay fallback: %s\n", ai.ID)
			}
		case errors.Is(err, coordinator.ErrNoRelay):
			// coordinator advertises no relay — nothing to do
		default:
			fmt.Printf("WARNING: coordinator relay not configured (%v); if a key is pinned this may indicate tampering\n", err)
		}
	}

	n, err := node.New(ctx, nodeOpts...)
	if err != nil {
		log.Fatalf("start node: %v", err)
	}
	defer n.Close()
	selfID := n.ID().String()

	// Operational counters (exit outcomes, etc.), optionally scraped over HTTP.
	metricsReg := metrics.NewRegistry()
	if *metricsAddr != "" {
		mux := http.NewServeMux()
		mux.Handle("/metrics", metricsReg.Handler())
		ms := &http.Server{Addr: *metricsAddr, Handler: mux}
		go func() {
			if err := ms.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("metrics server", "err", err)
			}
		}()
		go func() { <-ctx.Done(); _ = ms.Close() }()
		fmt.Printf("metrics: http://%s/metrics\n", *metricsAddr)
	}

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
	currentTrustScope := func() string { return *trustScope }

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
		// Optional pinned resolver so the exit doesn't depend on its host's
		// (possibly poisoned/censored) system DNS.
		dnsResolver, err := dnsx.Resolver(*dnsServer)
		if err != nil {
			log.Fatalf("dns-server: %v", err)
		}
		if *dnsServer != "" {
			slog.Info("exit DNS resolver pinned", "server", *dnsServer)
		}
		// SSRF guard: block dialing private/loopback/metadata targets at connect
		// time (after DNS resolution, so hostname→internal rebinding is caught too),
		// unless the operator opts in with -allow-private-targets.
		dial := func(ctx context.Context, addr string) (net.Conn, error) {
			d := net.Dialer{Control: exit.DialControl(*allowPrivateTargets), Resolver: dnsResolver}
			return d.DialContext(ctx, "tcp", addr)
		}
		policy, err := buildExitPolicy(*exitScope, *blockPorts, *blockDomains, *maxConns, *maxConnsPerReq, *minReputation, *allowPrivateTargets, *reqRate, *reqBurst, *reqBandwidth, *reqBandwidthBurst)
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
					slog.Warn("threat feed fetch", "err", err)
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
			Dial:    dial,
			Guard:   guard,
			TierOf:  tierOf,
			RepOf:   repOf,
			Report:  report,
			Audit:   auditFn,
			Metrics: func(e string) { metricsReg.Inc("exit_" + e) },
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
		var dd *dhtDiscovery              // Tier C decentralized discovery (opt-in)
		var memberCert, issuerCert []byte // credential chain for the private discovery plane
		if *invite != "" || *bootstrapFounder {
			adm, err := coord.Join(*invite, selfID)
			if err != nil && *bootstrapURL != "" {
				// Cold-start self-heal (C2): every known coordinator is unreachable,
				// so pull a signed endpoint list from the out-of-band channel and
				// retry. RefreshFromBootstrap verifies against the pinned key, so a
				// tampering CDN can't redirect us onto adversary coordinators.
				fmt.Printf("join failed (%v); fetching bootstrap list from %s\n", err, *bootstrapURL)
				if berr := coord.RefreshFromBootstrap(*bootstrapURL); berr != nil {
					log.Fatalf("bootstrap self-heal failed: %v (join error: %v)", berr, err)
				}
				fmt.Printf("bootstrap: healed onto %d endpoint(s), retrying join\n", len(coord.Endpoints()))
				adm, err = coord.Join(*invite, selfID)
			}
			if err != nil {
				log.Fatalf("join via coordinator: %v", err)
			}
			fmt.Printf("joined network (vouched by %s)\n", adm.Issuer)
			memberCert, issuerCert = adm.MemberCert, adm.IssuerCert
			// C1: persist the member credential chain the coordinator issued, so
			// this node can later prove membership on the decentralized-discovery
			// plane (Tier C). Best-effort; absence just means no issuer configured.
			if len(adm.MemberCert) > 0 && *memberCertPath != "" {
				if err := saveMemberCredential(*memberCertPath, adm); err != nil {
					fmt.Printf("WARNING: could not save member credential: %v\n", err)
				} else {
					fmt.Printf("member credential: saved to %s\n", *memberCertPath)
				}
			}
		}

		if *group != "" {
			if err := coord.JoinGroup(*group, selfID, *groupSecret); err != nil {
				slog.Warn("join group", "group", *group, "err", err)
			} else {
				fmt.Printf("joined small-network group %s\n", *group)
			}
		}
		if *endorse != "" {
			if from, to, ok := strings.Cut(*endorse, ":"); ok {
				if err := coord.EndorseGroup(from, to); err != nil {
					slog.Warn("endorse", "from", from, "to", to, "err", err)
				} else {
					fmt.Printf("endorsed group %s -> %s\n", from, to)
				}
			} else {
				slog.Warn("bad -endorse; want fromGroup:toGroup", "value", *endorse)
			}
		}

		registerOnce(coord, selfID, n.AddrStrings(), *region, isExitOn(), n.ExitLoad())
		go keepRegistered(ctx, coord, selfID, n, *region, isExitOn)

		// EXPERIMENTAL Tier C (opt-in via -dht): start a private authenticated DHT
		// as a redundant discovery plane. Compile/unit verified only — the
		// gater/relay/DHT/NAT interaction needs two real machines to validate.
		if *dhtEnable {
			// Prefer a credential file (may hold an out-of-band exit cert); else use
			// the one issued on join.
			if *memberCertPath != "" {
				if mc, ic, err := loadMemberCredential(*memberCertPath); err == nil {
					memberCert, issuerCert = mc, ic
				}
			}
			dd = startDHTDiscovery(ctx, n, coord, *rootKeyStr, *dhtEpoch, *region, *asExit, memberCert, issuerCert, selfID)
		}

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
				scopeSet   bool
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

			rankWeights := routing.Weights{Trust: *rankTrustW, Latency: *rankLatencyW, Load: *rankLoadW}
			// One tracker for the node's lifetime so latency smoothing accumulates
			// across successive re-discovery rounds.
			latTracker := routing.NewLatencyTracker(*latencyAlpha)
			connectIn := func(sc trust.Scope, region string) (string, error) {
				scopeMu.Lock()
				curScope, scopeSet = sc, true
				scopeMu.Unlock()
				ai, err := discoverExit(ctx, n, coord, region, selfID, sc, dd, rankWeights, latTracker)
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

			needsSetup = func() bool { scopeMu.Lock(); defer scopeMu.Unlock(); return !scopeSet }
			currentTrustScope = func() string {
				scopeMu.Lock()
				defer scopeMu.Unlock()
				if scopeSet {
					return curScope.String()
				}
				return *trustScope
			}
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
				sc, ok := curScope, scopeSet
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
					if *webAddr == "" {
						log.Fatalf("discover exit: %v", err)
					}
					slog.Warn("no exit available yet; local controls remain online", "region", *toRegion, "err", err)
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
					slog.Warn("tun auto-route teardown", "err", err)
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
				TrustScope:    currentTrustScope(),
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
		var createInviteFn func(int) (string, error)
		if coord != nil {
			joinGroupFn = func(gid, secret string) error { return coord.JoinGroup(gid, selfID, secret) }
			endorseFn = func(from, to string) error { return coord.EndorseGroup(from, to) }
			createInviteFn = func(maxUses int) (string, error) { return coord.CreateInvite(maxUses) }
		}
		webSrv := webui.NewServer(statusFn, webui.Controls{
			SwitchRegion: switchTo,
			Setup:        setupFn,
			JoinGroup:    joinGroupFn,
			Endorse:      endorseFn,
			CreateInvite: createInviteFn,
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
			slog.Warn("socks server stopped", "err", err)
		}
	}()
	fmt.Printf("SOCKS5 proxy on %s\n", socksAddr)
}

// discoverExit queries the directory and picks the best exit in the desired
// region within the user's trust scope, ranked by trust, then measured latency,
// then reported load.
func discoverExit(ctx context.Context, n *node.Node, c *coordinator.Client, region, selfID string, scope trust.Scope, dd *dhtDiscovery, w routing.Weights, lat *routing.LatencyTracker) (peer.AddrInfo, error) {
	nodes, tiers, err := c.DirectoryFor(selfID)
	if err != nil {
		// With Tier C enabled we can still try DHT-only discovery when the
		// coordinator is entirely unreachable; otherwise surface the error.
		if dd == nil {
			return peer.AddrInfo{}, err
		}
		slog.Warn("coordinator unreachable; attempting DHT-only discovery", "err", err)
		nodes, tiers = nil, map[string]trust.Tier{}
	} else if c.LastDirectoryStale() {
		slog.Warn("coordinator unreachable; using cached (possibly stale) directory")
	}
	tierOf := func(p string) trust.Tier { return tiers[p] }

	// Tier C (opt-in): union the authoritative directory with exits discovered and
	// verified on the private DHT. DHT-only exits enter as strangers, so a
	// conservative scope excludes them (the DHT plane cannot vouch trust).
	if dd != nil {
		if dhtExits := resolveDHTExits(ctx, n, dd, region); len(dhtExits) > 0 {
			nodes, tierOf = routing.MergeExits(nodes, tierOf, dhtExits)
			fmt.Printf("DHT discovery: %d verified exit(s) merged\n", len(dhtExits))
		}
	}

	// Probe latency to each in-scope candidate; load comes from the directory.
	load := make(map[string]int, len(nodes))
	latency := make(map[string]int, len(nodes))
	for _, nd := range nodes {
		load[nd.PeerID] = nd.Load
		if nd.PeerID == selfID || !nd.WantExit || nd.Region != region || !scope.Allows(tierOf(nd.PeerID)) {
			continue
		}
		ai, err := node.AddrInfoFromStrings(nd.PeerID, nd.Addrs)
		if err != nil {
			continue
		}
		pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		if rtt, err := n.Ping(pctx, ai); err == nil {
			// Smooth reachable samples with EWMA across re-discovery rounds.
			latency[nd.PeerID] = lat.Observe(nd.PeerID, int(rtt.Milliseconds()))
		} else {
			// Unreachable this round: rank last, but don't poison the smoothed
			// history so a recovered exit isn't penalized for many rounds.
			latency[nd.PeerID] = 1 << 30
		}
		cancel()
	}

	metricsFn := func(p string) routing.Metrics {
		return routing.Metrics{LatencyMs: latency[p], Load: load[p]}
	}
	// Opt-in weighted composite ranking when any weight is set; otherwise the
	// default lexicographic order (trust → latency → load).
	var ranked []coordinator.NodeInfo
	if w != (routing.Weights{}) {
		ranked = routing.RankExitsWeighted(nodes, region, selfID, scope, tierOf, metricsFn, w)
	} else {
		ranked = routing.RankExits(nodes, region, selfID, scope, tierOf, metricsFn)
	}
	if len(ranked) == 0 {
		return peer.AddrInfo{}, fmt.Errorf("no exit in region %q within %s trust scope", region, scope)
	}
	best := ranked[0]
	fmt.Printf("selected exit %s in region %s (trust: %s, latency: %dms, load: %d)\n",
		best.PeerID, best.Region, tierOf(best.PeerID), latency[best.PeerID], load[best.PeerID])
	return node.AddrInfoFromStrings(best.PeerID, best.Addrs)
}

// dhtDiscovery holds the state for Tier C decentralized discovery (opt-in via
// -dht). EXPERIMENTAL: this wiring is compile- and unit-test-verified only; the
// gater/relay/DHT/NAT interaction requires two networked machines to validate
// end-to-end and has not been exercised on real infrastructure.
type dhtDiscovery struct {
	dht   *node.PrivateDHT
	root  crypto.PubKey
	epoch uint64
	equiv *membership.EquivocationGuard
}

// resolveDHTExits resolves and verifies exits for a region on the private DHT and
// returns the eligible ones as routing candidates.
func resolveDHTExits(ctx context.Context, n *node.Node, dd *dhtDiscovery, region string) []routing.DHTExit {
	rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	verified := n.ResolveExits(rctx, dd.dht, string(membership.RoleExit), region, dd.epoch, dd.root, membership.VerifyOpts{}, nil, dd.equiv, 0, 16)
	out := make([]routing.DHTExit, 0, len(verified))
	for _, ve := range verified {
		if !ve.Eligible {
			continue
		}
		out = append(out, routing.DHTExit{PeerID: ve.Record.Subject, Region: ve.Record.Region, Addrs: ve.Record.Addrs})
	}
	return out
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
		slog.Warn("register with coordinator", "err", err)
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
func buildExitPolicy(scopeStr, portsCSV, domainsCSV string, maxConns, maxConnsPerReq, minReputation int, allowPrivate bool, reqRate, reqBurst, reqBandwidth, reqBandwidthBurst float64) (exit.Policy, error) {
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
		MaxConnsPerRequester:   maxConnsPerReq,
		RequesterRatePerSec:    reqRate,
		RequesterBurst:         reqBurst,
		RequesterBytesPerSec:   reqBandwidth,
		RequesterByteBurst:     reqBandwidthBurst,
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
// splitEndpoints parses a comma-separated coordinator flag into trimmed,
// non-empty base URLs used for failover.
func splitEndpoints(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

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

// saveMemberCredential persists the member credential chain (member cert +
// root-signed issuer cert) the coordinator issued on join, owner-only and
// atomically. The node presents this chain to prove membership on the Tier C
// decentralized-discovery plane.
func saveMemberCredential(path string, adm coordinator.JoinResult) error {
	doc := struct {
		MemberCert json.RawMessage `json:"memberCert"`
		IssuerCert json.RawMessage `json:"issuerCert"`
	}{MemberCert: adm.MemberCert, IssuerCert: adm.IssuerCert}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// loadMemberCredential reads a member credential chain previously saved by
// saveMemberCredential.
func loadMemberCredential(path string) (memberCert, issuerCert []byte, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var doc struct {
		MemberCert json.RawMessage `json:"memberCert"`
		IssuerCert json.RawMessage `json:"issuerCert"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, nil, err
	}
	return doc.MemberCert, doc.IssuerCert, nil
}

// startDHTDiscovery sets up the EXPERIMENTAL Tier C private authenticated DHT: it
// pins the root, presents this node's credential over member-auth, seeds the
// member set from the coordinator directory, starts the DHT, and — when this node
// is an exit — signs, serves, and advertises its own record. Returns nil (with a
// warning) if prerequisites are missing. NOTE: unverified on real infrastructure.
func startDHTDiscovery(ctx context.Context, n *node.Node, coord *coordinator.Client, rootKeyStr string, epoch uint64, region string, asExit bool, memberCert, issuerCert []byte, selfID string) *dhtDiscovery {
	if rootKeyStr == "" {
		fmt.Println("WARNING: -dht requires -root-key (the pinned offline root); Tier C discovery disabled")
		return nil
	}
	root, err := discovery.DecodePublicKey(rootKeyStr)
	if err != nil {
		fmt.Printf("WARNING: bad -root-key: %v; Tier C discovery disabled\n", err)
		return nil
	}
	if len(memberCert) == 0 || len(issuerCert) == 0 {
		fmt.Println("WARNING: no member credential (join an issuer-configured coordinator or provide -member-cert); Tier C discovery disabled")
		return nil
	}
	n.SetMemberCredential(memberCert, issuerCert)

	// Seed the member set + DHT bootstrap peers from the coordinator directory —
	// the set of peers the coordinator has admitted as members.
	ms := node.NewMemberSet()
	ms.Add(n.ID())
	var bootstrap []peer.AddrInfo
	if dirNodes, err := coord.Directory(); err == nil {
		for _, dn := range dirNodes {
			pid, err := peer.Decode(dn.PeerID)
			if err != nil {
				continue
			}
			ms.Add(pid)
			if ai, err := node.AddrInfoFromStrings(dn.PeerID, dn.Addrs); err == nil {
				bootstrap = append(bootstrap, ai)
			}
		}
	}

	pdht, err := n.StartPrivateDHT(ctx, ms, bootstrap)
	if err != nil {
		fmt.Printf("WARNING: start private DHT failed: %v; Tier C discovery disabled\n", err)
		return nil
	}
	fmt.Printf("DHT: private discovery plane up (%d seeded members)\n", ms.Len())

	if asExit {
		// Advertise ourselves as an exit. Consumers verify our record against our
		// member cert, so this only helps if that cert grants the exit role.
		addrs := node.SafeDialAddrs(n.AddrStrings())
		if rec, err := n.SignSelfRecord([]membership.Role{membership.RoleExit}, region, addrs, 10*time.Minute, uint64(time.Now().Unix())); err == nil {
			n.SetNodeRecord(rec)
			if err := pdht.Advertise(ctx, string(membership.RoleExit), region, epoch); err != nil {
				fmt.Printf("WARNING: DHT advertise failed: %v\n", err)
			} else {
				fmt.Printf("DHT: advertising as exit in region %s (epoch %d)\n", region, epoch)
			}
			go readvertiseExit(ctx, n, pdht, region, epoch)
		}
	}
	return &dhtDiscovery{dht: pdht, root: root, epoch: epoch, equiv: membership.NewEquivocationGuard(16)}
}

// readvertiseExit periodically re-signs (with a fresh generation) and re-advertises
// this exit's record so it stays fresh on the DHT.
func readvertiseExit(ctx context.Context, n *node.Node, pdht *node.PrivateDHT, region string, epoch uint64) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			addrs := node.SafeDialAddrs(n.AddrStrings())
			if rec, err := n.SignSelfRecord([]membership.Role{membership.RoleExit}, region, addrs, 10*time.Minute, uint64(time.Now().Unix())); err == nil {
				n.SetNodeRecord(rec)
				_ = pdht.Advertise(ctx, string(membership.RoleExit), region, epoch)
			}
		}
	}
}
