package device

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	minXcode = 26
	minMacOS = 15
)

const xcodeDownload = "install Xcode 26 or later from the Mac App Store or https://developer.apple.com/download/applications/, open it once, then re-run doctor"

// Check is one doctor result. OK is false for a failure; Fix says what to run.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
	// autoFix is an unprivileged command `doctor --fix` may run.
	autoFix []string
}

func run(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func majorOf(v string) int {
	n, _ := strconv.Atoi(strings.SplitN(strings.TrimSpace(v), ".", 2)[0])
	return n
}

func pass(name, detail string) Check { return Check{Name: name, OK: true, Detail: detail} }

func fail(name, detail, fix string) Check { return Check{Name: name, Detail: detail, Fix: fix} }

// Doctor checks this Mac for everything `sightmap device` needs and builds the
// helper (the first time, a minute or two). With fix, it runs the unprivileged
// fixes once and checks again. progress receives the helper build's output.
func Doctor(ctx context.Context, fix bool, progress io.Writer) []Check {
	checks := doctorChecks(ctx, progress)
	if !fix {
		return checks
	}
	fixed := false
	for _, c := range checks {
		if !c.OK && len(c.autoFix) > 0 {
			fmt.Fprintf(progress, "fixing %s: %s\n", c.Name, strings.Join(c.autoFix, " "))
			run(ctx, time.Hour, c.autoFix[0], c.autoFix[1:]...)
			fixed = true
		}
	}
	if fixed {
		checks = doctorChecks(ctx, progress)
	}
	return checks
}

func doctorChecks(ctx context.Context, progress io.Writer) []Check {
	if runtime.GOOS != "darwin" {
		return []Check{fail("macos", "this is "+runtime.GOOS, "sightmap device drives the iOS Simulator, which needs a Mac")}
	}
	checks := []Check{checkMacOS(ctx), checkArch(ctx), checkXcodeSelect(ctx), checkXcodeVersion(ctx)}
	checks = append(checks, checkSimulators(ctx)...)
	checks = append(checks, checkSwift(ctx))
	for _, c := range checks {
		if !c.OK {
			return append(checks, Check{Name: "helper", Detail: "not built: fix the checks above first"})
		}
	}
	if path, err := BuildHelper(ctx, progress); err != nil {
		e := AsError(err)
		checks = append(checks, fail("helper", e.Message, e.Hint))
	} else {
		checks = append(checks, pass("helper", path))
	}
	return checks
}

func checkMacOS(ctx context.Context) Check {
	v, _ := run(ctx, 10*time.Second, "sw_vers", "-productVersion")
	if majorOf(v) < minMacOS {
		return fail("macos", "macOS "+v, fmt.Sprintf("update to macOS %d or later (System Settings > General > Software Update)", minMacOS))
	}
	return pass("macos", "macOS "+v)
}

func checkArch(ctx context.Context) Check {
	if out, _ := run(ctx, 10*time.Second, "sysctl", "-n", "hw.optional.arm64"); out != "1" {
		return fail("apple-silicon", "this Mac is "+runtime.GOARCH, "an Apple Silicon (arm64) Mac is required; the simulator helper uses arm64-only frameworks")
	}
	return pass("apple-silicon", "arm64")
}

func checkXcodeSelect(ctx context.Context) Check {
	out, err := run(ctx, 10*time.Second, "xcode-select", "-p")
	if err != nil {
		return fail("xcode-select", "xcode-select -p failed: no developer directory", "xcode-select --install, then "+xcodeDownload)
	}
	if !strings.HasSuffix(out, ".app/Contents/Developer") {
		fix := xcodeDownload + ", then sudo xcode-select -s /Applications/Xcode.app/Contents/Developer"
		if apps, _ := run(ctx, 10*time.Second, "/bin/sh", "-c", "ls -d /Applications/Xcode*.app 2>/dev/null"); apps != "" {
			list := strings.Split(apps, "\n")
			fix = "sudo xcode-select -s " + list[len(list)-1] + "/Contents/Developer"
		}
		return fail("xcode-select", "points at "+out+" (Command Line Tools), not an Xcode.app", fix)
	}
	return pass("xcode-select", out)
}

var xcodeVersion = regexp.MustCompile(`(?m)^Xcode (\S+)`)

func checkXcodeVersion(ctx context.Context) Check {
	out, err := run(ctx, time.Minute, "xcodebuild", "-version")
	m := xcodeVersion.FindStringSubmatch(out)
	if err != nil || m == nil {
		return fail("xcode-version", "xcodebuild -version failed: "+out, xcodeDownload)
	}
	if majorOf(m[1]) < minXcode {
		return fail("xcode-version", fmt.Sprintf("Xcode %s; need %d+", m[1], minXcode), xcodeDownload)
	}
	if _, err := run(ctx, time.Minute, "xcodebuild", "-checkFirstLaunchStatus"); err != nil {
		return fail("xcode-version", "Xcode "+m[1]+", but first-launch setup is incomplete", "sudo xcodebuild -runFirstLaunch")
	}
	return pass("xcode-version", "Xcode "+m[1])
}

func checkSimulators(ctx context.Context) []Check {
	out, err := run(ctx, time.Minute, "xcrun", "simctl", "list", "runtimes", "-j")
	if err != nil {
		return []Check{fail("simctl", "xcrun simctl failed: "+out, "fix xcode-select first, then: sudo xcodebuild -runFirstLaunch")}
	}
	var runtimes struct {
		Runtimes []struct {
			Identifier           string `json:"identifier"`
			Version              string `json:"version"`
			Platform             string `json:"platform"`
			IsAvailable          bool   `json:"isAvailable"`
			SupportedDeviceTypes []struct {
				Name          string `json:"name"`
				Identifier    string `json:"identifier"`
				ProductFamily string `json:"productFamily"`
			} `json:"supportedDeviceTypes"`
		} `json:"runtimes"`
	}
	json.Unmarshal([]byte(out), &runtimes)
	var versions []string
	newest := -1
	for i, rt := range runtimes.Runtimes {
		if rt.Platform == "iOS" && rt.IsAvailable {
			versions = append(versions, "iOS "+rt.Version)
			newest = i
		}
	}
	if newest < 0 {
		c := fail("ios-runtime", "no available iOS Simulator runtime", "xcodebuild -downloadPlatform iOS")
		c.autoFix = []string{"xcodebuild", "-downloadPlatform", "iOS"}
		return []Check{c}
	}
	checks := []Check{pass("ios-runtime", strings.Join(versions, ", "))}
	devices, _ := ListDevices(ctx)
	if len(devices) > 0 {
		var booted []string
		for _, d := range devices {
			if d.State == "Booted" {
				booted = append(booted, d.Name)
			}
		}
		detail := fmt.Sprintf("%d iOS simulators", len(devices))
		if len(booted) > 0 {
			detail += "; booted: " + strings.Join(booted, ", ")
		}
		return append(checks, pass("ios-device", detail))
	}
	rt := runtimes.Runtimes[newest]
	for _, t := range rt.SupportedDeviceTypes {
		if t.ProductFamily == "iPhone" {
			c := fail("ios-device", "no simulator devices for iOS "+rt.Version, fmt.Sprintf("xcrun simctl create %q %s %s", t.Name, t.Identifier, rt.Identifier))
			c.autoFix = []string{"xcrun", "simctl", "create", t.Name, t.Identifier, rt.Identifier}
			return append(checks, c)
		}
	}
	return append(checks, fail("ios-device", "no simulator devices for iOS "+rt.Version, "create one in Xcode > Window > Devices and Simulators"))
}

func checkSwift(ctx context.Context) Check {
	out, err := run(ctx, time.Minute, "xcrun", "swift", "--version")
	for _, line := range strings.Split(out, "\n") {
		if err == nil && strings.Contains(line, "Swift version") {
			return pass("swift", strings.TrimSpace(line))
		}
	}
	return fail("swift", "xcrun swift --version failed", "fix xcode-select first; Swift ships with Xcode")
}
