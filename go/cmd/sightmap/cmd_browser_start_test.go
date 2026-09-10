package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sightmap/sightmap/go/browser"
)

func TestSandboxHint(t *testing.T) {
	// A sandbox-signature stderr yields a --no-sandbox hint...
	if h := sandboxHint("Failed to move to new namespace: No usable sandbox!", false); !strings.Contains(h, "--no-sandbox") {
		t.Errorf("expected a --no-sandbox hint, got %q", h)
	}
	// ...unless --no-sandbox is already set (then the failure is something else).
	if h := sandboxHint("No usable sandbox!", true); h != "" {
		t.Errorf("expected no hint when --no-sandbox already present, got %q", h)
	}
	// Unrelated failures get no sandbox hint.
	if h := sandboxHint("Missing X server or $DISPLAY", false); h != "" {
		t.Errorf("expected no hint for a non-sandbox failure, got %q", h)
	}
}

func TestShouldAutoHeadless(t *testing.T) {
	if runtime.GOOS != "linux" {
		// Off Linux there is always a usable display; never auto-headless.
		t.Setenv("DISPLAY", "")
		t.Setenv("WAYLAND_DISPLAY", "")
		if shouldAutoHeadless() {
			t.Errorf("shouldAutoHeadless must be false off Linux")
		}
		return
	}
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	if !shouldAutoHeadless() {
		t.Errorf("expected auto-headless on Linux with no display")
	}
	t.Setenv("DISPLAY", ":0")
	if shouldAutoHeadless() {
		t.Errorf("expected no auto-headless when DISPLAY is set")
	}
}

func TestStripStartFlags(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		strip []string
		want  []string
	}{
		{"bool flag alone", []string{"--detach"}, []string{"detach", "log-file"}, nil},
		{"bool =true form", []string{"--detach=true"}, []string{"detach", "log-file"}, nil},
		{"single dash", []string{"-detach"}, []string{"detach", "log-file"}, nil},
		{
			"keeps other flags",
			[]string{"--headless", "--detach", "--port", "9"},
			[]string{"detach", "log-file"},
			[]string{"--headless", "--port", "9"},
		},
		{
			"value flag with separate value token",
			[]string{"--log-file", "/tmp/x", "--url", "http://y"},
			[]string{"detach", "log-file"},
			[]string{"--url", "http://y"},
		},
		{
			"value flag =form",
			[]string{"--log-file=/tmp/x", "--headless"},
			[]string{"detach", "log-file"},
			[]string{"--headless"},
		},
		{
			"both stripped",
			[]string{"--detach", "--log-file", "/tmp/x", "--headless"},
			[]string{"detach", "log-file"},
			[]string{"--headless"},
		},
		{
			"does not eat the token after a bool flag",
			[]string{"--detach", "--port", "7890"},
			[]string{"detach", "log-file"},
			[]string{"--port", "7890"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := stripStartFlags(tc.args, tc.strip...)
			if !slices.Equal(got, tc.want) {
				t.Errorf("stripStartFlags(%v, %v) = %v, want %v", tc.args, tc.strip, got, tc.want)
			}
		})
	}
}

// fakeCDP serves a minimal Chrome /json/version so isPortAlive treats the port
// as a live CDP endpoint.
func fakeCDP(t *testing.T) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"Browser":"fake/1","webSocketDebuggerUrl":"ws://127.0.0.1/devtools/browser/x"}`)
	}))
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().(*net.TCPAddr).Port
}

// fakeServer serves a minimal /sightmap/version so serverAlive treats the port
// as a live sightmap HTTP server.
func fakeServer(t *testing.T) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sightmap/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"version":"1"}`)
	}))
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().(*net.TCPAddr).Port
}

// TestWaitDetachedReady_RejectsZeroServerPort guards the `start --detach --port 0`
// regression: a daemon that wrote serverPort=0 into the session file used to be
// reported ready (0 was read as "legacy/unknown"), so `start` exited 0 while every
// client command then refused the session as "no running session". A session
// whose server port is unknown must never count as ready, even with CDP alive.
func TestWaitDetachedReady_RejectsZeroServerPort(t *testing.T) {
	dir := t.TempDir()
	cdp := fakeCDP(t)
	if err := browser.WriteSessionInfo(dir, browser.SessionInfo{Port: cdp, PID: 1}); err != nil {
		t.Fatal(err)
	}
	childDone := make(chan error) // never fires: the daemon is "still running"

	info, ready, err := waitDetachedReady(dir, childDone, 600*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected child-exit error: %v", err)
	}
	if ready {
		t.Fatalf("session with serverPort=0 reported ready: %+v", info)
	}
	// The last observed session is handed back so the caller can explain the
	// missing server port in its timeout error.
	if info.Port != cdp || info.ServerPort != 0 {
		t.Fatalf("expected last-seen session info on timeout, got %+v", info)
	}
}

// TestWaitDetachedReady_ReadyWithBothPorts is the happy path: CDP and the
// sightmap server both answering on the recorded (concrete) ports.
func TestWaitDetachedReady_ReadyWithBothPorts(t *testing.T) {
	dir := t.TempDir()
	cdp := fakeCDP(t)
	server := fakeServer(t)
	if err := browser.WriteSessionInfo(dir, browser.SessionInfo{Port: cdp, PID: 1, ServerPort: server}); err != nil {
		t.Fatal(err)
	}
	childDone := make(chan error)

	info, ready, err := waitDetachedReady(dir, childDone, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected child-exit error: %v", err)
	}
	if !ready {
		t.Fatalf("expected ready with cdp=%d server=%d, got %+v", cdp, server, info)
	}
	if info.Port != cdp || info.ServerPort != server {
		t.Fatalf("ready info = %+v, want port=%d serverPort=%d", info, cdp, server)
	}
}
