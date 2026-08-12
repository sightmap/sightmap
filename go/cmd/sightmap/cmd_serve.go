package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sightmap/sightmap/go/sightmap"
)

// servedSightmap is the thin transport envelope the extension fetches from
// /sightmap: serve-only metadata (a site label and a cache-bust version stamp)
// wrapped around the canonical Corpus wire. The Corpus body is byte-identical to
// what a go-get/library consumer sees — serve no longer re-compiles the corpus
// into a bespoke joined-selector shape. Cache-busting is unchanged: the extension
// polls /sightmap/version and reloads when the stamp changes.
type servedSightmap struct {
	Site    string           `json:"site"`
	Version string           `json:"version"`
	Corpus  *sightmap.Corpus `json:"corpus"`
}

// loadServedSightmap loads the corpus and wraps it with serve metadata. The
// version is a monotonic wall-clock stamp used only for cache-busting; the
// selector-join / flattening the extension needs is done client-side now, so the
// wire stays the canonical Corpus shape (selectors[] arrays, components nested
// under each view).
func loadServedSightmap(sightmapDir, siteName string) (servedSightmap, error) {
	corp, err := sightmap.Load(sightmapDir)
	if err != nil {
		return servedSightmap{}, err
	}
	return servedSightmap{
		Site:    siteName,
		Version: strconv.FormatInt(time.Now().UnixMilli(), 10),
		Corpus:  corp,
	}, nil
}

func runServeSightmap(args []string) error {
	fset := flag.NewFlagSet("serve-sightmap", flag.ContinueOnError)
	portFlag := fset.Int("port", 7891, "HTTP port to listen on")
	sightmapDir := fset.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	if err := fset.Parse(args); err != nil {
		return err
	}

	siteName := filepath.Base(cwd())

	// Initial load.
	compiled, err := loadServedSightmap(*sightmapDir, siteName)
	if err != nil {
		return fmt.Errorf("serve-sightmap: initial load: %w", err)
	}

	var mu sync.RWMutex
	current := compiled

	reload := func() {
		c, loadErr := loadServedSightmap(*sightmapDir, siteName)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "[serve-sightmap] load error: %v\n", loadErr)
			return
		}
		mu.Lock()
		current = c
		mu.Unlock()
		fmt.Fprintf(os.Stderr, "[serve-sightmap] reloaded (v%s)\n", c.Version)
	}

	// File watcher.
	go watchSightmapDir(*sightmapDir, reload)

	// HTTP handlers.
	mux := http.NewServeMux()
	mux.HandleFunc("/sightmap/version", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		v := current.Version
		mu.RUnlock()
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"version":%q}`, v)
	})
	mux.HandleFunc("/sightmap", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		c := current
		mu.RUnlock()
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(c)
	})

	addr := fmt.Sprintf("127.0.0.1:%d", *portFlag)
	fmt.Fprintf(os.Stderr, "[serve-sightmap] listening on http://%s\n", addr)

	srv := &http.Server{Addr: addr, Handler: mux}
	return srv.ListenAndServe()
}

// watchSightmapDir watches dir for YAML file changes and calls onChange
// (debounced to 200ms) whenever a .yaml or .yml file is modified.
func watchSightmapDir(dir string, onChange func()) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[serve-sightmap] watcher error: %v\n", err)
		return
	}
	defer watcher.Close()

	// Add the directory and any subdirectories, except non-corpus dirs
	// (snapshots/ blobs) — watching them would recompile the corpus (and bump
	// its version) on every snapshot write.
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path != dir && (d.Name() == "review" || d.Name() == "snapshots") {
			return fs.SkipDir
		}
		return watcher.Add(path)
	})

	var debounce *time.Timer
	for {
		select {
		case ev, ok := <-watcher.Events:
			if !ok {
				return
			}
			ext := strings.ToLower(filepath.Ext(ev.Name))
			if ext != ".yaml" && ext != ".yml" {
				continue
			}
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(200*time.Millisecond, onChange)
		case watchErr, ok := <-watcher.Errors:
			if !ok {
				return
			}
			fmt.Fprintf(os.Stderr, "[serve-sightmap] watcher error: %v\n", watchErr)
		}
	}
}

// cwd returns the current working directory, falling back to "." on error.
func cwd() string {
	d, err := os.Getwd()
	if err != nil {
		return "."
	}
	return d
}
