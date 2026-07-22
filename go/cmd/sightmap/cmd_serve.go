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
	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

type compiledComponent struct {
	Name        string         `json:"name"`
	Selector    string         `json:"selector"`    // selectors joined with ", "
	ParentChain []string       `json:"parentChain"` // always array, never null
	Properties  []compiledProp `json:"properties"`
}

type compiledProp struct {
	Name      string `json:"name"`
	Extract   string `json:"extract"`
	Transform string `json:"transform,omitempty"`
}

type compiledView struct {
	Name  string `json:"name"`
	Route string `json:"route"`
}

type compiledSightmapJSON struct {
	Site           string                         `json:"site"`
	Version        string                         `json:"version"`
	Views          []compiledView                 `json:"views"`
	Globals        []compiledComponent            `json:"globals"`
	ViewComponents map[string][]compiledComponent `json:"viewComponents"`
}

func normaliseComponent(c match.SightmapComponent) compiledComponent {
	chain := c.ParentChain
	if chain == nil {
		chain = []string{} // always emit [], never null
	}
	props := make([]compiledProp, 0, len(c.Properties))
	for _, p := range c.Properties {
		props = append(props, compiledProp{Name: p.Name, Extract: p.Extract, Transform: p.Transform})
	}
	return compiledComponent{
		Name:        c.Name,
		Selector:    strings.Join(c.Selectors, ", "),
		ParentChain: chain,
		Properties:  props,
	}
}

func compileCorpus(sightmapDir, siteName string) (compiledSightmapJSON, error) {
	corp, err := sightmap.DirLoader(sightmapDir).Load()
	if err != nil {
		return compiledSightmapJSON{}, err
	}

	globals := make([]compiledComponent, 0, len(corp.GlobalComponents))
	for _, c := range corp.GlobalComponents {
		globals = append(globals, normaliseComponent(c))
	}

	viewList := make([]compiledView, 0, len(corp.Views))
	viewComponents := make(map[string][]compiledComponent, len(corp.Views))
	for _, v := range corp.Views {
		if v.Route == "" {
			continue
		}
		viewList = append(viewList, compiledView{Name: v.Name, Route: v.Route})
		comps := make([]compiledComponent, 0, len(v.Components))
		for _, c := range v.Components {
			comps = append(comps, normaliseComponent(c))
		}
		viewComponents[v.Route] = comps
	}

	return compiledSightmapJSON{
		Site:           siteName,
		Version:        strconv.FormatInt(time.Now().UnixMilli(), 10),
		Views:          viewList,
		Globals:        globals,
		ViewComponents: viewComponents,
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

	// Initial compile.
	compiled, err := compileCorpus(*sightmapDir, siteName)
	if err != nil {
		return fmt.Errorf("serve-sightmap: initial compile: %w", err)
	}

	var mu sync.RWMutex
	current := compiled

	recompile := func() {
		c, compErr := compileCorpus(*sightmapDir, siteName)
		if compErr != nil {
			fmt.Fprintf(os.Stderr, "[serve-sightmap] compile error: %v\n", compErr)
			return
		}
		mu.Lock()
		current = c
		mu.Unlock()
		fmt.Fprintf(os.Stderr, "[serve-sightmap] recompiled (v%s)\n", c.Version)
	}

	// File watcher.
	go watchSightmapDir(*sightmapDir, recompile)

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

	addr := fmt.Sprintf(":%d", *portFlag)
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
