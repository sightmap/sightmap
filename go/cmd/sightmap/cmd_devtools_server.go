// Devtools query surface hosted by the `browser start` daemon. These endpoints
// are the first slice of the daemon protocol: the collector (a session-lifetime
// CDP client) buffers console/network, and per-command CLI queries read it here
// rather than re-attaching to Chrome. Filters and pagination run daemon-side; the
// CLI console/network commands are thin clients.
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/sightmap"
)

// maxInjectBytes caps a persisted script's size. Generous enough for a bundled
// runtime while refusing a pathological upload.
const maxInjectBytes = 16 << 20 // 16 MiB

// registerDevtoolsHandlers wires the /devtools/* endpoints onto mux. The
// collector is supplied via ptr because it is created only after Chrome is
// ready, whereas handlers must be registered before the server starts serving.
// Until the collector exists the endpoints return 503.
//
// sightmapDir is the corpus location. The console/network handlers annotate each
// observed record with the corpus defs that classify it, loading the corpus
// fresh per request so on-disk edits are reflected the same way the component
// CLI's per-invocation sightmap.Load() is (corpora are tiny; the I/O is
// negligible). A missing or malformed corpus degrades to no annotations rather
// than failing the query.
func registerDevtoolsHandlers(mux *http.ServeMux, ptr *atomic.Pointer[browser.Collector], sightmapDir string) {
	load := func(w http.ResponseWriter) (*browser.Collector, bool) {
		if c := ptr.Load(); c != nil {
			return c, true
		}
		http.Error(w, "collector not ready", http.StatusServiceUnavailable)
		return nil, false
	}

	// corpusNow loads the corpus fresh, returning nil on any error so the
	// handler degrades to un-annotated entries.
	corpusNow := func() *sightmap.Corpus {
		c, err := sightmap.Load(sightmapDir)
		if err != nil {
			return nil
		}
		return c
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
		writeJSON(w, map[string]any{"entries": annotateConsole(corpusNow(), entries), "dropped": dropped})
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
		writeJSON(w, map[string]any{"entries": annotateNetwork(corpusNow(), entries), "dropped": dropped})
	})

	// Persistent script injection. POST registers a script (raw body) and returns
	// its id; DELETE?id= removes one; GET lists them. The collector holds the
	// registry because it is the session-lifetime CDP client — a script must
	// outlive any single per-command connection.
	mux.HandleFunc("/inject", func(w http.ResponseWriter, r *http.Request) {
		c, ok := load(w)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, map[string]any{"scripts": c.PersistentScripts()})
		case http.MethodPost:
			// Read one byte past the cap so an over-limit body is detected
			// rather than silently truncated (the same cap+1 idiom
			// go/atlas/fetch.go uses). A plain LimitReader at exactly the cap
			// presents EOF to ReadAll, so a body larger than the cap would
			// otherwise be cut to maxInjectBytes with err == nil and stored
			// verbatim.
			src, err := io.ReadAll(io.LimitReader(r.Body, maxInjectBytes+1))
			if err != nil {
				http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
				return
			}
			if len(src) > maxInjectBytes {
				http.Error(w, "script too large", http.StatusRequestEntityTooLarge)
				return
			}
			if len(src) == 0 {
				http.Error(w, "empty script", http.StatusBadRequest)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			id, err := c.AddPersistentScript(ctx, string(src))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{"id": id, "bytes": len(src)})
		case http.MethodDelete:
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, "id required", http.StatusBadRequest)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			removed, err := c.RemovePersistentScript(ctx, id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !removed {
				http.Error(w, "no such script id", http.StatusNotFound)
				return
			}
			writeJSON(w, map[string]any{"removed": id})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
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
