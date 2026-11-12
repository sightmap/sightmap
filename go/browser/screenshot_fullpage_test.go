package browser

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"math"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// A full-page screenshot pins the viewport to the page's own layout width
// (clientWidth, which excludes a classic scrollbar) and leaves its height at
// the window's, so vh units resolve as the probe saw them. It clips to the
// pre-override content size and undoes the override afterwards.
func TestScreenshotWithOptions_FullPagePinsLayoutWidth(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("fake"))

	type call struct {
		method string
		params json.RawMessage
	}
	var mu sync.Mutex
	var calls []call

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		mu.Lock()
		calls = append(calls, call{method, params})
		mu.Unlock()
		switch method {
		case "Page.getLayoutMetrics":
			return json.RawMessage(`{
				"cssContentSize": {"x": 0, "y": 0, "width": 1153, "height": 26212},
				"cssLayoutViewport": {"pageX": 0, "pageY": 3000, "clientWidth": 1153, "clientHeight": 913}
			}`)
		case "Page.captureScreenshot":
			return json.RawMessage(`{"data":"` + encoded + `"}`)
		}
		return json.RawMessage(`{}`)
	})

	if _, err := ScreenshotWithOptions(context.Background(), conn, ScreenshotOptions{FullPage: true}); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	got := append([]call(nil), calls...)
	mu.Unlock()
	idx := func(method string) int {
		for i, c := range got {
			if c.method == method {
				return i
			}
		}
		return -1
	}
	var order []string
	for _, c := range got {
		order = append(order, c.method)
	}

	override, capture, cleared := idx("Emulation.setDeviceMetricsOverride"), idx("Page.captureScreenshot"), idx("Emulation.clearDeviceMetricsOverride")
	if override < 0 || capture < 0 || cleared < 0 || !(override < capture && capture < cleared) {
		t.Fatalf("want setDeviceMetricsOverride < captureScreenshot < clearDeviceMetricsOverride; calls: %v", order)
	}

	var dm struct {
		Width, Height     int
		DeviceScaleFactor float64
		Mobile            bool
	}
	if err := json.Unmarshal(got[override].params, &dm); err != nil {
		t.Fatal(err)
	}
	if dm.Width != 1153 || dm.Height != 913 || dm.DeviceScaleFactor != 0 || dm.Mobile {
		t.Errorf("setDeviceMetricsOverride = %+v, want 1153x913 (the layout viewport), deviceScaleFactor 0, not mobile", dm)
	}

	var shot struct {
		CaptureBeyondViewport bool `json:"captureBeyondViewport"`
		Clip                  struct{ X, Y, Width, Height, Scale float64 }
	}
	if err := json.Unmarshal(got[capture].params, &shot); err != nil {
		t.Fatal(err)
	}
	if c := shot.Clip; c.X != 0 || c.Y != 0 || c.Width != 1153 || c.Height != 26212 || c.Scale != 1 {
		t.Errorf("clip = %+v, want {0 0 1153 26212 1}", c)
	}
	if !shot.CaptureBeyondViewport {
		t.Error("want captureBeyondViewport:true")
	}

	var hidden []bool
	for _, c := range got {
		if c.method == "Emulation.setScrollbarsHidden" {
			var p struct{ Hidden bool }
			_ = json.Unmarshal(c.params, &p)
			hidden = append(hidden, p.Hidden)
		}
	}
	if len(hidden) != 2 || !hidden[0] || hidden[1] {
		t.Errorf("setScrollbarsHidden = %v, want [true false]", hidden)
	}

	scrolled := false
	for _, c := range got[capture:] {
		if c.method == "Runtime.evaluate" && strings.Contains(string(c.params), "scrollTo(0, 3000)") {
			scrolled = true
		}
	}
	if !scrolled {
		t.Errorf("want the scroll position (0, 3000) restored after capture; calls: %v", order)
	}
}

// fullPageChrome starts a headless Chrome at a 700x600 window, DPR 2, and
// returns a context bounding the test and a connection to its tab. It skips
// unless SIGHTMAP_CHROME_E2E=1.
func fullPageChrome(t *testing.T) (context.Context, *CDPConn) {
	t.Helper()
	if os.Getenv("SIGHTMAP_CHROME_E2E") == "" {
		t.Skip("set SIGHTMAP_CHROME_E2E=1 to run against a real Chrome")
	}
	bin := os.Getenv("SIGHTMAP_CHROME")
	if bin == "" {
		var err error
		if bin, err = FindChrome(); err != nil {
			t.Skip(err)
		}
	}
	// Not t.TempDir: Chrome's helper processes can still be writing the
	// profile when the cleanup runs, which fails the test.
	profile, err := os.MkdirTemp("", "sightmap-fullpage-")
	if err != nil {
		t.Fatal(err)
	}
	port, err := FindFreePort(9600)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "--headless=new", "--no-first-run", "--no-default-browser-check",
		"--disable-gpu", "--no-sandbox", "--window-size=700,600", "--force-device-scale-factor=2",
		fmt.Sprintf("--remote-debugging-port=%d", port), "--user-data-dir="+profile, "about:blank")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = os.RemoveAll(profile)
	})
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	var conn *CDPConn
	for conn == nil {
		if ctx.Err() != nil {
			t.Fatal("Chrome's CDP endpoint never came up")
		}
		if _, c, err := CreateTab(ctx, addr, ""); err == nil {
			conn = c
		} else {
			time.Sleep(200 * time.Millisecond)
		}
	}
	t.Cleanup(func() { conn.Close() })

	return ctx, conn
}

// TestFullPageScreenshotMatchesLayout_Chrome checks against a real Chrome
// that an element's box, measured before a full-page screenshot, is where the
// screenshot draws it on a page whose height depends on its width, and that
// the page is left at its own layout. With captureBeyondViewport alone the
// classic scrollbar's 15 px go to the layout and the marker is drawn 60 px
// below its box.
//
// Opt-in: SIGHTMAP_CHROME_E2E=1 (uses FindChrome, or SIGHTMAP_CHROME).
func TestFullPageScreenshotMatchesLayout_Chrome(t *testing.T) {
	ctx, conn := fullPageChrome(t)

	// The tall block's height follows the layout width, as wrapped text does
	// on a long article, so any change of width moves the marker below it.
	page := `<!doctype html><html><body style="margin:0">` +
		`<div style="aspect-ratio:1/4;background:#eee"></div>` +
		`<div id="marker" style="height:40px;background:#f00"></div>` +
		`<div style="height:200px"></div></body></html>`
	if err := NavigateAndWaitIdle(ctx, conn, "data:text/html,"+url.PathEscape(page), 5*time.Second); err != nil {
		t.Fatal(err)
	}

	raw, err := EvalJSON(ctx, conn, `(() => {
		const r = document.getElementById("marker").getBoundingClientRect();
		return {top: r.top + scrollY, scrollbar: innerWidth - document.documentElement.clientWidth, dpr: devicePixelRatio};
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var m struct{ Top, Scrollbar, DPR float64 }
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.Scrollbar == 0 {
		t.Skip("this Chrome draws overlay scrollbars; the layout cannot change width")
	}

	shot, err := ScreenshotWithOptions(ctx, conn, ScreenshotOptions{Format: "png", FullPage: true})
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(shot))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	x := b.Min.X + 10
	drawn := -1.0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		r, g, bl, _ := img.At(x, y).RGBA()
		if r>>8 > 200 && g>>8 < 60 && bl>>8 < 60 {
			drawn = float64(y-b.Min.Y) / m.DPR
			break
		}
	}
	if drawn < 0 {
		t.Fatalf("marker not found in the %dx%d screenshot", b.Dx(), b.Dy())
	}
	if math.Abs(drawn-m.Top) > 1 {
		t.Errorf("marker box top %.0f CSS px, drawn at %.1f: the screenshot was laid out differently", m.Top, drawn)
	}

	raw, err = EvalJSON(ctx, conn, `innerWidth - document.documentElement.clientWidth`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != fmt.Sprint(m.Scrollbar) {
		t.Errorf("scrollbar width after capture = %s, want %v (page left at another layout)", raw, m.Scrollbar)
	}
}

// A viewport this session set with ResizeViewport survives a full-page
// screenshot: restore re-applies it instead of clearing the override.
func TestScreenshotWithOptions_FullPageKeepsResize(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("fake"))
	var mu sync.Mutex
	var overrides []string
	cleared := false
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		mu.Lock()
		defer mu.Unlock()
		switch method {
		case "Emulation.setDeviceMetricsOverride":
			var p struct {
				Width, Height     int
				DeviceScaleFactor float64
			}
			_ = json.Unmarshal(params, &p)
			overrides = append(overrides, fmt.Sprintf("%dx%d@%g", p.Width, p.Height, p.DeviceScaleFactor))
		case "Emulation.clearDeviceMetricsOverride":
			cleared = true
		case "Page.getLayoutMetrics":
			return json.RawMessage(`{"cssContentSize": {"width": 800, "height": 5000},
				"cssLayoutViewport": {"clientWidth": 785, "clientHeight": 600}}`)
		case "Page.captureScreenshot":
			return json.RawMessage(`{"data":"` + encoded + `"}`)
		}
		return json.RawMessage(`{}`)
	})
	ctx := context.Background()
	if err := ResizeViewport(ctx, conn, 800, 600); err != nil {
		t.Fatal(err)
	}
	if _, err := ScreenshotWithOptions(ctx, conn, ScreenshotOptions{FullPage: true}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"800x600@1", "785x600@1", "800x600@1"}; fmt.Sprint(overrides) != fmt.Sprint(want) {
		t.Errorf("setDeviceMetricsOverride sizes = %v, want %v", overrides, want)
	}
	if cleared {
		t.Error("clearDeviceMetricsOverride called; the resize should have been re-applied")
	}
}

// TestFullPageScreenshotKeepsViewportUnits_Chrome checks against a real Chrome
// that a full-page screenshot draws vh-sized and viewport-fixed elements where
// the probe measured them. Pinning the viewport to the document's height made
// a 50vh block as tall as half the document and moved a bottom:0 bar to the
// document's end, so everything under the block was drawn far below its box.
//
// Opt-in: SIGHTMAP_CHROME_E2E=1 (uses FindChrome, or SIGHTMAP_CHROME).
func TestFullPageScreenshotKeepsViewportUnits_Chrome(t *testing.T) {
	ctx, conn := fullPageChrome(t)

	page := `<!doctype html><html><body style="margin:0">` +
		`<div id="hero" style="height:50vh;background:#00f"></div>` +
		`<div id="marker" style="height:40px;background:#f00"></div>` +
		`<div style="height:3000px;background:#ccc"></div>` +
		`<div id="bar" style="position:fixed;bottom:0;left:0;right:0;height:30px;background:#0a0"></div>` +
		`</body></html>`
	if err := NavigateAndWaitIdle(ctx, conn, "data:text/html,"+url.PathEscape(page), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	raw, err := EvalJSON(ctx, conn, `(() => {
		const top = id => document.getElementById(id).getBoundingClientRect().top + scrollY;
		return {marker: top("marker"), bar: top("bar"), dpr: devicePixelRatio};
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var m struct{ Marker, Bar, DPR float64 }
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}

	shot, err := ScreenshotWithOptions(ctx, conn, ScreenshotOptions{Format: "png", FullPage: true})
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(shot))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	firstRow := func(match func(r, g, bl uint32) bool) float64 {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			r, g, bl, _ := img.At(b.Min.X+10, y).RGBA()
			if match(r>>8, g>>8, bl>>8) {
				return float64(y-b.Min.Y) / m.DPR
			}
		}
		return -1
	}
	red := firstRow(func(r, g, bl uint32) bool { return r > 200 && g < 60 && bl < 60 })
	green := firstRow(func(r, g, bl uint32) bool { return g > 130 && r < 60 && bl < 60 })
	if red < 0 || green < 0 {
		t.Fatalf("marker (%.1f) or bar (%.1f) not found in the %dx%d screenshot", red, green, b.Dx(), b.Dy())
	}
	if math.Abs(red-m.Marker) > 1 {
		t.Errorf("marker below a 50vh block: box top %.0f CSS px, drawn at %.1f: the block was drawn at another height", m.Marker, red)
	}
	if math.Abs(green-m.Bar) > 1 {
		t.Errorf("fixed bottom bar: box top %.0f CSS px, drawn at %.1f", m.Bar, green)
	}
}
