package device

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Platforms a session can drive.
const (
	PlatformIOS     = "ios"
	PlatformAndroid = "android"
)

// The Android backend drives an emulator or a USB/Wi-Fi device through adb
// alone: `uiautomator dump` for the tree, `input` for touches and keys, `am`
// and `pm` for the app, logcat for its console. Nothing is installed on the
// device besides the app under test.

// ADB runs adb. Path is the adb binary; Run, when set, replaces running it
// (tests use a fake).
type ADB struct {
	Path string
	Run  func(ctx context.Context, args ...string) (string, error)
}

// FindADB returns the adb binary: $ANDROID_HOME (or $ANDROID_SDK_ROOT)
// /platform-tools/adb, else the one on PATH, else the SDK's default location.
func FindADB() (string, error) {
	for _, root := range androidSDKRoots() {
		p := filepath.Join(root, "platform-tools", exe("adb"))
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("adb"); err == nil {
		return p, nil
	}
	return "", withHint("no_adb", "install the Android SDK platform-tools (Android Studio's SDK Manager, or `sdkmanager platform-tools`) and set ANDROID_HOME, or put adb on PATH",
		"adb was not found")
}

// androidSDKRoots lists where the Android SDK may live, most explicit first.
func androidSDKRoots() []string {
	var roots []string
	for _, env := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := os.Getenv(env); v != "" {
			roots = append(roots, v)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, "Library", "Android", "sdk"), filepath.Join(home, "Android", "Sdk"),
			filepath.Join(home, "AppData", "Local", "Android", "Sdk"))
	}
	return roots
}

func exe(name string) string {
	if os.PathSeparator == '\\' {
		return name + ".exe"
	}
	return name
}

func (a ADB) run(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if a.Run != nil {
		return a.Run(ctx, args...)
	}
	cmd := exec.CommandContext(ctx, a.Path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return stdout.String(), withHint("adb_timeout", "check the device with `adb devices`; retry", "adb %s timed out after %s", strings.Join(args[:min(3, len(args))], " "), timeout)
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail == "" {
			detail = err.Error()
		}
		lines := strings.Split(detail, "\n")
		return stdout.String(), adbFailure(args, lines[len(lines)-1])
	}
	return stdout.String(), nil
}

func adbFailure(args []string, detail string) *Error {
	lower := strings.ToLower(detail)
	what := strings.Join(args[:min(4, len(args))], " ")
	switch {
	case strings.Contains(lower, "not found") && strings.Contains(lower, "device"),
		strings.Contains(lower, "offline"), strings.Contains(lower, "no devices"):
		return withHint("not_booted", "start the emulator or reconnect the device, check `adb devices`, then `sightmap device start` again", "adb %s failed: %s", what, detail)
	case strings.Contains(lower, "unauthorized"):
		return withHint("not_booted", "accept the USB debugging prompt on the device, then retry", "adb %s failed: %s", what, detail)
	}
	return errorf("adb_failed", "adb %s failed: %s", what, detail)
}

// command returns an adb command for a long-running stream such as logcat.
func (a ADB) command(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, a.Path, args...)
}

// shell runs command in the device's shell (the arguments are joined and parsed
// by the device's sh, so quote with shellQuote).
func (a ADB) shell(ctx context.Context, serial string, timeout time.Duration, command string) (string, error) {
	return a.run(ctx, timeout, "-s", serial, "shell", command)
}

// shellQuote quotes s for the device's sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// AndroidDevice is a device adb can see.
type AndroidDevice struct {
	Serial  string `json:"serial"`
	State   string `json:"state"` // device, offline, unauthorized, …
	Model   string `json:"model,omitempty"`
	AVD     string `json:"avd,omitempty"`
	Release string `json:"release,omitempty"` // Android version, e.g. "15"
}

// Emulator reports whether d is an emulator.
func (d AndroidDevice) Emulator() bool { return strings.HasPrefix(d.Serial, "emulator-") }

// Label names the device for people, e.g. "Pixel_8_API_35, Android 15".
func (d AndroidDevice) Label() string {
	name := cmpOr(d.AVD, cmpOr(d.Model, d.Serial))
	if d.Release == "" {
		return name
	}
	return fmt.Sprintf("%s, Android %s", name, d.Release)
}

// parseADBDevices reads `adb devices -l`.
func parseADBDevices(out string) []AndroidDevice {
	var devices []AndroidDevice
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.HasPrefix(line, "List of devices") || strings.HasPrefix(line, "*") {
			continue
		}
		d := AndroidDevice{Serial: fields[0], State: fields[1]}
		for _, f := range fields[2:] {
			if k, v, ok := strings.Cut(f, ":"); ok && k == "model" {
				d.Model = v
			}
		}
		devices = append(devices, d)
	}
	sort.SliceStable(devices, func(i, j int) bool { return devices[i].Serial < devices[j].Serial })
	return devices
}

// ListAndroidDevices returns the devices adb can see, with each online one's
// AVD name (emulators) and Android version filled in.
func (a ADB) ListAndroidDevices(ctx context.Context) ([]AndroidDevice, error) {
	out, err := a.run(ctx, 30*time.Second, "devices", "-l")
	if err != nil {
		return nil, err
	}
	devices := parseADBDevices(out)
	for i := range devices {
		a.describeDevice(ctx, &devices[i])
	}
	return devices, nil
}

func (a ADB) describeDevice(ctx context.Context, d *AndroidDevice) {
	if d.State != "device" {
		return
	}
	if out, err := a.shell(ctx, d.Serial, 10*time.Second, "getprop ro.build.version.release"); err == nil {
		d.Release = strings.TrimSpace(out)
	}
	if d.Model == "" {
		if out, err := a.shell(ctx, d.Serial, 10*time.Second, "getprop ro.product.model"); err == nil {
			d.Model = strings.TrimSpace(out)
		}
	}
	if d.Emulator() {
		if out, err := a.run(ctx, 10*time.Second, "-s", d.Serial, "emu", "avd", "name"); err == nil {
			d.AVD = strings.TrimSpace(strings.Split(strings.TrimSpace(out), "\n")[0])
		}
	}
}

// PickAndroidDevice picks wanted (a serial, AVD name or model) among the online
// devices or, without it, the only one online.
func PickAndroidDevice(devices []AndroidDevice, wanted string) (AndroidDevice, error) {
	var online []AndroidDevice
	for _, d := range devices {
		if wanted != "" && (d.Serial == wanted || d.AVD == wanted || d.Model == wanted) {
			if d.State != "device" {
				return d, withHint("not_booted", "wait for it to finish booting, or accept the USB debugging prompt, then retry", "%s is %s", d.Serial, d.State)
			}
			return d, nil
		}
		if d.State == "device" {
			online = append(online, d)
		}
	}
	if wanted != "" {
		return AndroidDevice{}, withHint("no_device", "name an AVD from `emulator -list-avds` to boot it, or check `adb devices`", "no connected device or AVD named %q", wanted)
	}
	switch len(online) {
	case 0:
		return AndroidDevice{}, withHint("no_device", "start an emulator (`emulator -avd NAME`, or `sightmap device start --platform android --device AVD_NAME`) or connect a device with USB debugging on",
			"no Android device is connected")
	case 1:
		return online[0], nil
	}
	var names []string
	for _, d := range online {
		names = append(names, d.Serial+" ("+d.Label()+")")
	}
	return AndroidDevice{}, withHint("usage", "pass --device SERIAL", "several Android devices are connected: %s", strings.Join(names, ", "))
}

// FindEmulator returns the Android emulator binary, or "".
func FindEmulator() string {
	for _, root := range androidSDKRoots() {
		p := filepath.Join(root, "emulator", exe("emulator"))
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("emulator"); err == nil {
		return p
	}
	return ""
}

// ListAVDs returns the AVDs the emulator can boot.
func ListAVDs(ctx context.Context) []string {
	emulator := FindEmulator()
	if emulator == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, emulator, "-list-avds").Output()
	if err != nil {
		return nil
	}
	var avds []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "INFO") && !strings.Contains(line, " ") {
			avds = append(avds, line)
		}
	}
	return avds
}

// BootAVD starts the emulator for avd in the background and waits until
// Android has finished booting, returning the new device.
func (a ADB) BootAVD(ctx context.Context, avd string, progress func(string)) (AndroidDevice, error) {
	emulator := FindEmulator()
	if emulator == "" {
		return AndroidDevice{}, withHint("no_emulator", "install the Android Emulator from Android Studio's SDK Manager", "the Android emulator was not found")
	}
	before, err := a.ListAndroidDevices(ctx)
	if err != nil {
		return AndroidDevice{}, err
	}
	known := map[string]bool{}
	for _, d := range before {
		known[d.Serial] = true
	}
	progress(fmt.Sprintf("booting the %s emulator…", avd))
	cmd := exec.Command(emulator, "-avd", avd, "-no-snapshot-save", "-no-boot-anim")
	cmd.Env = emulatorEnv(os.Environ(), runtime.GOOS)
	logPath := filepath.Join(os.TempDir(), "sightmap-emulator-"+avd+".log")
	if f, err := os.Create(logPath); err == nil {
		cmd.Stdout, cmd.Stderr = f, f
		defer f.Close()
	} else {
		logPath = ""
	}
	detachProcess(cmd)
	if err := cmd.Start(); err != nil {
		return AndroidDevice{}, errorf("emulator_failed", "cannot start the emulator: %v", err)
	}
	// The emulator outlives this process; waiting only notices it dying
	// during boot, which otherwise looks like a slow boot for 5 minutes.
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return AndroidDevice{}, ctx.Err()
		case err := <-exited:
			return AndroidDevice{}, emulatorExited(avd, logPath, err)
		case <-time.After(2 * time.Second):
		}
		devices, err := a.ListAndroidDevices(ctx)
		if err != nil {
			continue
		}
		for _, d := range devices {
			if known[d.Serial] || !d.Emulator() || d.State != "device" || (d.AVD != "" && d.AVD != avd) {
				continue
			}
			if out, _ := a.shell(ctx, d.Serial, 10*time.Second, "getprop sys.boot_completed"); strings.TrimSpace(out) == "1" {
				return d, nil
			}
		}
	}
	return AndroidDevice{}, withHint("emulator_failed", "start it yourself with `emulator -avd "+avd+"` and check its output", "the %s emulator did not finish booting within 5 minutes", avd)
}

// emulatorEnv is env for launching the emulator. On macOS the emulator's Qt
// only has the cocoa platform plugin, and a QT_QPA_PLATFORM set for some
// other platform (agent harnesses that mimic a Linux desktop set wayland)
// makes it exit at once.
func emulatorEnv(env []string, goos string) []string {
	if goos != "darwin" {
		return env
	}
	out := env[:0:0]
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "QT_QPA_PLATFORM="); ok && v != "cocoa" {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// emulatorExited explains an emulator that quit before it finished booting,
// quoting the end of its output.
func emulatorExited(avd, logPath string, waitErr error) error {
	detail := ""
	if logPath != "" {
		if data, err := os.ReadFile(logPath); err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			var keep []string
			for _, l := range lines {
				if strings.Contains(l, "FATAL") || strings.Contains(l, "Fatal") || strings.Contains(l, "ERROR") {
					keep = append(keep, strings.TrimSpace(l))
				}
			}
			if len(keep) == 0 && len(lines) > 0 {
				keep = lines[max(0, len(lines)-3):]
			}
			if len(keep) > 5 {
				keep = keep[len(keep)-5:]
			}
			detail = strings.Join(keep, "; ")
		}
	}
	hint := "run `emulator -avd " + avd + "` yourself to see why"
	if logPath != "" {
		hint = "see " + logPath + ", or run `emulator -avd " + avd + "` yourself"
	}
	if detail == "" {
		return withHint("emulator_failed", hint, "the %s emulator exited before it finished booting (%v)", avd, waitErr)
	}
	return withHint("emulator_failed", hint, "the %s emulator exited before it finished booting: %s", avd, detail)
}

// apkPackage reads an APK's package name with the SDK's aapt2 or aapt.
func apkPackage(ctx context.Context, apk string) (string, error) {
	var tools []string
	for _, root := range androidSDKRoots() {
		versions, _ := filepath.Glob(filepath.Join(root, "build-tools", "*"))
		sort.Sort(sort.Reverse(sort.StringSlice(versions)))
		for _, v := range versions {
			tools = append(tools, filepath.Join(v, exe("aapt2")), filepath.Join(v, exe("aapt")))
		}
	}
	for _, name := range []string{"aapt2", "aapt"} {
		if p, err := exec.LookPath(name); err == nil {
			tools = append(tools, p)
		}
	}
	for _, tool := range tools {
		if _, err := os.Stat(tool); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		out, err := exec.CommandContext(ctx, tool, "dump", "badging", apk).Output()
		cancel()
		if m := apkPackagePattern.FindSubmatch(out); m != nil {
			return string(m[1]), nil
		} else if err != nil {
			continue
		}
	}
	return "", withHint("usage", "pass the app id with --package com.example.app (or install the SDK build-tools so aapt2 can read it)", "cannot read the package name of %s", apk)
}

var apkPackagePattern = regexp.MustCompile(`package: name='([^']+)'`)

// APKPackage returns apk's package name.
func APKPackage(ctx context.Context, apk string) (string, error) {
	if !strings.HasSuffix(strings.ToLower(apk), ".apk") {
		return "", errorf("bad_app", "%s is not an .apk", apk)
	}
	if _, err := os.Stat(apk); err != nil {
		return "", errorf("bad_app", "%s does not exist", apk)
	}
	return apkPackage(ctx, apk)
}

// AndroidSim implements Sim with adb. "udid" is the device serial throughout.
type AndroidSim struct{ ADB ADB }

// AccessibilityEnabled implements Sim: uiautomator reads any app's tree, with
// no setting to turn on.
func (AndroidSim) AccessibilityEnabled(context.Context, string) bool { return true }

// EnableAccessibility implements Sim. Nothing needs turning on, but
// uiautomator reads nothing while the display sleeps, so it wakes the display,
// and keeps an emulator's display on from then on.
func (s AndroidSim) EnableAccessibility(ctx context.Context, serial string) error {
	if strings.HasPrefix(serial, "emulator-") {
		s.ADB.shell(ctx, serial, 15*time.Second, "svc power stayon true")
	}
	s.ADB.wakeIfAsleep(ctx, serial)
	return nil
}

var wakefulnessPattern = regexp.MustCompile(`mWakefulness=(\w+)`)

// wakefulness is the display's power state from `dumpsys power`: Awake,
// Asleep, Dozing or Dreaming, or "" when it can't be read.
func (a ADB) wakefulness(ctx context.Context, serial string) string {
	out, err := a.shell(ctx, serial, 15*time.Second, "dumpsys power")
	if err != nil {
		return ""
	}
	if m := wakefulnessPattern.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return ""
}

func sleeping(wakefulness string) bool {
	return wakefulness == "Asleep" || wakefulness == "Dozing" || wakefulness == "Dreaming"
}

// wakeIfAsleep wakes a display that is asleep, dozing or showing a
// screensaver, and dismisses a keyguard that has no PIN, pattern or
// password. It returns the state it found.
func (a ADB) wakeIfAsleep(ctx context.Context, serial string) (string, error) {
	state := a.wakefulness(ctx, serial)
	if !sleeping(state) {
		return state, nil
	}
	if _, err := a.shell(ctx, serial, 15*time.Second, "input keyevent KEYCODE_WAKEUP"); err != nil {
		return state, err
	}
	_, err := a.shell(ctx, serial, 15*time.Second, "wm dismiss-keyguard")
	return state, err
}

// Install implements Sim. -r replaces an installed copy, -t allows test-only
// (debug) APKs.
func (s AndroidSim) Install(ctx context.Context, serial, apk string) error {
	out, err := s.ADB.run(ctx, 5*time.Minute, "-s", serial, "install", "-r", "-t", apk)
	if err != nil {
		return err
	}
	if strings.Contains(out, "Failure") {
		return errorf("install_failed", "adb install %s: %s", apk, strings.TrimSpace(out))
	}
	return nil
}

// Launch implements Sim: it starts the package's launcher activity and returns
// the app's pid. stdout and stderr go to logcat on Android, so opts' files are
// unused.
func (s AndroidSim) Launch(ctx context.Context, serial, pkg string, opts LaunchOptions) (int, error) {
	if opts.Terminate {
		s.Terminate(ctx, serial, pkg)
	}
	out, _ := s.ADB.shell(ctx, serial, 30*time.Second, "cmd package resolve-activity --brief -c android.intent.category.LAUNCHER "+shellQuote(pkg))
	component := ""
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, pkg+"/") {
			component = line
		}
	}
	if component != "" {
		out, err := s.ADB.shell(ctx, serial, 2*time.Minute, "am start -W -n "+shellQuote(component))
		if err != nil {
			return 0, err
		}
		if strings.Contains(out, "Error:") {
			return 0, errorf("launch_failed", "am start %s: %s", component, strings.TrimSpace(out))
		}
	} else {
		out, err := s.ADB.shell(ctx, serial, time.Minute, "monkey -p "+shellQuote(pkg)+" -c android.intent.category.LAUNCHER 1")
		if err != nil {
			return 0, err
		}
		if strings.Contains(out, "No activities found") || strings.Contains(out, "monkey aborted") {
			return 0, withHint("app_not_installed", "pass --app path/to/app.apk to install it", "%s is not installed or has no launcher activity", pkg)
		}
	}
	for range 20 {
		if pid := s.pid(ctx, serial, pkg); pid > 0 {
			return pid, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return 0, errorf("launch_failed", "%s did not start (no process after launching it); check `sightmap device console list`", pkg)
}

func (s AndroidSim) pid(ctx context.Context, serial, pkg string) int {
	out, err := s.ADB.shell(ctx, serial, 10*time.Second, "pidof "+shellQuote(pkg))
	if err != nil {
		return 0
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return 0
	}
	pid, _ := strconv.Atoi(fields[0])
	return pid
}

// Terminate implements Sim.
func (s AndroidSim) Terminate(ctx context.Context, serial, pkg string) {
	s.ADB.shell(ctx, serial, 30*time.Second, "am force-stop "+shellQuote(pkg))
}

// Shutdown implements Sim. Only an emulator can be shut down.
func (s AndroidSim) Shutdown(ctx context.Context, serial string) {
	if strings.HasPrefix(serial, "emulator-") {
		s.ADB.run(ctx, time.Minute, "-s", serial, "emu", "kill")
	}
}

// OpenURL implements Sim.
func (s AndroidSim) OpenURL(ctx context.Context, serial, url string) error {
	out, err := s.ADB.shell(ctx, serial, 30*time.Second, "am start -W -a android.intent.action.VIEW -d "+shellQuote(url))
	if err != nil {
		return err
	}
	if strings.Contains(out, "Error:") {
		return errorf("openurl_failed", "nothing on the device handles %s: %s", url, strings.TrimSpace(out))
	}
	return nil
}

// Screenshot implements Sim.
func (s AndroidSim) Screenshot(ctx context.Context, serial, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	png, err := s.ADB.run(ctx, time.Minute, "-s", serial, "exec-out", "screencap", "-p")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(png, "\x89PNG") {
		return errorf("adb_failed", "screencap returned no PNG: %s", truncateRunes(strings.TrimSpace(png), 200))
	}
	return os.WriteFile(path, []byte(png), 0o644)
}

// androidKeys maps `device key` names to Android key codes.
var androidKeys = map[string]int{
	"return": 66, "enter": 66,
	"escape": 111, "esc": 111,
	"delete": 67, "backspace": 67,
	"forward_delete": 112, "forwarddelete": 112,
	"tab":   61,
	"space": 62,
	"up":    19, "down": 20, "left": 21, "right": 22,
	"home": 122, "end": 123,
	"page_up": 92, "pageup": 92, "page_down": 93, "pagedown": 93,
}

// androidButtons maps `device button` names to Android key codes.
var androidButtons = map[string]int{
	"home": 3, "back": 4, "menu": 82, "app_switch": 187, "recents": 187,
	"lock": 26, "power": 26, "side": 26,
	"volume_up": 24, "volume_down": 25,
}

// ADBHelper implements Helper for Android in-process: each request is one or
// two adb commands. Points are dp; the screen's density converts them to the
// pixels `input` takes.
type ADBHelper struct {
	ADB     ADB
	Serial  string
	Package string
	Log     func(string)

	mu       sync.Mutex
	screen   *AndroidScreen
	failures int
}

// NewADBHelper returns a helper for serial driving pkg.
func NewADBHelper(adb ADB, serial, pkg string, log func(string)) *ADBHelper {
	if log == nil {
		log = func(string) {}
	}
	return &ADBHelper{ADB: adb, Serial: serial, Package: pkg, Log: log}
}

// dumpPath is where uiautomator writes its dump on the device.
const dumpPath = "/data/local/tmp/sightmap-window.xml"

// imeMarker separates the dump from the input method window's state in one
// round trip.
const imeMarker = "__SIGHTMAP_IME__"

func (h *ADBHelper) display(ctx context.Context) (AndroidScreen, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.screen != nil {
		return *h.screen, nil
	}
	size, err := h.ADB.shell(ctx, h.Serial, 15*time.Second, "wm size")
	if err != nil {
		return AndroidScreen{}, err
	}
	density, err := h.ADB.shell(ctx, h.Serial, 15*time.Second, "wm density")
	if err != nil {
		return AndroidScreen{}, err
	}
	s := parseWM(size, density)
	if s.WidthPx == 0 || s.Density == 0 {
		return AndroidScreen{}, errorf("adb_failed", "cannot read the display size and density (wm size: %q, wm density: %q)", strings.TrimSpace(size), strings.TrimSpace(density))
	}
	// A tap in the status bar goes to the system, not the app. Its height
	// varies with the display cutout; an unreadable frame falls back to 24 dp.
	if bar, err := h.ADB.shell(ctx, h.Serial, 15*time.Second, "dumpsys window StatusBar"); err == nil {
		s.StatusBarPx = parseStatusBar(bar)
	}
	h.screen = &s
	return s, nil
}

func (h *ADBHelper) describe(ctx context.Context) (*Describe, error) {
	screen, err := h.display(ctx)
	if err != nil {
		return nil, err
	}
	// The old dump is removed first so a failed dump can't read back as the
	// current screen.
	command := "rm -f " + dumpPath + "; uiautomator dump " + dumpPath + " 2>&1; cat " + dumpPath + " 2>/dev/null; echo; echo " + imeMarker + "; dumpsys window InputMethod"
	var lastErr error
	asleep, checked := "", false
	// uiautomator gives up while the screen animates, and finds no window for a
	// moment during a transition; both pass. It also finds no window while the
	// display sleeps, which is woken once.
	for attempt := range 4 {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
		}
		out, err := h.ADB.shell(ctx, h.Serial, 45*time.Second, command)
		if err != nil {
			return nil, err
		}
		dump, ime, _ := strings.Cut(out, imeMarker)
		screen.Keyboard = parseIMEWindow(ime)
		d, err := ParseUIAutomator([]byte(dump), h.Package, screen)
		if err == nil {
			return d, nil
		}
		lastErr = err
		if Code(err) != "ax_unavailable" {
			break
		}
		if isNullRoot(err) && !checked {
			checked = true
			if state, werr := h.ADB.wakeIfAsleep(ctx, h.Serial); sleeping(state) {
				asleep = state
				if werr != nil {
					h.Log(fmt.Sprintf("the display was %s; waking it failed: %v", strings.ToLower(state), werr))
				} else {
					h.Log(fmt.Sprintf("the display was %s; woke it", strings.ToLower(state)))
				}
			}
		}
	}
	if asleep != "" && isNullRoot(lastErr) {
		return nil, withHint("ax_unavailable",
			fmt.Sprintf("wake and unlock the device: `adb -s %s shell input keyevent KEYCODE_WAKEUP`, then `adb -s %s shell wm dismiss-keyguard` (a PIN, pattern or password lock must be unlocked on the device), then retry", h.Serial, h.Serial),
			"the device's display was %s and uiautomator still finds no window after waking it", strings.ToLower(asleep))
	}
	return nil, lastErr
}

func (h *ADBHelper) px(ctx context.Context, v any) (int, error) {
	screen, err := h.display(ctx)
	if err != nil {
		return 0, err
	}
	f, ok := v.(float64)
	if !ok {
		switch n := v.(type) {
		case int:
			f = float64(n)
		default:
			return 0, errorf("bad_request", "expected a number, got %v", v)
		}
	}
	return int(f*screen.scale() + 0.5), nil
}

// inputText escapes text for `input text`: it takes %s for a space, and the
// device's shell parses the argument, so it is single-quoted.
func inputText(text string) (string, error) {
	for _, r := range text {
		if r > 126 || (r < 32 && r != '\n') {
			return "", withHint("bad_request", "adb can only type ASCII; type the rest on the device, or paste it into the field from the app",
				"adb input text cannot type %q", string(r))
		}
	}
	return shellQuote(strings.ReplaceAll(text, " ", "%s")), nil
}

// Request implements Helper.
func (h *ADBHelper) Request(ctx context.Context, cmd string, args map[string]any, out any) error {
	result, err := h.request(ctx, cmd, args)
	h.mu.Lock()
	if err != nil && (Code(err) == "not_booted" || Code(err) == "adb_timeout") {
		h.failures++
	} else if err == nil {
		h.failures = 0
	}
	h.mu.Unlock()
	if err != nil {
		return err
	}
	if out != nil && result != nil {
		return remarshal(result, out)
	}
	return nil
}

func (h *ADBHelper) request(ctx context.Context, cmd string, args map[string]any) (any, error) {
	num := func(k string) (int, error) { return h.px(ctx, args[k]) }
	switch cmd {
	case "hello":
		version, err := h.ADB.run(ctx, 15*time.Second, "version")
		if err != nil {
			return nil, err
		}
		state, err := h.ADB.run(ctx, 15*time.Second, "-s", h.Serial, "get-state")
		if err != nil {
			return nil, err
		}
		if s := strings.TrimSpace(state); s != "device" {
			return nil, withHint("not_booted", "wait for the device, then retry", "%s is %s", h.Serial, s)
		}
		first, _, _ := strings.Cut(strings.TrimSpace(version), "\n")
		return map[string]any{"version": "adb", "adb": first, "serial": h.Serial}, nil
	case "describe":
		return h.describe(ctx)
	case "tap":
		x, err := num("x")
		if err != nil {
			return nil, err
		}
		y, err := num("y")
		if err != nil {
			return nil, err
		}
		_, err = h.ADB.shell(ctx, h.Serial, 30*time.Second, fmt.Sprintf("input tap %d %d", x, y))
		return nil, err
	case "swipe":
		var p [4]int
		for i, k := range []string{"x1", "y1", "x2", "y2"} {
			v, err := num(k)
			if err != nil {
				return nil, err
			}
			p[i] = v
		}
		duration := 0.3
		if d, ok := args["duration"].(float64); ok && d > 0 {
			duration = d
		}
		_, err := h.ADB.shell(ctx, h.Serial, 30*time.Second, fmt.Sprintf("input swipe %d %d %d %d %d", p[0], p[1], p[2], p[3], int(duration*1000)))
		return nil, err
	case "type":
		text, _ := args["text"].(string)
		if text == "" {
			return nil, errorf("bad_request", "type needs text")
		}
		arg, err := inputText(text)
		if err != nil {
			return nil, err
		}
		_, err = h.ADB.shell(ctx, h.Serial, time.Minute, "input text "+arg)
		return nil, err
	case "key":
		name, _ := args["key"].(string)
		code, ok := androidKeys[strings.ToLower(name)]
		if !ok {
			return nil, errorf("bad_request", "unknown key %q; use one of %s", name, keyNames(androidKeys))
		}
		_, err := h.ADB.shell(ctx, h.Serial, 30*time.Second, fmt.Sprintf("input keyevent %d", code))
		return nil, err
	case "button":
		name, _ := args["name"].(string)
		code, ok := androidButtons[strings.ToLower(name)]
		if !ok {
			return nil, errorf("bad_request", "unknown button %q; use one of %s", name, keyNames(androidButtons))
		}
		_, err := h.ADB.shell(ctx, h.Serial, 30*time.Second, fmt.Sprintf("input keyevent %d", code))
		return nil, err
	}
	return nil, errorf("unknown_cmd", "unknown cmd %q", cmd)
}

func remarshal(v, out any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func keyNames(m map[string]int) string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// Alive implements Helper: false after adb lost the device.
func (h *ADBHelper) Alive() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.failures == 0
}

// Restarts implements Helper; there is no helper process to restart.
func (h *ADBHelper) Restarts() int { return 0 }

// Close implements Helper.
func (h *ADBHelper) Close() {}
