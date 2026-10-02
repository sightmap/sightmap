package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/browser"
)

// fallbackWarn is the substr every client-side resolver emits on stderr when it
// falls back to the default CDP port because no usable session file exists. It
// mirrors the literal in cdpAddrForDir (cli.go) — re-stated here so a future
// tweak to the prose doesn't silently drop the contract under test.
const fallbackWarn = "falling back to the default CDP port"

// captureStderr runs fn with os.Stderr redirected to a pipe and returns what
// was written. The resolvers route their warning through resolveCDPAddr's
// fmt.Fprintln(os.Stderr, ...), so observing stderr is the direct way to prove
// a command actually reached the warning-emitting helper rather than the older
// silent fallback. Output is small (a single warning line), so a synchronous
// read after fn returns cannot deadlock on the pipe buffer.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(out)
}

// TestResolveAddrWarnsOnFallback pins the navigate/eval resolver to the
// contract c15da21's changeset claims for "client commands": an explicit --addr
// wins silently, a present session file is honored silently, and the absence of
// a usable session file falls back to the default CDP port WITH a stderr warning
// — by delegating to resolveCDPAddr rather than the silent browser.DefaultAddr.
func TestResolveAddrWarnsOnFallback(t *testing.T) {
	t.Run("no session file warns and falls back to default port", func(t *testing.T) {
		dir := mkCorpusDir(t) // corpus dir exists, but no .session written
		var addr string
		stderr := captureStderr(t, func() {
			addr, _ = resolveAddr(nil, dir)
		})
		if want := fmt.Sprintf("localhost:%d", browser.DefaultCDPPort); addr != want {
			t.Errorf("resolveAddr(no session) addr = %q, want %q", addr, want)
		}
		if !strings.Contains(stderr, fallbackWarn) {
			t.Errorf("resolveAddr(no session) stderr = %q, want a warning containing %q", stderr, fallbackWarn)
		}
	})

	t.Run("explicit --addr wins and stays silent", func(t *testing.T) {
		dir := mkCorpusDir(t)
		// runNavigate/runEval call resolveSightmapDir first (stripping
		// --sightmap-dir), so resolveAddr sees only --addr and its value.
		args := []string{"--addr", "host:1234"}
		var addr string
		var rest []string
		stderr := captureStderr(t, func() {
			addr, rest = resolveAddr(args, dir)
		})
		if addr != "host:1234" {
			t.Errorf("resolveAddr(--addr) = %q, want host:1234", addr)
		}
		if len(rest) != 0 {
			t.Errorf("resolveAddr(--addr) rest = %v, want empty", rest)
		}
		if stderr != "" {
			t.Errorf("resolveAddr(--addr) stderr = %q, want empty (explicit addr is silent)", stderr)
		}
	})

	t.Run("present session file is honored silently", func(t *testing.T) {
		dir := mkCorpusDir(t)
		if err := browser.WriteSessionInfo(dir, browser.SessionInfo{Port: 9999, ServerPort: 8888}); err != nil {
			t.Fatalf("seed session file: %v", err)
		}
		var addr string
		stderr := captureStderr(t, func() {
			addr, _ = resolveAddr(nil, dir)
		})
		if addr != "localhost:9999" {
			t.Errorf("resolveAddr(session) = %q, want localhost:9999", addr)
		}
		if stderr != "" {
			t.Errorf("resolveAddr(session) stderr = %q, want empty (session file is silent)", stderr)
		}
	})
}

// TestSessionAddrWarnsOnFallback pins the four tabs subcommands' resolver
// (sessionAddr) to the same contract: a present session file is honored
// silently, and the absence of one falls back to the default CDP port WITH a
// stderr warning — by delegating to resolveCDPAddr rather than its former
// hand-rolled silent ReadSessionInfo-or-default logic. All four tabs commands
// (list/new/close/resize) route through this helper, so covering it here covers
// them without dialing Chrome.
func TestSessionAddrWarnsOnFallback(t *testing.T) {
	t.Run("no session file warns and falls back to default port", func(t *testing.T) {
		dir := mkCorpusDir(t)
		var addr string
		stderr := captureStderr(t, func() {
			addr = sessionAddr(dir)
		})
		if want := fmt.Sprintf("localhost:%d", browser.DefaultCDPPort); addr != want {
			t.Errorf("sessionAddr(no session) = %q, want %q", addr, want)
		}
		if !strings.Contains(stderr, fallbackWarn) {
			t.Errorf("sessionAddr(no session) stderr = %q, want a warning containing %q", stderr, fallbackWarn)
		}
	})

	t.Run("present session file is honored silently", func(t *testing.T) {
		dir := mkCorpusDir(t)
		if err := browser.WriteSessionInfo(dir, browser.SessionInfo{Port: 7777}); err != nil {
			t.Fatalf("seed session file: %v", err)
		}
		var addr string
		stderr := captureStderr(t, func() {
			addr = sessionAddr(dir)
		})
		if addr != "localhost:7777" {
			t.Errorf("sessionAddr(session) = %q, want localhost:7777", addr)
		}
		if stderr != "" {
			t.Errorf("sessionAddr(session) stderr = %q, want empty (session file is silent)", stderr)
		}
	})

	t.Run("out-of-range session port falls back and warns", func(t *testing.T) {
		// The former sessionAddr honored any positive port (even >65535, which is
		// not a valid TCP port). Delegating to resolveCDPAddr treats an
		// out-of-range port the same as a missing file: fall back + warn.
		dir := mkCorpusDir(t)
		if err := browser.WriteSessionInfo(dir, browser.SessionInfo{Port: 99999}); err != nil {
			t.Fatalf("seed session file: %v", err)
		}
		var addr string
		stderr := captureStderr(t, func() {
			addr = sessionAddr(dir)
		})
		if want := fmt.Sprintf("localhost:%d", browser.DefaultCDPPort); addr != want {
			t.Errorf("sessionAddr(out-of-range port) = %q, want %q", addr, want)
		}
		if !strings.Contains(stderr, fallbackWarn) {
			t.Errorf("sessionAddr(out-of-range port) stderr = %q, want a warning containing %q", stderr, fallbackWarn)
		}
	})
}
