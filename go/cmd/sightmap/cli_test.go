package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sightmap/sightmap/go/browser"
)

// TestCDPAddrForDir covers the client-side session resolution: an explicit --addr
// wins, a present session file is honored silently, and the absence of one falls
// back to the default CDP port WITH a warning — the fix for a command silently
// attaching to another agent's session on the default port.
//
// Each subtest creates the corpus dir first, so SessionFilePath keys on the
// per-corpus <dir>/.session rather than the shared $TMPDIR fallback — keeping the
// assertions hermetic.
func TestCDPAddrForDir(t *testing.T) {
	t.Run("explicit addr wins, no warning", func(t *testing.T) {
		addr, warn := cdpAddrForDir("host:1234", mkCorpusDir(t))
		if addr != "host:1234" || warn != "" {
			t.Fatalf("cdpAddrForDir(explicit) = (%q, %q), want (host:1234, \"\")", addr, warn)
		}
	})

	t.Run("present session file is honored silently", func(t *testing.T) {
		dir := mkCorpusDir(t)
		if err := browser.WriteSessionInfo(dir, browser.SessionInfo{Port: 9999, ServerPort: 8888}); err != nil {
			t.Fatalf("seed session file: %v", err)
		}
		addr, warn := cdpAddrForDir("", dir)
		if addr != "localhost:9999" || warn != "" {
			t.Fatalf("cdpAddrForDir(session) = (%q, %q), want (localhost:9999, \"\")", addr, warn)
		}
	})

	t.Run("missing session file falls back loudly", func(t *testing.T) {
		dir := mkCorpusDir(t) // exists, but no .session written
		addr, warn := cdpAddrForDir("", dir)
		want := fmt.Sprintf("localhost:%d", browser.DefaultCDPPort)
		if addr != want {
			t.Errorf("cdpAddrForDir(no session) addr = %q, want %q", addr, want)
		}
		if warn == "" {
			t.Error("cdpAddrForDir(no session) returned no warning; want a foreign-session warning")
		}
	})
}

// mkCorpusDir makes a fresh <tmp>/.sightmap directory and returns its path.
func mkCorpusDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".sightmap")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir corpus dir: %v", err)
	}
	return dir
}
