//go:build !windows

package device

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// recorder stands in for the simulator: commands spawned in it, `log stream`
// (a long-running process that writes its pid), and the Sim calls that matter
// to the session's lifecycle.
type recorder struct {
	mu     sync.Mutex
	events []string
	pids   []string
	dir    string
}

func newRecorder(t *testing.T) *recorder {
	r := &recorder{dir: t.TempDir()}
	spawn, stream, ents := simSpawn, logStream, entitlementsOf
	t.Cleanup(func() { simSpawn, logStream, entitlementsOf = spawn, stream, ents })
	simSpawn = func(_ context.Context, _ string, args ...string) error {
		r.add("spawn " + strings.Join(args, " "))
		return nil
	}
	logStream = func(ctx context.Context, _, _ string) *exec.Cmd {
		r.mu.Lock()
		pidFile := filepath.Join(r.dir, strconv.Itoa(len(r.pids)))
		r.pids = append(r.pids, pidFile)
		r.mu.Unlock()
		return exec.CommandContext(ctx, "sh", "-c", "echo $$ > "+pidFile+"; exec sleep 60")
	}
	entitlementsOf = func(_ context.Context, exe string) []byte {
		if strings.Contains(exe, "entitled") {
			return []byte("<key>get-task-allow</key><true/>")
		}
		return nil
	}
	return r
}

func (r *recorder) add(e string) {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
}

func (r *recorder) index(prefix string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.IndexFunc(r.events, func(e string) bool { return strings.HasPrefix(e, prefix) })
}

// streamPIDs waits for n log streams to start and write their pids.
func (r *recorder) streamPIDs(t *testing.T, n int) []int {
	t.Helper()
	var files []string
	for deadline := time.Now().Add(5 * time.Second); len(files) < n; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d log streams started", len(files), n)
		}
		r.mu.Lock()
		files = slices.Clone(r.pids)
		r.mu.Unlock()
	}
	var pids []int
	for _, f := range files {
		deadline := time.Now().Add(5 * time.Second)
		for {
			data, err := os.ReadFile(f)
			if pid, convErr := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && convErr == nil {
				pids = append(pids, pid)
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("log stream never started (%s)", f)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	return pids
}

type recordingSim struct {
	fakeSim
	r *recorder
}

func (s recordingSim) Install(_ context.Context, _, app string) error {
	s.r.add("install " + app)
	return nil
}
func (s recordingSim) Terminate(context.Context, string, string) { s.r.add("terminate") }
func (s recordingSim) Shutdown(context.Context, string)          { s.r.add("shutdown") }

func newLifecycleSession(t *testing.T, mode, appPath string) (*Session, *recorder) {
	t.Helper()
	r := newRecorder(t)
	s, _ := newTestSession(t, "describe-simple")
	s.Sim = recordingSim{r: r}
	s.Process, s.AppPath, s.Config.Network, s.MetroPort = "App", appPath, mode, 1
	s.Logs = &LogCollector{UDID: s.UDID, Process: s.Process, Console: s.Console, Log: s.Log}
	s.Logs.Start()
	s.Net = s.newNetCollector(context.Background())
	return s, r
}

func TestStopShutdownSwitchesHAROffFirst(t *testing.T) {
	s, r := newLifecycleSession(t, SourceHAR, "/apps/entitled/App.app")
	if r.index("spawn defaults write com.apple.CFNetwork har-capture-global") < 0 {
		t.Fatalf("HAR was not armed: %q", r.events)
	}
	// The console's and the network's.
	pids := r.streamPIDs(t, 2)
	ln, err := net.Listen("unix", filepath.Join(r.dir, "s.sock"))
	if err != nil {
		t.Fatal(err)
	}
	d := &Daemon{Session: s, listener: ln, stopped: make(chan struct{})}
	d.Stop(true)

	disarm, shutdown := r.index("spawn defaults delete com.apple.CFNetwork har-capture-global"), r.index("shutdown")
	if disarm < 0 || shutdown < 0 || disarm > shutdown {
		t.Errorf("HAR must be switched off before the simulator shuts down: %q", r.events)
	}
	for _, pid := range pids {
		if pidAlive(pid) {
			t.Errorf("log stream %d still running after stop", pid)
		}
	}
}

func TestRestartWithAppRechecksNetworkSources(t *testing.T) {
	s, r := newLifecycleSession(t, NetworkAuto, "/apps/plain/App.app")
	defer s.Close()
	if st := harState(s); st.State != stateUnavailable {
		t.Fatalf("before: har %+v", st)
	}
	if _, err := handle(t, s, "restart", ValueArgs{App: "/apps/entitled/App.app"}); err != nil {
		t.Fatal(err)
	}
	if r.index("install /apps/entitled/App.app") < 0 {
		t.Fatalf("not installed: %q", r.events)
	}
	if st := harState(s); st.State != stateActive {
		t.Errorf("after reinstalling an entitled build: har %+v", st)
	}
}

func harState(s *Session) NetSourceStatus {
	for _, st := range s.Net.Status().Sources {
		if st.Name == SourceHAR {
			return st
		}
	}
	return NetSourceStatus{}
}

type unreachableHelper struct{ fakeHelper }

func (*unreachableHelper) Request(context.Context, string, map[string]any, any) error {
	return errorf("helper_crashed", "the helper exited")
}

// A daemon that fails to start exits at once; what it armed must not outlive it.
func TestServeFailureClosesCollectors(t *testing.T) {
	r := newRecorder(t)
	s, _ := newTestSession(t, "describe-simple")
	s.Sim, s.Helper = recordingSim{r: r}, &unreachableHelper{}
	s.Process, s.AppPath, s.Config.Network, s.MetroPort = "App", "/apps/entitled/App.app", SourceHAR, 1
	if _, err := Serve(s, false, ""); Code(err) != "helper_crashed" {
		t.Fatalf("Serve = %v", err)
	}
	// A stream cancelled before it wrote its pid is gone too.
	r.mu.Lock()
	files := slices.Clone(r.pids)
	r.mu.Unlock()
	for _, f := range files {
		data, _ := os.ReadFile(f)
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pidAlive(pid) {
			t.Errorf("log stream %d still running after a failed start", pid)
		}
	}
	if r.index("spawn defaults delete com.apple.CFNetwork har-capture-global") < 0 {
		t.Errorf("HAR left on after a failed start: %q", r.events)
	}
}
