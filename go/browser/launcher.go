package browser

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
	Port       int    `json:"port"`                 // Chrome CDP (remote-debugging) port
	PID        int    `json:"pid"`                  // 0 if Chrome reused an existing process; the DAEMON's own pid in attach mode
	Pgid       int    `json:"pgid,omitempty"`       // process-group id (unix) for group-kill on stop
	Profile    string `json:"profile"`              // user-data-dir used for this session (empty in attach mode)
	ServerPort int    `json:"serverPort,omitempty"` // sightmap HTTP server port (0 = unknown/legacy)
	// Attached is true when the daemon attached to a caller-launched Chrome via
	// --attach rather than launching (and owning) its own. In that mode stop must
	// NOT kill the browser — it only signals the daemon to detach. PID then holds
	// the daemon's own pid, not Chrome's, and Profile is empty.
	Attached bool `json:"attached,omitempty"`
	// TargetID removed — use --tab flag on individual commands instead
}

// ReadSessionInfo reads and parses the session file for the corpus at
// sightmapDir. Returns an error if the file is absent or unparseable.
func ReadSessionInfo(sightmapDir string) (SessionInfo, error) {
	data, err := os.ReadFile(SessionFilePath(sightmapDir))
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
		// A parsed-but-portless object (e.g. a hand-written file whose keys don't
		// match, so Port stays 0) is not a usable session — surface it as an
		// unrecognized format rather than returning a silent Port:0.
		if info.Port <= 0 || info.Port > 65535 {
			return SessionInfo{}, fmt.Errorf(`session file: unrecognized format (expected {"port":N,"pid":N,...})`)
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
// without this the CDP allocation would collide with it.
func FindFreePortExcluding(start int, exclude ...int) (int, error) {
	excluded := make(map[int]bool, len(exclude))
	for _, p := range exclude {
		excluded[p] = true
	}
	for port := start; port < start+100; port++ {
		if excluded[port] {
			continue
		}
		// Probe on the IPv4 loopback specifically. Chrome's --remote-debugging-port
		// binds 127.0.0.1, and the sightmap server binds the same, so a loopback
		// probe detects BOTH. A bare ":port" probe binds the IPv6 wildcard on some
		// systems and would miss an existing IPv4-loopback listener — letting a
		// second daemon's server collide with a first daemon's CDP port.
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port found in range %d–%d", start, start+99)
}

// RemoveSessionFile deletes the session file for the corpus at sightmapDir, if present.
func RemoveSessionFile(sightmapDir string) error {
	return os.Remove(SessionFilePath(sightmapDir))
}

// WriteSessionInfo writes info to the session file for the corpus at sightmapDir.
func WriteSessionInfo(sightmapDir string, info SessionInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(SessionFilePath(sightmapDir), data, 0o600)
}

// FindChrome returns the path to the Chrome binary on the current platform,
// or an error if none is found.
func FindChrome() (string, error) {
	return findChrome(runtime.GOOS, InstalledCfTPath)
}

// findChrome resolves a Chrome binary for goos. managed supplies the
// sightmap-managed Chrome-for-Testing path (what `browser install` downloads);
// it is preferred on every platform because it supports --load-extension.
// InstalledCfTPath only reports ok when the binary actually exists. goos is a
// parameter so per-OS resolution is unit-testable on any host.
func findChrome(goos string, managed func() (string, bool)) (string, error) {
	// Managed Chrome for Testing first, on every platform. This is the #44 fix:
	// Linux and Windows previously ignored the managed install, so the documented
	// `browser install` → `browser start` flow failed on those OSes.
	if p, ok := managed(); ok {
		return p, nil
	}

	switch goos {
	case "darwin":
		// Prefer a legacy agent-browser CfT, then system Chrome (Canary before
		// stable; both block --load-extension but work for non-extension flows).
		var candidates []string
		if home, err := os.UserHomeDir(); err == nil {
			if matches, _ := filepath.Glob(filepath.Join(home, ".agent-browser", "browsers", "chrome-*",
				"Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing")); len(matches) > 0 {
				candidates = append(candidates, matches[len(matches)-1])
			}
		}
		candidates = append(candidates,
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		)
		for _, p := range candidates {
			if p != "" {
				if _, err := os.Stat(p); err == nil {
					return p, nil
				}
			}
		}
		return "", fmt.Errorf("chrome: no Chrome found — run `sightmap browser install` for a managed Chrome for Testing, or install Google Chrome")

	case "linux":
		for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
			if p, err := exec.LookPath(name); err == nil {
				return p, nil
			}
		}
		return "", fmt.Errorf("chrome: no Chrome found — run `sightmap browser install` to fetch a managed Chrome for Testing into ~/.sightmap/browsers/, or put google-chrome/chromium on PATH")

	case "windows":
		for _, p := range []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("PROGRAMFILES"), "Google", "Chrome", "Application", "chrome.exe"),
		} {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		return "", fmt.Errorf("chrome: no Chrome found — run `sightmap browser install` for a managed Chrome for Testing, or install Google Chrome")

	default:
		return "", fmt.Errorf("chrome: unsupported OS %q", goos)
	}
}

// DefaultAddr reads the session file for the corpus at sightmapDir and returns
// "localhost:PORT". Falls back to the DefaultCDPPort if no file exists.
func DefaultAddr(sightmapDir string) string {
	info, err := ReadSessionInfo(sightmapDir)
	if err != nil || info.Port <= 0 || info.Port > 65535 {
		return fmt.Sprintf("localhost:%d", DefaultCDPPort)
	}
	return fmt.Sprintf("localhost:%d", info.Port)
}

// SessionFilePath returns the path where the session for the corpus at
// sightmapDir records its ports. The file lives inside the corpus directory
// (<sightmapDir>/.session) so that concurrent sessions for different corpora
// never share a rendezvous file — the root cause of cross-agent crosstalk. An
// empty sightmapDir is treated as the default ".sightmap". Only when the corpus
// directory does not exist does it fall back to a single shared file under
// $TMPDIR (best-effort for the genuinely corpus-less case).
func SessionFilePath(sightmapDir string) string {
	if sightmapDir == "" {
		sightmapDir = ".sightmap"
	}
	if fi, err := os.Stat(sightmapDir); err == nil && fi.IsDir() {
		return filepath.Join(sightmapDir, ".session")
	}
	return filepath.Join(os.TempDir(), "sightmap-session")
}
