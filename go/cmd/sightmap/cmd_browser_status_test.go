package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestCDPVersionAlive guards the fix: cdpVersionAlive must
// only report alive for a genuine Chrome DevTools endpoint (HTTP 200 +
// webSocketDebuggerUrl), NOT for the sightmap HTTP server that can occupy the
// same port and answers /json/version with a 404.
func TestCDPVersionAlive(t *testing.T) {
	ctx := context.Background()

	t.Run("real DevTools endpoint", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/json/version" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"Browser":"Chrome/149.0","webSocketDebuggerUrl":"ws://127.0.0.1:9/devtools/browser/abc"}`))
				return
			}
			http.NotFound(w, r)
		}))
		defer srv.Close()
		if !cdpVersionAlive(ctx, addrOf(t, srv)) {
			t.Fatal("expected alive for a real DevTools /json/version response")
		}
	})

	t.Run("sightmap server 404 (port collision)", func(t *testing.T) {
		// Mimics the Go HTTP server that won the CDP port: it answers
		// but with a 404 page-not-found, never the DevTools JSON.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer srv.Close()
		if cdpVersionAlive(ctx, addrOf(t, srv)) {
			t.Fatal("expected NOT alive for a server that 404s /json/version")
		}
	})

	t.Run("200 but wrong JSON body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"1"}`))
		}))
		defer srv.Close()
		if cdpVersionAlive(ctx, addrOf(t, srv)) {
			t.Fatal("expected NOT alive for 200 without a browser webSocketDebuggerUrl")
		}
	})

	t.Run("nothing listening", func(t *testing.T) {
		// Bind then immediately close to obtain a port that is reliably free.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := ln.Addr().String()
		ln.Close()
		dctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if cdpVersionAlive(dctx, addr) {
			t.Fatal("expected NOT alive when no server is listening")
		}
	})
}

func addrOf(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	return strings.TrimPrefix(srv.URL, "http://")
}
