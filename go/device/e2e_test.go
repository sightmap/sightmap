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

// TestE2E drives the sample app (testdata/SampleApp) through the sightmap CLI
// on a real simulator. It runs only when SIGHTMAP_DEVICE_E2E names one (a name
// or UDID), which it shuts down at the end unless SIGHTMAP_DEVICE_E2E_KEEP_BOOTED
// is set. Pick a simulator nobody else is using:
//
//	SIGHTMAP_DEVICE_E2E="iPhone 16e" go test ./device -run TestE2E -v -timeout 15m
func TestE2E(t *testing.T) {
	sim := os.Getenv("SIGHTMAP_DEVICE_E2E")
	if sim == "" {
		t.Skip("set SIGHTMAP_DEVICE_E2E to a simulator name or UDID to run the end-to-end test")
	}
	work := t.TempDir()
	bin := filepath.Join(work, "sightmap")
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/sightmap").CombinedOutput(); err != nil {
		t.Fatalf("build sightmap: %v\n%s", err, out)
	}
	fixture, _ := filepath.Abs("testdata/SampleApp")
	out, err := exec.Command(filepath.Join(fixture, "build.sh")).Output()
	if err != nil {
		t.Fatalf("build the sample app: %v", err)
	}
	app := strings.TrimSpace(string(out))
	if err := os.CopyFS(filepath.Join(work, ".sightmap"), os.DirFS(filepath.Join(fixture, ".sightmap"))); err != nil {
		t.Fatal(err)
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
	expect := func(out string, wants ...string) {
		t.Helper()
		for _, want := range wants {
			if !strings.Contains(out, want) {
				t.Fatalf("output lacks %q:\n%s", want, out)
			}
		}
	}
	// snapshotUntil re-snapshots while the screen is still animating.
	snapshotUntil := func(want string) string {
		t.Helper()
		var out string
		for range 10 {
			out, _ = run("snapshot")
			if strings.Contains(out, want) {
				return out
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Fatalf("no snapshot showed %q", want)
		return out
	}

	t.Cleanup(func() {
		if os.Getenv("SIGHTMAP_DEVICE_E2E_KEEP_BOOTED") != "" {
			run("stop")
		} else {
			run("stop", "--shutdown")
		}
	})
	must("start", "--device", sim, "--app", app, "--detach", "--label", "e2e")

	snap := snapshotUntil("[View: Login]")
	expect(snap, `[SignInButton "Sign In"]`, "[EmailField]")
	expect(must("snapshot", "--json"), `"ok":true`, `"component":"SignInButton"`)

	must("click", "SignInButton")
	snapshotUntil("[LoginError")

	must("fill", "login-email", "agent@example.com")
	expect(must("fill", "login-password", "hunter2"), "secure characters")
	must("click", "login-submit")

	// iOS offers to save the password a few seconds after the login form goes
	// away, over the product list; a tap while it animates in can miss.
	sheetSeen := false
	for i := 0; i < 15; i++ {
		time.Sleep(time.Second)
		snap, _ = run("snapshot")
		if strings.Contains(snap, "[Overlay]") {
			sheetSeen = true
			run("click", `button[text="Not Now"]`)
			continue
		}
		if strings.Contains(snap, "[View: Products]") && (sheetSeen || i >= 4) {
			break
		}
	}
	expect(snap, "[View: Products]")
	expect(snap, "[ProductRow")
	expect(snap, `[TabBarItem title="Products"] (selected)`, `[TabBarItem title="Account"]`)

	refused, err := run("click", "product-row-150")
	if err == nil {
		t.Fatal("a click on a row further down the list was not refused")
	}
	expect(refused, "scroll down --until")

	must("scroll", "down", "--until", `ProductRow[title^="Product 150,"]`)
	must("click", "product-row-150")
	snapshotUntil(`[ProductName "Product 150"]`)
	must("click", "AddToCartButton")

	time.Sleep(time.Second)
	expect(must("console", "list", "--grep", "Product 150"), "added Product 150 to cart")
	expect(must("console", "list", "--grep", "signed in"), "signed in as agent@example.com")

	shot := filepath.Join(work, "shot.png")
	must("screenshot", "--out", shot)
	if data, err := os.ReadFile(shot); err != nil || !bytes.HasPrefix(data, []byte("\x89PNG")) {
		t.Fatalf("screenshot is not a PNG: %v", err)
	}

	runlog := must("run", "tail")
	expect(runlog, "click → ", "[AddToCartButton]", "(view ProductDetail)")
	logs, _ := filepath.Glob(filepath.Join(work, ".sightmap", ".device-runs", "*.jsonl"))
	if len(logs) != 1 {
		t.Fatalf("want one run log, got %v", logs)
	}
	data, _ := os.ReadFile(logs[0])
	if !strings.Contains(string(data), `"text":"\u003credacted\u003e"`) || strings.Contains(string(data), "hunter2") {
		t.Fatalf("the password was not redacted in the run log:\n%s", data)
	}

	expect(must("status"), "session running", "dev.sightmap.SimSample")
	expect(must("start", "--app", app, "--detach"), "installed "+app+" and relaunched")
	switched, err := run("start", "--bundle-id", "com.apple.Preferences", "--detach")
	if err == nil {
		t.Fatal("start with another app on a running session was not refused")
	}
	expect(switched, "sightmap device stop")
	offscreen, _ := run("tap-xy", "230", "5000")
	expect(offscreen, "not pixels")
}
