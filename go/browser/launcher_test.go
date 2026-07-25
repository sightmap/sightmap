package browser

import (
	"context"
	"net"
	"testing"
	"time"
)

// TestFindFreePortExcluding_SkipsExcluded guards against the port
// collision: two allocations that start from overlapping ranges must not return
// the same port. FindFreePort only probes (open+close) without holding the port,
// so without an exclusion the server and CDP allocations could collide.
func TestFindFreePortExcluding_SkipsExcluded(t *testing.T) {
	// Bind a port so the default starting point is taken, forcing the first
	// allocation to slide upward — mimicking a busy server default (7891).
	ln, err := net.Listen("tcp", ":0")
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

func TestLaunch(t *testing.T) {
	if _, err := FindChrome(); err != nil {
		t.Skip("Chrome not found:", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Force headless with CI-safe flags. The default launch is headed, which
	// can't start on a display-less CI runner: Chrome exits immediately and
	// never binds the debug port, surfacing as "connection refused". The extra
	// flags keep headless Chrome happy in constrained CI sandboxes/containers.
	conn, cleanup, err := Launch(ctx, LaunchOptions{
		StartURL:  "about:blank",
		Headless:  true,
		ExtraArgs: []string{"--no-sandbox", "--disable-dev-shm-usage"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	url, err := GetURL(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if url != "about:blank" {
		t.Errorf("got URL %q, want about:blank", url)
	}
}
