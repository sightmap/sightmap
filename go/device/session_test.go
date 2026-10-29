package device

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeHelper answers describe with a recorded tree and records input.
type fakeHelper struct {
	describe *Describe
	// frames, when set, replaces describe: one tree per read, the last repeating.
	frames []*Describe
	// scenes, when set, replaces describe: one tree per swipe so far, the last
	// repeating.
	scenes []*Describe
	swipes int
	inputs []string
}

func (f *fakeHelper) Request(_ context.Context, cmd string, args map[string]any, out any) error {
	switch cmd {
	case "describe":
		d := f.describe
		if len(f.scenes) > 0 {
			d = f.scenes[min(f.swipes, len(f.scenes)-1)]
		}
		if len(f.frames) > 0 {
			d = f.frames[0]
			if len(f.frames) > 1 {
				f.frames = f.frames[1:]
			}
		}
		data, _ := json.Marshal(d)
		return json.Unmarshal(data, out)
	case "hello":
		return nil
	}
	data, _ := json.Marshal(args)
	f.inputs = append(f.inputs, cmd+" "+string(data))
	if cmd == "swipe" {
		f.swipes++
	}
	return nil
}
func (f *fakeHelper) Alive() bool   { return true }
func (f *fakeHelper) Restarts() int { return 0 }
func (f *fakeHelper) Close()        {}

type fakeSim struct{ Simctl }

func (fakeSim) AccessibilityEnabled(context.Context, string) bool { return true }
func (fakeSim) EnableAccessibility(context.Context, string) error { return nil }
func (fakeSim) Launch(context.Context, string, string, LaunchOptions) (int, error) {
	return 42, nil
}

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

func TestRunLogRedactsSecrets(t *testing.T) {
	s, _ := newTestSession(t, "describe-simple")
	s.Helper.(*fakeHelper).describe = &Describe{Screen: Size{W: 400, H: 800}, Root: &RawElement{Type: "Application", Frame: Frame{W: 400, H: 800}, Children: []*RawElement{
		{Type: "SecureTextField", Identifier: "password", Frame: Frame{X: 10, Y: 100, W: 300, H: 40}},
		{Type: "TextField", Identifier: "email", Frame: Frame{X: 10, Y: 200, W: 300, H: 40}},
	}}}
	handle(t, s, "click", TargetArgs{Target: "password"})
	handle(t, s, "type", ValueArgs{Text: "typed-secret"})
	handle(t, s, "fill", TargetArgs{Target: "missing-field", Text: "unresolved-secret"})
	handle(t, s, "click", TargetArgs{Target: "email"})
	handle(t, s, "type", ValueArgs{Text: "a@example.com"})

	data, err := os.ReadFile(s.RunLog.Current().Path)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	for _, secret := range []string{"typed-secret", "unresolved-secret"} {
		if strings.Contains(log, secret) {
			t.Errorf("%s reached the run log:\n%s", secret, log)
		}
	}
	if !strings.Contains(log, "a@example.com") {
		t.Errorf("text typed into a plain field should be logged:\n%s", log)
	}
}

func TestConsolePagesForwardAndReportsDropped(t *testing.T) {
	c := NewConsole(5)
	for i := range 8 {
		c.Push(ConsoleEntry{Level: "stdout", Source: "stdout", Message: fmt.Sprintf("line %d", i)})
	}
	all := c.List(ConsoleFilter{})
	if all.Dropped != 3 || all.Last != 7 || all.Capacity != 5 || len(all.Entries) != 5 {
		t.Fatalf("list = %+v", all)
	}
	since := 1
	page := c.List(ConsoleFilter{Since: &since, Limit: 2})
	if page.Dropped != 1 || len(page.Entries) != 2 || page.Entries[0].Index != 3 || page.Last != 4 {
		t.Fatalf("page = %+v", page)
	}
	next := page.Last
	rest := c.List(ConsoleFilter{Since: &next})
	if rest.Dropped != 0 || len(rest.Entries) != 3 || rest.Last != 7 {
		t.Fatalf("rest = %+v", rest)
	}
	none := 7
	if empty := c.List(ConsoleFilter{Since: &none}); len(empty.Entries) != 0 || empty.Last != 7 {
		t.Fatalf("caught up = %+v", empty)
	}
}

func TestConsoleGrepCoversEverySource(t *testing.T) {
	c := NewConsole(10)
	c.Push(ConsoleEntry{Level: "info", Source: "os_log", Subsystem: "dev.example", Message: "Session started"})
	c.Push(ConsoleEntry{Level: "stdout", Source: "stdout", Message: "SESSION from print"})
	c.Push(ConsoleEntry{Level: "stderr", Source: "stderr", Message: "a session warning"})
	if got := c.List(ConsoleFilter{Grep: "session"}); got.Total != 3 {
		t.Errorf("grep matched %d of 3 sources: %+v", got.Total, got)
	}
}

func TestRunLogNotesAndRelaunch(t *testing.T) {
	s, _ := newTestSession(t, "describe-simple")
	s.Timing.ReadyTimeout = time.Millisecond
	if _, err := handle(t, s, "runNote", ValueArgs{Text: "recording started", Kind: "session-start"}); err != nil {
		t.Fatal(err)
	}
	if _, err := handle(t, s, "relaunch", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := handle(t, s, "runNote", ValueArgs{}); Code(err) != "usage" {
		t.Errorf("an empty note: %v", err)
	}
	entries, _ := ReadRun(s.RunLog.Current().Path, 0)
	if len(entries) < 2 {
		t.Fatalf("entries = %+v", entries)
	}
	note := entries[0]
	if note.Action != "note" || note.Kind != "session-start" || note.Text != "recording started" || note.Args != nil {
		t.Errorf("note = %+v", note)
	}
	if entries[1].Action != "relaunch" {
		t.Errorf("relaunch logged as %q", entries[1].Action)
	}
}

func TestClickSettleWaitsForAMissingTarget(t *testing.T) {
	s, _ := newTestSession(t, "describe-simple")
	s.Timing.RetryTimeout = time.Hour
	started := time.Now()
	if _, err := handle(t, s, "click", TargetArgs{Target: "not-on-screen", Settle: "200ms"}); Code(err) != "not_found" {
		t.Fatalf("err = %v", err)
	}
	if took := time.Since(started); took < 200*time.Millisecond || took > 5*time.Second {
		t.Errorf("--settle 200ms waited %s", took)
	}
	if _, err := handle(t, s, "click", TargetArgs{Target: "simple.primary", Settle: "soon"}); Code(err) != "usage" {
		t.Errorf("bad --settle: %v", err)
	}
}

// sliding is a screen sliding in from the right, with its row at x.
func sliding(x float64) *Describe {
	return &Describe{Screen: Size{W: 400, H: 800}, Root: &RawElement{Type: "Application", Frame: Frame{W: 400, H: 800}, Children: []*RawElement{
		{Type: "Other", Label: "List", Frame: Frame{X: x, Y: 0, W: 400, H: 800}, Children: []*RawElement{
			{Type: "Button", Identifier: "participant-row-1", Label: "Robert", Frame: Frame{X: x, Y: 100, W: 400, H: 60}},
		}},
	}}}
}

func TestClickWaitsForTheScreenToStopMoving(t *testing.T) {
	s, h := newTestSession(t, "describe-simple")
	s.Timing.RetryTimeout = time.Minute
	h.frames = []*Describe{sliding(300), sliding(200), sliding(100), sliding(0)}
	r, err := handle(t, s, "click", TargetArgs{Target: "participant-row-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.inputs) != 1 || !strings.Contains(h.inputs[0], `"x":200`) || r["retries"].(float64) < 3 {
		t.Fatalf("tapped %v (result %v), want one tap at the row's resting center x=200", h.inputs, r)
	}
}

func TestClickRefusesAScreenThatKeepsMoving(t *testing.T) {
	s, h := newTestSession(t, "describe-simple")
	s.Timing.RetryTimeout = 300 * time.Millisecond
	for i := range 5000 {
		h.frames = append(h.frames, sliding(float64(100+i%2)))
	}
	_, err := handle(t, s, "click", TargetArgs{Target: "participant-row-1"})
	if Code(err) != "unstable" || len(h.inputs) != 0 {
		t.Fatalf("err = %v, inputs %v", err, h.inputs)
	}
}

func TestScrollVectorsStayInTheMiddleBand(t *testing.T) {
	screen := rect{0, 0, 390, 844}
	for _, dir := range []string{"up", "down"} {
		v, err := scrollVector(dir, screen)
		if err != nil {
			t.Fatal(err)
		}
		y1, y2 := v["y1"].(float64), v["y2"].(float64)
		for _, y := range []float64{y1, y2} {
			if y < 0.25*844 || y > 0.85*844 {
				t.Errorf("%s swipes through y=%g, outside the middle band", dir, y)
			}
		}
		if (dir == "up") != (y2 > y1) {
			t.Errorf("%s swipes from %g to %g", dir, y1, y2)
		}
		if y1 < 0.3*844 && dir == "up" {
			t.Errorf("scroll up starts at y=%g, near the top where headers and system gestures are", y1)
		}
	}
}

func scheduleScreen(rowY float64) *Describe {
	return &Describe{Screen: Size{W: 390, H: 844}, Root: &RawElement{Type: "Application", Frame: Frame{W: 390, H: 844}, Children: []*RawElement{
		{Type: "Other", Label: "Header", Frame: Frame{X: 0, Y: 0, W: 390, H: 200}},
		{Type: "Table", Frame: Frame{X: 0, Y: 200, W: 390, H: 600}, Children: []*RawElement{
			{Type: "Button", Identifier: "masters-row", Label: "Masters Tournament", Frame: Frame{X: 0, Y: rowY, W: 390, H: 60}},
			{Type: "Button", Identifier: "other-row", Label: "Other", Frame: Frame{X: 0, Y: 400, W: 390, H: 60}},
		}},
	}}}
}

func TestScrollUpSwipesInsideTheListAndHintsTheRightWay(t *testing.T) {
	s, h := newTestSession(t, "describe-simple")
	h.describe = scheduleScreen(-1110)
	if _, err := handle(t, s, "scroll", ScrollArgs{Direction: "up"}); err != nil {
		t.Fatal(err)
	}
	var swipe struct{ Y1, Y2 float64 }
	json.Unmarshal([]byte(strings.TrimPrefix(h.inputs[0], "swipe ")), &swipe)
	if swipe.Y1 < 200 || swipe.Y2 > 800 || swipe.Y2 <= swipe.Y1 {
		t.Errorf("scroll up swiped %v, want upward within the table (y 200–800)", swipe)
	}

	_, err := handle(t, s, "click", TargetArgs{Target: "masters-row"})
	if Code(err) != "offscreen" || !strings.Contains(AsError(err).Hint, "scroll up --until") {
		t.Errorf("a row above the screen: %v (%s)", err, AsError(err).Hint)
	}
	h.inputs = nil
	_, err = handle(t, s, "scroll", ScrollArgs{Direction: "down", Until: "masters-row"})
	if Code(err) != "offscreen" || !strings.Contains(AsError(err).Hint, "scroll up --until") || len(h.inputs) != 0 {
		t.Errorf("scroll down toward a row above: %v (%s), inputs %v", err, AsError(err).Hint, h.inputs)
	}
}

// Scrolling up can leave a row with its center under the status bar, where a
// tap scrolls the list to the top instead: not tappable, so scroll on.
func TestScrollUpUntilClearsTheStatusBar(t *testing.T) {
	s, h := newTestSession(t, "describe-simple")
	h.scenes = []*Describe{scheduleScreen(-1110), scheduleScreen(-10), scheduleScreen(560)}

	snap, err := Annotate(Normalize(scheduleScreen(-10)), nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	row := byIdentifier(t, snap.Tree, "masters-row")
	if err := snap.AssertActionable(row, "masters-row"); Code(err) != "covered" || !strings.Contains(err.Error(), "status bar") || !strings.Contains(AsError(err).Hint, "scroll up --until") {
		t.Fatalf("a row centered at y=20: %v", err)
	}
	inList, _ := Annotate(Normalize(scheduleScreen(160)), nil, nil, "")
	if err := inList.AssertActionable(byIdentifier(t, inList.Tree, "masters-row"), "masters-row"); Code(err) != "covered" || !strings.Contains(err.Error(), "outside the visible part of its list") {
		t.Errorf("a row centered above its list's top edge: %v", err)
	}

	r, err := handle(t, s, "scroll", ScrollArgs{Direction: "up", Until: "masters-row"})
	if err != nil {
		t.Fatal(err)
	}
	if r["scrolls"].(float64) != 2 {
		t.Errorf("scroll up --until stopped after %v scroll(s), want 2: once past the status bar", r["scrolls"])
	}
}

// productList is SwiftUI's Products screen scrolled down: the collapsed
// navigation bar shows only as its title, outside the list of rows.
func productList(rowY float64) *Describe {
	rows := []*RawElement{{Type: "Button", Identifier: "product-row-30", Label: "Product 30", Frame: Frame{X: 16, Y: rowY, W: 358, H: 52}}}
	for i, y := range []float64{500, 552, 604} {
		rows = append(rows, &RawElement{Type: "Button", Identifier: fmt.Sprintf("product-row-%d", 40+i), Label: "Other", Frame: Frame{X: 16, Y: y, W: 358, H: 52}})
	}
	return &Describe{Screen: Size{W: 390, H: 844}, Root: &RawElement{Type: "Application", Frame: Frame{W: 390, H: 844}, Children: []*RawElement{
		{Type: "Heading", Label: "Products", Frame: Frame{X: 159, Y: 59, W: 72, H: 21}},
		{Type: "Group", Frame: Frame{W: 390, H: 844}, Children: rows},
	}}}
}

func TestRowsUnderACollapsedNavigationBarAreCovered(t *testing.T) {
	under, _ := Annotate(Normalize(productList(45)), nil, nil, "")
	if err := under.AssertActionable(byIdentifier(t, under.Tree, "product-row-30"), "product-row-30"); Code(err) != "covered" || !strings.Contains(err.Error(), "navigation bar") {
		t.Fatalf("a row centered at y=71 under the bar: %v", err)
	}
	clear, _ := Annotate(Normalize(productList(97)), nil, nil, "")
	if err := clear.AssertActionable(byIdentifier(t, clear.Tree, "product-row-30"), "product-row-30"); err != nil {
		t.Errorf("a row just below the bar: %v", err)
	}
}

// scroll --until finishes with the target in the middle of the screen: one on
// screen but under the navigation bar is nudged down rather than paged past.
func TestScrollUntilNudgesTheTargetToTheMiddle(t *testing.T) {
	s, h := newTestSession(t, "describe-simple")
	h.scenes = []*Describe{productList(45), productList(396)}
	r, err := handle(t, s, "scroll", ScrollArgs{Direction: "up", Until: "product-row-30"})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.inputs) != 1 {
		t.Fatalf("swipes %v, want one nudge", h.inputs)
	}
	var nudge struct{ Y1, Y2 float64 }
	json.Unmarshal([]byte(strings.TrimPrefix(h.inputs[0], "swipe ")), &nudge)
	if moved := nudge.Y2 - nudge.Y1; moved < 340 || moved > 360 {
		t.Errorf("nudge moved %g pt, want the row's center (71) to the middle (422)", moved)
	}
	if !strings.Contains(r["found"].(string), "product-row-30") && !strings.Contains(r["found"].(string), "Product 30") {
		t.Errorf("found %v", r["found"])
	}
}

func TestNavigationBarItemsAreNotCoveredByTheirOwnBar(t *testing.T) {
	d := &Describe{Screen: Size{W: 390, H: 844}, Root: &RawElement{Type: "Application", Frame: Frame{W: 390, H: 844}, Children: []*RawElement{
		{Type: "Button", Identifier: "BackButton", Label: "Products", Frame: Frame{X: 8, Y: 52, W: 100, H: 34}},
		{Type: "Heading", Label: "Product 30", Frame: Frame{X: 150, Y: 59, W: 90, H: 21}},
		{Type: "Button", Identifier: "product-detail-add", Label: "Add to Cart", Frame: Frame{X: 120, Y: 460, W: 150, H: 34}},
	}}}
	s, _ := Annotate(Normalize(d), nil, nil, "")
	if err := s.AssertActionable(byIdentifier(t, s.Tree, "BackButton"), "BackButton"); err != nil {
		t.Errorf("back button: %v", err)
	}
}
