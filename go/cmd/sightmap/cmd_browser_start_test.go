package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"

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

// fakeCDPServer stands up an HTTP server that answers /json/version the way a
// real Chrome DevTools endpoint does (200 + webSocketDebuggerUrl + Browser),
// which is what cdpVersionAlive/isPortAlive require before reporting CDP alive.
func fakeCDPServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/version" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Browser":"Chrome/149.0","webSocketDebuggerUrl":"ws://127.0.0.1:9/devtools/browser/abc"}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fakeSightmapServer stands up the sightmap HTTP server surface that serverAlive
// probes: /sightmap/version answering 200.
func fakeSightmapServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sightmap/version" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"test"}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// closedPort returns a TCP port that was bound on 127.0.0.1 and then released,
// so nothing is listening on it (connection refused). Used to simulate a reaped
// daemon whose HTTP server is gone.
func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return addr
}

// serverPort extracts the TCP port a httptest.Server is listening on.
func serverPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	tcp, ok := srv.Listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("httptest listener is not *net.TCPAddr: %T", srv.Listener.Addr())
	}
	return tcp.Port
}

// captureStderr swaps os.Stderr for a pipe, runs fn, and returns whatever fn
// wrote along with fn's error. The pipe is drained in the background so a
// large write can't block (our writes are tiny, but this keeps it safe).
func captureStderr(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()
	outCh := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		outCh <- string(b)
	}()
	fnErr := fn()
	w.Close()
	out := <-outCh
	return out, fnErr
}

// TestStartDetachedPrecheck pins the startDetached idempotency precheck's
// exit-code contract. In detach mode Chrome (CDP) and the sightmap HTTP daemon
// are separate processes on separate ports; a reaped daemon can leave CDP up
// while the server is gone. The precheck must NOT report that degraded state
// as "already running" (exit 0): every downstream command resolves the daemon
// via ServerPort and would fail with connection refused. It must instead print
// a degraded message and return a non-zero error, while keeping exit 0 for the
// healthy and legacy (no ServerPort) already-running cases.
func TestStartDetachedPrecheck(t *testing.T) {
	cdpSrv := fakeCDPServer(t)
	cdpPort := serverPort(t, cdpSrv)
	sightmapSrv := fakeSightmapServer(t)
	sightmapPort := serverPort(t, sightmapSrv)
	deadPort := closedPort(t)

	// Sanity: the fixtures behave the way the precheck probes them.
	if !isPortAlive(cdpPort) {
		t.Fatalf("CDP fixture on port %d is not alive for isPortAlive", cdpPort)
	}
	if !serverAlive(sightmapPort) {
		t.Fatalf("sightmap fixture on port %d is not alive for serverAlive", sightmapPort)
	}
	if serverAlive(deadPort) {
		t.Fatalf("dead port %d unexpectedly reported alive for serverAlive", deadPort)
	}

	for _, tc := range []struct {
		name       string
		info       browser.SessionInfo
		wantErr    bool
		wantErrSub string // substring expected in the returned error (empty: error should be nil)
		wantStderr string // required substring in stderr
		badStderr  string // forbidden substring in stderr
	}{
		{
			name:       "degraded: cdp up, server down => non-zero + degraded message",
			info:       browser.SessionInfo{Port: cdpPort, ServerPort: deadPort, PID: 4242, Profile: "/tmp/prof"},
			wantErr:    true,
			wantErrSub: "degraded",
			wantStderr: "⚠ degraded",
			badStderr:  "● already running",
		},
		{
			name:       "degraded: error message names the dead server port",
			info:       browser.SessionInfo{Port: cdpPort, ServerPort: deadPort, PID: 4242},
			wantErr:    true,
			wantErrSub: fmt.Sprintf(":%d", deadPort),
			wantStderr: fmt.Sprintf("server (:%d) is not responding", deadPort),
			badStderr:  "● already running",
		},
		{
			name:       "degraded: stderr directs user to stop then start --detach",
			info:       browser.SessionInfo{Port: cdpPort, ServerPort: deadPort, PID: 4242},
			wantErr:    true,
			wantStderr: "sightmap browser stop",
			badStderr:  "● already running",
		},
		{
			name:       "healthy: cdp up, server up => exit 0 + already running",
			info:       browser.SessionInfo{Port: cdpPort, ServerPort: sightmapPort, PID: 4242},
			wantErr:    false,
			wantStderr: "● already running",
			badStderr:  "⚠ degraded",
		},
		{
			name:       "healthy: already-running message echoes both ports",
			info:       browser.SessionInfo{Port: cdpPort, ServerPort: sightmapPort, PID: 4242},
			wantErr:    false,
			wantStderr: fmt.Sprintf("cdp=%d", cdpPort),
			badStderr:  "⚠ degraded",
		},
		{
			name:       "legacy: cdp up, serverPort 0 => exit 0 + already running (not degraded)",
			info:       browser.SessionInfo{Port: cdpPort, ServerPort: 0, PID: 4242},
			wantErr:    false,
			wantStderr: "● already running",
			badStderr:  "⚠ degraded",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := browser.WriteSessionInfo(dir, tc.info); err != nil {
				t.Fatal(err)
			}
			// The precheck returns from the degraded/healthy/legacy branches
			// before the re-exec path, so startDetached never execs here.
			stderr, err := captureStderr(t, func() error {
				return startDetached(nil, dir, "")
			})

			if tc.wantErr && err == nil {
				t.Fatalf("expected a non-nil error (non-zero exit); got nil\nstderr:\n%s", stderr)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil error (exit 0); got %v\nstderr:\n%s", err, stderr)
			}
			if tc.wantErr && tc.wantErrSub != "" && !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Errorf("error %q missing substring %q", err.Error(), tc.wantErrSub)
			}
			if tc.wantStderr != "" && !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("stderr missing %q\nstderr:\n%s", tc.wantStderr, stderr)
			}
			if tc.badStderr != "" && strings.Contains(stderr, tc.badStderr) {
				t.Errorf("stderr unexpectedly contains %q\nstderr:\n%s", tc.badStderr, stderr)
			}
		})
	}
}
