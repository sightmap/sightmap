// Shared CLI adapter glue for every command that drives a live Chrome session.
// This is adapter-tier code (package main, never imported): it declares the
// common flag set, maps connection/navigation into the browser subsystem, and
// centralises the atomic --out write. Reusable browser behaviour lives in the
// browser package; only flag wiring and CLI error prose live here.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/sightmap"
)

// liveFlags is the flag set shared by every command that connects to a live
// page. Register it on a FlagSet with addLiveFlags, then use its methods to
// connect, navigate, and load the corpus. Commands declare their own bespoke
// flags alongside it.
type liveFlags struct {
	addr        *string
	url         *string
	wait        *float64
	tab         *string
	sightmapDir *string
	cmd         string // command name, for error messages
}

// addLiveFlags registers the common live-session flags on fs and returns a
// handle whose methods drive the connection. cmd is the command name used in
// error messages.
func addLiveFlags(fs *flag.FlagSet, cmd string) *liveFlags {
	return &liveFlags{
		addr:        fs.String("addr", browser.DefaultAddr(), "CDP address (host:port)"),
		url:         fs.String("url", "", "Navigate to this URL before acting"),
		wait:        fs.Float64("wait", 0, "Extra seconds to wait after DOM stability is detected (default 0; raise for unusually slow pages)"),
		tab:         fs.String("tab", "", "Target tab ID (from 'browser start' output)"),
		sightmapDir: fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir"),
		cmd:         cmd,
	}
}

// connect attaches to the running browser session's tab. The returned cleanup
// must be deferred by the caller. There is no auto-launch: start a session with
// 'sightmap browser start' first (Connect's error says so).
func (lf *liveFlags) connect(ctx context.Context) (*browser.CDPConn, func(), error) {
	conn, err := browser.Connect(*lf.addr, *lf.tab)
	if err != nil {
		return nil, nil, err
	}
	return conn, func() { conn.Close() }, nil
}

// navOpts tunes how navigate waits for the page to settle.
type navOpts struct {
	idle    bool    // use the SPA-aware idle wait (loadEvent + networkIdle) rather than a plain DOM-ready wait
	minWait float64 // floor applied to --wait after navigation (0 = none)
}

// navigate applies --url and --wait per opts. It is a no-op when --url is empty
// except for an explicit --wait sleep.
func (lf *liveFlags) navigate(ctx context.Context, conn *browser.CDPConn, opts navOpts) error {
	if *lf.url != "" {
		var err error
		if opts.idle {
			err = browser.NavigateAndWaitIdle(ctx, conn, *lf.url, 8*time.Second)
		} else {
			err = browser.NavigateAndWait(ctx, conn, *lf.url)
		}
		if err != nil {
			return fmt.Errorf("%s: navigate: %v", lf.cmd, err)
		}
	}
	wait := *lf.wait
	if *lf.url != "" && wait < opts.minWait {
		wait = opts.minWait
	}
	if wait > 0 {
		time.Sleep(time.Duration(wait * float64(time.Second)))
	}
	return nil
}

// loadCorpus loads the corpus from --sightmap-dir, returning (nil, nil) when the
// directory is absent. Use this for commands that annotate opportunistically.
func (lf *liveFlags) loadCorpus() (*sightmap.Corpus, error) {
	if _, err := os.Stat(*lf.sightmapDir); err != nil {
		return nil, nil
	}
	return sightmap.Load(*lf.sightmapDir)
}

// requireCorpus loads the corpus, erroring when --sightmap-dir is absent. Use
// this for commands that cannot operate without a corpus.
func (lf *liveFlags) requireCorpus() (*sightmap.Corpus, error) {
	if _, err := os.Stat(*lf.sightmapDir); err != nil {
		return nil, fmt.Errorf("%s: sightmap dir %q not found — run 'sightmap validate' to check setup", lf.cmd, *lf.sightmapDir)
	}
	return sightmap.Load(*lf.sightmapDir)
}

// writeOut writes to path atomically (temp file + rename), or to stdout when
// path is "". emit receives the destination writer.
func writeOut(path string, emit func(w io.Writer) error) error {
	if path == "" {
		return emit(os.Stdout)
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output directory: %v", err)
		}
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create output file: %v", err)
	}
	if err := emit(f); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close output file: %v", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename output file: %v", err)
	}
	return nil
}
