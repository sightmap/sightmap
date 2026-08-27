package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
  start [--detach] [--port N] [--extensions PATH] [--url URL] [--profile DIR]  launch Chrome + sightmap server (blocks the shell; --detach backgrounds it)
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
  wait-for    --url PATTERN | --selector SEL | --component QUERY | --view NAME | --load  [--timeout-ms N]
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

// serverAlive reports whether the sightmap HTTP server (console/network + live
// corpus reload) answers on port. It is distinct from cdpVersionAlive, which
// probes Chrome's DevTools endpoint: the two are separate processes on separate
// ports, and a reaped daemon can leave Chrome's CDP up while the server is gone.
func serverAlive(port int) bool {
	if port <= 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://127.0.0.1:%d/sightmap/version", port), nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
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

// defaultProfileDir reconstructs the Chrome --user-data-dir the corpus at
// sightmapDir would use, mirroring browser start. Lets stop/status reap a
// profile's Chrome even when the session file is gone.
func defaultProfileDir(sightmapDir string) string {
	cfg := sightmap.LoadConfig(sightmapDir)
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
func runStop(args []string) error {
	sightmapDir, _ := resolveSightmapDir(args)
	info, infoErr := browser.ReadSessionInfo(sightmapDir)
	hasSession := infoErr == nil && (info.Port > 0 || info.PID > 0 || info.Pgid > 0)

	// Attached session: the browser is caller-owned, so never kill it. Signal the
	// daemon (info.PID is the daemon's own pid in attach mode) to detach, and drop
	// the session file. The daemon's own signal handler also tears down; both
	// removing the file is harmless.
	if hasSession && info.Attached {
		if info.PID > 0 {
			if proc, perr := os.FindProcess(info.PID); perr == nil {
				_ = proc.Signal(syscall.SIGTERM)
			}
		}
		_ = browser.RemoveSessionFile(sightmapDir)
		fmt.Fprintln(os.Stderr, "detached (browser left running)")
		return nil
	}

	profile := ""
	if hasSession {
		profile = info.Profile
	}
	if profile == "" {
		profile = defaultProfileDir(sightmapDir)
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
		os.Remove(browser.SessionFilePath(sightmapDir))
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
	sightmapDir, args := resolveSightmapDir(args)
	tabID, _ := resolveTab(args)

	info, err := browser.ReadSessionInfo(sightmapDir)
	if err != nil {
		// A session file that exists but doesn't parse is a different situation
		// from no session at all: report it loudly with the expected shape so a
		// hand-written or corrupted file is a one-step fix, not a red herring.
		if _, statErr := os.Stat(browser.SessionFilePath(sightmapDir)); statErr == nil {
			fmt.Printf("✗ unreadable session file: %v\n"+
				"  expected shape: {\"port\":N,\"pid\":N,...}\n"+
				"  delete it and run 'browser start' to launch a fresh session.\n", err)
			return nil
		}
		// No session file — but an orphan Chrome for this profile may still be alive
		// (e.g. a prior stop dropped the file without reaping the process).
		if profile := defaultProfileDir(sightmapDir); profileProcessAlive(profile) {
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
		if info.ServerPort > 0 && info.ServerPort == info.Port {
			fmt.Printf("  the sightmap server and CDP were assigned the same port (%d) — a known startup collision\n", info.Port)
		}
		if profileProcessAlive(info.Profile) {
			fmt.Println("  a Chrome process for this profile is still alive — run 'browser stop' to reap it.")
		}
		fmt.Println("  run 'browser start' to launch a fresh session.")
		os.Remove(browser.SessionFilePath(sightmapDir))
		return nil
	}

	addr := fmt.Sprintf("localhost:%d", info.Port)
	if info.ServerPort > 0 && !serverAlive(info.ServerPort) {
		// CDP is up but the sightmap HTTP server is gone — typically a reaped
		// daemon (Chrome is launched detached and outlives its launcher). A
		// CDP-only probe would call this "running", but console/network and live
		// corpus reload are dead, so report the true, degraded state.
		fmt.Printf("⚠ degraded  cdp=%d up, but the sightmap server (:%d) is not responding  pid=%d\n",
			info.Port, info.ServerPort, info.PID)
		fmt.Printf("  console/network + live corpus reload are unavailable (the daemon was likely reaped).\n")
		fmt.Printf("  recover: 'browser stop' then 'browser start' — use --detach in scripts so the daemon survives.\n")
		fmt.Printf("  profile: %s\n", info.Profile)
	} else if info.ServerPort > 0 {
		fmt.Printf("● running  cdp=%d  server=%d  pid=%d\n", info.Port, info.ServerPort, info.PID)
		fmt.Printf("  profile: %s\n", info.Profile)
	} else {
		fmt.Printf("● running  cdp=%d  pid=%d\n", info.Port, info.PID)
		fmt.Printf("  profile: %s\n", info.Profile)
	}

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

// resolveSightmapDir extracts the --sightmap-dir flag value from args if
// present (defaulting to ".sightmap"), returning it and the remaining args. It
// keys session-file lookup so concurrent sessions for different corpora stay
// isolated.
func resolveSightmapDir(args []string) (dir string, rest []string) {
	for i, a := range args {
		if a == "--sightmap-dir" && i+1 < len(args) {
			return args[i+1], append(args[:i:i], args[i+2:]...)
		}
	}
	return ".sightmap", args
}

// resolveAddr extracts the --addr flag value from args if present, returning the
// address and the remaining args. When --addr is absent it falls back to the
// CDP port recorded in the session file for the corpus at sightmapDir.
func resolveAddr(args []string, sightmapDir string) (addr string, rest []string) {
	for i, a := range args {
		if a == "--addr" && i+1 < len(args) {
			return args[i+1], append(args[:i:i], args[i+2:]...)
		}
	}
	return browser.DefaultAddr(sightmapDir), args
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
			"  sightmap browser start\n"+
			"  sightmap browser start --url https://...\n", addr)
	}
	return conn, nil
}

func runNavigate(args []string) error {
	sightmapDir, args := resolveSightmapDir(args)
	addr, args := resolveAddr(args, sightmapDir)
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

	ctx := context.Background()
	if err := browser.NavigateAndWait(ctx, conn, url); err != nil {
		return err
	}
	// The URL right after load reflects server-side (HTTP) redirects — e.g. HD
	// silently redirects some category URLs, and a caller needs to know where it
	// actually landed.
	finalURL, _ := browser.GetURL(ctx, conn)
	// A client-side (SPA) redirect fires AFTER the load event (an auth guard
	// bouncing /login → /, or / → /last-workspace). GetURL alone misses it; wait
	// briefly for a follow-up navigation so we report the real destination.
	if clientURL, moved := browser.AwaitNavigation(ctx, conn, 2*time.Second); moved && clientURL != "" {
		finalURL = clientURL
	}
	if finalURL != "" && finalURL != url {
		fmt.Fprintf(os.Stderr, "navigated to %s\n  (redirected to %s)\n", url, finalURL)
	} else {
		fmt.Fprintf(os.Stderr, "navigated to %s\n", url)
	}
	return nil
}

func runEval(args []string) error {
	sightmapDir, args := resolveSightmapDir(args)
	addr, args := resolveAddr(args, sightmapDir)
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
