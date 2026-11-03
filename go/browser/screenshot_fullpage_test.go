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
// (clientWidth, which excludes a classic scrollbar) and the document height
// before capturing, clips to the pre-override content size, and undoes the
// override and the scroll clamp afterwards.
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

	override, capture, clear := idx("Emulation.setDeviceMetricsOverride"), idx("Page.captureScreenshot"), idx("Emulation.clearDeviceMetricsOverride")
	if override < 0 || capture < 0 || clear < 0 || !(override < capture && capture < clear) {
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
	if dm.Width != 1153 || dm.Height != 26212 || dm.DeviceScaleFactor != 0 || dm.Mobile {
		t.Errorf("setDeviceMetricsOverride = %+v, want 1153x26212, deviceScaleFactor 0, not mobile", dm)
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

// TestFullPageScreenshotMatchesLayout_Chrome checks against a real Chrome
// that an element's box, measured before a full-page screenshot, is where the
// screenshot draws it on a page whose height depends on its width, and that
// the page is left at its own layout. With captureBeyondViewport alone the
// classic scrollbar's 15 px go to the layout and the marker is drawn 60 px
// below its box.
//
// Opt-in: SIGHTMAP_CHROME_E2E=1 (uses FindChrome, or SIGHTMAP_CHROME).
func TestFullPageScreenshotMatchesLayout_Chrome(t *testing.T) {
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
	defer cancel()
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
	defer conn.Close()

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
			var p struct{ Width, Height int }
			_ = json.Unmarshal(params, &p)
			overrides = append(overrides, fmt.Sprintf("%dx%d", p.Width, p.Height))
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
	if want := []string{"800x600", "785x5000", "800x600"}; fmt.Sprint(overrides) != fmt.Sprint(want) {
		t.Errorf("setDeviceMetricsOverride sizes = %v, want %v", overrides, want)
	}
	if cleared {
		t.Error("clearDeviceMetricsOverride called; the resize should have been re-applied")
	}
}
