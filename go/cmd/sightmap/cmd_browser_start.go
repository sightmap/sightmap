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
	"runtime"
	"slices"
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
	detachFlag := fset.Bool("detach", false, "Run the daemon in the background (its own session) and return once it is ready. Use this in scripts/agents: plain 'start' blocks the shell for the whole session.")
	logFileFlag := fset.String("log-file", "", "With --detach, where to write the daemon log (default: ~/.sightmap/logs/<site>-daemon.log)")
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

	// Detached mode: re-exec ourselves as a background daemon and return once it
	// is serving, so the launching shell is never held. Must come before any of
	// the launch work below — the re-exec'd child does all of that.
	if *detachFlag {
		return startDetached(args, *sightmapDir, *logFileFlag)
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
	// Headless auto-detection: on Linux with no display, a non-headless Chrome
	// dies with "Missing X server or $DISPLAY". Default to headless so a headless
	// host just works for agents; an explicit --headless or a real display skips it.
	if !*headlessFlag && shouldAutoHeadless() {
		*headlessFlag = true
		fmt.Fprintln(os.Stderr, "start: no display detected ($DISPLAY/$WAYLAND_DISPLAY unset) — running headless (pass --headless to silence).")
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
		report := chromeStderr.tailReport()
		return fmt.Errorf("start: chrome did not become ready: %w\n"+
			"  binary: %s\n  args:   %s\n%s%s",
			pollErr, chromePath, strings.Join(chromeArgs, " "), report,
			sandboxHint(report, slices.Contains(chromeArgs, "--no-sandbox")))
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
	fmt.Fprintf(os.Stderr, "  This is a foreground daemon — it holds THIS shell until you Ctrl-C.\n")
	fmt.Fprintf(os.Stderr, "  Run other 'sightmap browser' commands from a DIFFERENT shell, or launch with --detach in scripts/agents.\n")

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

// ── detach ──────────────────────────────────────────────────────────────────

// startDetached re-execs `browser start` (minus --detach) as a background daemon
// in its own session, redirects its output to a log file, waits until it is
// actually serving (session file + CDP + HTTP server), then returns. This is the
// scriptable/agent entry point: unlike bare `start` it does NOT hold the shell,
// and unlike `nohup start &` the daemon is not reaped when the launching shell
// tears down (it setsid's into its own session).
func startDetached(args []string, sightmapDir, logFile string) error {
	// Idempotent: if a live daemon already owns this corpus, don't spawn another.
	if info, err := browser.ReadSessionInfo(sightmapDir); err == nil && info.Port > 0 && isPortAlive(info.Port) {
		fmt.Fprintf(os.Stderr, "● already running  cdp=%d  server=%d  (see 'sightmap browser status')\n", info.Port, info.ServerPort)
		return nil
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("start --detach: locate self: %w", err)
	}

	logPath := logFile
	if logPath == "" {
		logPath = defaultDaemonLog(sightmapDir)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("start --detach: create log dir: %w", err)
	}
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("start --detach: open log %s: %w", logPath, err)
	}
	defer logf.Close()

	childArgs := append([]string{"browser", "start"}, stripStartFlags(args, "detach", "log-file")...)
	cmd := exec.Command(self, childArgs...)
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.Stdin = nil
	configureDetachedDaemon(cmd) // sysproc_*.go: setsid (unix) / detached (windows)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start --detach: launch daemon: %w", err)
	}
	daemonPID := 0
	if cmd.Process != nil {
		daemonPID = cmd.Process.Pid
	}
	// Watch for early exit (Chrome missing / sandbox / no display) so we fail fast
	// with the log instead of waiting out the whole readiness timeout. We never
	// block on this Wait; once the parent returns and exits, the setsid'd child is
	// reparented to init and keeps running.
	childDone := make(chan error, 1)
	go func() { childDone <- cmd.Wait() }()

	info, ready, exitErr := waitDetachedReady(sightmapDir, childDone, 25*time.Second)
	if exitErr != nil {
		return fmt.Errorf("start --detach: daemon exited before it was ready (%v)\n"+
			"  log: %s\n%s", exitErr, logPath, tailFile(logPath, 4000))
	}
	if !ready {
		return fmt.Errorf("start --detach: daemon did not become ready in time\n"+
			"  log: %s\n%s", logPath, tailFile(logPath, 4000))
	}

	fmt.Fprintf(os.Stderr, "● detached  cdp=%d  server=%d  daemon-pid=%d\n", info.Port, info.ServerPort, daemonPID)
	fmt.Fprintf(os.Stderr, "  log:  %s\n", logPath)
	fmt.Fprintf(os.Stderr, "  The daemon runs in the background and survives this shell.\n")
	fmt.Fprintf(os.Stderr, "  Drive it with other 'sightmap browser' commands; stop it with 'sightmap browser stop'.\n")
	return nil
}

// defaultDaemonLog picks a per-corpus daemon log under ~/.sightmap/logs, keyed by
// the site directory so concurrent corpora don't clash.
func defaultDaemonLog(sightmapDir string) string {
	home, _ := os.UserHomeDir()
	key := "default"
	if abs, err := filepath.Abs(sightmapDir); err == nil {
		if base := filepath.Base(filepath.Dir(abs)); base != "" && base != "." && base != string(os.PathSeparator) {
			key = base
		}
	}
	return filepath.Join(home, ".sightmap", "logs", key+"-daemon.log")
}

// waitDetachedReady polls until the daemon for sightmapDir is serving — session
// file written, CDP answering, and (when its port is known) the HTTP server up —
// or it returns early if the child process exits first (childDone) or the
// timeout elapses.
func waitDetachedReady(sightmapDir string, childDone <-chan error, timeout time.Duration) (browser.SessionInfo, bool, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-childDone:
			if err == nil {
				err = fmt.Errorf("process exited")
			}
			return browser.SessionInfo{}, false, err
		default:
		}
		info, err := browser.ReadSessionInfo(sightmapDir)
		if err == nil && info.Port > 0 && isPortAlive(info.Port) &&
			(info.ServerPort == 0 || serverAlive(info.ServerPort)) {
			return info, true, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return browser.SessionInfo{}, false, nil
}

// stripStartFlags removes the named flags (and their `=value` / separate-value
// forms) from a start arg list, so the detach parent can re-exec the child
// without them.
func stripStartFlags(args []string, names ...string) []string {
	drop := map[string]bool{}
	takesValue := map[string]bool{"--log-file": true, "-log-file": true}
	for _, n := range names {
		drop["--"+n] = true
		drop["-"+n] = true
	}
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if drop[a] {
			if takesValue[a] {
				i++ // skip the separate value token
			}
			continue
		}
		if eq := strings.IndexByte(a, '='); eq > 0 && drop[a[:eq]] {
			continue
		}
		out = append(out, a)
	}
	return out
}

// tailFile returns up to the last maxBytes of the file at path, for surfacing a
// failed daemon's log without dumping the whole thing.
func tailFile(path string, maxBytes int64) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if int64(len(data)) > maxBytes {
		data = data[int64(len(data))-maxBytes:]
	}
	return string(data)
}

// shouldAutoHeadless reports whether to default to headless: only on Linux with
// no X or Wayland display, where a non-headless Chrome would die with "Missing X
// server or $DISPLAY". macOS/Windows always have a usable display.
func shouldAutoHeadless() bool {
	return runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
}

// sandboxHint returns an actionable note when Chrome's launch failure looks like
// its zygote sandbox being blocked (unprivileged user namespaces clamped by
// AppArmor or a container policy). We deliberately do NOT auto-add --no-sandbox
// (it's a security downgrade) but point straight at the fix. Empty when the
// failure isn't sandbox-related or --no-sandbox is already set.
func sandboxHint(chromeStderr string, hasNoSandbox bool) string {
	if hasNoSandbox {
		return ""
	}
	for _, sig := range []string{"No usable sandbox", "Running as root without --no-sandbox", "namespace sandbox"} {
		if strings.Contains(chromeStderr, sig) {
			return "\n  hint: this host blocks Chrome's sandbox (unprivileged user namespaces are restricted —\n" +
				"        common under AppArmor on recent Ubuntu and inside containers).\n" +
				"        rerun with --chrome-flag=--no-sandbox"
		}
	}
	return ""
}

// runAttachedSession runs the daemon against a caller-launched Chrome reached at
// attachAddr (host:port CDP endpoint) instead of launching its own. It is a
// deliberately degraded mode (see the --attach flag help): no owned profile or
// extension guarantees, capture is complete only from attach onward, and the
// browser is left running when the daemon stops. The collector and devtools
// server are identical to the owned-launch path, since the collector needs
// nothing but a CDP address.
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
	pollCtx, pollCancel := context.WithCancel(context.Background())
	defer pollCancel()
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-pollCtx.Done():
				return
			case <-t.C:
				// The probe sets no client timeout of its own, so pollCtx is what
				// bounds it: a browser that accepts the socket and never answers
				// would otherwise wedge this loop and never report the browser gone.
				if !cdpVersionAlive(pollCtx, cdpAddr) {
					if pollCtx.Err() != nil {
						return // shutting down, not a dead browser
					}
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

	// Stop HTTP server and drop the session file. The browser is NOT touched.
	shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)
	os.Remove(browser.SessionFilePath(sightmapDir))
	return nil
}
