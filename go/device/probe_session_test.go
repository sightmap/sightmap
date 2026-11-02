package device

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeProbe serves ping and dump on the probe socket for udid, like the dylib.
type fakeProbe struct {
	pid   int
	dump  []byte
	dumps atomic.Int32
	ln    net.Listener
	// crash, when set, makes a dump end like an app that died while
	// serializing a host of that class.
	crash    atomic.Value
	lastArgs atomic.Value
}

func startFakeProbe(t *testing.T, udid string, pid int) *fakeProbe {
	t.Helper()
	f, err := os.Open("testdata/probe-catalog.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	dump, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	path := ProbeSocketPath(udid)
	os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	p := &fakeProbe{pid: pid, dump: dump, ln: ln}
	t.Cleanup(func() { ln.Close(); os.Remove(path) })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go p.serve(conn)
		}
	}()
	return p
}

func (p *fakeProbe) serve(conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		var req struct {
			ID   int            `json:"id"`
			Cmd  string         `json:"cmd"`
			Args map[string]any `json:"args"`
		}
		json.Unmarshal(sc.Bytes(), &req)
		switch req.Cmd {
		case "ping":
			fmt.Fprintf(conn, `{"id":%d,"ok":true,"result":{"pid":%d}}`+"\n", req.ID, p.pid)
		case "dump":
			p.dumps.Add(1)
			p.lastArgs.Store(req.Args)
			if c, _ := p.crash.Load().(string); c != "" {
				fmt.Fprintf(conn, `{"progress":{"cls":%q}}`+"\n", c)
				return
			}
			fmt.Fprintf(conn, `{"id":%d,"ok":true,"result":%s}`+"\n", req.ID, p.dump)
		}
	}
}

func newProbeSession(t *testing.T) (*Session, *fakeHelper) {
	t.Helper()
	s, h := newTestSession(t, "describe-simple")
	h.describe = catalogDescribe()
	data, err := os.ReadFile("testdata/SampleApp/.sightmap/views/catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.SightmapDir, "views", "catalog.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	// Socket paths are derived from the UDID; keep tests apart.
	s.UDID = fmt.Sprintf("probe-%d-%s", os.Getpid(), t.Name())
	s.Probe = "/nonexistent/probe.dylib"
	return s, h
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func TestProbeSessionWarmsUpCachesAndRefetchesOnChange(t *testing.T) {
	s, h := newProbeSession(t)
	p := startFakeProbe(t, s.UDID, 42)
	if _, err := handle(t, s, "launch", nil); err != nil {
		t.Fatal(err)
	}
	if st := s.probeWait(5 * time.Second); st.State != "loaded" {
		t.Fatalf("probe state = %+v", st)
	}
	waitFor(t, "the warm-up dump", func() bool { return p.dumps.Load() == 1 && s.ProbeStatus().WarmupMS >= 0 })

	r, err := handle(t, s, "snapshot", SnapshotArgs{})
	if err != nil {
		t.Fatal(err)
	}
	text := r["text"].(string)
	if !strings.Contains(text, `[CatalogRow maker="Acme, Inc." price="9.99"`) || strings.Contains(text, "[Probe]") {
		t.Fatalf("snapshot:\n%s", text)
	}
	if n := p.dumps.Load(); n != 2 {
		t.Fatalf("dumps after the first snapshot = %d, want 2 (warm-up + snapshot)", n)
	}

	// Same layout: snapshot and the settle reads behind a click reuse the dump.
	if _, err := handle(t, s, "snapshot", SnapshotArgs{}); err != nil {
		t.Fatal(err)
	}
	if _, err := handle(t, s, "click", TargetArgs{Target: "catalog-row-2"}); err != nil {
		t.Fatal(err)
	}
	if n := p.dumps.Load(); n != 2 {
		t.Fatalf("dumps with an unchanged layout = %d, want 2", n)
	}
	if st := s.ProbeStatus(); st.CacheHits < 2 || st.Dumps != 1 || st.LastDump == nil || st.LastDump.Hosts != 3 {
		t.Fatalf("status = %+v", st)
	}

	// A changed label is a changed screen.
	h.describe.Root.Children[3].Label = "Mug, Large, Kiln, $11.00"
	if _, err := handle(t, s, "snapshot", SnapshotArgs{}); err != nil {
		t.Fatal(err)
	}
	if n := p.dumps.Load(); n != 3 {
		t.Fatalf("dumps after the layout changed = %d, want 3", n)
	}
}

func TestProbeTargetOnlyTheProbeShowsIsResolvedAndKeptOutOfTheRunLog(t *testing.T) {
	s, h := newProbeSession(t)
	p := startFakeProbe(t, s.UDID, 42)
	if _, err := handle(t, s, "launch", nil); err != nil {
		t.Fatal(err)
	}
	s.probeWait(5 * time.Second)
	waitFor(t, "the warm-up dump", func() bool { return p.dumps.Load() == 1 })
	// No snapshot first: the click's plain read can't see the Text, so it
	// fetches once.
	if _, err := handle(t, s, "click", TargetArgs{Target: `text[text="Kiln"]`, Settle: "2s"}); err != nil {
		t.Fatal(err)
	}
	if len(h.inputs) != 1 || !strings.HasPrefix(h.inputs[0], "tap ") {
		t.Fatalf("inputs = %v", h.inputs)
	}
	entries, err := ReadRun(s.RunLog.Current().Path, 0)
	if err != nil || len(entries) == 0 {
		t.Fatalf("run log = %v, %v", entries, err)
	}
	if tg := entries[len(entries)-1].Target; tg == nil || tg.Role != "text" || tg.Text != "" {
		t.Fatalf("run log target = %+v; a probe node's text stays out of the run log", tg)
	}
}

func TestProbeThatNeverAnswersFallsBackToThePlainTree(t *testing.T) {
	defer func(d time.Duration) { probeStartTimeout = d }(probeStartTimeout)
	probeStartTimeout = 300 * time.Millisecond
	s, _ := newProbeSession(t)
	if _, err := handle(t, s, "launch", nil); err != nil {
		t.Fatal(err)
	}
	st := s.probeWait(5 * time.Second)
	if st.State != "unavailable" || !strings.Contains(st.Reason, "did not load the probe") {
		t.Fatalf("status = %+v", st)
	}
	r, err := handle(t, s, "snapshot", SnapshotArgs{})
	if err != nil {
		t.Fatal(err)
	}
	text := r["text"].(string)
	if !strings.Contains(text, `[CatalogRow summary="Socks, Wool, Pair of 2, Acme, Inc., $9.99"]`) || strings.Contains(text, "[Maker") ||
		!strings.Contains(text, "[Probe] not loaded") || strings.Contains(text, "view[type=") {
		t.Fatalf("snapshot:\n%s", text)
	}
	ready := s.WaitReady(context.Background())
	if p, ok := ready["probe"].(*ProbeStatus); !ok || p.State != "unavailable" {
		t.Fatalf("ready = %v", ready)
	}
}

func TestProbeSocketOfAnotherProcessIsNotTrusted(t *testing.T) {
	defer func(d time.Duration) { probeStartTimeout = d }(probeStartTimeout)
	probeStartTimeout = 300 * time.Millisecond
	s, _ := newProbeSession(t)
	startFakeProbe(t, s.UDID, 7)
	if _, err := handle(t, s, "launch", nil); err != nil {
		t.Fatal(err)
	}
	if st := s.probeWait(5 * time.Second); st.State != "unavailable" || !strings.Contains(st.Reason, "pid 7") {
		t.Fatalf("status = %+v", st)
	}
}

func TestBuildProbeCompilesAndLinks(t *testing.T) {
	if _, err := exec.LookPath("xcrun"); err != nil || runtime.GOOS != "darwin" {
		t.Skip("needs Xcode")
	}
	if out, err := exec.Command("xcrun", "-sdk", "iphonesimulator", "--show-sdk-path").CombinedOutput(); err != nil {
		t.Skipf("no iOS Simulator SDK: %s", out)
	}
	t.Setenv("SIGHTMAP_DEVICE_CACHE", t.TempDir())
	t.Setenv("SIGHTMAP_DEVICE_PROBE", "")
	path, err := BuildProbe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if again, err := BuildProbe(context.Background()); err != nil || again != path {
		t.Fatalf("second build = %s, %v; want the cached %s", again, err, path)
	}
}

func TestProbeDumpThatTakesTheAppDownExcludesThatHostClass(t *testing.T) {
	s, _ := newProbeSession(t)
	p := startFakeProbe(t, s.UDID, 42)
	if _, err := handle(t, s, "launch", nil); err != nil {
		t.Fatal(err)
	}
	s.probeWait(5 * time.Second)
	waitFor(t, "the warm-up dump", func() bool { return p.dumps.Load() == 1 })
	p.crash.Store("DetailHostingView")
	r, err := handle(t, s, "snapshot", SnapshotArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if text := r["text"].(string); !strings.Contains(text, "[Probe] not loaded") || !strings.Contains(text, "DetailHostingView") {
		t.Fatalf("snapshot:\n%s", text)
	}
	if st := s.ProbeStatus(); st.State != "unavailable" || len(st.Excluded) != 1 {
		t.Fatalf("status = %+v", st)
	}

	p.crash.Store("")
	if _, err := handle(t, s, "launch", nil); err != nil {
		t.Fatal(err)
	}
	s.probeWait(5 * time.Second)
	waitFor(t, "the warm-up dump", func() bool { return p.dumps.Load() == 3 })
	args, _ := p.lastArgs.Load().(map[string]any)
	if skip, _ := args["skipClasses"].([]any); len(skip) != 1 || skip[0] != "DetailHostingView" {
		t.Fatalf("dump args after relaunch = %v", args)
	}
	if st := s.ProbeStatus(); st.State != "loaded" || len(st.Excluded) != 1 {
		t.Fatalf("status after relaunch = %+v", st)
	}
}
