package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DefaultCDPPort is the Chrome remote-debugging (CDP) port sightmap starts
// from. It deliberately avoids Chrome's conventional 9222 — that port is
// crowded (other Chrome-based tooling defaults there too), so sightmap groups
// its ports next to the HTTP server default (7891) instead.
const DefaultCDPPort = 7892

// SessionInfo is written to the session file by Launch and by cmd/browser.
// It lets other tools (snapshot, sel-probe, dump) find the running Chrome
// instance without requiring an explicit --addr flag.
//
// ServerPort is the sightmap HTTP server port (default 7891). It is recorded
// here so callers can display it and so the extension config can be validated.
type SessionInfo struct {
	Port       int    `json:"port"`
	PID        int    `json:"pid"`                  // 0 if Chrome reused an existing process
	Pgid       int    `json:"pgid,omitempty"`       // process-group id (unix) for group-kill on stop
	Profile    string `json:"profile"`              // user-data-dir used for this session
	ServerPort int    `json:"serverPort,omitempty"` // sightmap HTTP server port (0 = unknown/legacy)
	// TargetID removed — use --tab flag on individual commands instead
}

// ReadSessionInfo reads and parses the session file. Returns an error if
// the file is absent or unparseable.
func ReadSessionInfo() (SessionInfo, error) {
	data, err := os.ReadFile(sessionFilePath())
	if err != nil {
		return SessionInfo{}, err
	}
	s := strings.TrimSpace(string(data))
	// New format: JSON object.
	if strings.HasPrefix(s, "{") {
		var info SessionInfo
		if err := json.Unmarshal([]byte(s), &info); err != nil {
			return SessionInfo{}, fmt.Errorf("session file: %w", err)
		}
		return info, nil
	}
	// Legacy format: plain port number.
	port, err := strconv.Atoi(s)
	if err != nil || port <= 0 || port > 65535 {
		return SessionInfo{}, fmt.Errorf("session file: invalid content")
	}
	return SessionInfo{Port: port}, nil
}

// FindFreePort finds the first available TCP port starting from start.
// It tries up to 100 consecutive ports before giving up.
func FindFreePort(start int) (int, error) {
	return FindFreePortExcluding(start)
}

// FindFreePortExcluding finds the first available TCP port starting from start,
// skipping any port listed in exclude. This is needed when allocating several
// ports in one go: FindFreePort only *probes* a port (it opens then immediately
// closes a listener) without holding it, so two back-to-back calls starting from
// the same value will return the SAME port. Passing the already-resolved port(s)
// as exclusions guarantees the allocations are mutually distinct — e.g. when the
// sightmap server's default port is busy it slides onto the CDP default, and
// without this the CDP allocation would collide with it (go-pcol).
func FindFreePortExcluding(start int, exclude ...int) (int, error) {
	excluded := make(map[int]bool, len(exclude))
	for _, p := range exclude {
		excluded[p] = true
	}
	for port := start; port < start+100; port++ {
		if excluded[port] {
			continue
		}
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port found in range %d–%d", start, start+99)
}

// RemoveSessionFile deletes the session file, if present.
func RemoveSessionFile() error {
	return os.Remove(sessionFilePath())
}

// WriteSessionInfo writes info to the session file.
func WriteSessionInfo(info SessionInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(sessionFilePath(), data, 0o600)
}

// LaunchOptions controls how a Chrome subprocess is started.
type LaunchOptions struct {
	Headless       bool     // false by default — headed is bot-friendlier
	Port           int      // 0 = pick a free port automatically
	UserDataDir    string   // "" = create a temp dir (removed on cleanup)
	StartURL       string   // navigate here after launch; "" = about:blank
	HideAutomation bool     // add --disable-blink-features=AutomationControlled
	ExtraArgs      []string // passed verbatim to Chrome
}

// Launch starts a Chrome subprocess and returns a connected CDPConn.
// cleanup kills the process and removes any auto-created UserDataDir.
func Launch(ctx context.Context, opts LaunchOptions) (*CDPConn, func(), error) {
	binary, err := FindChrome()
	if err != nil {
		return nil, nil, err
	}

	port := opts.Port
	if port == 0 {
		ln, listenErr := net.Listen("tcp", "127.0.0.1:0")
		if listenErr != nil {
			return nil, nil, fmt.Errorf("launch: find free port: %w", listenErr)
		}
		port = ln.Addr().(*net.TCPAddr).Port
		ln.Close()
	}

	userDataDir := opts.UserDataDir
	var tmpDir string
	if userDataDir == "" {
		d, mkErr := os.MkdirTemp("", "sightmap-chrome-*")
		if mkErr != nil {
			return nil, nil, fmt.Errorf("launch: create user data dir: %w", mkErr)
		}
		tmpDir = d
		userDataDir = tmpDir
	}

	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		fmt.Sprintf("--user-data-dir=%s", userDataDir),
		"--no-first-run",
		"--no-default-browser-check",
	}
	if opts.HideAutomation {
		args = append(args, "--disable-blink-features=AutomationControlled")
	}
	if opts.Headless {
		args = append(args, "--headless=new")
	}
	if opts.StartURL != "" {
		args = append(args, opts.StartURL)
	}
	args = append(args, opts.ExtraArgs...)

	cmd := exec.CommandContext(ctx, binary, args...)
	if startErr := cmd.Start(); startErr != nil {
		if tmpDir != "" {
			os.RemoveAll(tmpDir)
		}
		return nil, nil, fmt.Errorf("launch: start chrome: %w", startErr)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if pollErr := pollCDPReady(ctx, addr); pollErr != nil {
		cmd.Process.Kill()
		cmd.Wait()
		if tmpDir != "" {
			os.RemoveAll(tmpDir)
		}
		return nil, nil, fmt.Errorf("launch: %w", pollErr)
	}

	// Write session file so CLI tools can find the running instance.
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	_ = WriteSessionInfo(SessionInfo{
		Port:    port,
		PID:     pid,
		Profile: userDataDir,
	})

	cleanup := func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
		if tmpDir != "" {
			os.RemoveAll(tmpDir)
		}
		os.Remove(sessionFilePath())
	}

	conn, dialErr := DialCDP(ctx, addr)
	if dialErr != nil {
		cleanup()
		return nil, nil, fmt.Errorf("launch: dial cdp: %w", dialErr)
	}

	return conn, cleanup, nil
}

// FindChrome returns the path to the Chrome binary on the current platform,
// or an error if none is found.
func FindChrome() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		// Build candidate list: prefer Chrome for Testing (supports --load-extension),
		// then Canary, then stable Chrome (which blocks --load-extension on macOS).
		var candidates []string

		// 1. Sightmap-managed CfT install (~/.sightmap/browsers/).
		if p, ok := InstalledCfTPath(); ok {
			candidates = append(candidates, p)
		}

		// 2. agent-browser-managed CfT (~/.agent-browser/browsers/) — legacy.
		if home, err := os.UserHomeDir(); err == nil {
			if matches, _ := filepath.Glob(filepath.Join(home, ".agent-browser", "browsers", "chrome-*",
				"Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing")); len(matches) > 0 {
				// filepath.Glob returns sorted; take the last for the highest version.
				candidates = append(candidates, matches[len(matches)-1])
			}
		}

		// 3. System Chrome (Canary before stable; both block --load-extension but
		//    are useful as a fallback for non-extension workflows).
		candidates = append(candidates,
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		)
		for _, p := range candidates {
			if p == "" {
				continue
			}
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		return "", fmt.Errorf("chrome: no Chrome binary found on macOS")

	case "linux":
		names := []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"}
		for _, name := range names {
			if p, err := exec.LookPath(name); err == nil {
				return p, nil
			}
		}
		return "", fmt.Errorf("chrome: no Chrome binary found in PATH on Linux")

	case "windows":
		candidates := []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("PROGRAMFILES"), "Google", "Chrome", "Application", "chrome.exe"),
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		return "", fmt.Errorf("chrome: no Chrome binary found on Windows")

	default:
		return "", fmt.Errorf("chrome: unsupported OS %q", runtime.GOOS)
	}
}

// DefaultAddr reads the session file written by Launch and returns
// "localhost:PORT". Falls back to the DefaultCDPPort if no file exists.
func DefaultAddr() string {
	info, err := ReadSessionInfo()
	if err != nil || info.Port <= 0 || info.Port > 65535 {
		return fmt.Sprintf("localhost:%d", DefaultCDPPort)
	}
	return fmt.Sprintf("localhost:%d", info.Port)
}

// sessionFilePath returns the path where Launch writes the port number.
// Uses .sightmap/.session if a .sightmap/ directory exists in the working
// directory, otherwise falls back to $TMPDIR/sightmap-session.
func sessionFilePath() string {
	if _, err := os.Stat(".sightmap"); err == nil {
		return filepath.Join(".sightmap", ".session")
	}
	return filepath.Join(os.TempDir(), "sightmap-session")
}

// pollCDPReady polls http://ADDR/json/version every 100 ms until Chrome
// responds or the deadline (10 s) / ctx is exceeded.
func pollCDPReady(ctx context.Context, addr string) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reqCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		req, _ := http.NewRequestWithContext(reqCtx, "GET", "http://"+addr+"/json/version", nil)
		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err == nil {
			resp.Body.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("chrome did not become ready within 10s at %s: %w", addr, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
