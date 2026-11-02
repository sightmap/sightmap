package device_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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

	var udid string
	t.Cleanup(func() {
		run("stop")
		if os.Getenv("SIGHTMAP_DEVICE_E2E_KEEP_BOOTED") == "" && udid != "" {
			exec.Command("xcrun", "simctl", "shutdown", udid).Run()
		}
	})
	must("start", "--device", sim, "--app", app, "--detach", "--label", "e2e")
	var session struct{ UDID string }
	data, _ := os.ReadFile(filepath.Join(work, ".sightmap", ".device-session"))
	if json.Unmarshal(data, &session); session.UDID == "" {
		t.Fatalf("no udid in the session file:\n%s", data)
	}
	udid = session.UDID
	simctl := func(args ...string) (string, error) {
		out, err := exec.Command("xcrun", append([]string{"simctl", "spawn", udid}, args...)...).CombinedOutput()
		return string(out), err
	}

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

	// A click right after a full-screen modal starts presenting, and another
	// right after it starts going away, land only if each waits for the
	// transition to end; UIKit drops touches until then.
	must("click", "OfferButton")
	must("click", "ClaimOfferButton")
	snapshotUntil(`[ClaimOfferButton "Claimed"]`)
	must("click", "CloseOfferButton")
	must("click", "AddToCartButton")
	snapshotUntil(`[AddToCartButton "Added"]`)

	time.Sleep(time.Second)
	expect(must("console", "list", "--grep", "Product 150"), "added Product 150 to cart", "claimed the offer on Product 150")
	expect(must("console", "list", "--grep", "signed in"), "signed in as agent@example.com")

	// A segmented control comes out of process without its segments; a Form
	// toggle's center is its label, which ignores taps, and its switch toggles
	// reliably only on a press.
	must("click", "tab-account")
	must("click", "account-controls")
	snapshotUntil(`[RangeSegment title="Week"]`)
	must("click", `RangeSegment[title="Week"]`)
	snapshotUntil(`[RangeSegment title="Week"] (selected)`)
	must("click", "NotifySwitch")
	time.Sleep(time.Second)
	expect(must("console", "list", "--grep", "notify"), "notify off")

	// The Network tab's requests go to this server instead of httpbin.org.
	// Fetch Report's response is large enough that CFNetwork logs its HAR
	// entry in several parts.
	report := `{"rows":"` + strings.Repeat("x", 200<<10-11) + `"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			io.WriteString(w, report)
			return
		}
		body, _ := io.ReadAll(r.Body)
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "sample-cookie"})
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"json": json.RawMessage(body), "url": r.URL.Path})
	}))
	defer server.Close()
	if out, err := simctl("defaults", "write", "dev.sightmap.SimSample", "networkBaseURL", server.URL+"/anything"); err != nil {
		t.Fatalf("defaults write: %v\n%s", err, out)
	}
	simctl("defaults", "delete", "dev.sightmap.SimSample", "networkFetchPaths")
	expect(must("status"), "har      active")
	// iOS 26 can expose the SwiftUI tab bar as one element without its tabs;
	// Network is the second of four.
	if _, err := run("click", "tab-network"); err != nil {
		var snap struct {
			Result struct{ Tree map[string]any }
		}
		json.Unmarshal([]byte(must("snapshot", "--json")), &snap)
		bar := findNamed(snap.Result.Tree, "Tab Bar")
		if bar == nil {
			t.Fatal("no tab bar to switch to the Network tab")
		}
		b := bar["bounds"].(map[string]any)
		x, y := b["x"].(float64)+b["width"].(float64)*3/8, b["y"].(float64)+b["height"].(float64)/2
		must("tap-xy", strconv.FormatFloat(x, 'f', 0, 64), strconv.FormatFloat(y, 'f', 0, 64))
	}
	must("click", "network-send")
	snapshotUntil(`"HTTP 201"`)
	var list string
	for range 10 {
		if list = must("network", "list"); strings.Contains(list, "[CreateOrder]") {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	expect(list, "[CreateOrder]", "POST "+server.URL+"/anything/sightmap/order-1 → 201", "har)", "{order=1, echoed=1}")
	m := regexp.MustCompile(`#(\d+) \[CreateOrder\]`).FindStringSubmatch(list)
	if m == nil {
		t.Fatalf("no CreateOrder entry:\n%s", list)
	}
	redacted := must("network", "get", m[1])
	expect(redacted, "Authorization: <redacted>", "Set-Cookie: <redacted>", `"sessionToken":"<redacted>"`, "order = 1")
	if strings.Contains(redacted, "sample-not-a-secret") || strings.Contains(redacted, "sample-cookie") {
		t.Fatalf("network get leaks a credential:\n%s", redacted)
	}
	expect(must("network", "get", m[1], "--unredacted"), "Authorization: Bearer sample-not-a-secret")
	expect(must("network", "list", "--json", "--grep", "createorder"), `"matches":["CreateOrder"]`, `"source":"har"`)

	must("click", "network-fetch")
	snapshotUntil(`"Fetched 1"`)
	reportLine := regexp.MustCompile(`(?m)^#(\d+) \[Report\] +GET ` + regexp.QuoteMeta(server.URL) + `/anything/sightmap/report → 200 \(.*\)$`)
	for range 10 {
		if list = must("network", "list"); reportLine.MatchString(list) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	m = reportLine.FindStringSubmatch(list)
	if m == nil || !strings.HasSuffix(m[0], "har)") {
		t.Fatalf("no har entry for the report:\n%s", list)
	}
	expect(must("network", "get", m[1]), fmt.Sprintf("Response body (application/json, %d bytes", len(report)))

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
	data, _ = os.ReadFile(logs[0])
	if !strings.Contains(string(data), `"text":"\u003credacted\u003e"`) || strings.Contains(string(data), "hunter2") {
		t.Fatalf("the password was not redacted in the run log:\n%s", data)
	}
	if strings.Contains(string(data), "sample-not-a-secret") || strings.Contains(string(data), "sessionToken") {
		t.Fatalf("a network body reached the run log:\n%s", data)
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

	// The Catalog tab's rows join a title and a maker that both contain ", "
	// into one label. Without the probe the corpus still names each row and
	// leaves unset the fields it reads from the row's Texts; with it, they are
	// read exactly.
	signIn := func() {
		t.Helper()
		snapshotUntil("[View: Login]")
		must("fill", "login-email", "agent@example.com")
		must("fill", "login-password", "hunter2")
		must("click", "login-submit")
		for i := 0; i < 15; i++ {
			time.Sleep(time.Second)
			snap, _ := run("snapshot")
			if strings.Contains(snap, "[Overlay]") {
				run("click", `button[text="Not Now"]`)
				continue
			}
			if strings.Contains(snap, "[View: Products]") && i >= 3 {
				return
			}
		}
		t.Fatal("sign-in never reached the product list")
	}
	signIn()
	must("click", "tab-catalog")
	plain := snapshotUntil("[View: Catalog]")
	expect(plain, `[CatalogRow summary="Socks, Wool, Pair of 2, Acme, Inc., $9.99"]`)
	if strings.Contains(plain, "maker=") || strings.Contains(plain, "[Probe]") {
		t.Fatalf("without --probe the rows should carry only their joined label:\n%s", plain)
	}

	must("stop")
	expect(must("start", "--device", sim, "--app", app, "--probe", "--detach", "--label", "e2e-probe"), "probe:   loaded")
	signIn()
	must("click", "tab-catalog")
	rich := snapshotUntil(`title="Socks, Wool, Pair of 2"`)
	expect(rich,
		`[CatalogRow maker="Acme, Inc." price="9.99" summary="Socks, Wool, Pair of 2, Acme, Inc., $9.99" title="Socks, Wool, Pair of 2"]`,
		`[CatalogRow maker="Bright, Light & Co." price="24.5" summary="Desk Lamp, Bright, Light & Co., $24.50" title="Desk Lamp"]`,
		`view[type="CatalogRow"]`)
	expect(must("status"), "probe:   loaded", "warm-up")
	// A Text only the probe reports is a target like any other.
	must("click", `text[text="Kiln"]`)
	snapshotUntil(`"Item 3"`)

	must("stop")
	if out, err := simctl("defaults", "read", "com.apple.CFNetwork", "har-capture-global"); err == nil {
		t.Errorf("stop left HAR capture on: %s", out)
	}
	if out, _ := simctl("log", "config", "--status", "--subsystem", "com.apple.CFNetwork", "--category", "HAR"); !strings.Contains(out, "OFF") {
		t.Errorf("stop left the HAR log category on: %s", out)
	}
	simctl("defaults", "delete", "dev.sightmap.SimSample", "networkBaseURL")
}

func findNamed(n map[string]any, name string) map[string]any {
	if n["name"] == name {
		return n
	}
	children, _ := n["children"].([]any)
	for _, c := range children {
		if child, ok := c.(map[string]any); ok {
			if found := findNamed(child, name); found != nil {
				return found
			}
		}
	}
	return nil
}
