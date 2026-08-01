package browser

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestFindChromeManagedPreferredPerOS is the #44 regression guard: a managed
// Chrome-for-Testing install must be returned on every platform (Linux and
// Windows previously ignored it, so `browser install` → `browser start` failed).
func TestFindChromeManagedPreferredPerOS(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "chrome")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	managed := func() (string, bool) { return bin, true }
	for _, goos := range []string{"linux", "windows", "darwin"} {
		got, err := findChrome(goos, managed)
		if err != nil {
			t.Errorf("findChrome(%q, managed) error: %v", goos, err)
			continue
		}
		if got != bin {
			t.Errorf("findChrome(%q, managed) = %q, want managed %q", goos, got, bin)
		}
	}
}

// TestFindChromeLinuxErrorMentionsInstall verifies the Linux no-Chrome error
// points at `browser install` (the docs' remedy), rather than the old bare
// "no Chrome binary found in PATH".
func TestFindChromeLinuxErrorMentionsInstall(t *testing.T) {
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if _, err := exec.LookPath(name); err == nil {
			t.Skip("a system chrome is on PATH; skipping the no-chrome error test")
		}
	}
	_, err := findChrome("linux", func() (string, bool) { return "", false })
	if err == nil {
		t.Fatal("expected an error with no managed install and no PATH chrome")
	}
	if !strings.Contains(err.Error(), "browser install") {
		t.Errorf("Linux no-chrome error should mention `browser install`, got: %v", err)
	}
}

// TestSessionFileIsolationByCorpus is the regression guard for cross-agent
// crosstalk: the session file must live inside the corpus directory so that
// concurrent sessions for different corpora never share a rendezvous file (which
// let a second `browser start` piggyback on the first agent's Chrome).
func TestSessionFileIsolationByCorpus(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	if got, want := SessionFilePath(dirA), filepath.Join(dirA, ".session"); got != want {
		t.Fatalf("SessionFilePath(dirA) = %q, want %q", got, want)
	}
	if SessionFilePath(dirA) == SessionFilePath(dirB) {
		t.Fatal("distinct corpora must not share a session file")
	}

	if err := WriteSessionInfo(dirA, SessionInfo{Port: 1111, ServerPort: 2222}); err != nil {
		t.Fatal(err)
	}
	if err := WriteSessionInfo(dirB, SessionInfo{Port: 3333, ServerPort: 4444}); err != nil {
		t.Fatal(err)
	}
	if a, err := ReadSessionInfo(dirA); err != nil || a.Port != 1111 || a.ServerPort != 2222 {
		t.Fatalf("ReadSessionInfo(dirA) = %+v, %v; want Port 1111 ServerPort 2222", a, err)
	}
	if b, err := ReadSessionInfo(dirB); err != nil || b.Port != 3333 {
		t.Fatalf("ReadSessionInfo(dirB) = %+v, %v; want Port 3333", b, err)
	}
	if got := DefaultAddr(dirA); got != "localhost:1111" {
		t.Fatalf("DefaultAddr(dirA) = %q, want localhost:1111", got)
	}

	if err := RemoveSessionFile(dirA); err != nil {
		t.Fatalf("RemoveSessionFile(dirA): %v", err)
	}
	if _, err := os.Stat(SessionFilePath(dirA)); !os.IsNotExist(err) {
		t.Fatalf("session file for dirA should be gone, stat err = %v", err)
	}
}

// TestSessionFilePathFallback verifies the corpus-less fallback: when the
// sightmap dir does not exist, the session file lands under $TMPDIR.
func TestSessionFilePathFallback(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-corpus")
	if got, want := SessionFilePath(missing), filepath.Join(os.TempDir(), "sightmap-session"); got != want {
		t.Fatalf("SessionFilePath(missing) = %q, want %q", got, want)
	}
}

// TestFindFreePortExcluding_SkipsExcluded guards against the port
// collision: two allocations that start from overlapping ranges must not return
// the same port. FindFreePort only probes (open+close) without holding the port,
// so without an exclusion the server and CDP allocations could collide.
func TestFindFreePortExcluding_SkipsExcluded(t *testing.T) {
	// Bind a port on the IPv4 loopback so the default starting point is taken,
	// forcing the first allocation to slide upward — mimicking a busy server
	// default (7891). Loopback matches how the sightmap server and Chrome's CDP
	// actually bind, and how FindFreePort now probes.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port

	serverPort, err := FindFreePort(busy)
	if err != nil {
		t.Fatal(err)
	}
	if serverPort == busy {
		t.Fatalf("FindFreePort returned the busy port %d", busy)
	}

	// Allocating the CDP port from the same starting point WITHOUT excluding the
	// server port reproduces the collision (the server hasn't bound yet here).
	if collided, _ := FindFreePort(busy); collided != serverPort {
		t.Skipf("environment did not reproduce the collision (got %d, server %d); test is best-effort", collided, serverPort)
	}

	// With the exclusion, the CDP port must differ from the server port.
	cdpPort, err := FindFreePortExcluding(busy, serverPort)
	if err != nil {
		t.Fatal(err)
	}
	if cdpPort == serverPort {
		t.Fatalf("FindFreePortExcluding returned the excluded port %d", serverPort)
	}
	if cdpPort == busy {
		t.Fatalf("FindFreePortExcluding returned the busy port %d", busy)
	}
}
