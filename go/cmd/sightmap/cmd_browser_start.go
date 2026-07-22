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
	"sync"
	"syscall"
	"time"

	"github.com/sightmap/sightmap/go/browser"
)

func runBrowserStart(args []string) error {
	fset := flag.NewFlagSet("start", flag.ContinueOnError)
	portFlag := fset.Int("port", 7891, "Sightmap HTTP server port (0 = auto-allocate)")
	cdpPortFlag := fset.Int("cdp-port", browser.DefaultCDPPort, "Chrome remote debugging port (0 = auto-allocate)")
	sightmapDir := fset.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	extensionsFlag := fset.String("extensions", "", "Comma-separated extension paths to load")
	urlFlag := fset.String("url", "", "Navigate here after launch")
	profileFlag := fset.String("profile", "", "Chrome user data dir (default: ~/.sightmap/profiles/default)")
	headlessFlag := fset.Bool("headless", false, "Run headless")
	waitFlag := fset.Float64("wait", 0, "Seconds to wait after navigation")
	if err := fset.Parse(args); err != nil {
		return err
	}

	// Apply .sightmap/config.yaml defaults for flags not explicitly set.
	cfg := loadSiteConfig(*sightmapDir)
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

	// ── Detect existing Chrome for this site ──────────────────────────────────
	existingInfo, _ := browser.ReadSessionInfo()
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
	// would fail to bind (go-pcol).
	resolvedCDPPort, err := browser.FindFreePortExcluding(*cdpPortFlag, resolvedServerPort)
	if err != nil {
		return fmt.Errorf("start: cdp port: %w", err)
	}

	// ── Start sightmap HTTP server in background ──────────────────────────────
	siteName := filepath.Base(cwd())
	compiled, err := compileCorpus(*sightmapDir, siteName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start: sightmap compile warning: %v\n", err)
		// non-fatal: continue without corpus
	}

	var mu sync.RWMutex
	current := compiled

	recompile := func() {
		c, compErr := compileCorpus(*sightmapDir, siteName)
		if compErr != nil {
			fmt.Fprintf(os.Stderr, "[serve-sightmap] compile error: %v\n", compErr)
			return
		}
		mu.Lock()
		current = c
		mu.Unlock()
		fmt.Fprintf(os.Stderr, "[serve-sightmap] recompiled (v%s)\n", c.Version)
	}

	go watchSightmapDir(*sightmapDir, recompile)

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

	srvAddr := fmt.Sprintf(":%d", resolvedServerPort)
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
		// This shouldn't happen since we pre-allocated via FindFreePort,
		// but handle the unlikely race gracefully.
		return fmt.Errorf("start: sightmap server could not bind port %d: %w", resolvedServerPort, srvErr)
	}

	// ── Launch Chrome ─────────────────────────────────────────────────────────
	chromePath, err := browser.FindChrome()
	if err != nil {
		return err
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
	extReinstalled := false
	if extPath == "" {
		extracted, reinstalled, extractErr := ensureExtension()
		if extractErr != nil {
			return fmt.Errorf("start: cannot load extension: %w\n"+
				"  Chrome will not be launched. Check the binary or clear ~/.sightmap/extension/ and retry.", extractErr)
		}
		extPath = extracted
		extReinstalled = reinstalled
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

	cmd := exec.Command(chromePath, chromeArgs...)
	setSysProcAttr(cmd) // platform-specific detach (defined in sysproc_*.go)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: launch chrome: %w", err)
	}

	cdpAddr := fmt.Sprintf("127.0.0.1:%d", resolvedCDPPort)
	if pollErr := pollCDPReady(cdpAddr); pollErr != nil {
		cmd.Process.Kill()
		return fmt.Errorf("start: chrome did not become ready: %w", pollErr)
	}

	// Write session file.
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	_ = browser.WriteSessionInfo(browser.SessionInfo{
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
		// If we just (re)installed a newer extension, force the service worker to
		// re-register from the new on-disk files — Chrome otherwise restores a stale
		// cached worker even across a full restart (go-erld). Done before any
		// navigation so the content script binds to the fresh worker.
		if extReinstalled {
			if rErr := browser.ReloadExtension(ctx, cdpAddr); rErr != nil {
				fmt.Fprintf(os.Stderr, "start: warning: could not reload extension: %v\n", rErr)
			} else {
				fmt.Fprintln(os.Stderr, "  reloaded extension to the new version (no manual chrome://extensions reload needed)")
			}
		}
		// Capture the initial tab ID.
		if tabs, listErr := browser.ListTabs(ctx, cdpAddr); listErr == nil && len(tabs) > 0 {
			initialTabID = tabs[0].TargetID
		}
		if *urlFlag != "" {
			// Retry navigation (same go-0029 fix as browser launch).
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
		_ = browser.RemoveSessionFile()
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
	os.Remove(sessionFilePath())

	return nil
}
