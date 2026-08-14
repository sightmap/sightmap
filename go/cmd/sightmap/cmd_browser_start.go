package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/sightmap"
)

func runBrowserStart(args []string) error {
	fset := flag.NewFlagSet("start", flag.ContinueOnError)
	portFlag := fset.Int("port", 7891, "Sightmap HTTP server port (0 = auto-allocate)")
	cdpPortFlag := fset.Int("cdp-port", browser.DefaultCDPPort, "Chrome remote debugging port (0 = auto-allocate)")
	attachFlag := fset.String("attach", "", "Attach to an already-running Chrome's CDP endpoint (host:port) instead of launching one (degraded mode: no owned profile, capture is complete only from attach onward, browser is left running on stop)")
	sightmapDir := fset.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	extensionsFlag := fset.String("extensions", "", "Comma-separated extension paths to load")
	urlFlag := fset.String("url", "", "Navigate here after launch")
	profileFlag := fset.String("profile", "", "Chrome user data dir (default: ~/.sightmap/profiles/default)")
	headlessFlag := fset.Bool("headless", false, "Run headless")
	waitFlag := fset.Float64("wait", 0, "Seconds to wait after navigation")
	chromeBinaryFlag := fset.String("chrome-binary", "", "Path to a specific Chrome binary (overrides auto-detection)")
	var chromeFlags stringSliceFlag
	fset.Var(&chromeFlags, "chrome-flag", "Extra flag to pass to Chrome (repeatable), e.g. --chrome-flag=--no-sandbox")
	if err := fset.Parse(args); err != nil {
		return err
	}

	// Apply .sightmap/config.yaml defaults for flags not explicitly set.
	cfg := sightmap.LoadConfig(*sightmapDir)
	{
		explicit := map[string]bool{}
		fset.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
		if !explicit["wait"] && cfg.Snapshot.Wait > 0 {
			*waitFlag = cfg.Snapshot.Wait
		}
		if !explicit["port"] && cfg.Browser.Port > 0 {
			*portFlag = cfg.Browser.Port
		}
	}

	// Attach mode: hand off to the degraded, browser-not-owned path. It shares the
	// devtools server + collector with the owned-launch path below but skips the
	// launch/profile/extension machinery entirely.
	if *attachFlag != "" {
		return runAttachedSession(*attachFlag, *sightmapDir, *portFlag, *urlFlag)
	}

	// Resolve profile early so the existing-Chrome check can match it.
	resolvedProfile := *profileFlag
	if resolvedProfile == "" {
		profileName := cfg.Name
		if profileName == "" {
			profileName = filepath.Base(cwd())
		}
		home, _ := os.UserHomeDir()
		resolvedProfile = filepath.Join(home, ".sightmap", "profiles", profileName)
	}
	_ = os.MkdirAll(resolvedProfile, 0o700)

	// ── Detect existing Chrome for this corpus ────────────────────────────────
	// The session file is keyed to --sightmap-dir, so this only ever matches a
	// prior session for THIS corpus — never another agent's browser.
	existingInfo, _ := browser.ReadSessionInfo(*sightmapDir)
	if existingInfo.Port > 0 && isPortAlive(existingInfo.Port) {
		// Chrome is already running — open a new tab and return immediately.
		cdpAddr := fmt.Sprintf("localhost:%d", existingInfo.Port)
		ctx := context.Background()

		navigateURL := *urlFlag
		tabID, conn, err := browser.CreateTab(ctx, cdpAddr, navigateURL)
		if err != nil {
			return fmt.Errorf("start: open tab in existing Chrome: %w", err)
		}
		if navigateURL != "" {
			// CreateTab via /json/new?URL navigates immediately; wait for load.
			loadCtx, loadCancel := context.WithTimeout(ctx, 15*time.Second)
			_ = browser.WaitForLoad(loadCtx, conn)
			loadCancel()
		}
		conn.Close()

		fmt.Fprintf(os.Stderr, "● tab    cdp=%d  tab=%s\n", existingInfo.Port, tabID)
		fmt.Fprintf(os.Stderr, "  Chrome was already running for this site; opened a NEW tab for you.\n")
		fmt.Fprintf(os.Stderr, "  Pass --tab %s on EVERY command this session — other tabs belong to other agents.\n", tabID)
		return nil
	}

	// ── New Chrome launch ─────────────────────────────────────────────────────

	// Resolve ports: 0 means auto-allocate starting from default.
	resolvedServerPort, err := browser.FindFreePort(*portFlag)
	if err != nil {
		return fmt.Errorf("start: sightmap server: %w", err)
	}
	// Exclude the server port: if the server's default was busy it may have slid
	// onto the CDP default, and FindFreePort doesn't hold the port it returns, so
	// an unguarded CDP allocation would pick the same number and Chrome's DevTools
	// would fail to bind.
	resolvedCDPPort, err := browser.FindFreePortExcluding(*cdpPortFlag, resolvedServerPort)
	if err != nil {
		return fmt.Errorf("start: cdp port: %w", err)
	}

	// ── Start sightmap HTTP server in background ──────────────────────────────
	siteName := filepath.Base(cwd())
	srv, collectorPtr, err := startDevtoolsServer(*sightmapDir, siteName, resolvedServerPort)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}

	// ── Launch Chrome ─────────────────────────────────────────────────────────
	chromePath := *chromeBinaryFlag
	if chromePath == "" {
		found, findErr := browser.FindChrome()
		if findErr != nil {
			return findErr
		}
		chromePath = found
	}

	chromeArgs := []string{
		fmt.Sprintf("--remote-debugging-port=%d", resolvedCDPPort),
		fmt.Sprintf("--user-data-dir=%s", resolvedProfile),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-blink-features=AutomationControlled",
	}
	if *headlessFlag {
		chromeArgs = append(chromeArgs, "--headless=new")
	}
	extPath := *extensionsFlag
	if extPath == "" {
		extracted, extractErr := ensureExtension()
		if extractErr != nil {
			return fmt.Errorf("start: cannot load extension: %w\n"+
				"  Chrome will not be launched. Check the binary or clear ~/.sightmap/extension/ and retry.", extractErr)
		}
		extPath = extracted
	}
	if extPath != "" {
		absExt, absErr := filepath.Abs(extPath)
		if absErr != nil {
			absExt = extPath
		}
		chromeArgs = append(chromeArgs,
			"--load-extension="+absExt,
			"--disable-extensions-except="+absExt,
		)
	}

	// Root containers (the standard agent/CI env) need --no-sandbox or Chrome
	// refuses to start; add it automatically. Caller --chrome-flag values append
	// last so they can override.
	chromeArgs = finalChromeArgs(chromeArgs, os.Geteuid(), chromeFlags)

	var chromeStderr boundedBuffer
	cmd := exec.Command(chromePath, chromeArgs...)
	cmd.Stderr = &chromeStderr
	setSysProcAttr(cmd) // platform-specific detach (defined in sysproc_*.go)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: launch chrome: %w", err)
	}

	cdpAddr := fmt.Sprintf("127.0.0.1:%d", resolvedCDPPort)
	if pollErr := pollCDPReady(cdpAddr); pollErr != nil {
		cmd.Process.Kill()
		_ = cmd.Wait() // let the stderr copy goroutine finish before reading the buffer
		return fmt.Errorf("start: chrome did not become ready: %w\n"+
			"  binary: %s\n  args:   %s\n%s",
			pollErr, chromePath, strings.Join(chromeArgs, " "), chromeStderr.tailReport())
	}

	// Start the console/network collector against the now-ready session and
	// publish it to the devtools handlers registered above.
	collector := browser.NewCollector(cdpAddr)
	collector.Start()
	collectorPtr.Store(collector)
	defer collector.Stop()

	// Write session file.
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	_ = browser.WriteSessionInfo(*sightmapDir, browser.SessionInfo{
		Port: resolvedCDPPort, PID: pid, Pgid: pid, Profile: resolvedProfile,
		ServerPort: resolvedServerPort,
	})

	var initialTabID string
	ctx := context.Background()
	conn, dialErr := browser.DialCDP(ctx, cdpAddr)
	if dialErr == nil {
		// Inject server port into the extension's chrome.storage.local via CDP.
		// This replaces the shared sightmap-config.json file approach — each
		// Chrome profile has isolated storage, so sessions can't cross-talk.
		if injErr := browser.InjectExtensionServerPort(ctx, cdpAddr, resolvedServerPort); injErr != nil {
			fmt.Fprintf(os.Stderr, "start: warning: could not inject extension port: %v\n", injErr)
		}
		// Capture the initial tab ID.
		if tabs, listErr := browser.ListTabs(ctx, cdpAddr); listErr == nil && len(tabs) > 0 {
			initialTabID = tabs[0].TargetID
		}
		if *urlFlag != "" {
			// Retry navigation (Chrome may reuse an existing process and lag on
			// registering the initial tab).
			var navErr error
			for attempt := range 3 {
				if attempt > 0 {
					time.Sleep(300 * time.Millisecond)
				}
				navErr = browser.NavigateAndWait(ctx, conn, *urlFlag)
				if navErr == nil {
					break
				}
			}
			if navErr != nil {
				fmt.Fprintf(os.Stderr, "start: warning: could not navigate to %s: %v\n", *urlFlag, navErr)
			}
			if *waitFlag > 0 {
				time.Sleep(time.Duration(*waitFlag * float64(time.Second)))
			}
		}
		conn.Close()
	}

	fmt.Fprintf(os.Stderr, "● ready  port=%d  cdp=%d  pid=%d  tab=%s\n",
		resolvedServerPort, resolvedCDPPort, pid, initialTabID)
	fmt.Fprintf(os.Stderr, "  Addr: --addr localhost:%d\n", resolvedCDPPort)
	fmt.Fprintf(os.Stderr, "  Tab:  --tab %s  (pass this on commands once other agents open tabs here)\n", initialTabID)
	fmt.Fprintf(os.Stderr, "  Press Ctrl-C to stop.\n")

	// ── Block until signal or Chrome exits ───────────────────────────────────
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	chromeDone := make(chan error, 1)
	go func() { chromeDone <- cmd.Wait() }()

	select {
	case <-sigCh:
		fmt.Fprintln(os.Stderr, "\nstopping...")
	case <-chromeDone:
		// Chrome exited on its own (user quit the browser window).
		// Exit cleanly so the sightmap server releases its port.
		fmt.Fprintln(os.Stderr, "\nChrome exited — sightmap server stopping")
		// Chrome is already gone; skip the SIGTERM/kill below.
		_ = browser.RemoveSessionFile(*sightmapDir)
		return nil
	}

	// Signal path: stop Chrome.
	if cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-chromeDone:
		case <-time.After(3 * time.Second):
			cmd.Process.Kill()
		}
	}

	// Stop HTTP server.
	shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)

	// Remove session file.
	os.Remove(browser.SessionFilePath(*sightmapDir))

	return nil
}

// startDevtoolsServer builds and starts the sightmap HTTP server for a browser
// session on serverPort: it serves the live corpus (/sightmap, /sightmap/version,
// hot-reloaded from sightmapDir) and the devtools query surface (network/console).
// It returns the running server plus the collector pointer the devtools handlers
// read from — the caller Stores the live collector into it once the CDP session
// is ready. Both the owned-launch and --attach paths share this.
func startDevtoolsServer(sightmapDir, siteName string, serverPort int) (*http.Server, *atomic.Pointer[browser.Collector], error) {
	compiled, err := loadServedSightmap(sightmapDir, siteName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start: sightmap load warning: %v\n", err)
		// non-fatal: continue without corpus
	}

	var mu sync.RWMutex
	current := compiled

	reload := func() {
		c, loadErr := loadServedSightmap(sightmapDir, siteName)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "[serve-sightmap] load error: %v\n", loadErr)
			return
		}
		mu.Lock()
		current = c
		mu.Unlock()
		fmt.Fprintf(os.Stderr, "[serve-sightmap] reloaded (v%s)\n", c.Version)
	}

	go watchSightmapDir(sightmapDir, reload)

	mux := http.NewServeMux()
	mux.HandleFunc("/sightmap/version", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		v := current.Version
		mu.RUnlock()
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"version":%q}`, v)
	})
	mux.HandleFunc("/sightmap", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		c := current
		mu.RUnlock()
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(c)
	})

	// Devtools query surface. The collector is created after the CDP session is
	// ready; the handlers return 503 until then.
	collectorPtr := &atomic.Pointer[browser.Collector]{}
	registerDevtoolsHandlers(mux, collectorPtr, sightmapDir)

	// Bind the sightmap server on the IPv4 loopback (not the ":port" wildcard) so
	// it shares an address family with Chrome's CDP and with FindFreePort's probe.
	// This keeps concurrent daemons from landing one daemon's server on another's
	// CDP port, and keeps the server local-only.
	srvAddr := fmt.Sprintf("127.0.0.1:%d", serverPort)
	srv := &http.Server{Addr: srvAddr, Handler: mux}
	srvReady := make(chan error, 1)
	go func() {
		ln, listenErr := net.Listen("tcp", srvAddr)
		if listenErr != nil {
			srvReady <- listenErr
			return
		}
		srvReady <- nil
		fmt.Fprintf(os.Stderr, "[serve-sightmap] listening on http://%s\n", srvAddr)
		_ = srv.Serve(ln)
	}()
	if srvErr := <-srvReady; srvErr != nil {
		return nil, nil, fmt.Errorf("sightmap server could not bind port %d: %w", serverPort, srvErr)
	}
	return srv, collectorPtr, nil
}

// runAttachedSession runs the daemon against a caller-launched Chrome reached at
// attachAddr (host:port CDP endpoint) instead of launching its own. It is a
// deliberately degraded mode (see the --attach flag help): no owned profile or
// extension guarantees, capture is complete only from attach onward, and the
// browser is left running when the daemon stops. The collector and devtools
// server are identical to the owned-launch path — the collector only ever needed
// a CDP address.
func runAttachedSession(attachAddr, sightmapDir string, serverPortFlag int, urlFlag string) error {
	// Normalize the endpoint. A bare ":port" or "port" is treated as localhost.
	host, port, err := net.SplitHostPort(attachAddr)
	if err != nil {
		// Allow a bare port number.
		if p, perr := strconv.Atoi(strings.TrimSpace(attachAddr)); perr == nil && p > 0 {
			host, port = "", strconv.Itoa(p)
		} else {
			return fmt.Errorf("start --attach: invalid CDP endpoint %q (want host:port)", attachAddr)
		}
	}
	if host == "" {
		host = "localhost"
	}
	cdpPort, err := strconv.Atoi(port)
	if err != nil || cdpPort <= 0 || cdpPort > 65535 {
		return fmt.Errorf("start --attach: invalid CDP port in %q", attachAddr)
	}
	cdpAddr := net.JoinHostPort(host, port)

	// Verify the endpoint is actually reachable before we advertise a session.
	if !cdpVersionAlive(context.Background(), cdpAddr) {
		return fmt.Errorf("start --attach: no Chrome CDP endpoint answering at %s\n"+
			"  Launch Chrome with --remote-debugging-port=%d first, then retry.", cdpAddr, cdpPort)
	}

	resolvedServerPort, err := browser.FindFreePort(serverPortFlag)
	if err != nil {
		return fmt.Errorf("start --attach: sightmap server: %w", err)
	}

	siteName := filepath.Base(cwd())
	srv, collectorPtr, err := startDevtoolsServer(sightmapDir, siteName, resolvedServerPort)
	if err != nil {
		return fmt.Errorf("start --attach: %w", err)
	}

	collector := browser.NewCollector(cdpAddr)
	collector.Start()
	collectorPtr.Store(collector)
	defer collector.Stop()

	// Record the session. PID is the DAEMON's pid (there is no owned Chrome), and
	// Attached tells stop to detach rather than kill. Profile is empty.
	_ = browser.WriteSessionInfo(sightmapDir, browser.SessionInfo{
		Port: cdpPort, PID: os.Getpid(), ServerPort: resolvedServerPort, Attached: true,
	})

	// Install signal handling and liveness polling up front, so the daemon is
	// always interruptible even while the best-effort setup below is still running.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	gone := make(chan struct{})
	stopPoll := make(chan struct{})
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stopPoll:
				return
			case <-t.C:
				if !cdpVersionAlive(context.Background(), cdpAddr) {
					close(gone)
					return
				}
			}
		}
	}()

	ctx := context.Background()
	// Best-effort extension port injection, off the main path: on a browser without
	// the sightmap extension (common in attach mode) the service-worker lookup
	// retries for several seconds, which must not delay readiness or block stop.
	go func() {
		injCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if injErr := browser.InjectExtensionServerPort(injCtx, cdpAddr, resolvedServerPort); injErr != nil {
			fmt.Fprintf(os.Stderr, "start --attach: note: could not inject extension port: %v\n", injErr)
		}
	}()

	var tabID string
	if urlFlag != "" {
		// Open our own tab rather than disturbing the caller's existing tabs.
		id, conn, tabErr := browser.CreateTab(ctx, cdpAddr, urlFlag)
		if tabErr != nil {
			fmt.Fprintf(os.Stderr, "start --attach: warning: could not open tab for %s: %v\n", urlFlag, tabErr)
		} else {
			tabID = id
			loadCtx, loadCancel := context.WithTimeout(ctx, 15*time.Second)
			_ = browser.WaitForLoad(loadCtx, conn)
			loadCancel()
			conn.Close()
		}
	}

	fmt.Fprintf(os.Stderr, "● attached  cdp=%s  server=%d  tab=%s\n", cdpAddr, resolvedServerPort, tabID)
	fmt.Fprintf(os.Stderr, "  Addr: --addr %s\n", cdpAddr)
	fmt.Fprintf(os.Stderr, "  Degraded mode: browser is caller-owned; console/network capture is live from now on.\n")
	fmt.Fprintf(os.Stderr, "  Press Ctrl-C (or 'browser stop') to detach — the browser is left running.\n")

	// Block until a signal, or until the attached browser goes away.
	select {
	case <-sigCh:
		fmt.Fprintln(os.Stderr, "\ndetaching (browser left running)...")
	case <-gone:
		fmt.Fprintln(os.Stderr, "\nattached browser went away — detaching")
	}
	close(stopPoll)

	// Stop HTTP server and drop the session file. The browser is NOT touched.
	shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)
	os.Remove(browser.SessionFilePath(sightmapDir))
	return nil
}
