// Devtools query surface hosted by the `browser start` daemon. These endpoints
// are the first slice of the daemon protocol: the collector (a session-lifetime
// CDP client) buffers console/network, and per-command CLI queries read it here
// rather than re-attaching to Chrome. Filters and pagination run daemon-side; the
// CLI console/network commands are thin clients.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/sightmap/sightmap/go/browser"
)

// registerDevtoolsHandlers wires the /devtools/* endpoints onto mux. The
// collector is supplied via ptr because it is created only after Chrome is
// ready, whereas handlers must be registered before the server starts serving.
// Until the collector exists the endpoints return 503.
func registerDevtoolsHandlers(mux *http.ServeMux, ptr *atomic.Pointer[browser.Collector]) {
	load := func(w http.ResponseWriter) (*browser.Collector, bool) {
		if c := ptr.Load(); c != nil {
			return c, true
		}
		http.Error(w, "collector not ready", http.StatusServiceUnavailable)
		return nil, false
	}

	mux.HandleFunc("/devtools/console", func(w http.ResponseWriter, r *http.Request) {
		c, ok := load(w)
		if !ok {
			return
		}
		q := r.URL.Query()
		entries, dropped := c.Console(browser.ConsoleFilter{
			Level: q.Get("level"),
			Tab:   q.Get("tab"),
			Limit: atoiDefault(q.Get("limit"), 0),
		})
		writeJSON(w, map[string]any{"entries": entries, "dropped": dropped})
	})

	mux.HandleFunc("/devtools/network", func(w http.ResponseWriter, r *http.Request) {
		c, ok := load(w)
		if !ok {
			return
		}
		q := r.URL.Query()
		entries, dropped := c.Network(browser.NetworkFilter{
			ResourceType: q.Get("type"),
			URLSubstr:    q.Get("url"),
			Tab:          q.Get("tab"),
			Limit:        atoiDefault(q.Get("limit"), 0),
		})
		writeJSON(w, map[string]any{"entries": entries, "dropped": dropped})
	})

	mux.HandleFunc("/devtools/network/body", func(w http.ResponseWriter, r *http.Request) {
		c, ok := load(w)
		if !ok {
			return
		}
		index, err := strconv.Atoi(r.URL.Query().Get("index"))
		if err != nil {
			http.Error(w, "index required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		var body []byte
		var found bool
		if r.URL.Query().Get("kind") == "request" {
			body, found, err = c.RequestBody(ctx, index)
		} else {
			body, found, err = c.ResponseBody(ctx, index)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "no such request index", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(body)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}
