package device_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAndroidE2E drives an installed or given app through the sightmap CLI on
// a real emulator or device. It runs only when SIGHTMAP_ANDROID_E2E names the
// device (a serial or AVD name) and SIGHTMAP_ANDROID_E2E_APP names the app, as
// an .apk path or a package that is already installed:
//
//	SIGHTMAP_ANDROID_E2E=emulator-5554 SIGHTMAP_ANDROID_E2E_APP=path/to/app-debug.apk \
//	  go test ./device -run TestAndroidE2E -v -timeout 10m
//
// It snapshots the launch screen and checks the screenshot, console, run log
// and relaunch paths.
func TestAndroidE2E(t *testing.T) {
	serial, app := os.Getenv("SIGHTMAP_ANDROID_E2E"), os.Getenv("SIGHTMAP_ANDROID_E2E_APP")
	if serial == "" || app == "" {
		t.Skip("set SIGHTMAP_ANDROID_E2E (device serial or AVD) and SIGHTMAP_ANDROID_E2E_APP (.apk or package) to run the Android end-to-end test")
	}
	work := t.TempDir()
	bin := filepath.Join(work, "sightmap")
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/sightmap").CombinedOutput(); err != nil {
		t.Fatalf("build sightmap: %v\n%s", err, out)
	}
	run := func(args ...string) (string, error) {
		cmd := exec.Command(bin, append([]string{"device"}, args...)...)
		cmd.Dir = work
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		started := time.Now()
		err := cmd.Run()
		t.Logf("$ sightmap device %s  (%d ms)\n%s%s", strings.Join(args, " "), time.Since(started).Milliseconds(), stdout.String(), stderr.String())
		return stdout.String() + stderr.String(), err
	}
	must := func(args ...string) string {
		t.Helper()
		out, err := run(args...)
		if err != nil {
			t.Fatalf("sightmap device %s: %v", strings.Join(args, " "), err)
		}
		return out
	}
	t.Cleanup(func() { run("stop") })

	must("doctor", "--platform", "android")
	start := []string{"start", "--platform", "android", "--device", serial, "--detach"}
	if strings.HasSuffix(app, ".apk") {
		abs, _ := filepath.Abs(app)
		start = append(start, "--app", abs)
	} else {
		start = append(start, "--package", app)
	}
	if out := must(start...); !strings.Contains(out, "android session started") {
		t.Fatalf("start output:\n%s", out)
	}
	var snap string
	for range 10 {
		out, err := run("snapshot")
		if err == nil {
			snap = out
			break
		}
		time.Sleep(time.Second)
	}
	if snap == "" {
		t.Fatal("no snapshot showed an interactive element")
	}
	must("screenshot", "--out", filepath.Join(work, "shot.png"))
	if data, err := os.ReadFile(filepath.Join(work, "shot.png")); err != nil || !bytes.HasPrefix(data, []byte("\x89PNG")) {
		t.Errorf("screenshot is not a PNG (%v)", err)
	}
	must("console", "list", "--limit", "5")
	must("relaunch")
	if out := must("run", "tail"); !strings.Contains(out, "relaunch") {
		t.Errorf("run log lacks the relaunch:\n%s", out)
	}
}
