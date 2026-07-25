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
	"path/filepath"
	"syscall"
	"time"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/sightmap"
)

func runBrowser(args []string) error {
	if len(args) == 0 {
		browserUsage()
		return nil
	}
	switch args[0] {
	case "install":
		return runBrowserInstall(args[1:])
	case "start":
		return runBrowserStart(args[1:])
	case "launch":
		return runLaunch(args[1:])
	case "stop":
		return runStop(args[1:])
	case "status":
		return runStatus(args[1:])
	case "navigate":
		return runNavigate(args[1:])
	case "eval":
		return runEval(args[1:])
	case "click":
		return runClick(args[1:])
	case "fill":
		return runFill(args[1:])
	case "hover":
		return runHover(args[1:])
	case "keypress", "key-press":
		return runKeyPress(args[1:])
	case "scroll":
		return runScroll(args[1:])
	case "drag":
		return runDrag(args[1:])
	case "wait-for", "wait_for":
		return runWaitFor(args[1:])
	case "dialog":
		return runDialog(args[1:])
	case "screenshot":
		return runScreenshot(args[1:])
	case "bounds":
		return runBounds(args[1:])
	case "clear-storage":
		return runClearStorage(args[1:])
	case "tabs":
		return runTabs(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "sightmap browser: unknown subcommand %q\n", args[0])
		browserUsage()
		return nil
	}
}

func browserUsage() {
	fmt.Fprint(os.Stderr, `sightmap browser — Chrome session management and interaction

Setup:
  install                                                           download Chrome for Testing

Session:
  start [--port N] [--extensions PATH] [--url URL] [--profile DIR]  launch Chrome + sightmap server
  launch [--headless] [--port N] [--profile DIR] [--extensions PATH[,PATH]] [--url URL] [--wait N]
  stop
  status
  navigate <url>
  eval <script>

Interaction (IDs from sightmap snapshot output):
  click    COMPONENT-ID | --x N --y N
  fill     COMPONENT-ID VALUE
  hover    COMPONENT-ID | --x N --y N
  keypress KEY                        (Enter Tab Escape ArrowUp/Down Backspace ...)
  scroll   [--component-id ID] [--delta-x N] [--delta-y N]
  drag        COMPONENT-ID --delta-x N --delta-y N
  wait-for    --url PATTERN | --selector SEL | --load  [--timeout-ms N]
  dialog      accept|dismiss [--text INPUT]
  screenshot  [--out FILE] [--stdout] [--component NAME | --selector SEL] [--expand-pct N]
  bounds      QUERY... | --selector SEL | --all   viewport-% bounding boxes (JSON)

Session utilities:
  clear-storage [--origin URL]   wipe cookies + storage (reset session state)

Tabs:
  tabs list
  tabs new [URL]
  tabs close  <target-id>
  tabs resize <width> <height>

`)
}

// ── launch ────────────────────────────────────────────────────────────────────

func runLaunch(args []string) error {
	fs := flag.NewFlagSet("launch", flag.ContinueOnError)
	headless := fs.Bool("headless", false, "Run headless")
	port := fs.Int("port", browser.DefaultCDPPort, "CDP port (0 = pick a free one)")
	profile := fs.String("profile", "", "User data dir (default: ~/.sightmap/profiles/default)")
	url := fs.String("url", "", "Navigate here after launch")
	extensions := fs.String("extensions", "", "Comma-separated extension directory paths to load (--load-extension)")
	wait := fs.Float64("wait", 0, "Seconds to wait after navigation")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Check for a live existing session.
	if info, err := browser.ReadSessionInfo(); err == nil && info.Port > 0 {
		if isPortAlive(info.Port) {
			return fmt.Errorf("a session is already running on port %d\n"+
				"Run 'browser stop' first, or 'browser status' to inspect it.", info.Port)
		}
		// Stale session file — clean it up silently.
		os.Remove(sessionFilePath())
	}

	chromePath, err := browser.FindChrome()
	if err != nil {
		return err
	}

	resolvedPort := *port
	if resolvedPort == 0 {
		p, err := freePort()
		if err != nil {
			return fmt.Errorf("find free port: %w", err)
		}
		resolvedPort = p
	}

	resolvedProfile := *profile
	if resolvedProfile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("home dir: %w", err)
		}
		resolvedProfile = filepath.Join(home, ".sightmap", "profiles", "default")
	}
	if err := os.MkdirAll(resolvedProfile, 0o700); err != nil {
		return fmt.Errorf("create profile dir: %w", err)
	}

	chromeArgs := []string{
		fmt.Sprintf("--remote-debugging-port=%d", resolvedPort),
		fmt.Sprintf("--user-data-dir=%s", resolvedProfile),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-blink-features=AutomationControlled", // always for authoring
	}
	if *headless {
		chromeArgs = append(chromeArgs, "--headless=new")
	}
	if *extensions != "" {
		absExt, absErr := filepath.Abs(*extensions)
		if absErr != nil {
			absExt = *extensions
		}
		chromeArgs = append(chromeArgs,
			"--load-extension="+absExt,
			"--disable-extensions-except="+absExt,
		)
	}
	if fs.NArg() > 0 {
		chromeArgs = append(chromeArgs, fs.Args()...)
	}

	// Use plain exec.Command — NOT exec.CommandContext — so Chrome keeps
	// running after this process exits.
	cmd := exec.Command(chromePath, chromeArgs...)

	// Detach: new process group so Chrome isn't killed by terminal signals
	// sent to our process.
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start chrome: %w", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", resolvedPort)
	if err := pollCDPReady(addr); err != nil {
		// Chrome may have exited or reused an existing process. If the port
		// is responding anyway (macOS process reuse), treat as success.
		if !isPortAlive(resolvedPort) {
			cmd.Process.Kill()
			cmd.Wait()
			return fmt.Errorf("chrome did not become ready: %w", err)
		}
	}

	// Record the PID — may be 0 if the process already exited due to macOS
	// reusing an existing Chrome instance.
	pid := 0
	if proc := cmd.Process; proc != nil {
		pid = proc.Pid
	}

	if err := browser.WriteSessionInfo(browser.SessionInfo{
		Port:    resolvedPort,
		PID:     pid,
		Profile: resolvedProfile,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write session file: %v\n", err)
	}

	ctx := context.Background()

	if *url != "" {
		// Retry DialCDP + NavigateAndWait up to 3 times with 300ms gaps.
		// On macOS, Chrome may reuse an existing process: the port responds to
		// /json/version immediately but the initial tab isn't registered in
		// /json/list for ~200ms, causing DialCDP to get a malformed response.
		var navErr error
		for attempt := range 3 {
			if attempt > 0 {
				time.Sleep(300 * time.Millisecond)
			}
			conn, dialErr := browser.DialCDP(ctx, addr)
			if dialErr != nil {
				navErr = dialErr
				continue
			}
			navErr = browser.NavigateAndWait(ctx, conn, *url)
			conn.Close()
			if navErr == nil {
				break
			}
		}
		if navErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not navigate to %s: %v\n", *url, navErr)
		}
		if *wait > 0 {
			time.Sleep(time.Duration(*wait * float64(time.Second)))
		}
	}

	// Print port to stdout (scripts can capture it).
	fmt.Println(resolvedPort)
	fmt.Fprintf(os.Stderr, "Chrome launched on port %d  (pid %d)\n", resolvedPort, pid)
	fmt.Fprintf(os.Stderr, "Profile: %s\n", resolvedProfile)
	return nil
}

// sessionFilePath delegates to the browser package's internal logic by
// re-reading what WriteSessionInfo would write. We use a local copy of the
// path-finding logic here for the stale-file cleanup case.
func sessionFilePath() string {
	if _, err := os.Stat(".sightmap"); err == nil {
		return filepath.Join(".sightmap", ".session")
	}
	return filepath.Join(os.TempDir(), "sightmap-session")
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port, nil
}

// cdpVersionAlive reports whether a genuine Chrome DevTools endpoint is
// listening at addr (host:port). It is deliberately stricter than a bare
// TCP/HTTP probe: the sightmap HTTP server can occupy the same port and answer
// /json/version with a 404, which an err==nil check mistakes for a live browser
// . A real DevTools endpoint returns HTTP 200 with a JSON body that
// carries a browser-level webSocketDebuggerUrl (and a "Browser" string), so we
// require those before declaring CDP alive.
func cdpVersionAlive(ctx context.Context, addr string) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/json/version", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var v struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
		Browser              string `json:"Browser"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return false
	}
	return v.WebSocketDebuggerURL != "" || v.Browser != ""
}

func isPortAlive(port int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return cdpVersionAlive(ctx, fmt.Sprintf("127.0.0.1:%d", port))
}

func pollCDPReady(addr string) error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		alive := cdpVersionAlive(ctx, addr)
		cancel()
		if alive {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for CDP at %s", addr)
}

// ── stop ──────────────────────────────────────────────────────────────────────

// defaultProfileDir reconstructs the Chrome --user-data-dir this site would use,
// mirroring browser start. Lets stop/status reap a profile's Chrome even when the
// session file is gone.
func defaultProfileDir() string {
	cfg := sightmap.LoadConfig(".sightmap")
	name := cfg.Name
	if name == "" {
		name = filepath.Base(cwd())
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sightmap", "profiles", name)
}

// portOrProfileAlive reports whether the CDP port answers OR any process for the
// profile is still running — the real "is Chrome still up" check.
func portOrProfileAlive(port int, profile string) bool {
	if port > 0 && isPortAlive(port) {
		return true
	}
	return profile != "" && profileProcessAlive(profile)
}

// runStop reaps the session's Chrome robustly: kill the whole process GROUP (not
// just the leader — macOS can relaunch the recorded PID), fall back to reaping by
// the profile --user-data-dir (covers a missing session file and PID reuse), and
// only remove the session file once the port AND profile are actually dead (so we
// never leave an invisible orphan).
func runStop(_ []string) error {
	info, infoErr := browser.ReadSessionInfo()
	hasSession := infoErr == nil && (info.Port > 0 || info.PID > 0 || info.Pgid > 0)

	profile := ""
	if hasSession {
		profile = info.Profile
	}
	if profile == "" {
		profile = defaultProfileDir()
	}

	acted := false
	switch {
	case hasSession && info.Pgid > 0:
		fmt.Fprintf(os.Stderr, "stopping Chrome (pgid %d, port %d)...\n", info.Pgid, info.Port)
		_ = terminateGroup(info.Pgid)
		acted = true
	case hasSession && info.PID > 0:
		fmt.Fprintf(os.Stderr, "stopping Chrome (pid %d, port %d)...\n", info.PID, info.Port)
		if proc, perr := os.FindProcess(info.PID); perr == nil {
			_ = proc.Signal(syscall.SIGTERM)
			acted = true
		}
	}

	// Grace period, then SIGKILL the group if still up.
	if acted {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && portOrProfileAlive(info.Port, profile) {
			time.Sleep(200 * time.Millisecond)
		}
		if info.Pgid > 0 && portOrProfileAlive(info.Port, profile) {
			_ = killGroup(info.Pgid)
		}
	}

	// Profile-path fallback: reap survivors / the no-session orphan.
	if !hasSession || portOrProfileAlive(info.Port, profile) {
		if reapByProfile(profile) {
			acted = true
			time.Sleep(400 * time.Millisecond)
		}
	}

	// Only drop the session file once nothing is alive.
	if portOrProfileAlive(info.Port, profile) {
		return fmt.Errorf("Chrome may still be running for this profile after stop attempts\n"+
			"  try manually: pkill -f %q", profile)
	}
	if hasSession {
		os.Remove(sessionFilePath())
	}
	if !acted {
		fmt.Fprintln(os.Stderr, "no active session")
		return nil
	}
	fmt.Fprintln(os.Stderr, "stopped")
	return nil
}

// ── status ────────────────────────────────────────────────────────────────────

func runStatus(args []string) error {
	tabID, _ := resolveTab(args)

	info, err := browser.ReadSessionInfo()
	if err != nil {
		// No session file — but an orphan Chrome for this profile may still be alive
		// (e.g. a prior stop dropped the file without reaping the process).
		if profile := defaultProfileDir(); profileProcessAlive(profile) {
			fmt.Printf("⚠ orphan  Chrome is running for this profile but there is no session file\n"+
				"  profile: %s\n  run 'browser stop' to reap it.\n", profile)
			return nil
		}
		fmt.Println("○ no session")
		return nil
	}

	if !isPortAlive(info.Port) {
		// The recorded port is not answering as a Chrome DevTools endpoint. This
		// covers both a fully-dead session and the case where the sightmap
		// HTTP server occupied the CDP port (so /json/version 404s). Either way the
		// session is unusable — clear it and tell the user how to recover.
		fmt.Printf("✗ unreachable  cdp=%d pid=%d  (CDP not responding; removing stale session file)\n",
			info.Port, info.PID)
		if info.ServerPort == info.Port {
			fmt.Printf("  the sightmap server and CDP were assigned the same port (%d) — a known startup collision\n", info.Port)
		}
		if profileProcessAlive(info.Profile) {
			fmt.Println("  a Chrome process for this profile is still alive — run 'browser stop' to reap it.")
		}
		fmt.Println("  run 'browser start' to launch a fresh session.")
		os.Remove(sessionFilePath())
		return nil
	}

	addr := fmt.Sprintf("localhost:%d", info.Port)
	fmt.Printf("● running  cdp=%d  pid=%d\n", info.Port, info.PID)
	fmt.Printf("  profile: %s\n", info.Profile)

	// Enumerate CONTENT tabs (the extension side panel is excluded) so an agent
	// can see exactly which tab IDs to pass via --tab. Page-affecting commands
	// require --tab whenever more than one is open.
	tabs, listErr := browser.ListTabs(context.Background(), addr)
	if listErr != nil || len(tabs) == 0 {
		fmt.Println("  tabs: (none)")
		return nil
	}
	fmt.Printf("  tabs (%d):\n", len(tabs))
	for _, t := range tabs {
		marker := "  "
		if tabID != "" && t.TargetID == tabID {
			marker = "→ "
		}
		url := t.URL
		if len(url) > 70 {
			url = url[:67] + "..."
		}
		fmt.Printf("  %s--tab %s  %s\n", marker, t.TargetID, url)
	}
	if len(tabs) > 1 {
		fmt.Println("  multiple tabs open — pass --tab <ID> on page commands to avoid crosstalk.")
	}
	return nil
}

// ── navigate / eval ───────────────────────────────────────────────────────────

// resolveAddr extracts the --addr flag value from args if present,
// returning the address and the remaining args.
func resolveAddr(args []string) (addr string, rest []string) {
	for i, a := range args {
		if a == "--addr" && i+1 < len(args) {
			return args[i+1], append(args[:i:i], args[i+2:]...)
		}
	}
	return browser.DefaultAddr(), args
}

// resolveTab extracts the --tab flag value from args, returning the tabID and
// remaining args. Returns ("", args) if --tab is absent.
func resolveTab(args []string) (tabID string, rest []string) {
	for i, a := range args {
		if a == "--tab" && i+1 < len(args) {
			return args[i+1], append(args[:i:i], args[i+2:]...)
		}
	}
	return "", args
}

func dial(addr string) (*browser.CDPConn, error) {
	ctx := context.Background()
	conn, err := browser.DialCDP(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Chrome at %s\n"+
			"Start a session first:\n"+
			"  browser launch\n"+
			"  browser launch --url https://...\n", addr)
	}
	return conn, nil
}

func runNavigate(args []string) error {
	addr, args := resolveAddr(args)
	tabID, args := resolveTab(args)
	if len(args) == 0 {
		return fmt.Errorf("usage: browser navigate <url>")
	}
	url := args[0]

	conn, err := browser.Connect(addr, tabID)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := browser.NavigateAndWait(context.Background(), conn, url); err != nil {
		return err
	}
	// Print the FINAL url after any server-side redirects, not the one we asked
	// for — e.g. HD silently redirects some category URLs to a different
	// category, and a caller needs to know where it actually landed.
	finalURL, urlErr := browser.GetURL(context.Background(), conn)
	if urlErr == nil && finalURL != "" && finalURL != url {
		fmt.Fprintf(os.Stderr, "navigated to %s\n  (redirected to %s)\n", url, finalURL)
	} else {
		fmt.Fprintf(os.Stderr, "navigated to %s\n", url)
	}
	return nil
}

func runEval(args []string) error {
	addr, args := resolveAddr(args)
	tabID, args := resolveTab(args)
	if len(args) == 0 {
		return fmt.Errorf("usage: browser eval <script>")
	}
	script := args[0]

	conn, err := browser.Connect(addr, tabID)
	if err != nil {
		return err
	}
	defer conn.Close()

	result, err := browser.EvalJSON(context.Background(), conn, script)
	if err != nil {
		return err
	}

	// Pretty-print if valid JSON object/array, otherwise raw.
	var v interface{}
	if jsonErr := json.Unmarshal(result, &v); jsonErr == nil {
		out, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Println(string(result))
	}
	return nil
}
