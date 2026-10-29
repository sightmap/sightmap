package device

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeHelper answers describe with a recorded tree and records input.
type fakeHelper struct {
	describe *Describe
	inputs   []string
}

func (f *fakeHelper) Request(_ context.Context, cmd string, args map[string]any, out any) error {
	switch cmd {
	case "describe":
		data, _ := json.Marshal(f.describe)
		return json.Unmarshal(data, out)
	case "hello":
		return nil
	}
	data, _ := json.Marshal(args)
	f.inputs = append(f.inputs, cmd+" "+string(data))
	return nil
}
func (f *fakeHelper) Alive() bool   { return true }
func (f *fakeHelper) Restarts() int { return 0 }
func (f *fakeHelper) Close()        {}

type fakeSim struct{ Simctl }

func (fakeSim) AccessibilityEnabled(context.Context, string) bool { return true }

func newTestSession(t *testing.T, describe string) (*Session, *fakeHelper) {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("testdata", "corpus"))); err != nil {
		t.Fatal(err)
	}
	h := &fakeHelper{describe: loadDescribe(t, describe)}
	s := NewSession(Config{UDID: "UDID", BundleID: "dev.example", SightmapDir: dir}, h, fakeSim{})
	s.Timing = Timing{RetryTimeout: time.Millisecond, RetryInterval: time.Millisecond, Settle: time.Millisecond}.withDefaults()
	return s, h
}

func handle(t *testing.T, s *Session, cmd string, args any) (map[string]any, error) {
	t.Helper()
	raw, _ := json.Marshal(args)
	result, err := s.Handle(context.Background(), cmd, raw)
	var out map[string]any
	data, _ := json.Marshal(result)
	json.Unmarshal(data, &out)
	return out, err
}

func TestSessionClickTapsCenterAndLogsTheStep(t *testing.T) {
	s, h := newTestSession(t, "describe-simple")
	r, err := handle(t, s, "click", TargetArgs{Target: "PrimaryButton"})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.inputs) != 1 || !strings.HasPrefix(h.inputs[0], "tap ") || !strings.Contains(r["target"].(string), "[PrimaryButton") {
		t.Fatalf("inputs %v, result %v", h.inputs, r)
	}
	entries, err := ReadRun(s.RunLog.Current().Path, 0)
	if err != nil || len(entries) != 1 {
		t.Fatalf("run log = %v, %v", entries, err)
	}
	e := entries[0]
	if e.Action != "click" || !e.OK || e.View != "Simple" || e.Target == nil || e.Target.Component != "PrimaryButton" || e.Target.Identifier != "simple.primary" {
		t.Errorf("entry = %+v target %+v", e, e.Target)
	}
}

func TestSessionRefusesOffscreenAndLogsTheFailure(t *testing.T) {
	s, h := newTestSession(t, "describe-heavy")
	snap, err := handle(t, s, "snapshot", SnapshotArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snap["text"].(string), "[Off-screen]") {
		t.Fatalf("snapshot lacks [Off-screen]:\n%s", snap["text"])
	}
	var offscreen string
	for _, n := range s.last.Nodes {
		if n.IsInteractive && !n.IsVisible && Identifier(n) != "" {
			offscreen = Identifier(n)
			break
		}
	}
	_, err = handle(t, s, "click", TargetArgs{Target: offscreen})
	if Code(err) != "offscreen" || len(h.inputs) != 0 {
		t.Fatalf("err = %v, inputs %v", err, h.inputs)
	}
	entries, _ := ReadRun(s.RunLog.Current().Path, 1)
	if len(entries) != 1 || entries[0].OK || entries[0].Error.Code != "offscreen" {
		t.Errorf("entries = %+v", entries)
	}
}

func TestSessionRejectsOffscreenPoints(t *testing.T) {
	s, h := newTestSession(t, "describe-simple")
	_, err := handle(t, s, "tapXY", PointArgs{X: 100, Y: 5000})
	if Code(err) != "off_screen_point" || !strings.Contains(err.Error(), "not pixels") || len(h.inputs) != 0 {
		t.Fatalf("err = %v, inputs %v", err, h.inputs)
	}
}

func TestSessionReloadsAnEditedCorpus(t *testing.T) {
	s, _ := newTestSession(t, "describe-simple")
	if _, err := handle(t, s, "click", TargetArgs{Target: "Greeting"}); Code(err) != "not_found" {
		t.Fatalf("err = %v", err)
	}
	extra := "version: 1\ncomponents:\n  - name: Greeting\n    selector: 'image[identifier=\"2.circle.fill\"]'\n"
	if err := os.WriteFile(filepath.Join(s.SightmapDir, "extra.yaml"), []byte(extra), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := handle(t, s, "click", TargetArgs{Target: "Greeting"}); err != nil {
		t.Fatalf("after the edit: %v", err)
	}
}

func TestConsoleParseAndFilter(t *testing.T) {
	c := NewConsole(3)
	for _, line := range []string{
		`{"eventType":"logEvent","eventMessage":"signed in as a@example.com","messageType":"Info","subsystem":"dev.example","category":"app","timestamp":"t1"}`,
		`{"eventType":"logEvent","eventMessage":"noise","messageType":"Error","subsystem":"com.apple.UIKit"}`,
		`{"eventType":"activityCreateEvent"}`,
		`not json`,
	} {
		if e, ok := parseLogLine(line); ok {
			c.Push(e)
		}
	}
	c.Push(ConsoleEntry{Level: "stdout", Source: "stdout", Message: "added Product 1 to cart"})
	c.Push(ConsoleEntry{Level: "stderr", Source: "stderr", Message: "warning"})
	if got := c.List(ConsoleFilter{}); got.Total != 2 {
		t.Errorf("Apple frameworks and the evicted entry are out: %+v", got)
	}
	if got := c.List(ConsoleFilter{All: true}); got.Total != 3 {
		t.Errorf("--all: %+v", got)
	}
	if got := c.List(ConsoleFilter{Grep: "PRODUCT"}); got.Total != 1 || got.Entries[0].Index != 2 {
		t.Errorf("grep: %+v", got)
	}
	if got := c.List(ConsoleFilter{Level: "stderr,error", All: true}); got.Total != 2 {
		t.Errorf("level: %+v", got)
	}
	if _, ok := c.Get(0); ok {
		t.Error("entry 0 should have been evicted")
	}
}

func TestRunLogLabelsAndReadsSince(t *testing.T) {
	r := NewRunLog(t.TempDir())
	info, err := r.Begin("Checkout flow!")
	if err != nil || !strings.HasSuffix(info.Run, "-checkout-flow") {
		t.Fatalf("run = %q, %v", info.Run, err)
	}
	for _, a := range []string{"start", "click", "fill"} {
		r.Append(RunEntry{Action: a, OK: true})
	}
	entries, _ := ReadRun(info.Path, 1)
	if len(entries) != 2 || entries[0].Action != "click" || entries[1].Seq != 3 {
		t.Errorf("entries = %+v", entries)
	}
}

func TestPickDevice(t *testing.T) {
	devices := []Device{
		{UDID: "A", Name: "iPhone 17", RuntimeVersion: "26.0", State: "Shutdown"},
		{UDID: "B", Name: "iPhone 17", RuntimeVersion: "26.2", State: "Shutdown"},
		{UDID: "C", Name: "iPad Air", RuntimeVersion: "26.2", State: "Shutdown"},
	}
	if d, _ := PickDevice(devices, ""); d.UDID != "B" {
		t.Errorf("default = %s, want the iPhone on the newest runtime", d.UDID)
	}
	if d, _ := PickDevice(devices, "iPhone 17"); d.UDID != "B" {
		t.Errorf("by name = %s", d.UDID)
	}
	if d, _ := PickDevice(devices, "c"); d.UDID != "C" {
		t.Errorf("by UDID = %s", d.UDID)
	}
	devices[0].State = "Booted"
	if d, _ := PickDevice(devices, ""); d.UDID != "A" {
		t.Errorf("a booted device wins: %s", d.UDID)
	}
	if _, err := PickDevice(devices, "Pixel"); Code(err) != "device_not_found" {
		t.Errorf("err = %v", err)
	}
}
