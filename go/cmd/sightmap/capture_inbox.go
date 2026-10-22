// The capture inbox: an unsorted queue of captures, one per page as it was
// when someone (a person driving the overlay, or an agent) chose to keep it.
//
// `capture` files a capture under the view a corpus route matches, which
// presumes the views are already known. A tool that LEARNS the views from the
// captures (surveyor infers routes from every captured URL) cannot ask for that
// up front without circularity, so captures for it go to
// .sightmap/inbox/<stamp>.snap instead: no view lookup, no novelty gate, always
// a screenshot. Sorting the inbox into views is the consumer's job.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/observe"
	"github.com/sightmap/sightmap/go/sightmap"
	"github.com/sightmap/sightmap/go/viewset"
)

// inboxDir is the queue under a corpus dir.
func inboxDir(sightmapDir string) string { return filepath.Join(sightmapDir, "inbox") }

// reserveInboxPath claims a new capture's path in the inbox and returns it.
// Stamps have one-second resolution, so a second capture in the same second
// takes a -2, -3, ... suffix rather than overwriting the first. The name is
// claimed by creating it (empty) with O_EXCL: writers are separate processes
// (the daemon, `capture --inbox`, parallel agents), so only the filesystem
// can arbitrate between them. The caller then renames its content onto the
// reserved name, or removes it on failure.
func reserveInboxPath(sightmapDir string, t time.Time) (string, error) {
	if err := os.MkdirAll(inboxDir(sightmapDir), 0o755); err != nil {
		return "", fmt.Errorf("create inbox: %v", err)
	}
	base := filepath.Join(inboxDir(sightmapDir), viewset.Stamp(t))
	p := base + ".snap"
	for n := 2; ; n++ {
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			f.Close()
			return p, nil
		}
		if !os.IsExist(err) {
			return "", fmt.Errorf("reserve %s: %v", p, err)
		}
		p = fmt.Sprintf("%s-%d.snap", base, n)
	}
}

// inboxCapture observes the page loaded in conn and writes it to the inbox:
// the .snap (header, tree, coverage, as `capture` writes it), its .tree.json
// sibling, and a full-page screenshot. corpus may be nil; when given, the
// .snap is annotated with it (and names the matching view, if any) exactly as
// `snapshot` would, but the capture's place never depends on it.
func inboxCapture(ctx context.Context, conn *browser.CDPConn, sightmapDir string, corpus *sightmap.Corpus, opts observe.FormatOpts, visibleOnly bool) (string, *observe.Result, error) {
	res, err := observe.Page(ctx, conn, corpus, observe.Options{VisibleOnly: visibleOnly})
	if err != nil {
		return "", nil, fmt.Errorf("observe: %v", err)
	}
	if res.Root == nil {
		return "", nil, fmt.Errorf("observe: the page has no content")
	}
	snapPath, err := writeInboxCapture(sightmapDir, time.Now(), res, opts)
	if err != nil {
		return "", nil, err
	}
	writeCaptureScreenshot(ctx, conn, snapPath)
	return snapPath, res, nil
}

// writeInboxCapture persists an observed page to the inbox under a fresh stamp
// for t, and returns the .snap path. The .snap is the capture; its .tree.json
// sibling is a convenience, so losing it is a warning here as in writeCapture,
// while a .snap that could not be written leaves nothing behind.
func writeInboxCapture(sightmapDir string, t time.Time, res *observe.Result, opts observe.FormatOpts) (string, error) {
	snapPath, err := reserveInboxPath(sightmapDir, t)
	if err != nil {
		return "", err
	}
	// writeOut stages at <snapPath>.tmp, which is ours alone now that the
	// name is reserved, and renames onto the reservation.
	if err := writeOut(snapPath, func(w io.Writer) error {
		if res.View == nil {
			// Without a matched view Format writes no url line (or none at
			// all, with no corpus); the inbox exists to be sorted by URL, so
			// the header always carries it.
			fmt.Fprintf(w, "url: %s\n\n", res.URL)
		}
		observe.Format(w, res, opts)
		return nil
	}); err != nil {
		os.Remove(snapPath) // the empty reservation is not a capture
		return "", err
	}
	if err := writeTreeJSON(res.Root, viewset.TreePath(snapPath)); err != nil {
		fmt.Fprintf(os.Stderr, "capture: tree-out: %v\n", err)
	}
	return snapPath, nil
}

// captureRequest is the body of POST /capture. The overlay knows its tab's
// URL, not its CDP target id, so it names the page by URL; the daemon finds
// the content tab showing it.
type captureRequest struct {
	URL string `json:"url"`
}

// registerCaptureHandler wires POST /capture onto mux: capture the session tab
// showing the requested URL into the inbox, and report where it went. Captures
// are serialised, since two at once on one tab would interleave their CDP
// traffic and could read a page between the other's screenshot and tree.
func registerCaptureHandler(mux *http.ServeMux, sightmapDir, cdpAddr string) {
	var mu sync.Mutex
	mux.HandleFunc("/capture", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "POST")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req captureRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil || req.URL == "" {
			http.Error(w, `want {"url": "<the page's URL>"}`, http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()

		tab, err := tabShowing(ctx, cdpAddr, req.URL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		conn, err := browser.ConnectTab(ctx, cdpAddr, tab)
		if err != nil {
			http.Error(w, fmt.Sprintf("connect to tab %s: %v", tab, err), http.StatusBadGateway)
			return
		}
		defer conn.Close()
		corpus, cErr := sightmap.Load(sightmapDir)
		if cErr != nil {
			corpus = nil // annotation only; never a reason to refuse a capture
		}
		cfg := sightmap.LoadConfig(sightmapDir)
		path, res, err := inboxCapture(ctx, conn, sightmapDir, corpus,
			observe.FormatOpts{Trace: cfg.Snapshot.Trace}, !cfg.Snapshot.IncludeHidden)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(os.Stderr, "[capture] %s  %s\n", path, res.URL)
		out := map[string]any{"path": path, "url": res.URL, "stamp": strings.TrimSuffix(filepath.Base(path), ".snap")}
		if res.View != nil {
			out["view"] = res.View.Name
		}
		writeJSON(w, out)
	})
}

// tabShowing is the one content tab at addr whose URL is url. Ambiguity is an
// error rather than a guess: capturing a different tab's page would file the
// wrong page under a stamp the person believes is the one in front of them.
func tabShowing(ctx context.Context, addr, url string) (string, error) {
	tabs, err := browser.ListTabs(ctx, addr)
	if err != nil {
		return "", fmt.Errorf("list tabs at %s: %v", addr, err)
	}
	var ids []string
	for _, t := range tabs {
		if t.URL == url {
			ids = append(ids, t.TargetID)
		}
	}
	switch len(ids) {
	case 1:
		return ids[0], nil
	case 0:
		return "", fmt.Errorf("no tab in this session shows %s", url)
	default:
		return "", fmt.Errorf("%d tabs show %s; close all but one to capture it", len(ids), url)
	}
}
