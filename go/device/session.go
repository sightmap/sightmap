package device

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sightmap/sightmap/go/match"
	"github.com/sightmap/sightmap/go/sightmap"
)

// Config describes the app a Session drives.
type Config struct {
	// Platform is PlatformIOS (the default when empty) or PlatformAndroid.
	Platform string `json:"platform,omitempty"`
	// UDID is the simulator's UDID, or on Android the device serial.
	UDID       string `json:"udid"`
	DeviceName string `json:"deviceName,omitempty"`
	// BundleID is the app's bundle id, or on Android its package name.
	BundleID string `json:"bundleId"`
	AppPath  string `json:"appPath,omitempty"`
	// Process names the app's process for its logs: the executable on iOS, the
	// package on Android.
	Process     string `json:"process"`
	SightmapDir string `json:"sightmapDir"`
	// RunDir holds the app's redirected stdout/stderr and the daemon log.
	RunDir   string `json:"runDir,omitempty"`
	RunLabel string `json:"runLabel,omitempty"`
	// ConsoleLimit is how many console entries the session keeps; 0 means 5000.
	ConsoleLimit int `json:"consoleLimit,omitempty"`
	// Network is the network source mode (NetworkModes); "" means auto.
	Network string `json:"network,omitempty"`
	// NetworkLimit is how many network entries the session keeps; 0 means 1000.
	NetworkLimit int `json:"networkLimit,omitempty"`
	// MetroPort is where the rn-cdp source looks for Metro; 0 means the app's
	// RCT_jsLocation default, else 8081.
	MetroPort int `json:"metroPort,omitempty"`
}

// Timing tunes how long a Session waits for the screen; zero fields take the
// defaults.
type Timing struct {
	Settle        time.Duration // after a tap before reading a field back
	ScrollSettle  time.Duration // after a swipe before re-reading
	OverlaySettle time.Duration // for a system sheet to finish animating in
	FillReadback  time.Duration // how long fill waits for the typed text to show
	ReadyTimeout  time.Duration // how long start waits for a readable tree
	RetryInterval time.Duration // between re-reads while the screen moves
	RetryTimeout  time.Duration // how long click/fill wait for a missing target or a moving screen
}

// androidTiming is the Android default: `uiautomator dump` takes a second or
// more per read, so waits that span several reads are longer.
var androidTiming = Timing{ReadyTimeout: 60 * time.Second, RetryTimeout: 8 * time.Second, FillReadback: 4 * time.Second}

// or fills t's zero fields from d.
func (t Timing) or(d Timing) Timing {
	for _, f := range []struct{ field, def *time.Duration }{
		{&t.Settle, &d.Settle}, {&t.ScrollSettle, &d.ScrollSettle}, {&t.OverlaySettle, &d.OverlaySettle},
		{&t.FillReadback, &d.FillReadback}, {&t.ReadyTimeout, &d.ReadyTimeout}, {&t.RetryInterval, &d.RetryInterval},
		{&t.RetryTimeout, &d.RetryTimeout},
	} {
		if *f.field == 0 {
			*f.field = *f.def
		}
	}
	return t
}

func (t Timing) withDefaults() Timing {
	def := func(d *time.Duration, v time.Duration) {
		if *d == 0 {
			*d = v
		}
	}
	def(&t.Settle, 350*time.Millisecond)
	def(&t.ScrollSettle, 150*time.Millisecond)
	def(&t.OverlaySettle, 800*time.Millisecond)
	def(&t.FillReadback, 2*time.Second)
	def(&t.ReadyTimeout, 20*time.Second)
	def(&t.RetryInterval, 150*time.Millisecond)
	// A Debug React Native build can take a few seconds to draw a screen
	// after a navigation.
	def(&t.RetryTimeout, 3*time.Second)
	return t
}

// Session drives one app on one simulator: it reads and annotates the screen,
// acts on it, and records each step in the run log. The daemon serves one
// Session; a library caller can own one directly.
type Session struct {
	Config
	Helper  Helper
	Sim     Sim
	Console *Console
	Logs    *LogCollector
	Network *Network
	Net     *NetCollector
	RunLog  *RunLog
	Timing  Timing
	Log     func(string)

	StartedAt time.Time
	AppPID    int

	mu         sync.Mutex // serializes commands
	corpus     corpusCache
	last       *Snapshot
	screen     Size
	lastTarget *sightmap.ComponentNode
	lastSnap   *Snapshot
	lastView   string
	// secureFocus is true while the last element tapped is a secure text
	// field, so text typed into it stays out of the run log.
	secureFocus bool
	axOn        *bool
	readable    bool
}

// NewSession returns a Session for cfg using helper and sim.
func NewSession(cfg Config, helper Helper, sim Sim) *Session {
	return &Session{
		Config:    cfg,
		Helper:    helper,
		Sim:       sim,
		Console:   NewConsole(cmp.Or(cfg.ConsoleLimit, 5000)),
		Network:   NewNetwork(cmp.Or(cfg.NetworkLimit, 1000)),
		RunLog:    NewRunLog(cfg.SightmapDir),
		Log:       func(string) {},
		StartedAt: time.Now(),
		corpus:    corpusCache{dir: cfg.SightmapDir},
	}
}

func (s *Session) stdoutPath() string {
	if s.RunDir == "" {
		return ""
	}
	return filepath.Join(s.RunDir, "app-stdout.log")
}

func (s *Session) stderrPath() string {
	if s.RunDir == "" {
		return ""
	}
	return filepath.Join(s.RunDir, "app-stderr.log")
}

// corpusCache reloads the corpus whenever a YAML file under the directory
// changes, so edits apply to the next snapshot without restarting the session.
type corpusCache struct {
	dir     string
	stamp   string
	corpus  *sightmap.Corpus
	matcher *match.Matcher
}

func (c *corpusCache) get() (*sightmap.Corpus, *match.Matcher, error) {
	var b strings.Builder
	filepath.WalkDir(c.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !(strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			return nil
		}
		if info, err := d.Info(); err == nil {
			fmt.Fprintf(&b, "%s %d %d\n", path, info.Size(), info.ModTime().UnixNano())
		}
		return nil
	})
	stamp := b.String()
	if stamp == "" {
		c.stamp, c.corpus, c.matcher = "", nil, nil
		return nil, nil, nil
	}
	if stamp != c.stamp {
		corpus, err := sightmap.Load(c.dir)
		if err != nil {
			return nil, nil, withHint("corpus_invalid", "fix the file, then run `sightmap validate`", "cannot load the corpus at %s: %v", c.dir, err)
		}
		c.stamp, c.corpus, c.matcher = stamp, corpus, match.NewMatcher(corpus)
	}
	return c.corpus, c.matcher, nil
}

func (s *Session) describe(ctx context.Context) (*Describe, error) {
	var d Describe
	err := s.Helper.Request(ctx, "describe", map[string]any{"udid": s.UDID}, &d)
	return &d, err
}

// describeSettled waits out a system sheet animating in, which leaves no
// readable tree for a moment once the app had one.
func (s *Session) describeSettled(ctx context.Context) (*Describe, error) {
	d, err := s.describe(ctx)
	if err == nil {
		s.readable = true
		return d, nil
	}
	if Code(err) != "ax_disabled" || !s.readable {
		return nil, err
	}
	time.Sleep(s.Timing.OverlaySettle)
	return s.describe(ctx)
}

func (s *Session) accessibilityOn(ctx context.Context) bool {
	if s.axOn == nil {
		on := s.Sim.AccessibilityEnabled(ctx, s.UDID)
		s.axOn = &on
	}
	return *s.axOn
}

// fresh reads and annotates the screen.
func (s *Session) fresh(ctx context.Context, view string) (*Snapshot, error) {
	d, err := s.describeSettled(ctx)
	if err != nil {
		if Code(err) == "ax_disabled" {
			return nil, emptyTreeError(s.UDID, s.accessibilityOn(ctx))
		}
		return nil, err
	}
	corpus, m, err := s.corpus.get()
	if err != nil {
		return nil, err
	}
	snap, err := Annotate(Normalize(d), corpus, m, view)
	if err != nil {
		return nil, err
	}
	s.screen = snap.Screen
	s.lastView = snap.ViewName()
	s.lastSnap = snap
	return snap, nil
}

// settled re-reads until two reads agree on every element's place, so a
// list's momentum has stopped.
func (s *Session) settled(ctx context.Context, timeout time.Duration) (*Snapshot, error) {
	snap, err := s.fresh(ctx, "")
	if err != nil || snap.Settled {
		return snap, err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		next, err := s.fresh(ctx, "")
		if err != nil {
			return nil, err
		}
		if layout(next, true) == layout(snap, true) {
			return next, nil
		}
		snap = next
	}
	return snap, nil
}

// layout fingerprints where things are: frames, not labels, since clocks and
// counters change text without anything moving.
func layout(s *Snapshot, withLabels bool) string {
	var b strings.Builder
	for _, n := range s.Nodes {
		label := ""
		if withLabels {
			label = n.Name
		}
		fmt.Fprintf(&b, "%s|%s|%s|%d,%d,%d,%d\n", n.Role, Identifier(n), label, n.Bounds.X, n.Bounds.Y, n.Bounds.Width, n.Bounds.Height)
	}
	return b.String()
}

func (s *Session) input(ctx context.Context, cmd string, args map[string]any) error {
	args["udid"] = s.UDID
	return s.Helper.Request(ctx, cmd, args, nil)
}

// LaunchApp launches the app; terminate restarts a running instance and
// re-enables app accessibility first.
func (s *Session) LaunchApp(ctx context.Context, terminate bool) (map[string]any, error) {
	if terminate {
		if err := s.Sim.EnableAccessibility(ctx, s.UDID); err != nil {
			return nil, err
		}
	}
	pid, err := s.Sim.Launch(ctx, s.UDID, s.BundleID, LaunchOptions{Stdout: s.stdoutPath(), Stderr: s.stderrPath(), Terminate: terminate, Env: s.Net.LaunchEnv()})
	if err != nil {
		return nil, err
	}
	s.AppPID, s.axOn, s.readable, s.last, s.secureFocus = pid, nil, false, nil, false
	if s.Logs != nil {
		s.Logs.SetPID(pid)
	}
	return map[string]any{"bundleId": s.BundleID, "pid": pid}, nil
}

// WaitReady waits until the accessibility tree is readable and shows something
// to act on, which lags a launch by a moment.
func (s *Session) WaitReady(ctx context.Context) map[string]any {
	deadline := time.Now().Add(s.Timing.ReadyTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		snap, err := s.fresh(ctx, "")
		if err == nil && snap.Coverage.Total > 0 {
			return map[string]any{"ready": true, "interactive": snap.Coverage.Total}
		}
		if err != nil {
			switch Code(err) {
			case "ax_error", "ax_unavailable", "ax_disabled", "helper_crashed":
				lastErr = err
			default:
				return map[string]any{"ready": false, "error": AsError(err)}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr != nil {
		return map[string]any{"ready": false, "error": AsError(lastErr)}
	}
	return map[string]any{"ready": false, "interactive": 0}
}

// loggedActions are the commands recorded in the run log, by run-log name.
var loggedActions = map[string]string{
	"snapshot": "snapshot", "click": "click", "fill": "fill", "scroll": "scroll", "swipe": "swipe", "tapXY": "tap-xy",
	"button": "button", "key": "key", "type": "type", "openurl": "openurl", "launch": "launch", "relaunch": "relaunch", "restart": "relaunch", "runNote": "note",
}

// Handle runs one command with JSON args and returns its result. Commands run
// one at a time.
func (s *Session) Handle(ctx context.Context, cmd string, raw json.RawMessage) (any, error) {
	if cmd == "status" || cmd == "ping" {
		return s.handle(ctx, cmd, raw)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	action, logged := loggedActions[cmd]
	if !logged {
		return s.handle(ctx, cmd, raw)
	}
	s.lastTarget = nil
	result, err := s.handle(ctx, cmd, raw)
	var args map[string]any
	json.Unmarshal(raw, &args)
	for k, v := range args {
		if v == nil || v == "" || v == false || k == "json" {
			delete(args, k)
		}
	}
	if _, ok := args["text"]; ok && s.redactText(cmd) {
		args["text"] = "<redacted>"
	}
	entry := RunEntry{Action: action, Args: args, View: s.lastView, OK: err == nil, Target: s.runTarget()}
	if cmd == "runNote" {
		kind, _ := args["kind"].(string)
		text, _ := args["text"].(string)
		entry = RunEntry{Action: action, Kind: kind, Text: text, OK: err == nil}
	}
	if err != nil {
		e := AsError(err)
		entry.Error = &Error{Code: e.Code, Message: e.Message}
	}
	if _, werr := s.RunLog.Append(entry); werr != nil {
		s.Log("run log write failed: " + werr.Error())
	}
	return result, err
}

// redactText reports whether cmd's text could be a secret: a fill into a
// secure field, or one whose field never resolved (it might have been one); or
// a type while a secure field has focus.
func (s *Session) redactText(cmd string) bool {
	switch cmd {
	case "fill":
		return s.lastTarget == nil || s.lastTarget.Role == "securetextfield"
	case "type":
		return s.secureFocus
	}
	return false
}

func (s *Session) runTarget() *RunTarget {
	n := s.lastTarget
	if n == nil {
		return nil
	}
	t := &RunTarget{ID: n.Id, Identifier: Identifier(n), Text: n.Name, Role: n.Role}
	if s.lastSnap != nil {
		t.Component = s.lastSnap.Component(n)
	}
	return t
}

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &v); err != nil {
			return v, errorf("bad_request", "bad arguments: %v", err)
		}
	}
	return v, nil
}

// SnapshotArgs are the snapshot command's arguments.
type SnapshotArgs struct {
	View             string `json:"view,omitempty"`
	IncludeOffscreen bool   `json:"includeOffscreen,omitempty"`
	Out              string `json:"out,omitempty"`
	TreeOut          string `json:"treeOut,omitempty"`
	JSON             bool   `json:"json,omitempty"`
}

// SnapshotResult is the snapshot command's result.
type SnapshotResult struct {
	Text         string         `json:"text"`
	View         string         `json:"view,omitempty"`
	ViewInferred bool           `json:"viewInferred,omitempty"`
	Coverage     CoverageCounts `json:"coverage"`
	Conflicts    int            `json:"conflicts,omitempty"`
	Tree         *AnnotatedNode `json:"tree,omitempty"`
	Screen       Size           `json:"screen"`
	Memory       []Memory       `json:"memory,omitempty"`
}

// CoverageCounts is the snapshot's coverage.
type CoverageCounts struct {
	Interactive int `json:"interactive"`
	T1          int `json:"t1"`
	T2          int `json:"t2"`
	T3          int `json:"t3"`
}

// TargetArgs name the element click and fill act on.
type TargetArgs struct {
	Target string `json:"target"`
	Text   string `json:"text,omitempty"`
	Clear  bool   `json:"clear,omitempty"`
	// Settle overrides how long to wait for a missing target or a moving
	// screen (Timing.RetryTimeout), as a Go duration such as "5s".
	Settle string `json:"settle,omitempty"`
}

func (a TargetArgs) settle(def time.Duration) (time.Duration, error) {
	if a.Settle == "" {
		return def, nil
	}
	d, err := time.ParseDuration(a.Settle)
	if err != nil || d < 0 {
		return 0, errorf("usage", "--settle takes a duration such as 3s or 500ms, not %q", a.Settle)
	}
	return d, nil
}

// ScrollArgs are the scroll command's arguments.
type ScrollArgs struct {
	Direction string `json:"direction"`
	In        string `json:"in,omitempty"`
	Until     string `json:"until,omitempty"`
	Max       int    `json:"max,omitempty"`
}

// PointArgs carry screen points for tap-xy and swipe.
type PointArgs struct {
	X        float64 `json:"x,omitempty"`
	Y        float64 `json:"y,omitempty"`
	X1       float64 `json:"x1,omitempty"`
	Y1       float64 `json:"y1,omitempty"`
	X2       float64 `json:"x2,omitempty"`
	Y2       float64 `json:"y2,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}

// ValueArgs carry the single value of type, key, button, openurl, screenshot,
// console get and run new.
type ValueArgs struct {
	Text  string `json:"text,omitempty"`
	Key   string `json:"key,omitempty"`
	Name  string `json:"name,omitempty"`
	URL   string `json:"url,omitempty"`
	Out   string `json:"out,omitempty"`
	Index int    `json:"index,omitempty"`
	Label string `json:"label,omitempty"`
	Kind  string `json:"kind,omitempty"`
	App   string `json:"appPath,omitempty"`
	// Unredacted shows a network entry's credentials (network get).
	Unredacted bool `json:"unredacted,omitempty"`
}

func (s *Session) handle(ctx context.Context, cmd string, raw json.RawMessage) (any, error) {
	switch cmd {
	case "ping":
		return map[string]bool{"pong": true}, nil
	case "ready":
		return s.WaitReady(ctx), nil
	case "status":
		return s.status(), nil
	case "snapshot":
		a, err := decode[SnapshotArgs](raw)
		if err != nil {
			return nil, err
		}
		return s.snapshot(ctx, a)
	case "click":
		a, err := decode[TargetArgs](raw)
		if err != nil {
			return nil, err
		}
		settle, err := a.settle(s.Timing.RetryTimeout)
		if err != nil {
			return nil, err
		}
		r, err := s.tap(ctx, a.Target, settle)
		if err != nil {
			return nil, err
		}
		return r.result(), nil
	case "fill":
		a, err := decode[TargetArgs](raw)
		if err != nil {
			return nil, err
		}
		return s.fill(ctx, a)
	case "scroll":
		a, err := decode[ScrollArgs](raw)
		if err != nil {
			return nil, err
		}
		return s.scroll(ctx, a)
	case "swipe":
		a, err := decode[PointArgs](raw)
		if err != nil {
			return nil, err
		}
		if err := s.assertOnScreen(ctx, "swipe", [2]float64{a.X1, a.Y1}, [2]float64{a.X2, a.Y2}); err != nil {
			return nil, err
		}
		args := map[string]any{"x1": a.X1, "y1": a.Y1, "x2": a.X2, "y2": a.Y2}
		if a.Duration > 0 {
			args["duration"] = a.Duration
		}
		s.last = nil
		return a, s.input(ctx, "swipe", args)
	case "tapXY":
		a, err := decode[PointArgs](raw)
		if err != nil {
			return nil, err
		}
		if err := s.assertOnScreen(ctx, "tap-xy", [2]float64{a.X, a.Y}); err != nil {
			return nil, err
		}
		return map[string]float64{"x": a.X, "y": a.Y}, s.input(ctx, "tap", map[string]any{"x": a.X, "y": a.Y})
	case "type", "key", "button":
		a, err := decode[ValueArgs](raw)
		if err != nil {
			return nil, err
		}
		args := map[string]any{"text": a.Text, "key": a.Key, "name": a.Name}
		return a, s.input(ctx, cmd, args)
	case "openurl":
		a, err := decode[ValueArgs](raw)
		if err != nil {
			return nil, err
		}
		s.last = nil
		return map[string]string{"url": a.URL}, s.Sim.OpenURL(ctx, s.UDID, a.URL)
	case "screenshot":
		a, err := decode[ValueArgs](raw)
		if err != nil {
			return nil, err
		}
		return map[string]string{"out": a.Out}, s.Sim.Screenshot(ctx, s.UDID, a.Out)
	case "launch":
		return s.LaunchApp(ctx, false)
	case "relaunch":
		return s.relaunch(ctx)
	case "restart":
		a, err := decode[ValueArgs](raw)
		if err != nil {
			return nil, err
		}
		if a.App != "" {
			if err := s.Sim.Install(ctx, s.UDID, a.App); err != nil {
				return nil, err
			}
			s.AppPath = a.App
			// The new build may differ in what the network sources depend on,
			// such as the get-task-allow entitlement; they are armed again
			// before it launches.
			if s.Net != nil {
				s.Net.Close()
				s.Net = s.newNetCollector(context.WithoutCancel(ctx))
			}
		}
		r, err := s.relaunch(ctx)
		if r != nil {
			r["installed"] = a.App
		}
		return r, err
	case "consoleList":
		f, err := decode[ConsoleFilter](raw)
		if err != nil {
			return nil, err
		}
		return s.Console.List(f), nil
	case "consoleGet":
		a, err := decode[ValueArgs](raw)
		if err != nil {
			return nil, err
		}
		e, ok := s.Console.Get(a.Index)
		if !ok {
			return nil, withHint("not_found", "run `sightmap device console list` for current indexes", "no console entry #%d (the buffer holds the most recent %d)", a.Index, s.Console.Capacity)
		}
		return e, nil
	case "networkList":
		if err := s.networkSupported(); err != nil {
			return nil, err
		}
		f, err := decode[NetworkFilter](raw)
		if err != nil {
			return nil, err
		}
		if f.Source != "" && f.Source != SourceHAR && f.Source != SourceRNCDP && f.Source != SourceSummary {
			return nil, errorf("usage", "--source takes har, rn-cdp or summary, not %q", f.Source)
		}
		view, err := s.networkView(f.Unredacted)
		if err != nil {
			return nil, err
		}
		return s.Network.List(f, view), nil
	case "networkGet":
		if err := s.networkSupported(); err != nil {
			return nil, err
		}
		a, err := decode[ValueArgs](raw)
		if err != nil {
			return nil, err
		}
		e, ok := s.Network.Get(a.Index)
		if !ok {
			return nil, withHint("not_found", "run `sightmap device network list` for current indexes", "no network entry #%d (the buffer holds the most recent %d)", a.Index, s.Network.Capacity)
		}
		view, err := s.networkView(a.Unredacted)
		if err != nil {
			return nil, err
		}
		return view(e), nil
	case "runNew":
		a, err := decode[ValueArgs](raw)
		if err != nil {
			return nil, err
		}
		return s.RunLog.Begin(a.Label)
	case "runNote":
		a, err := decode[ValueArgs](raw)
		if err != nil {
			return nil, err
		}
		if a.Text == "" {
			return nil, errorf("usage", "run note needs the text to record")
		}
		return map[string]string{"kind": a.Kind, "text": a.Text}, nil
	case "runCurrent":
		return s.RunLog.Current(), nil
	}
	return nil, errorf("unknown_cmd", "unknown device command %q", cmd)
}

// networkView redacts an entry (unless unredacted) and then matches it
// against the corpus, so a property extracted from a credential stays hidden.
func (s *Session) networkView(unredacted bool) (func(NetworkEntry) NetworkEntry, error) {
	corpus, _, err := s.corpus.get()
	if err != nil {
		return nil, err
	}
	return func(e NetworkEntry) NetworkEntry {
		if !unredacted {
			e = e.Redact()
		}
		return e.Annotate(corpus)
	}, nil
}

func (s *Session) relaunch(ctx context.Context) (map[string]any, error) {
	r, err := s.LaunchApp(ctx, true)
	if err != nil {
		return nil, err
	}
	for k, v := range s.WaitReady(ctx) {
		r[k] = v
	}
	return r, nil
}

// Status is the status command's result.
type Status struct {
	Config
	PID       int    `json:"pid"`
	AppPID    int    `json:"appPid,omitempty"`
	StartedAt string `json:"startedAt"`
	Helper    struct {
		Alive    bool `json:"alive"`
		Restarts int  `json:"restarts"`
	} `json:"helper"`
	Console struct {
		Streaming bool `json:"streaming"`
		Entries   int  `json:"entries"`
	} `json:"console"`
	Network *NetStatus `json:"network,omitempty"`
	Run     RunInfo    `json:"run"`
}

func (s *Session) status() Status {
	st := Status{Config: s.Config, PID: os.Getpid(), AppPID: s.AppPID, StartedAt: s.StartedAt.UTC().Format(time.RFC3339), Run: s.RunLog.Current()}
	st.Helper.Alive, st.Helper.Restarts = s.Helper.Alive(), s.Helper.Restarts()
	st.Console.Entries = s.Console.Len()
	if s.Logs != nil {
		st.Console.Streaming = s.Logs.Streaming()
	}
	if s.Net != nil {
		net := s.Net.Status()
		st.Network = &net
	}
	return st
}

func (s *Session) snapshot(ctx context.Context, a SnapshotArgs) (*SnapshotResult, error) {
	snap, err := s.fresh(ctx, a.View)
	if err != nil {
		return nil, err
	}
	s.last = snap
	text := snap.Render(RenderOpts{IncludeOffscreen: a.IncludeOffscreen, UDID: s.UDID, AccessibilityOn: s.accessibilityOn(ctx)})
	if a.Out != "" {
		if err := writeFileAtomic(a.Out, []byte(text+"\n")); err != nil {
			return nil, err
		}
		if err := writeJSON(a.Out+".tree.json", snap.Root); err != nil {
			return nil, err
		}
	}
	if a.TreeOut != "" {
		if err := writeJSON(a.TreeOut, snap.Root); err != nil {
			return nil, err
		}
	}
	r := &SnapshotResult{
		Text: text, View: snap.ViewName(), ViewInferred: snap.ViewInferred, Screen: snap.Screen, Memory: snap.Memory,
		Coverage:  CoverageCounts{Interactive: snap.Coverage.Total, T1: snap.Coverage.T1, T2: snap.Coverage.T2, T3: snap.Coverage.T3},
		Conflicts: len(snap.Conflicts),
	}
	if a.JSON {
		r.Tree = snap.AnnotatedTree()
	}
	return r, nil
}

func writeJSON(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path+".tmp", data, 0o644); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

// assertOnScreen rejects points outside the screen, in points as describe
// reports it. Without a readable tree, the last known size applies.
func (s *Session) assertOnScreen(ctx context.Context, cmd string, points ...[2]float64) error {
	screen := s.screen
	if screen.W == 0 {
		if snap, err := s.fresh(ctx, ""); err == nil {
			screen = snap.Screen
		}
	}
	if screen.W == 0 || screen.H == 0 {
		return nil
	}
	for _, p := range points {
		if p[0] < 0 || p[1] < 0 || p[0] > screen.W || p[1] > screen.H {
			return withHint("off_screen_point", fmt.Sprintf("pick a point within 0–%g x 0–%g; divide screenshot pixel coordinates by the device scale (2 or 3 on iOS, dpi/160 on Android)", screen.W, screen.H),
				"(%g, %g) is outside the %gx%g pt screen; %s takes screen points (origin top-left), not pixels", p[0], p[1], screen.W, screen.H, cmd)
		}
	}
	return nil
}

// Errors that can clear up once a sheet or navigation finishes animating.
var retryable = map[string]bool{"not_found": true, "stale_target": true, "covered": true, "offscreen": true, "disabled": true}

type tapped struct {
	snap    *Snapshot
	node    *sightmap.ComponentNode
	x, y    int
	retries int
}

func (t tapped) result() map[string]any {
	return map[string]any{"target": t.snap.Describe(t.node), "x": t.x, "y": t.y, "retries": t.retries}
}

// tap re-reads the screen, re-resolves target, checks it is tappable and taps
// its center. Right after a sheet closes or a navigation pops, the target can
// be missing or still moving, so it re-reads until two reads agree and the
// target is there, for up to settle.
func (s *Session) tap(ctx context.Context, target string, settle time.Duration) (tapped, error) {
	numeric := IsSnapshotID(target)
	base := s.last
	if numeric && base == nil {
		return tapped{}, withHint("not_found", "run `sightmap device snapshot` first", "no snapshot to resolve id %s against", target)
	}
	view := ""
	if base != nil && !base.ViewInferred {
		view = base.ViewName()
	}
	var original *sightmap.ComponentNode
	if numeric {
		n, err := Resolve(base, target)
		if err != nil {
			return tapped{}, err
		}
		original = n
	}
	locate := func(snap *Snapshot) (*sightmap.ComponentNode, error) {
		if numeric {
			return Relocate(snap, original, target)
		}
		return Resolve(snap, target)
	}
	deadline := time.Now().Add(settle)
	retries := 0
	var prev *Snapshot
	var prevNode *sightmap.ComponentNode
	snap, err := s.fresh(ctx, view)
	if err != nil {
		return tapped{}, err
	}
	for {
		node, failure := locate(snap)
		if node != nil {
			s.lastTarget = node
		}
		if failure == nil {
			failure = snap.AssertActionable(node, target)
		}
		if failure != nil && !retryable[Code(failure)] {
			return tapped{}, failure
		}
		// A screen mid-transition can already show the target; a tap then
		// lands on whatever is passing under that point. Tap only once two
		// reads in a row agree on where the target and the screen are.
		stable := node != nil && (snap.Settled || prev != nil && prevNode != nil && sameElement(prevNode, node) &&
			*prevNode.Bounds == *node.Bounds && topLayout(prev) == topLayout(snap))
		outOfTime := time.Now().After(deadline)
		if failure == nil && stable {
			x, y := Center(node)
			if err := s.input(ctx, "tap", map[string]any{"x": x, "y": y}); err != nil {
				return tapped{}, err
			}
			s.secureFocus = node.Role == "securetextfield"
			return tapped{snap: snap, node: node, x: x, y: y, retries: retries}, nil
		}
		if outOfTime && failure == nil && prev != nil {
			return tapped{}, withHint("unstable", "retry, or wait longer with `--settle 6s`; `sightmap device snapshot` shows where the screen is",
				"%s was found, but the screen kept moving for %s, so it was not tapped", snap.Describe(node), settle)
		}
		waitable := failure != nil && (Code(failure) == "not_found" || Code(failure) == "stale_target" || !stable)
		if failure != nil && (outOfTime || !waitable) {
			return tapped{}, s.notFound(ctx, snap, target, failure, numeric)
		}
		time.Sleep(s.Timing.RetryInterval)
		next, err := s.fresh(ctx, view)
		if err != nil {
			return tapped{}, err
		}
		retries++
		prev, prevNode, snap = snap, node, next
	}
}

var exactText = regexp.MustCompile(`\[text="([^"]*)"\]`)

// exactTextHint reminds that [text="…"] matches a whole label, for a target
// that found nothing.
func exactTextHint(target string) string {
	m := exactText.FindStringSubmatch(target)
	if m == nil {
		return ""
	}
	return fmt.Sprintf("; [text=%q] matches the whole label, so use [text*=%q] for part of one", m[1], m[1])
}

// sameElement reports whether a and b, from two reads, are the same element.
func sameElement(a, b *sightmap.ComponentNode) bool {
	return a.Role == b.Role && Identifier(a) == Identifier(b) && a.Name == b.Name
}

// topLayout fingerprints the frames of the root's children: what moves when a
// screen slides in or a sheet animates, and not text such as a clock.
func topLayout(s *Snapshot) string {
	var b strings.Builder
	for _, n := range s.Root.Children {
		fmt.Fprintf(&b, "%s|%s|%d,%d,%d,%d\n", n.Role, Identifier(n), n.Bounds.X, n.Bounds.Y, n.Bounds.Width, n.Bounds.Height)
	}
	return b.String()
}

// sparseError is the not-found error for a tree that exposes next to nothing,
// naming its likely causes; nil when the tree looks healthy.
func (s *Session) sparseError(ctx context.Context, snap *Snapshot, message string) error {
	note := snap.sparseNote(s.UDID, s.accessibilityOn(ctx))
	if note == nil {
		return nil
	}
	return withHint("not_found", "check the causes listed, in order, then snapshot again",
		"%s; the screen exposes next to nothing to accessibility, so the target is not just further down a list:\n%s", message, strings.Join(note, "\n"))
}

// overlayError explains that a system sheet from another process is in front
// of the app, so nothing of the app's can be found or scrolled.
func overlayError(snap *Snapshot, message string) error {
	return withHint("covered", "dismiss it with one of its buttons, e.g. `sightmap device click 'button[text=\"Not Now\"]'`, then retry",
		"%s: a system sheet from another process (%s) is in front of the app", message, quote(snap.Root.Name))
}

func (s *Session) notFound(ctx context.Context, snap *Snapshot, target string, err error, numeric bool) error {
	if Code(err) != "not_found" {
		return err
	}
	if snap.Overlay {
		return overlayError(snap, err.Error())
	}
	if sparse := s.sparseError(ctx, snap, err.Error()); sparse != nil {
		return sparse
	}
	if numeric {
		return err
	}
	return withHint("not_found", fmt.Sprintf("if it is further down a list, run `sightmap device scroll down --until %s`; otherwise `sightmap device snapshot`%s", quote(target), exactTextHint(target)),
		"%s; long lists expose only the rows near the screen", err.Error())
}

func (s *Session) fill(ctx context.Context, a TargetArgs) (map[string]any, error) {
	settle, err := a.settle(s.Timing.RetryTimeout)
	if err != nil {
		return nil, err
	}
	t, err := s.tap(ctx, a.Target, settle)
	if err != nil {
		return nil, err
	}
	time.Sleep(s.Timing.Settle)
	relocate := func() (*Snapshot, *sightmap.ComponentNode, error) {
		snap, err := s.fresh(ctx, "")
		if err != nil {
			return nil, nil, err
		}
		n, err := Relocate(snap, t.node, a.Target)
		return snap, n, err
	}
	placeholder := ""
	if a.Clear {
		_, field, err := relocate()
		if err != nil {
			return nil, err
		}
		count := len([]rune(field.Value))
		// The cursor can be anywhere in the field: delete forward, then back.
		for _, key := range []string{"forward_delete", "delete"} {
			for range count {
				if err := s.input(ctx, "key", map[string]any{"key": key}); err != nil {
					return nil, err
				}
			}
		}
		if count > 0 {
			time.Sleep(s.Timing.Settle)
		}
		// Every character is gone by now; a field still showing text shows its placeholder.
		_, field, err = relocate()
		if err != nil {
			return nil, err
		}
		placeholder = field.Value
	}
	if a.Text != "" {
		if err := s.input(ctx, "type", map[string]any{"text": a.Text}); err != nil {
			return nil, err
		}
	}
	secure := t.node.Role == "securetextfield"
	took := func(n *sightmap.ComponentNode) bool {
		return secure || a.Text == "" || strings.Contains(strings.ToLower(n.Value), strings.ToLower(a.Text))
	}
	var snap *Snapshot
	var after *sightmap.ComponentNode
	deadline := time.Now().Add(s.Timing.FillReadback)
	for {
		time.Sleep(s.Timing.Settle)
		if snap, after, err = relocate(); err != nil {
			return nil, err
		}
		if took(after) || time.Now().After(deadline) {
			break
		}
	}
	showsPlaceholder := placeholder != "" && after.Value == placeholder && a.Text == ""
	value := after.Value
	if showsPlaceholder {
		value = ""
	}
	if !took(after) {
		return nil, withHint("fill_mismatch", "retry with --clear, or check with `sightmap device snapshot` that the field took focus",
			"typed %s but %s shows %s", quote(a.Text), snap.Describe(after), quote(value))
	}
	if secure {
		value = fmt.Sprintf("(%d secure characters)", len([]rune(value)))
	}
	r := t.result()
	r["value"] = value
	if showsPlaceholder {
		r["placeholder"] = placeholder
	}
	return r, nil
}

// scrollSpeed is in points per second. Faster swipes fling the list, which can
// carry a target past the screen between two reads; this speed stops where the
// finger stops.
const scrollSpeed = 300

func scrollVector(direction string, r rect) (map[string]any, error) {
	x, y, w, h := float64(r.x), float64(r.y), float64(r.w), float64(r.h)
	cx, cy := x+w/2, y+h/2
	var x1, y1, x2, y2 float64
	// A swipe that starts near the top lands on a header or navigation bar, or
	// reads as a system gesture, and moves nothing; both directions keep their
	// start and end in the region's middle band.
	switch direction {
	case "down":
		x1, y1, x2, y2 = cx, y+h*0.75, cx, y+h*0.25
	case "up":
		x1, y1, x2, y2 = cx, y+h*0.35, cx, y+h*0.85
	case "right":
		x1, y1, x2, y2 = x+w*0.8, cy, x+w*0.2, cy
	case "left":
		x1, y1, x2, y2 = x+w*0.2, cy, x+w*0.8, cy
	default:
		return nil, errorf("usage", "scroll direction must be up, down, left or right, not %q", direction)
	}
	duration := math.Max(0.3, math.Round(math.Hypot(x2-x1, y2-y1)/scrollSpeed*100)/100)
	return map[string]any{"x1": x1, "y1": y1, "x2": x2, "y2": y2, "duration": duration}, nil
}

// inMiddleBand reports whether n's center is in the middle 60% of the screen
// along the scroll axis, clear of the bars at either edge.
func (t *Tree) inMiddleBand(n *sightmap.ComponentNode, direction string) bool {
	cx, cy := Center(n)
	if direction == "left" || direction == "right" {
		return float64(cx) >= 0.2*t.Screen.W && float64(cx) <= 0.8*t.Screen.W
	}
	return float64(cy) >= max(0.2*t.Screen.H, float64(t.topBar(n))) && float64(cy) <= 0.8*t.Screen.H
}

// nudge is a slow swipe, centered on the screen's middle, that moves n's
// center to the middle along the scroll axis.
func (t *Tree) nudge(n *sightmap.ComponentNode, direction string) map[string]any {
	cx, cy := Center(n)
	mx, my := t.Screen.W/2, t.Screen.H/2
	dx, dy := 0.0, my-float64(cy)
	if direction == "left" || direction == "right" {
		dx, dy = mx-float64(cx), 0
	}
	x1, y1 := mx-dx/2, my-dy/2
	return map[string]any{"x1": x1, "y1": y1, "x2": x1 + dx, "y2": y1 + dy,
		"duration": math.Max(0.3, math.Round(math.Hypot(dx, dy)/scrollSpeed*100)/100)}
}

var opposite = map[string]string{"down": "up", "up": "down", "left": "right", "right": "left"}

func (s *Session) scroll(ctx context.Context, a ScrollArgs) (map[string]any, error) {
	if a.Max == 0 {
		a.Max = 20
	}
	snap, err := s.fresh(ctx, "")
	if err != nil {
		return nil, err
	}
	region := snap.scrollRegion()
	if a.In != "" {
		container, err := Resolve(snap, a.In)
		if err != nil {
			return nil, err
		}
		s.lastTarget = container
		region = clipRect(rectOf(container), rect{0, 0, int(snap.Screen.W), int(snap.Screen.H)})
	}
	vector, err := scrollVector(a.Direction, region)
	if err != nil {
		return nil, err
	}
	swipe := func() error {
		args := map[string]any{}
		for k, v := range vector {
			args[k] = v
		}
		return s.input(ctx, "swipe", args)
	}
	if a.Until == "" {
		if err := swipe(); err != nil {
			return nil, err
		}
		time.Sleep(s.Timing.ScrollSettle)
		_, err := s.settled(ctx, 4*time.Second)
		s.last = nil
		return map[string]any{"direction": a.Direction, "scrolls": 1}, err
	}
	notFound := fmt.Sprintf("%s not found", quote(a.Until))
	previous, stalls, nudges := "", 0, 0
	for i := 0; i <= a.Max; i++ {
		if i > 0 {
			if snap, err = s.settled(ctx, 4*time.Second); err != nil {
				return nil, err
			}
		}
		found, ferr := Resolve(snap, a.Until)
		matches := 1
		// --until asks for the target on screen; when it matches several
		// elements, the first one that can be acted on ends the scroll.
		var amb *ambiguousError
		if errors.As(ferr, &amb) {
			matches, found, ferr = len(amb.nodes), amb.nodes[0], nil
			for _, n := range amb.nodes {
				if snap.AssertActionable(n, a.Until) == nil {
					found = n
					break
				}
			}
		}
		if ferr == nil {
			ferr = snap.AssertActionable(found, a.Until)
		}
		// A target that is on screen but near an edge is under a bar (or will
		// be once the list settles); a short swipe brings it to the middle,
		// where a full page could carry it off the screen again.
		if found != nil && (ferr == nil || Code(ferr) == "covered") && snap.OnScreen(found) && !snap.inMiddleBand(found, a.Direction) && nudges < 3 && i < a.Max {
			nudges++
			if err := s.input(ctx, "swipe", snap.nudge(found, a.Direction)); err != nil {
				return nil, err
			}
			time.Sleep(s.Timing.ScrollSettle)
			continue
		}
		if ferr == nil {
			s.lastTarget, s.last = found, snap
			r := map[string]any{"direction": a.Direction, "scrolls": i, "found": snap.Describe(found), "id": found.Id}
			if matches > 1 {
				r["matches"] = matches
			}
			return r, nil
		}
		if c := Code(ferr); c != "not_found" && c != "offscreen" && c != "covered" {
			return nil, ferr
		}
		if Code(ferr) == "offscreen" {
			if want := snap.towards(found); want != a.Direction {
				return nil, withHint("offscreen", fmt.Sprintf("run `sightmap device scroll %s --until %s`", want, quote(a.Until)),
					"%s is %s the screen; scrolling %s moves away from it", snap.Describe(found), map[string]string{"up": "above", "down": "below", "left": "left of", "right": "right of"}[want], a.Direction)
			}
		}
		if snap.Overlay {
			return nil, overlayError(snap, notFound)
		}
		if i == 0 {
			if sparse := s.sparseError(ctx, snap, notFound); sparse != nil {
				return nil, sparse
			}
		}
		// One swipe can land while the screen is still settling and move
		// nothing; two in a row that move nothing mean the end of the list.
		text := layout(snap, true)
		if text == previous {
			stalls++
		} else {
			stalls = 0
		}
		if stalls >= 2 {
			s.last = snap
			return nil, withHint("not_found", fmt.Sprintf("try `sightmap device scroll %s --until %s`%s", opposite[a.Direction], quote(a.Until), exactTextHint(a.Until)),
				"%s; reached the end after scrolling %s %d time(s)", notFound, a.Direction, i)
		}
		previous = text
		if i == a.Max {
			break
		}
		if err := swipe(); err != nil {
			return nil, err
		}
		time.Sleep(s.Timing.ScrollSettle)
	}
	s.last = snap
	return nil, withHint("not_found", "raise --max, or check the target with `sightmap device snapshot`"+exactTextHint(a.Until), "%s after %d scroll(s) %s", notFound, a.Max, a.Direction)
}

// Close stops the helper and the log and network collectors.
func (s *Session) Close() {
	s.Helper.Close()
	s.closeCollectors()
}

func (s *Session) closeCollectors() {
	if s.Logs != nil {
		s.Logs.Close()
	}
	if s.Net != nil {
		s.Net.Close()
	}
}

// networkSupported refuses network commands where no source can run.
func (s *Session) networkSupported() error {
	if s.Platform == PlatformAndroid {
		return withHint("unsupported", "on Android, read the app's requests from its analytics session or a proxy for now", "network capture is not available on Android yet")
	}
	return nil
}
