package device

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sightmap/sightmap/go/sightmap"
)

var pixel = AndroidScreen{WidthPx: 1080, HeightPx: 2400, Density: 420}

func loadUIAutomator(t *testing.T, name, pkg string, screen AndroidScreen) *Describe {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".xml"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := ParseUIAutomator(data, pkg, screen)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestParseUIAutomatorViews(t *testing.T) {
	d := loadUIAutomator(t, "uiautomator-views", "dev.sightmap.sample", pixel)
	if d.Overlay || d.Screen.W != 411.43 || d.Screen.H != 914.29 {
		t.Fatalf("screen %+v overlay %v; want 411.43x914.29 dp", d.Screen, d.Overlay)
	}
	tree := Normalize(d)
	if tree.Root.Role != "application" {
		t.Errorf("root role = %s", tree.Root.Role)
	}
	email := byIdentifier(t, tree, "login_email")
	if email.Role != "textfield" || email.Name != "Email" || !email.IsInteractive {
		t.Errorf("email = %s %q interactive=%v", email.Role, email.Name, email.IsInteractive)
	}
	if pw := byIdentifier(t, tree, "login_password"); pw.Role != "securetextfield" || pw.Name != "Password" {
		t.Errorf("password = %s %q", pw.Role, pw.Name)
	}
	if remember := byIdentifier(t, tree, "remember"); remember.Role != "checkbox" || remember.State["checked"] != "true" || remember.Name != "Remember me" {
		t.Errorf("remember = %s %q %v", remember.Role, remember.Name, remember.State)
	}
	submit := byIdentifier(t, tree, "login_submit")
	if submit.Role != "button" || submit.Name != "Sign in" || *submit.Bounds != (sightmap.Bounds{X: 24, Y: 328, Width: 363, Height: 48}) {
		t.Errorf("submit = %s %q %+v; want bounds in dp", submit.Role, submit.Name, *submit.Bounds)
	}
	if forgot := byIdentifier(t, tree, "forgot"); Enabled(forgot) || forgot.Element.Attrs["disabled"] != "" {
		t.Errorf("forgot is disabled: state %v attrs %v", forgot.State, forgot.Element.Attrs)
	}
	if list := byIdentifier(t, tree, "recent_list"); list.Role != "collection" {
		t.Errorf("RecyclerView role = %s", list.Role)
	}
	row := byIdentifier(t, tree, "recent_row")
	if row.Role != "other" || !row.IsInteractive || row.Name != "Product 1, $6.37" {
		t.Errorf("a clickable row is an interactive other named by its text, got %s %q interactive=%v", row.Role, row.Name, row.IsInteractive)
	}
	home := byIdentifier(t, tree, "nav_home")
	if home.Name != "Home" || home.State["selected"] != "true" || !home.IsInteractive {
		t.Errorf("nav item = %q %v interactive=%v", home.Name, home.State, home.IsInteractive)
	}
	for _, n := range tree.Nodes {
		if id := Identifier(n); id == "content" || id == "statusBarBackground" || strings.Contains(id, ":id/") {
			t.Errorf("identifier %q should be dropped or unqualified", id)
		}
	}
}

func TestParseUIAutomatorCompose(t *testing.T) {
	d := loadUIAutomator(t, "uiautomator-compose", "dev.sightmap.compose", pixel)
	tree := Normalize(d)
	search := byIdentifier(t, tree, "search")
	if search.Role != "textfield" || search.Name != "Search products" || search.Value != "" {
		t.Errorf("search = %s %q value %q", search.Role, search.Name, search.Value)
	}
	if list := byIdentifier(t, tree, "product_list"); list.Role != "scrollview" {
		t.Errorf("a scrollable Compose View is a scrollview, got %s", list.Role)
	}
	row := byIdentifier(t, tree, "product_row_1")
	if !row.IsInteractive || row.Element.Attrs["testid"] != "product_row_1" || row.Element.Id != "product_row_1" {
		t.Errorf("row = interactive %v attrs %v id %q", row.IsInteractive, row.Element.Attrs, row.Element.Id)
	}
	if sw := byIdentifier(t, tree, "notifications"); sw.Role != "switch" || sw.State["checked"] != "false" || sw.Name != "Notifications" {
		t.Errorf("switch = %s %q %v", sw.Role, sw.Name, sw.State)
	}
	if add := byIdentifier(t, tree, "add_to_cart"); add.Role != "button" || add.Name != "Add to cart" {
		t.Errorf("a Compose Button takes its Text child's text, got %s %q", add.Role, add.Name)
	}
}

func TestAnnotateAndroidInfersViews(t *testing.T) {
	corpus := loadCorpus(t, "android-corpus")
	snap, err := Annotate(Normalize(loadUIAutomator(t, "uiautomator-views", "dev.sightmap.sample", pixel)), corpus, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if snap.ViewName() != "Login" || !snap.ViewInferred {
		t.Fatalf("view = %q inferred %v", snap.ViewName(), snap.ViewInferred)
	}
	n, err := Resolve(snap, "SignInButton")
	if err != nil || Identifier(n) != "login_submit" {
		t.Fatalf("SignInButton = %v, %v", n, err)
	}
	if _, err := Resolve(snap, `RecentRow[title^="Product 2"]`); err != nil {
		t.Errorf("RecentRow by title: %v", err)
	}

	snap, err = Annotate(Normalize(loadUIAutomator(t, "uiautomator-compose", "dev.sightmap.compose", pixel)), corpus, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if snap.ViewName() != "Products" {
		t.Fatalf("view = %q", snap.ViewName())
	}
	if n, err := Resolve(snap, `ProductRow[title^="Product 2"]`); err != nil || Identifier(n) != "product_row_2" {
		t.Errorf("ProductRow = %v, %v", n, err)
	}
}

func TestParseUIAutomatorForeignWindowIsAnOverlay(t *testing.T) {
	d := loadUIAutomator(t, "uiautomator-permission", "dev.sightmap.sample", pixel)
	if !d.Overlay || d.Root.Label != "com.google.android.permissioncontroller" {
		t.Fatalf("overlay %v root %q", d.Overlay, d.Root.Label)
	}
	snap, err := Annotate(Normalize(d), nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := Resolve(snap, `button[text="Allow"]`); err != nil || Identifier(n) != "permission_allow_button" {
		t.Errorf("Allow = %v, %v", n, err)
	}
}

func TestParseUIAutomatorErrorsAndRotation(t *testing.T) {
	_, err := ParseUIAutomator([]byte("ERROR: could not get idle state.\n"), "x", pixel)
	if Code(err) != "ax_unavailable" {
		t.Errorf("idle-state error = %v (%s)", err, Code(err))
	}
	_, err = ParseUIAutomator([]byte("ERROR: UiAutomationService android.accessibilityservice.IAccessibilityServiceClient@1 already registered!\n"), "x", pixel)
	if Code(err) != "ax_error" || !strings.Contains(AsError(err).Hint, "Appium") {
		t.Errorf("already-registered error = %+v", AsError(err))
	}
	d, err := ParseUIAutomator([]byte(`<hierarchy rotation="1"><node class="android.widget.Button" package="x" text="Go" clickable="true" enabled="true" bounds="[0,0][420,210]"/></hierarchy>`), "x", pixel)
	if err != nil {
		t.Fatal(err)
	}
	if d.Screen.W != 914.29 || d.Screen.H != 411.43 {
		t.Errorf("landscape screen = %+v", d.Screen)
	}
	if b := d.Root.Children[0].Frame; b.W != 160 || b.H != 80 {
		t.Errorf("frame = %+v, want 160x80 dp", b)
	}
}

func TestParseWMAndIME(t *testing.T) {
	s := parseWM("Physical size: 1080x2400\nOverride size: 720x1600\n", "Physical density: 420\n")
	if s.WidthPx != 720 || s.HeightPx != 1600 || s.Density != 420 {
		t.Errorf("wm = %+v", s)
	}
	ime := `WINDOW MANAGER WINDOWS (dumpsys window windows)
  Window #2 Window{6c1f2a u0 InputMethod}:
    mDisplayId=0 rootTaskId=1 mSession=Session{8a1 1234:u0a10115} mClient=android.os.BinderProxy@1
    mOwnerUid=10115 showForAllUsers=true package=com.google.android.inputmethod.latin appop=NONE
    mHasSurface=true isReadyForDisplay()=true mWindowRemovalAllowed=false
    Frames: parent=[0,0][1080,2400] display=[0,0][1080,2400] frame=[0,1527][1080,2400] last=[0,1527][1080,2400] insetsChanged=false
    isOnScreen=true isVisible=true
`
	k := parseIMEWindow(ime)
	if k == nil || *k != [4]int{0, 1527, 1080, 2400} {
		t.Fatalf("ime frame = %v", k)
	}
	if parseIMEWindow(strings.ReplaceAll(ime, "isVisible=true", "isVisible=false")) != nil {
		t.Error("a hidden input method is no keyboard")
	}
	screen := pixel
	screen.Keyboard = k
	d := loadUIAutomator(t, "uiautomator-compose", "dev.sightmap.compose", screen)
	snap, err := Annotate(Normalize(d), nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	add, err := Resolve(snap, "add_to_cart")
	if err != nil {
		t.Fatal(err)
	}
	if err := snap.AssertActionable(add, "add_to_cart"); Code(err) != "covered" {
		t.Errorf("a button under the keyboard is covered, got %v", err)
	}
}

// fakeADB answers adb commands from a table of command prefixes and records
// every command.
type fakeADB struct {
	answers map[string]string
	calls   []string
}

func (f *fakeADB) run(_ context.Context, args ...string) (string, error) {
	cmd := strings.Join(args, " ")
	f.calls = append(f.calls, cmd)
	best := ""
	for prefix := range f.answers {
		if strings.Contains(cmd, prefix) && len(prefix) > len(best) {
			best = prefix
		}
	}
	return f.answers[best], nil
}

func (f *fakeADB) called(substr string) string {
	for _, c := range f.calls {
		if strings.Contains(c, substr) {
			return c
		}
	}
	return ""
}

func newFakeADB(t *testing.T, dump string) *fakeADB {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", dump+".xml"))
	if err != nil {
		t.Fatal(err)
	}
	return &fakeADB{answers: map[string]string{
		"wm size":          "Physical size: 1080x2400\n",
		"wm density":       "Physical density: 420\n",
		"uiautomator":      string(data) + "\n" + imeMarker + "\n",
		"version":          "Android Debug Bridge version 1.0.41\nVersion 35.0.2\n",
		"get-state":        "device\n",
		"resolve-activity": "priority=0 preferredOrder=0 match=0x108000 specificIndex=-1 isDefault=false\ndev.sightmap.compose/.MainActivity\n",
		"pidof":            "4321\n",
		"am start":         "Starting: Intent { cmp=dev.sightmap.compose/.MainActivity }\nStatus: ok\n",
	}}
}

func TestADBHelperInputInPixels(t *testing.T) {
	f := newFakeADB(t, "uiautomator-compose")
	h := NewADBHelper(ADB{Run: f.run}, "emulator-5554", "dev.sightmap.compose", nil)
	ctx := context.Background()
	if err := h.Request(ctx, "tap", map[string]any{"x": 100.0, "y": 200.0}, nil); err != nil {
		t.Fatal(err)
	}
	if c := f.called("input tap"); c != "-s emulator-5554 shell input tap 263 525" {
		t.Errorf("tap = %q", c)
	}
	h.Request(ctx, "swipe", map[string]any{"x1": 200.0, "y1": 600.0, "x2": 200.0, "y2": 200.0, "duration": 0.5}, nil)
	if c := f.called("input swipe"); !strings.HasSuffix(c, "input swipe 525 1575 525 525 500") {
		t.Errorf("swipe = %q", c)
	}
	h.Request(ctx, "type", map[string]any{"text": "it's a test"}, nil)
	if c := f.called("input text"); !strings.HasSuffix(c, `input text 'it'\''s%sa%stest'`) {
		t.Errorf("type = %q", c)
	}
	if err := h.Request(ctx, "type", map[string]any{"text": "café"}, nil); Code(err) != "bad_request" {
		t.Errorf("non-ASCII text = %v", err)
	}
	h.Request(ctx, "key", map[string]any{"key": "return"}, nil)
	h.Request(ctx, "button", map[string]any{"name": "back"}, nil)
	if f.called("input keyevent 66") == "" || f.called("input keyevent 4") == "" {
		t.Errorf("keys = %v", f.calls)
	}
	if err := h.Request(ctx, "button", map[string]any{"name": "siri"}, nil); Code(err) != "bad_request" || !strings.Contains(err.Error(), "app_switch") {
		t.Errorf("unknown button = %v", err)
	}
	var d Describe
	if err := h.Request(ctx, "describe", map[string]any{"udid": "emulator-5554"}, &d); err != nil || d.Root == nil || len(d.Root.Children) == 0 {
		t.Fatalf("describe = %+v, %v", d, err)
	}
	if c := f.called("uiautomator"); !strings.Contains(c, "rm -f "+dumpPath) || !strings.Contains(c, "dumpsys window InputMethod") {
		t.Errorf("describe command = %q", c)
	}
}

func TestAndroidSessionClicksInPixels(t *testing.T) {
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("testdata", "android-corpus"))); err != nil {
		t.Fatal(err)
	}
	f := newFakeADB(t, "uiautomator-compose")
	adb := ADB{Run: f.run}
	s := NewSession(Config{Platform: PlatformAndroid, UDID: "emulator-5554", BundleID: "dev.sightmap.compose", SightmapDir: dir},
		NewADBHelper(adb, "emulator-5554", "dev.sightmap.compose", nil), AndroidSim{ADB: adb})
	s.Timing = Timing{RetryTimeout: time.Millisecond, RetryInterval: time.Millisecond, Settle: time.Millisecond}.withDefaults()
	r, err := handle(t, s, "click", TargetArgs{Target: "AddToCart"})
	if err != nil {
		t.Fatal(err)
	}
	// add_to_cart is [63,2169][1017,2316] px: 24,826 363x56 dp, center 205,854 dp.
	if c := f.called("input tap"); !strings.HasSuffix(c, "input tap 538 2242") {
		t.Errorf("tap = %q (result %v)", c, r)
	}
	entries, err := ReadRun(s.RunLog.Current().Path, 0)
	if err != nil || len(entries) != 1 || entries[0].View != "Products" || entries[0].Target.Component != "AddToCart" {
		t.Errorf("run log = %+v, %v", entries, err)
	}
}

func TestAndroidSimLaunch(t *testing.T) {
	f := newFakeADB(t, "uiautomator-compose")
	sim := AndroidSim{ADB: ADB{Run: f.run}}
	pid, err := sim.Launch(context.Background(), "emulator-5554", "dev.sightmap.compose", LaunchOptions{Terminate: true})
	if err != nil || pid != 4321 {
		t.Fatalf("pid %d, %v", pid, err)
	}
	stop, start := f.called("am force-stop"), f.called("am start")
	if stop == "" || !strings.Contains(start, "-n 'dev.sightmap.compose/.MainActivity'") {
		t.Errorf("calls = %v", f.calls)
	}
}

func TestParseADBDevicesAndPick(t *testing.T) {
	devices := parseADBDevices(`List of devices attached
emulator-5554          device product:sdk_gphone64_arm64 model:sdk_gphone64_arm64 device:emu64a transport_id:1
R5CT1234ABC            unauthorized usb:1-1 transport_id:3
`)
	if len(devices) != 2 || devices[0].Serial != "R5CT1234ABC" || devices[1].Model != "sdk_gphone64_arm64" {
		t.Fatalf("devices = %+v", devices)
	}
	devices[1].AVD, devices[1].Release = "Pixel_8_API_35", "15"
	d, err := PickAndroidDevice(devices, "")
	if err != nil || d.Serial != "emulator-5554" || d.Label() != "Pixel_8_API_35, Android 15" {
		t.Errorf("default pick = %+v, %v", d, err)
	}
	if d, err := PickAndroidDevice(devices, "Pixel_8_API_35"); err != nil || d.Serial != "emulator-5554" {
		t.Errorf("pick by AVD = %+v, %v", d, err)
	}
	if _, err := PickAndroidDevice(devices, "R5CT1234ABC"); Code(err) != "not_booted" {
		t.Errorf("an unauthorized device = %v", err)
	}
	if _, err := PickAndroidDevice(devices, "Pixel_9"); Code(err) != "no_device" {
		t.Errorf("unknown device = %v", err)
	}
	if _, err := PickAndroidDevice(nil, ""); Code(err) != "no_device" {
		t.Errorf("no devices = %v", err)
	}
}

func TestParseLogcatLine(t *testing.T) {
	pid, e, ok := parseLogcatLine("10-29 15:31:15.495  4321  4350 I CartRepository: POST /cart -> 200")
	if !ok || pid != 4321 || e.Level != "info" || e.Subsystem != "CartRepository" || e.Message != "POST /cart -> 200" || e.Source != "logcat" {
		t.Errorf("entry = %d %+v %v", pid, e, ok)
	}
	_, e, _ = parseLogcatLine("10-29 15:31:15.501  4321  4321 I System.out: hello from println")
	if e.Level != "stdout" || e.Message != "hello from println" {
		t.Errorf("System.out = %+v", e)
	}
	_, e, _ = parseLogcatLine("10-29 15:31:16.002  4321  4321 E AndroidRuntime: FATAL EXCEPTION: main")
	if e.Level != "error" || e.Subsystem != "AndroidRuntime" {
		t.Errorf("crash = %+v", e)
	}
	if _, _, ok := parseLogcatLine("--------- beginning of main"); ok {
		t.Error("a logcat banner is not an entry")
	}
}

func TestNetworkIsRefusedOnAndroid(t *testing.T) {
	s := &Session{Config: Config{Platform: PlatformAndroid}}
	if err := s.networkSupported(); Code(err) != "unsupported" {
		t.Errorf("err = %v", err)
	}
	if err := (&Session{}).networkSupported(); err != nil {
		t.Errorf("iOS err = %v", err)
	}
}

// uiautomator-pixel9a.xml is a real dump of a sample app's
// click screen, taken on a Pixel 9a (Android 17).
func TestParseUIAutomatorRealDump(t *testing.T) {
	d := loadUIAutomator(t, "uiautomator-pixel9a", "com.example.sampleapp", AndroidScreen{WidthPx: 1080, HeightPx: 2424, Density: 420})
	snap, err := Annotate(Normalize(d), nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if d.Overlay || !snap.Settled || snap.Coverage.Total != 16 {
		t.Errorf("overlay %v settled %v interactive %d; want the app's tree, settled, 16 interactive", d.Overlay, snap.Settled, snap.Coverage.Total)
	}
	for target, want := range map[string]string{
		"buttonClick":                   "JUST CLICK",
		"buttonClickAndLong4":           "CLICK AND LONG CLICK 4",
		`[text="NOTHING"]`:              "NOTHING",
		`[identifier="toolbar"] button`: "",
	} {
		n, err := Resolve(snap, target)
		if err != nil || n.Role != "button" || n.Name != want {
			t.Errorf("%s = %+v, %v; want button %q", target, n, err, want)
		}
	}
}

func TestAndroidClickReadsTheScreenOnce(t *testing.T) {
	dir := t.TempDir()
	f := newFakeADB(t, "uiautomator-compose")
	adb := ADB{Run: f.run}
	s := NewSession(Config{Platform: PlatformAndroid, UDID: "emulator-5554", BundleID: "dev.sightmap.compose", SightmapDir: dir},
		NewADBHelper(adb, "emulator-5554", "dev.sightmap.compose", nil), AndroidSim{ADB: adb})
	s.Timing = Timing{RetryTimeout: time.Millisecond, RetryInterval: time.Millisecond, Settle: time.Millisecond}.withDefaults()
	if _, err := handle(t, s, "click", TargetArgs{Target: "add_to_cart"}); err != nil {
		t.Fatal(err)
	}
	dumps := 0
	for _, c := range f.calls {
		if strings.Contains(c, "uiautomator dump") {
			dumps++
		}
	}
	if dumps != 1 {
		t.Errorf("click took %d dumps; uiautomator waits for an idle screen, so one is enough", dumps)
	}
}

func TestScrollUntilStopsOnTheFirstOfSeveralMatches(t *testing.T) {
	dir := t.TempDir()
	f := newFakeADB(t, "uiautomator-compose")
	adb := ADB{Run: f.run}
	s := NewSession(Config{Platform: PlatformAndroid, UDID: "emulator-5554", BundleID: "dev.sightmap.compose", SightmapDir: dir},
		NewADBHelper(adb, "emulator-5554", "dev.sightmap.compose", nil), AndroidSim{ADB: adb})
	s.Timing = Timing{RetryTimeout: time.Millisecond, RetryInterval: time.Millisecond, Settle: time.Millisecond, ScrollSettle: time.Millisecond}.withDefaults()
	r, err := handle(t, s, "scroll", ScrollArgs{Direction: "down", Until: `[text*="Product 2"]`})
	if err != nil {
		t.Fatal(err)
	}
	if r["matches"] != 2.0 || r["scrolls"] != 0.0 || f.called("input swipe") != "" {
		t.Errorf("result %v, swipe %q; want the first of 2 matches without scrolling", r, f.called("input swipe"))
	}

	_, err = handle(t, s, "scroll", ScrollArgs{Direction: "down", Until: `[text="Product"]`, Max: 3})
	if Code(err) != "not_found" || !strings.Contains(AsError(err).Hint, `[text*="Product"]`) {
		t.Errorf("an exact [text=] miss hints at text*=: %+v", AsError(err))
	}
}
