package device

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

// ProbeStatus is the in-app probe's state, as status and start report it.
type ProbeStatus struct {
	// State is "starting" (launched, not answering yet), "loaded" or
	// "unavailable".
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
	Dylib  string `json:"dylib"`
	Socket string `json:"socket"`
	// WarmupMS is how long the first dump after launch took; SwiftUI's first
	// debug-graph walk in a process is several times slower than later ones.
	WarmupMS int64      `json:"warmupMs,omitempty"`
	LastDump *ProbeStat `json:"lastDump,omitempty"`
	// CacheHits counts reads that reused the last dump because the screen's
	// layout had not changed.
	CacheHits int `json:"cacheHits"`
	Dumps     int `json:"dumps"`
	// Excluded are the hosting view classes left out of dumps because the app
	// exited while SwiftUI serialized one of them.
	Excluded []string `json:"excluded,omitempty"`
}

// ProbeStat describes one dump.
type ProbeStat struct {
	Hosts   int   `json:"hosts"`
	Skipped int   `json:"skipped"`
	Bytes   int   `json:"bytes"`
	InAppMS int64 `json:"inAppMs"`
	TotalMS int64 `json:"totalMs"`
	Items   int   `json:"items"`
}

type probeState struct {
	mu     sync.Mutex
	status ProbeStatus
	gen    int // bumped on every launch, so a stale warm-up does not report
	ready  chan struct{}
	key    string
	items  []*probeItem
	// excluded outlives launches: a class that took the app down once would
	// again.
	excluded []string
}

func (s *Session) probeSocket() string { return ProbeSocketPath(s.UDID) }

// probeLaunched resets the probe's state for a fresh launch and, in the
// background, waits for it to answer and runs the slow first dump.
func (s *Session) probeLaunched(pid int) {
	p := &s.probe
	p.mu.Lock()
	p.gen++
	gen := p.gen
	ready := make(chan struct{})
	p.ready = ready
	p.key, p.items = "", nil
	p.status = ProbeStatus{State: "starting", Dylib: s.Probe, Socket: s.probeSocket(), Excluded: p.excluded}
	p.mu.Unlock()
	go s.probeWarmup(gen, pid, ready)
}

func (s *Session) probeWarmup(gen, pid int, ready chan struct{}) {
	ctx := context.Background()
	deadline := time.Now().Add(s.probeStart())
	var lastErr error
	for {
		var pong struct {
			PID int `json:"pid"`
		}
		pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		lastErr = ProbeRequest(pctx, s.probeSocket(), "ping", nil, &pong)
		cancel()
		if lastErr == nil && (pid == 0 || pong.PID == pid) {
			break
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("the probe socket belongs to pid %d, not the app's pid %d", pong.PID, pid)
		} else {
			lastErr = fmt.Errorf("nothing answered on %s", s.probeSocket())
		}
		if time.Now().After(deadline) {
			reason := s.probeFailure(ctx, lastErr)
			s.probeSet(gen, func(st *ProbeStatus) { st.State, st.Reason = "unavailable", reason })
			s.Log("probe: unavailable: " + reason)
			close(ready)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	s.probeSet(gen, func(st *ProbeStatus) { st.State = "loaded" })
	close(ready)
	started := time.Now()
	wctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	var dump ProbeDump
	if err := ProbeRequest(wctx, s.probeSocket(), "dump", s.probeDumpArgs(), &dump); err != nil {
		s.probeFailed(gen, err)
		s.Log("probe: warm-up dump failed: " + err.Error())
		return
	}
	ms := time.Since(started).Milliseconds()
	s.probeSet(gen, func(st *ProbeStatus) { st.WarmupMS = ms })
	s.Log(fmt.Sprintf("probe: loaded; warm-up dump %d ms", ms))
}

func (s *Session) probeSet(gen int, f func(*ProbeStatus)) {
	s.probe.mu.Lock()
	defer s.probe.mu.Unlock()
	if s.probe.gen == gen {
		f(&s.probe.status)
	}
}

// ProbeStatus returns the probe's state, or nil without --probe.
func (s *Session) ProbeStatus() *ProbeStatus {
	if s.Probe == "" {
		return nil
	}
	s.probe.mu.Lock()
	defer s.probe.mu.Unlock()
	st := s.probe.status
	if st.State == "" {
		st = ProbeStatus{State: "starting", Dylib: s.Probe, Socket: s.probeSocket()}
	}
	return &st
}

// probeWait waits, up to timeout, for a launch's probe to answer or give up.
func (s *Session) probeWait(timeout time.Duration) *ProbeStatus {
	s.probe.mu.Lock()
	ready := s.probe.ready
	s.probe.mu.Unlock()
	if ready != nil {
		select {
		case <-ready:
		case <-time.After(timeout):
		}
	}
	return s.ProbeStatus()
}

// probeFailure explains why a launched app's probe never answered.
func (s *Session) probeFailure(ctx context.Context, err error) string {
	if app := s.installedApp(ctx); app != "" && HardenedRuntime(ctx, app) {
		return "the app is signed with the hardened runtime, which makes dyld ignore DYLD_INSERT_LIBRARIES; " +
			"build the simulator configuration with ENABLE_HARDENED_RUNTIME = NO to use --probe"
	}
	return fmt.Sprintf("the app did not load the probe within %s (%v); "+
		"it may clear DYLD_* variables, or have exited", s.probeStart(), err)
}

func (s *Session) installedApp(ctx context.Context) string {
	if s.AppPath != "" {
		return s.AppPath
	}
	app, _ := AppContainer(ctx, s.UDID, s.BundleID)
	return app
}

// HardenedRuntime reports whether the app bundle's executable is signed with
// the hardened runtime.
func HardenedRuntime(ctx context.Context, app string) bool {
	out, _ := exec.CommandContext(ctx, "codesign", "-dv", app).CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "CodeDirectory ") && strings.Contains(line, "runtime") {
			return true
		}
	}
	return false
}

// probeMode says how much a read may ask of the probe.
type probeMode int

const (
	// probeNone: the plain out-of-process tree.
	probeNone probeMode = iota
	// probeCached: merge the last dump if the screen's layout is unchanged,
	// else read plain. The settle loops behind click, fill and scroll re-read
	// several times a second and must not pay for a dump each time.
	probeCached
	// probeFetch: merge, dumping again when the layout changed.
	probeFetch
)

func (s *Session) probeDumpArgs() map[string]any {
	s.probe.mu.Lock()
	defer s.probe.mu.Unlock()
	return map[string]any{"swiftui": true, "onScreen": true, "skipClasses": append([]string{}, s.probe.excluded...)}
}

// probeFailed records a dump that took the app down, so later dumps leave that
// hosting view class out.
func (s *Session) probeFailed(gen int, err error) {
	var crash *ProbeCrash
	if !errors.As(err, &crash) {
		return
	}
	p := &s.probe
	p.mu.Lock()
	defer p.mu.Unlock()
	// A relaunch ends the previous app's dump too; that is not a crash.
	if p.gen != gen {
		return
	}
	if !slices.Contains(p.excluded, crash.Class) {
		p.excluded = append(p.excluded, crash.Class)
	}
	p.status.Excluded = p.excluded
	p.status.State, p.status.Reason = "unavailable", crash.Error()+"; later dumps leave that view out. Relaunch the app (`sightmap device relaunch`)"
}

// mergeProbe adds the probe's structure to d. A probe that is missing or
// fails leaves the out-of-process tree as it is.
func (s *Session) mergeProbe(ctx context.Context, d *Describe, mode probeMode) bool {
	if s.Probe == "" || mode == probeNone || d.Overlay || d.Root == nil {
		return false
	}
	key := describeKey(d)
	p := &s.probe
	p.mu.Lock()
	if p.key == key && p.items != nil {
		items := p.items
		p.status.CacheHits++
		p.mu.Unlock()
		MergeProbe(d, items, s.mergeOptions())
		return true
	}
	state, gen := p.status.State, p.gen
	p.mu.Unlock()
	if mode != probeFetch || state == "unavailable" {
		return false
	}
	if state == "starting" {
		s.probeWait(s.probeStart())
	}
	started := time.Now()
	var dump ProbeDump
	dctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := ProbeRequest(dctx, s.probeSocket(), "dump", s.probeDumpArgs(), &dump); err != nil {
		s.probeFailed(gen, err)
		s.Log("probe: " + err.Error())
		return false
	}
	items, err := ProbeItems(&dump)
	if err != nil {
		s.Log("probe: " + err.Error())
		return false
	}
	stat := &ProbeStat{Hosts: len(dump.Hosts), Skipped: dump.Meta.Skipped, InAppMS: int64(dump.Meta.Timing["total"]), Items: len(items)}
	for _, h := range dump.Hosts {
		stat.Bytes += h.Info.Bytes
	}
	MergeProbe(d, items, s.mergeOptions())
	stat.TotalMS = time.Since(started).Milliseconds()
	p.mu.Lock()
	p.key, p.items = key, items
	p.status.LastDump = stat
	p.status.Dumps++
	p.mu.Unlock()
	s.Log(fmt.Sprintf("probe: dump of %d hosts (%d off screen skipped), %d bytes; in-app %d ms, total %d ms, %d items",
		stat.Hosts, stat.Skipped, stat.Bytes, stat.InAppMS, stat.TotalMS, stat.Items))
	return true
}

func (s *Session) mergeOptions() MergeOptions {
	return MergeOptions{Fields: s.ProbeFields != ProbeFieldsNone}
}

// describeKey fingerprints the out-of-process tree: every element's role,
// identifier, label, value and frame. A dump is reused while it is unchanged.
func describeKey(d *Describe) string {
	h := fnv.New64a()
	var walk func(e *RawElement)
	walk = func(e *RawElement) {
		fmt.Fprintf(h, "%s|%s|%s|%s|%s|%.0f,%.0f,%.0f,%.0f\n", e.Type, e.Role, e.Identifier, e.Label, e.Value, e.Frame.X, e.Frame.Y, e.Frame.W, e.Frame.H)
		for _, c := range e.Children {
			walk(c)
		}
	}
	walk(d.Root)
	return fmt.Sprintf("%x", h.Sum64())
}

// probeNote is the snapshot's note when the probe is on but did not shape it.
func probeNote(st *ProbeStatus) string {
	switch st.State {
	case "unavailable":
		return "[Probe] not loaded, so this is the plain accessibility tree: " + st.Reason
	case "starting":
		return "[Probe] the app has not answered yet; this is the plain accessibility tree. Snapshot again in a moment."
	}
	return "[Probe] the in-app read failed (see the daemon log); this is the plain accessibility tree."
}

func (s *Session) probeStart() time.Duration { return cmp.Or(s.Timing.ProbeStart, 15*time.Second) }
