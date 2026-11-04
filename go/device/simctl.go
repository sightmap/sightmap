package device

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Simulator lifecycle, install, launch, screenshots and logs go through
// `xcrun simctl`; only the accessibility tree and input go through the helper.

// Sim is the simctl surface a Session uses, so tests can fake it.
type Sim interface {
	EnableAccessibility(ctx context.Context, udid string) error
	AccessibilityEnabled(ctx context.Context, udid string) bool
	Install(ctx context.Context, udid, appPath string) error
	Launch(ctx context.Context, udid, bundleID string, opts LaunchOptions) (int, error)
	Terminate(ctx context.Context, udid, bundleID string)
	Shutdown(ctx context.Context, udid string)
	OpenURL(ctx context.Context, udid, url string) error
	Screenshot(ctx context.Context, udid, path string) error
}

// LaunchOptions controls Simctl.Launch.
type LaunchOptions struct {
	Stdout, Stderr string // files the app's stdout/stderr are redirected to
	Terminate      bool   // terminate a running instance first
}

// Simctl implements Sim with xcrun simctl.
type Simctl struct{}

func xcrun(ctx context.Context, timeout time.Duration, env []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "xcrun", args...)
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		lines := strings.Split(detail, "\n")
		return stdout.String(), errorf("simctl_failed", "xcrun %s failed: %s", strings.Join(args[:min(2, len(args))], " "), lines[len(lines)-1])
	}
	return stdout.String(), nil
}

// Device is an available iOS simulator.
type Device struct {
	UDID           string `json:"udid"`
	Name           string `json:"name"`
	State          string `json:"state"`
	Runtime        string `json:"runtime"`
	RuntimeVersion string `json:"runtimeVersion"`
}

// Label names the device for people, e.g. "iPhone 17 Pro, iOS 26.2".
func (d Device) Label() string { return fmt.Sprintf("%s, iOS %s", d.Name, d.RuntimeVersion) }

// ListDevices returns the available iOS simulators.
func ListDevices(ctx context.Context) ([]Device, error) {
	out, err := xcrun(ctx, 60*time.Second, nil, "simctl", "list", "devices", "available", "-j")
	if err != nil {
		return nil, err
	}
	var list struct {
		Devices map[string][]Device `json:"devices"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		return nil, err
	}
	var devices []Device
	for runtime, ds := range list.Devices {
		_, version, ok := strings.Cut(runtime, "SimRuntime.iOS-")
		if !ok {
			continue
		}
		for _, d := range ds {
			d.Runtime, d.RuntimeVersion = runtime, strings.ReplaceAll(version, "-", ".")
			devices = append(devices, d)
		}
	}
	sort.SliceStable(devices, func(i, j int) bool { return devices[i].UDID < devices[j].UDID })
	return devices, nil
}

func versionLess(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(as), len(bs)); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return x < y
		}
	}
	return false
}

// PickDevice picks wanted (a UDID or a name; a booted one and then the newest
// runtime win among same-named devices) or, without it, a booted device, else
// the iPhone on the newest runtime.
func PickDevice(devices []Device, wanted string) (Device, error) {
	if wanted != "" {
		var byName []Device
		for _, d := range devices {
			if strings.EqualFold(d.UDID, wanted) {
				return d, nil
			}
			if d.Name == wanted {
				byName = append(byName, d)
			}
		}
		if len(byName) == 0 {
			return Device{}, withHint("device_not_found", "list them with `xcrun simctl list devices available`", "no available iOS simulator named or with UDID %q", wanted)
		}
		sort.SliceStable(byName, func(i, j int) bool {
			if bi, bj := byName[i].State == "Booted", byName[j].State == "Booted"; bi != bj {
				return bi
			}
			return versionLess(byName[j].RuntimeVersion, byName[i].RuntimeVersion)
		})
		return byName[0], nil
	}
	for _, d := range devices {
		if d.State == "Booted" {
			return d, nil
		}
	}
	var iphones []Device
	for _, d := range devices {
		if strings.HasPrefix(d.Name, "iPhone") {
			iphones = append(iphones, d)
		}
	}
	if len(iphones) == 0 {
		return Device{}, withHint("device_not_found", "run `sightmap device doctor` for the fix", "no available iOS simulator")
	}
	sort.SliceStable(iphones, func(i, j int) bool { return versionLess(iphones[j].RuntimeVersion, iphones[i].RuntimeVersion) })
	return iphones[0], nil
}

// Boot boots udid (a no-op when booted) and waits until it has finished booting.
func Boot(ctx context.Context, udid string) error {
	if _, err := xcrun(ctx, 2*time.Minute, nil, "simctl", "boot", udid); err != nil && !strings.Contains(err.Error(), "current state: Booted") {
		return err
	}
	_, err := xcrun(ctx, 5*time.Minute, nil, "simctl", "bootstatus", udid)
	return err
}

// PrepareDevice boots d and turns on app accessibility. The accessibility server
// reads that setting only at boot, so the first enable reboots the device once;
// progress receives a line for each slow step.
func PrepareDevice(ctx context.Context, d Device, progress func(string)) error {
	var sim Simctl
	if d.State != "Booted" {
		progress(fmt.Sprintf("booting %s (%s)…", d.Name, d.UDID))
		if err := Boot(ctx, d.UDID); err != nil {
			return err
		}
	}
	if sim.AccessibilityEnabled(ctx, d.UDID) {
		return nil
	}
	progress(fmt.Sprintf("enabling app accessibility on %s; rebooting it once…", d.Name))
	if err := sim.EnableAccessibility(ctx, d.UDID); err != nil {
		return err
	}
	sim.Shutdown(ctx, d.UDID)
	return Boot(ctx, d.UDID)
}

// AccessibilityEnabled implements Sim.
func (Simctl) AccessibilityEnabled(ctx context.Context, udid string) bool {
	out, err := xcrun(ctx, 30*time.Second, nil, "simctl", "spawn", udid, "defaults", "read", "com.apple.Accessibility", "ApplicationAccessibilityEnabled")
	return err == nil && strings.TrimSpace(out) == "1"
}

// EnableAccessibility implements Sim.
func (Simctl) EnableAccessibility(ctx context.Context, udid string) error {
	_, err := xcrun(ctx, 30*time.Second, nil, "simctl", "spawn", udid, "defaults", "write", "com.apple.Accessibility", "ApplicationAccessibilityEnabled", "-bool", "true")
	return err
}

// Install implements Sim.
func (Simctl) Install(ctx context.Context, udid, appPath string) error {
	_, err := xcrun(ctx, 5*time.Minute, nil, "simctl", "install", udid, appPath)
	return err
}

// Launch implements Sim. NSUnbufferedIO keeps `print` output from sitting in a
// buffer until the app exits.
func (Simctl) Launch(ctx context.Context, udid, bundleID string, opts LaunchOptions) (int, error) {
	args := []string{"simctl", "launch"}
	if opts.Terminate {
		args = append(args, "--terminate-running-process")
	}
	if opts.Stdout != "" {
		args = append(args, "--stdout="+opts.Stdout)
	}
	if opts.Stderr != "" {
		args = append(args, "--stderr="+opts.Stderr)
	}
	out, err := xcrun(ctx, 2*time.Minute, []string{"SIMCTL_CHILD_NSUnbufferedIO=YES"}, append(args, udid, bundleID)...)
	if err != nil {
		return 0, err
	}
	fields := strings.Split(strings.TrimSpace(out), ":")
	pid, _ := strconv.Atoi(strings.TrimSpace(fields[len(fields)-1]))
	return pid, nil
}

// Terminate implements Sim.
func (Simctl) Terminate(ctx context.Context, udid, bundleID string) {
	xcrun(ctx, 30*time.Second, nil, "simctl", "terminate", udid, bundleID)
}

// Shutdown implements Sim.
func (Simctl) Shutdown(ctx context.Context, udid string) {
	xcrun(ctx, 2*time.Minute, nil, "simctl", "shutdown", udid)
}

// OpenURL implements Sim.
func (Simctl) OpenURL(ctx context.Context, udid, url string) error {
	_, err := xcrun(ctx, 30*time.Second, nil, "simctl", "openurl", udid, url)
	return err
}

// Screenshot implements Sim.
func (Simctl) Screenshot(ctx context.Context, udid, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	_, err := xcrun(ctx, time.Minute, nil, "simctl", "io", udid, "screenshot", path)
	return err
}

// AppContainer returns the installed app bundle's path for bundleID.
func AppContainer(ctx context.Context, udid, bundleID string) (string, error) {
	out, err := xcrun(ctx, 30*time.Second, nil, "simctl", "get_app_container", udid, bundleID, "app")
	if err != nil {
		return "", withHint("app_not_installed", "pass --app path/to/MyApp.app to install it", "%s is not installed on this simulator", bundleID)
	}
	return strings.TrimSpace(out), nil
}

// PlistValue reads key from appPath's Info.plist.
func PlistValue(appPath, key string) (string, error) {
	plist := filepath.Join(appPath, "Info.plist")
	if _, err := os.Stat(plist); err != nil {
		return "", errorf("bad_app", "%s has no Info.plist; pass the .app bundle built for the simulator", appPath)
	}
	out, err := exec.Command("plutil", "-extract", key, "raw", "-o", "-", plist).Output()
	if err != nil {
		return "", errorf("bad_app", "%s has no %s", plist, key)
	}
	return strings.TrimSpace(string(out)), nil
}
