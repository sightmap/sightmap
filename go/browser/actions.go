package browser

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/sightmap"
)

//go:embed actions.js
var actionsJSSource string

// actionsJS is actionsJSSource with its __smDeepQuery dependency (deepquery.js)
// composed in once here, so every call site below just does actionsJS+<call>
// -- there's no "did I remember to prepend DeepQueryJS" to get wrong.
var actionsJS = DeepQueryJS + "\n" + actionsJSSource

// Navigate sends Page.navigate. Does NOT wait for load.
func Navigate(ctx context.Context, conn *CDPConn, url string) error {
	_, err := conn.call(ctx, "Page.navigate", map[string]interface{}{"url": url})
	return err
}

// WaitForLoad blocks until Page.loadEventFired or ctx done.
// Calls EnableDomain("Page") first.
func WaitForLoad(ctx context.Context, conn *CDPConn) error {
	if err := conn.EnableDomain(ctx, "Page"); err != nil {
		return fmt.Errorf("WaitForLoad: enable Page domain: %w", err)
	}
	ch := conn.Subscribe("Page.loadEventFired")
	defer conn.Unsubscribe("Page.loadEventFired", ch)

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-conn.done:
		return fmt.Errorf("cdp: connection closed while waiting for load")
	}
}

// NavigateAndWait subscribes to Page.loadEventFired BEFORE navigating
// (avoids race where fast pages fire the event before we subscribe).
func NavigateAndWait(ctx context.Context, conn *CDPConn, url string) error {
	if err := conn.EnableDomain(ctx, "Page"); err != nil {
		return fmt.Errorf("NavigateAndWait: enable Page domain: %w", err)
	}
	ch := conn.Subscribe("Page.loadEventFired")
	defer conn.Unsubscribe("Page.loadEventFired", ch)

	if err := Navigate(ctx, conn, url); err != nil {
		return err
	}

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-conn.done:
		return fmt.Errorf("cdp: connection closed while waiting for load")
	}
}

// AwaitNavigation waits up to maxWait for the page to navigate — either a full
// document navigation (Page.frameNavigated) or a same-document SPA navigation
// via history.pushState/replaceState (Page.navigatedWithinDocument). It returns
// the settled final URL and true once a navigation is observed, or ("", false)
// if none happens within maxWait. After the first navigation it waits a short
// grace for chained redirects (e.g. /login → / → /workspace) to settle, so the
// returned URL is the final destination rather than an intermediate hop.
//
// This surfaces async navigation that a click or a client-side redirect triggers
// AFTER the initial load event has already fired — the signal an agent needs to
// know an action actually moved the page. Callers should compare the result
// against the pre-action URL and only report a move when it actually changed.
func AwaitNavigation(ctx context.Context, conn *CDPConn, maxWait time.Duration) (string, bool) {
	if err := conn.EnableDomain(ctx, "Page"); err != nil {
		return "", false
	}
	frameCh := conn.Subscribe("Page.frameNavigated")
	defer conn.Unsubscribe("Page.frameNavigated", frameCh)
	docCh := conn.Subscribe("Page.navigatedWithinDocument")
	defer conn.Unsubscribe("Page.navigatedWithinDocument", docCh)

	const grace = 400 * time.Millisecond
	overall := time.NewTimer(maxWait)
	defer overall.Stop()
	var graceTimer *time.Timer
	var graceCh <-chan time.Time // nil until the first navigation; a nil channel never selects
	navigated := false

	for {
		select {
		case <-frameCh:
			navigated = true
		case <-docCh:
			navigated = true
		case <-graceCh:
			url, _ := GetURL(ctx, conn)
			return url, true
		case <-overall.C:
			if navigated {
				url, _ := GetURL(ctx, conn)
				return url, true
			}
			return "", false
		case <-ctx.Done():
			return "", false
		case <-conn.done:
			return "", false
		}
		// A navigation was seen; (re)arm the grace timer to absorb chained
		// redirects, and return once navigations go quiet for `grace`.
		if graceTimer == nil {
			graceTimer = time.NewTimer(grace)
			defer graceTimer.Stop()
		} else {
			if !graceTimer.Stop() {
				select {
				case <-graceTimer.C:
				default:
				}
			}
			graceTimer.Reset(grace)
		}
		graceCh = graceTimer.C
	}
}

// GetURL returns the current page URL.
// Uses Target.getTargetInfo — the result has a targetInfo.url field.
// BringToFront foregrounds the tab (CDP Page.bringToFront), making it the active,
// VISIBLE tab in its window. This matters for more than looks: a backgrounded tab
// is throttled to visibilityState "hidden", where Chrome starves
// requestAnimationFrame (observed at 0 frames/s on a detached jetblue session).
// Any interaction that needs frames then degrades or no-ops -- scrollIntoView
// can't paint, elementFromPoint reads a stale layout, and a widget's own
// rAF-driven open/commit animation never advances (the custom <select> options
// that "click" but don't commit). Foregrounding the tab restores a live frame
// clock, so it is the general fix for that whole class, not a per-widget hack.
func BringToFront(ctx context.Context, conn *CDPConn) error {
	if _, err := conn.call(ctx, "Page.bringToFront", map[string]interface{}{}); err != nil {
		return fmt.Errorf("BringToFront: %w", err)
	}
	return nil
}

func GetURL(ctx context.Context, conn *CDPConn) (string, error) {
	result, err := conn.call(ctx, "Target.getTargetInfo", map[string]interface{}{})
	if err != nil {
		return "", fmt.Errorf("GetURL: %w", err)
	}

	var resp struct {
		TargetInfo struct {
			URL string `json:"url"`
		} `json:"targetInfo"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return "", fmt.Errorf("GetURL: unmarshal: %w", err)
	}
	return resp.TargetInfo.URL, nil
}

// NavigateAndWaitIdle navigates to url and waits for both loadEventFired and
// networkIdle (React/SPA safe). Subscribes before navigating to avoid the race
// where a fast SPA router settles before we subscribe.
func NavigateAndWaitIdle(ctx context.Context, conn *CDPConn, url string, maxWait time.Duration) error {
	if err := conn.EnableDomain(ctx, "Page"); err != nil {
		return fmt.Errorf("NavigateAndWaitIdle: enable Page: %w", err)
	}
	if _, err := conn.call(ctx, "Page.setLifecycleEventsEnabled", map[string]interface{}{
		"enabled": true,
	}); err != nil {
		_ = err // non-fatal
	}

	// Subscribe BEFORE navigating
	loadCh := conn.Subscribe("Page.loadEventFired")
	defer conn.Unsubscribe("Page.loadEventFired", loadCh)
	lifeCh := conn.Subscribe("Page.lifecycleEvent")
	defer conn.Unsubscribe("Page.lifecycleEvent", lifeCh)

	if err := Navigate(ctx, conn, url); err != nil {
		return err
	}

	deadline := time.NewTimer(maxWait)
	defer deadline.Stop()

	var loadFired bool
	var networkIdle bool

	for {
		if loadFired && networkIdle {
			return nil
		}
		select {
		case <-loadCh:
			loadFired = true
		case raw, ok := <-lifeCh:
			if !ok {
				return nil
			}
			var ev struct {
				Name    string `json:"name"`
				FrameId string `json:"frameId"`
			}
			if err := json.Unmarshal(raw, &ev); err == nil {
				if ev.Name == "networkIdle" {
					networkIdle = true
				}
			}
		case <-deadline.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-conn.done:
			return nil
		}
	}
}

// EvalJSON evaluates script, unwraps Runtime.evaluate's double-envelope,
// returns the raw JSON value. Errors on JS exception.
//
// awaitPromise is on: when the expression evaluates to a Promise, CDP resolves
// it and returns the settled value (a rejection surfaces as a JS exception).
// This is a no-op for synchronous results, so driving async page/tool code
// (e.g. an awaited executeTool()) returns the real value instead of an opaque
// unresolved-promise handle. Callers bound long-running work via ctx.
func EvalJSON(ctx context.Context, conn *CDPConn, script string) (json.RawMessage, error) {
	result, err := conn.call(ctx, "Runtime.evaluate", map[string]interface{}{
		"expression":    script,
		"returnByValue": true,
		"awaitPromise":  true,
	})
	if err != nil {
		return nil, err
	}

	// CDP Runtime.evaluate response: {"result":{"type":"...","value":{...}},"exceptionDetails":{...}}
	var evalResp struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(result, &evalResp); err != nil {
		return nil, fmt.Errorf("EvalJSON: unmarshal eval response: %w", err)
	}
	if evalResp.ExceptionDetails != nil {
		return nil, fmt.Errorf("EvalJSON: JS exception: %s", evalResp.ExceptionDetails.Text)
	}
	return evalResp.Result.Value, nil
}

// DispatchMouseMove sends a CDP mouseMoved event to the current page.
func DispatchMouseMove(ctx context.Context, conn *CDPConn, x, y float64) error {
	_, err := conn.call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
		"type": "mouseMoved", "x": x, "y": y, "buttons": 0,
	})
	return err
}

// DispatchMouseScroll sends a CDP mouseWheel event to the current page.
func DispatchMouseScroll(ctx context.Context, conn *CDPConn, x, y, deltaX, deltaY float64) error {
	_, err := conn.call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
		"type": "mouseWheel", "x": x, "y": y, "deltaX": deltaX, "deltaY": deltaY,
	})
	return err
}

// ClearBrowserCookies clears ALL cookies in the browser profile via CDP,
// including httpOnly cookies that JavaScript document.cookie cannot touch.
func ClearBrowserCookies(ctx context.Context, conn *CDPConn) error {
	if _, err := conn.call(ctx, "Network.enable", map[string]interface{}{}); err != nil {
		return fmt.Errorf("ClearBrowserCookies: enable: %w", err)
	}
	if _, err := conn.call(ctx, "Network.clearBrowserCookies", map[string]interface{}{}); err != nil {
		return fmt.Errorf("ClearBrowserCookies: %w", err)
	}
	return nil
}

// ClearStorageForOrigin clears localStorage, IndexedDB, service-worker caches,
// and other origin-scoped storage for the given origin URL via CDP.
func ClearStorageForOrigin(ctx context.Context, conn *CDPConn, origin string) error {
	_, err := conn.call(ctx, "Storage.clearDataForOrigin", map[string]interface{}{
		"origin":       origin,
		"storageTypes": "all",
	})
	if err != nil {
		return fmt.Errorf("ClearStorageForOrigin(%s): %w", origin, err)
	}
	return nil
}

// ScreenshotClip is the region to capture, in viewport-relative CSS pixels —
// the same coordinate space as Element.getBoundingClientRect and the
// `browser bounds` command. ScreenshotWithOptions converts it to the
// document-relative clip CDP expects by adding the current scroll offset.
type ScreenshotClip struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// ScreenshotOptions controls how Page.captureScreenshot is issued.
type ScreenshotOptions struct {
	Format           string          // "png" (default) or "jpeg"
	Quality          int             // JPEG quality 0-100 (ignored for PNG); 0 → 80
	OptimizeForSpeed bool            // CDP optimizeForSpeed flag — faster at cost of quality
	PauseAnimations  bool            // send Animation.setPlaybackRate(0) before capture
	StopLoading      bool            // call Page.stopLoading() before capture to halt ad/iframe repaints
	Clip             *ScreenshotClip // nil = full viewport; otherwise clip to this (viewport-relative) box
	FullPage         bool            // capture the whole scrollable document, not just the viewport (excludes Clip)
}

// Screenshot captures the viewport as PNG, returns raw bytes.
// fromSurface+captureBeyondViewport:false avoids hanging on pages with heavy
// cross-origin iframes (e.g. PayPal on PDPs), but can stall on file:// pages.
// We detect the current URL and skip those flags for non-HTTP(S) pages.
func Screenshot(ctx context.Context, conn *CDPConn) ([]byte, error) {
	return ScreenshotWithOptions(ctx, conn, ScreenshotOptions{})
}

// ScreenshotWithOptions captures the viewport and returns raw image bytes.
// The format, quality, speed, and animation behaviour are controlled by opts.
func ScreenshotWithOptions(ctx context.Context, conn *CDPConn, opts ScreenshotOptions) ([]byte, error) {
	format := opts.Format
	if format == "" {
		format = "png"
	}

	params := map[string]interface{}{"format": format}

	if format == "jpeg" {
		q := opts.Quality
		if q == 0 {
			q = 80
		}
		params["quality"] = q
	}

	if opts.OptimizeForSpeed {
		params["optimizeForSpeed"] = true
	}

	// Apply iframe-hang-prevention flags on HTTP(S) pages unless we're on the
	// JPEG+optimizeForSpeed fast path (Chrome uses a different internal code
	// path for that combination that doesn't hang on cross-origin iframes).
	fastPath := format == "jpeg" && opts.OptimizeForSpeed
	if !fastPath {
		if urlRaw, err := EvalJSON(ctx, conn, actionsJS+"\n__smLocationProtocol()"); err == nil {
			var proto string
			if json.Unmarshal(urlRaw, &proto) == nil && (proto == "http:" || proto == "https:") {
				params["captureBeyondViewport"] = false
				params["fromSurface"] = true
			}
		}
	}

	// A clip captures a specific region addressed in document coordinates, so it
	// needs the beyond-viewport surface and the scroll offset added to the
	// (viewport-relative) box. This overrides the captureBeyondViewport:false set
	// above for HTTP(S) pages.
	if opts.Clip != nil {
		sx, sy := 0.0, 0.0
		if raw, err := EvalJSON(ctx, conn, actionsJS+"\n__smScrollPosition()"); err == nil {
			var s struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			}
			if json.Unmarshal(raw, &s) == nil {
				sx, sy = s.X, s.Y
			}
		}
		params["clip"] = map[string]interface{}{
			"x":      opts.Clip.X + sx,
			"y":      opts.Clip.Y + sy,
			"width":  opts.Clip.Width,
			"height": opts.Clip.Height,
			"scale":  1,
		}
		params["captureBeyondViewport"] = true
	}

	// A full-page capture clips to the document's content size, which is already
	// in document coordinates (no scroll offset), from the beyond-viewport surface.
	if opts.FullPage {
		clip, restore, err := pinFullPageViewport(ctx, conn)
		if err != nil {
			return nil, err
		}
		defer restore()
		params["clip"] = clip
		params["captureBeyondViewport"] = true
	}

	if opts.StopLoading {
		// Halt any in-flight resource loads (ads, iframes) that keep
		// the compositor busy and prevent captureScreenshot from resolving.
		_, _ = conn.call(ctx, "Page.stopLoading", map[string]interface{}{})
		// Brief settle: let the compositor produce one more frame after stopping.
		time.Sleep(100 * time.Millisecond)
	}

	if opts.PauseAnimations {
		// Best-effort: pause all CSS/Web animations before capture.
		_, _ = conn.call(ctx, "Animation.setPlaybackRate", map[string]interface{}{"playbackRate": 0})
	}

	result, err := conn.call(ctx, "Page.captureScreenshot", params)

	if opts.PauseAnimations {
		// Best-effort: restore animation playback regardless of capture outcome.
		// Use a fresh context so a timed-out capture ctx doesn't block the restore.
		restoreCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, _ = conn.call(restoreCtx, "Animation.setPlaybackRate", map[string]interface{}{"playbackRate": 1})
		cancel()
	}

	if err != nil {
		return nil, fmt.Errorf("Screenshot: %w", err)
	}

	var resp struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("Screenshot: unmarshal: %w", err)
	}

	data, err := base64.StdEncoding.DecodeString(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("Screenshot: base64 decode: %w", err)
	}
	return data, nil
}

type layoutMetrics struct {
	CSSContentSize struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"cssContentSize"`
	CSSLayoutViewport struct {
		ClientWidth  float64 `json:"clientWidth"`
		ClientHeight float64 `json:"clientHeight"`
	} `json:"cssLayoutViewport"`
}

func getLayoutMetrics(ctx context.Context, conn *CDPConn) (layoutMetrics, error) {
	var m layoutMetrics
	raw, err := conn.call(ctx, "Page.getLayoutMetrics", map[string]interface{}{})
	if err != nil {
		return m, fmt.Errorf("Screenshot: layout metrics: %w", err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("Screenshot: layout metrics: %w", err)
	}
	return m, nil
}

// pinFullPageViewport pins the viewport width for a full-page screenshot to
// the width the page is already laid out at, and returns the document clip to
// capture and a func that undoes the override.
//
// captureBeyondViewport keeps the window width, scrollbar included. With
// nothing left to scroll the scrollbar disappears and its width goes into the
// layout, so text rewraps and the document gets shorter than the probe
// measured, and Chrome leaves the page at that wider layout afterwards. The
// layout viewport's clientWidth excludes the scrollbar, so overriding to that
// width, with scrollbars hidden, keeps the probe's layout. With overlay
// scrollbars the width is unchanged and this is a no-op.
//
// The height is left at the window's: vh, svh and dvh resolve against the
// viewport, so a taller one would stretch vh-sized elements and move
// position:fixed ones to the document's bottom, away from the boxes the probe
// measured. captureBeyondViewport renders the whole document without it.
// Because the viewport keeps its size, the page keeps its scroll position
// (the capture runs there, and fixed elements are drawn at that offset), so
// nothing needs restoring but the override.
//
// The override belongs to this CDP session, so other clients' emulation and
// the window size are untouched. Afterwards the session's own earlier
// override (ResizeViewport) is re-applied, or the override cleared if there
// was none.
func pinFullPageViewport(ctx context.Context, conn *CDPConn) (map[string]interface{}, func(), error) {
	m, err := getLayoutMetrics(ctx, conn)
	if err != nil {
		return nil, nil, err
	}
	// Clip to the size measured before the override: that is the layout the
	// probe saw. Read again under the override, Chrome can report the content
	// a scrollbar narrower.
	clip := map[string]interface{}{
		"x": 0, "y": 0,
		"width":  m.CSSContentSize.Width,
		"height": m.CSSContentSize.Height,
		"scale":  1,
	}
	noop := func() {}
	// Keep the pixel scale the page is drawn at: this session's ResizeViewport
	// override if there is one, else the real devicePixelRatio (0).
	conn.metricsMu.Lock()
	prev := conn.metrics
	conn.metricsMu.Unlock()
	var dsf interface{} = 0
	if v, ok := prev["deviceScaleFactor"]; ok {
		dsf = v
	}
	vw := math.Ceil(m.CSSLayoutViewport.ClientWidth)
	// clientHeight excludes a classic horizontal scrollbar, which vh includes:
	// on a page that overflows sideways 100vh comes out about 15 px shorter
	// than the probe saw, the same skew vw has from the width pin. Accepted.
	vh := math.Ceil(m.CSSLayoutViewport.ClientHeight)
	if vw <= 0 || vh <= 0 {
		return clip, noop, nil
	}
	if _, err := conn.call(ctx, "Emulation.setDeviceMetricsOverride", map[string]interface{}{
		"width":             int(vw),
		"height":            int(vh),
		"deviceScaleFactor": dsf,
		"mobile":            false,
	}); err != nil {
		// The capture still works, at captureBeyondViewport's own layout.
		return clip, noop, nil
	}
	_, _ = conn.call(ctx, "Emulation.setScrollbarsHidden", map[string]interface{}{"hidden": true})
	restore := func() {
		rctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = conn.call(rctx, "Emulation.setScrollbarsHidden", map[string]interface{}{"hidden": false})
		if prev != nil {
			_, _ = conn.call(rctx, "Emulation.setDeviceMetricsOverride", prev)
		} else {
			_, _ = conn.call(rctx, "Emulation.clearDeviceMetricsOverride", map[string]interface{}{})
		}
	}
	return clip, restore, nil
}

// ResolveBySightmapID resolves a probe id to a live element via its persisted
// data-sightmap-id attribute (set on the DOM during the prior extraction) and
// returns a ComponentNode carrying the element's CURRENT bounding box.
//
// Unlike a full ExtractComponents pass, this does NOT re-probe the page, so it
// does not reassign data-sightmap-id attributes — the id minted by an earlier
// snapshot/bounds call stays valid as long as the element has not been
// remounted. This is what keeps the classic snapshot -> click <id> loop stable
// across re-renders: no extraction-local id is recomputed between the
// call that minted the id and the action that consumes it.
//
// Returns an error when no element carries the id (the node was removed or the
// page was re-probed since), so callers can prompt for a fresh snapshot.
func ResolveBySightmapID(ctx context.Context, conn *CDPConn, id string) (*sightmap.ComponentNode, error) {
	selJSON, _ := json.Marshal(fmt.Sprintf("[data-sightmap-id=%q]", id))
	script := actionsJS + fmt.Sprintf("\n__smBoundsBySelector(%s)", selJSON)
	raw, err := EvalJSON(ctx, conn, script)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", id, err)
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, fmt.Errorf(
			"component %q not found in live DOM (data-sightmap-id absent; re-run sightmap snapshot)", id)
	}
	var b sightmap.Bounds
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("resolve %q: unmarshal bounds: %w", id, err)
	}
	return &sightmap.ComponentNode{Id: id, Bounds: &b}, nil
}

// HoverAt moves the mouse to absolute page coordinates x, y without clicking.
func HoverAt(ctx context.Context, conn *CDPConn, x, y int) error {
	_, err := conn.call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
		"type": "mouseMoved", "x": x, "y": y, "button": "none", "clickCount": 0,
	})
	return err
}

// ClickAt dispatches a left mouse click at absolute page coordinates x, y.
func ClickAt(ctx context.Context, conn *CDPConn, x, y int) error {
	events := []map[string]interface{}{
		{"type": "mouseMoved", "x": x, "y": y, "button": "none", "clickCount": 0},
		{"type": "mousePressed", "x": x, "y": y, "button": "left", "clickCount": 1},
		{"type": "mouseReleased", "x": x, "y": y, "button": "left", "clickCount": 1},
	}
	for _, params := range events {
		if _, err := conn.call(ctx, "Input.dispatchMouseEvent", params); err != nil {
			return fmt.Errorf("ClickAt: %w", err)
		}
	}
	return nil
}

// clickTarget is the live geometry of an element after scrolling it to the
// center of the viewport: whether it is reachable (on-screen and not covered by
// another element) and the viewport coordinates of its center.
type clickTarget struct {
	Found      bool `json:"found"`
	InViewport bool `json:"inViewport"`
	Hit        bool `json:"hit"`
	CX         int  `json:"cx"`
	CY         int  `json:"cy"`
}

// locateForClick scrolls the element carrying data-sightmap-id=id to the center
// of the viewport and reports its resulting geometry, including whether its
// center is actually the top-most element there (so a target hidden behind an
// overlay is reported as not-hit rather than clicked through).
func locateForClick(ctx context.Context, conn *CDPConn, id string) (clickTarget, error) {
	selJSON, _ := json.Marshal(fmt.Sprintf("[data-sightmap-id=%q]", id))
	script := actionsJS + fmt.Sprintf("\n__smLocateForClick(%s)", selJSON)
	raw, err := EvalJSON(ctx, conn, script)
	if err != nil {
		return clickTarget{}, err
	}
	var t clickTarget
	if err := json.Unmarshal(raw, &t); err != nil {
		return clickTarget{}, err
	}
	return t, nil
}

// Click scrolls node into view, verifies it is reachable, and dispatches a mouse
// click to its center, returning the viewport coordinates it clicked. It errors
// loudly when the target cannot be positioned in the viewport or is covered by
// another element, instead of silently dispatching a click into empty space (the
// off-screen no-op). A node with no data-sightmap-id and no bounds falls back to
// A node with no data-sightmap-id and no bounds falls back to
// a selector-based JS click() and returns (-1, -1).
func Click(ctx context.Context, conn *CDPConn, node *sightmap.ComponentNode) (int, int, error) {
	if node.Id != "" {
		if t, err := locateForClick(ctx, conn, node.Id); err == nil && t.Found {
			if !t.InViewport {
				return -1, -1, fmt.Errorf("click: %q could not be scrolled into the viewport (it may be hidden or have zero size)", node.Id)
			}
			if !t.Hit {
				return -1, -1, fmt.Errorf("click: %q is covered by another element at its center — it may be behind an overlay or modal", node.Id)
			}
			if err := ClickAt(ctx, conn, t.CX, t.CY); err != nil {
				return -1, -1, err
			}
			return t.CX, t.CY, nil
		}
		// locate failed (no data-sightmap-id on the element, or an eval error);
		// fall through to the bounds/selector paths below.
	}
	if node.Bounds == nil {
		sel := node.Element.SelectorString()
		if sel == "" {
			return -1, -1, fmt.Errorf("click: node %q has no bounds and no selector", node.Id)
		}
		selJSON, _ := json.Marshal(sel)
		_, err := EvalJSON(ctx, conn, actionsJS+fmt.Sprintf("\n__smClickBySelector(%s)", selJSON))
		return -1, -1, err
	}
	x := node.Bounds.X + node.Bounds.Width/2
	y := node.Bounds.Y + node.Bounds.Height/2
	if err := ClickAt(ctx, conn, x, y); err != nil {
		return -1, -1, err
	}
	return x, y, nil
}

// Hover moves the mouse to the center of node's bounding box.
func Hover(ctx context.Context, conn *CDPConn, node *sightmap.ComponentNode) error {
	if node.Bounds == nil {
		return fmt.Errorf("Hover: node %q has no bounds", node.Id)
	}
	x := node.Bounds.X + node.Bounds.Width/2
	y := node.Bounds.Y + node.Bounds.Height/2
	_, err := conn.call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
		"type": "mouseMoved", "x": x, "y": y, "button": "none", "clickCount": 0,
	})
	return err
}

// Fill clears the current value of a form field and types value into it.
// Clicks the element first, then Ctrl+A to select all, then types each rune.
func Fill(ctx context.Context, conn *CDPConn, node *sightmap.ComponentNode, value string) error {
	if _, _, err := Click(ctx, conn, node); err != nil {
		return fmt.Errorf("Fill: click to focus: %w", err)
	}
	// Select all: modifiers=10 (Ctrl=2 | Meta/Cmd=8)
	if _, err := conn.call(ctx, "Input.dispatchKeyEvent", map[string]interface{}{
		"type": "keyDown", "key": "a", "modifiers": 10,
	}); err != nil {
		return fmt.Errorf("Fill: select-all keyDown: %w", err)
	}
	if _, err := conn.call(ctx, "Input.dispatchKeyEvent", map[string]interface{}{
		"type": "keyUp", "key": "a", "modifiers": 0,
	}); err != nil {
		return fmt.Errorf("Fill: select-all keyUp: %w", err)
	}
	// Type each rune.
	for _, r := range value {
		ch := string(r)
		if _, err := conn.call(ctx, "Input.dispatchKeyEvent", map[string]interface{}{
			"type": "keyDown", "key": ch, "text": ch,
		}); err != nil {
			return fmt.Errorf("Fill: keyDown %q: %w", ch, err)
		}
		if _, err := conn.call(ctx, "Input.dispatchKeyEvent", map[string]interface{}{
			"type": "keyUp", "key": ch,
		}); err != nil {
			return fmt.Errorf("Fill: keyUp %q: %w", ch, err)
		}
	}
	return verifyFilled(ctx, conn, node.Id, value)
}

// verifyFilled reads the target's value back after typing and reports an error
// when a non-empty value was typed but the field is still empty — the signature
// of a React-controlled input where plain Fill's select-all+type does not stick.
// It is best-effort: an unreadable value (no id, non-input element) is not an
// error, and a non-empty-but-transformed value (masked inputs) is accepted.
func verifyFilled(ctx context.Context, conn *CDPConn, id, value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if v, ok := readInputValue(ctx, conn, id); ok && strings.TrimSpace(v) == "" {
		return fmt.Errorf("fill: typed %q but the field is still empty afterwards — it may be a React-controlled input; retry with --clear", value)
	}
	return nil
}

// readInputValue returns the current .value of the element carrying
// data-sightmap-id=id. ok is false when the id is empty, the element is gone, or
// it has no value property.
func readInputValue(ctx context.Context, conn *CDPConn, id string) (string, bool) {
	if id == "" {
		return "", false
	}
	selJSON, _ := json.Marshal(fmt.Sprintf("[data-sightmap-id=%q]", id))
	raw, err := EvalJSON(ctx, conn, actionsJS+fmt.Sprintf("\n__smReadValueBySelector(%s)", selJSON))
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var v string
	if json.Unmarshal(raw, &v) != nil {
		return "", false
	}
	return v, true
}

// ClearAndFill focuses node, clears its current value via the native JS setter
// (reliable on React-controlled inputs where Ctrl+A may not select), then types
// value using keystroke events. Use instead of Fill when a React input accumulates
// text across multiple fill calls.
func ClearAndFill(ctx context.Context, conn *CDPConn, node *sightmap.ComponentNode, value string) error {
	if _, _, err := Click(ctx, conn, node); err != nil {
		return fmt.Errorf("ClearAndFill: click to focus: %w", err)
	}
	// Clear the field via JS native setter — works on React-controlled inputs.
	if _, err := conn.call(ctx, "Runtime.evaluate", map[string]interface{}{
		"expression": actionsJS + "\n__smClearActiveElement()",
	}); err != nil {
		return fmt.Errorf("ClearAndFill: clear: %w", err)
	}
	// Type the new value character by character (same as Fill).
	for _, r := range value {
		ch := string(r)
		if _, err := conn.call(ctx, "Input.dispatchKeyEvent", map[string]interface{}{
			"type": "keyDown", "key": ch, "text": ch,
		}); err != nil {
			return fmt.Errorf("ClearAndFill: keyDown %q: %w", ch, err)
		}
		if _, err := conn.call(ctx, "Input.dispatchKeyEvent", map[string]interface{}{
			"type": "keyUp", "key": ch,
		}); err != nil {
			return fmt.Errorf("ClearAndFill: keyUp %q: %w", ch, err)
		}
	}
	return verifyFilled(ctx, conn, node.Id, value)
}

// keyMap maps common key names to their CDP key/code/text representations.
var keyMap = map[string]struct{ key, code, text string }{
	"Enter":      {"Enter", "Enter", "\r"},
	"Tab":        {"Tab", "Tab", "\t"},
	"Escape":     {"Escape", "Escape", ""},
	"Backspace":  {"Backspace", "Backspace", ""},
	"Delete":     {"Delete", "Delete", ""},
	"ArrowUp":    {"ArrowUp", "ArrowUp", ""},
	"ArrowDown":  {"ArrowDown", "ArrowDown", ""},
	"ArrowLeft":  {"ArrowLeft", "ArrowLeft", ""},
	"ArrowRight": {"ArrowRight", "ArrowRight", ""},
	" ":          {" ", "Space", " "},
}

// KeyPress dispatches a key down + key up event pair.
// key names: "Enter", "Tab", "Escape", "ArrowDown", "ArrowUp", "Backspace", etc.
func KeyPress(ctx context.Context, conn *CDPConn, key string) error {
	k, ok := keyMap[key]
	if !ok {
		k = struct{ key, code, text string }{key, key, ""}
	}
	downParams := map[string]interface{}{
		"type": "keyDown",
		"key":  k.key,
		"code": k.code,
	}
	if k.text != "" {
		downParams["text"] = k.text
	}
	if _, err := conn.call(ctx, "Input.dispatchKeyEvent", downParams); err != nil {
		return fmt.Errorf("KeyPress: keyDown %q: %w", key, err)
	}
	upParams := map[string]interface{}{
		"type": "keyUp",
		"key":  k.key,
		"code": k.code,
	}
	if _, err := conn.call(ctx, "Input.dispatchKeyEvent", upParams); err != nil {
		return fmt.Errorf("KeyPress: keyUp %q: %w", key, err)
	}
	return nil
}

// ScrollIntoView scrolls node into the visible viewport.
// Tries querySelector(selector) first; falls back to window.scrollTo using the
// node's bounds when no selector is available or querySelector fails.
// querySelector is wrapped in try-catch so a syntactically invalid selector
// (e.g. a bare numeric probe ID) degrades silently to the bounds path rather
// than surfacing as an EvalJSON JS exception.
func ScrollIntoView(ctx context.Context, conn *CDPConn, node *sightmap.ComponentNode) error {
	if sel := node.Element.SelectorString(); sel != "" {
		selJSON, _ := json.Marshal(sel)
		script := actionsJS + fmt.Sprintf("\n__smScrollIntoViewBySelector(%s)", selJSON)
		if _, err := EvalJSON(ctx, conn, script); err == nil {
			return nil
		}
	}
	// No valid selector or eval failed — scroll to the element's approximate
	// Y position using its bounds.
	if node.Bounds != nil {
		y := int(node.Bounds.Y)
		_, err := EvalJSON(ctx, conn, actionsJS+fmt.Sprintf("\n__smScrollToY(%d)", y))
		return err
	}
	return fmt.Errorf("ScrollIntoView: node %q has no selector and no bounds", node.Id)
}

// ScrollBy scrolls the page by deltaX, deltaY pixels via window.scrollBy.
// Uses JS eval rather than Input.dispatchMouseEvent: mouse-wheel CDP events
// race with page re-renders — Chrome can close the renderer-level connection
// while waiting for the Input.dispatchMouseEvent response, producing spurious
// "connection closed" errors. Runtime.evaluate is handled at the browser level
// and is not affected by renderer lifecycle events.
func ScrollBy(ctx context.Context, conn *CDPConn, deltaX, deltaY int) error {
	_, err := EvalJSON(ctx, conn, actionsJS+fmt.Sprintf("\n__smScrollBy(%d,%d)", deltaX, deltaY))
	return err
}

// WaitForURL polls GetURL every 200ms until the URL contains pattern or ctx is done.
func WaitForURL(ctx context.Context, conn *CDPConn, pattern string) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		url, _ := GetURL(ctx, conn)
		if strings.Contains(url, pattern) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// WaitForSelector polls document.querySelector every 200ms until the element
// exists in the DOM or ctx is done.
func WaitForSelector(ctx context.Context, conn *CDPConn, selector string) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		selJSON, _ := json.Marshal(selector)
		result, err := EvalJSON(ctx, conn, actionsJS+fmt.Sprintf("\n__smSelectorExists(%s)", selJSON))
		if err == nil {
			var found bool
			if json.Unmarshal(result, &found) == nil && found {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// HandleDialog accepts or dismisses a pending browser dialog.
// action must be "accept" or "dismiss". text is used as input for prompt dialogs.
func HandleDialog(ctx context.Context, conn *CDPConn, action, text string) error {
	_, err := conn.call(ctx, "Page.handleJavaScriptDialog", map[string]interface{}{
		"accept":     action == "accept",
		"promptText": text,
	})
	return err
}

// dragStepDelay is the pause between the mouse moves of a drag, about one frame
// at 60 Hz, so a page's pointermove and animation-frame handlers see a gesture
// and not a teleport. Tests set it to zero.
var dragStepDelay = 16 * time.Millisecond

// DefaultDragSteps is the number of mouse moves a drag makes when DragOptions
// does not say.
const DefaultDragSteps = 10

// DragOptions says where a drag goes and how it gets there. The drop point is
// To when set (see ToPosition), otherwise the start point plus (DeltaX, DeltaY).
type DragOptions struct {
	// Steps is the number of mouseMoved events between press and release; the
	// last lands on the drop point. 0 means DefaultDragSteps.
	Steps int
	// FromOffset is where inside the source to press, in pixels from its
	// top-left corner. Nil: a point on the source that is not an interactive
	// child.
	FromOffset *[2]int
	// To is the component to drop on; its Id must be set (a node resolved from
	// a probe id or a component query has one). Mutually exclusive with a delta.
	To *sightmap.ComponentNode
	// ToPosition picks the drop point inside To: "center" (the default), "above"
	// (4 px inside its top edge) or "below" (4 px inside its bottom edge).
	ToPosition string
	// DeltaX and DeltaY move the drop point from the start point when To is nil.
	DeltaX, DeltaY int
}

// dragEdgeInset is how far inside To's edge an "above" or "below" drop lands.
const dragEdgeInset = 4

// dragPrep is what __smDragPrepare returns.
type dragPrep struct {
	Bounds sightmap.Bounds `json:"bounds"`
	View   struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"view"`
	X   int  `json:"x"`
	Y   int  `json:"y"`
	Hit bool `json:"hit"` // the source is what is top-most at (X, Y)
}

// Drag moves a component by (deltaX, deltaY) pixels. See DragWith.
func Drag(ctx context.Context, conn *CDPConn, node *sightmap.ComponentNode, deltaX, deltaY int) error {
	return DragWith(ctx, conn, node, DragOptions{DeltaX: deltaX, DeltaY: deltaY})
}

// dropPoint is the viewport point a drag ends at.
func dropPoint(startX, startY int, to *sightmap.Bounds, opts DragOptions) (int, int, error) {
	if to == nil {
		return startX + opts.DeltaX, startY + opts.DeltaY, nil
	}
	x := to.X + to.Width/2
	switch opts.ToPosition {
	case "", "center":
		return x, to.Y + to.Height/2, nil
	case "above":
		return x, to.Y + min(dragEdgeInset, to.Height/2), nil
	case "below":
		return x, to.Y + to.Height - 1 - min(dragEdgeInset-1, to.Height/2), nil
	}
	return 0, 0, fmt.Errorf("drag: unknown drop position %q (use above, below or center)", opts.ToPosition)
}

// dropEdge names the part of the target a drop position aims at, for errors.
func dropEdge(position string) string {
	switch position {
	case "above":
		return "top edge"
	case "below":
		return "bottom edge"
	}
	return "center"
}

// DragWith drags src with a pointer, the way a person does: scroll src to the
// middle of the viewport unless it is already fully visible, press inside it,
// move to the drop point in Steps small steps with the button held, release.
// Pointer-event drags (sortable lists, boards, sliders)
// ignore a press that jumps straight to the target, because they wait for the
// pointer to travel past an activation distance and then follow it.
//
// It drives the pointer only; it does not start a native HTML5 drag-and-drop.
func DragWith(ctx context.Context, conn *CDPConn, src *sightmap.ComponentNode, opts DragOptions) error {
	if src.Id == "" {
		return fmt.Errorf("drag: source has no id to scroll to")
	}
	if opts.To != nil && (opts.DeltaX != 0 || opts.DeltaY != 0) {
		return fmt.Errorf("drag: give a target component or a delta, not both")
	}
	if opts.To == nil && opts.DeltaX == 0 && opts.DeltaY == 0 {
		return fmt.Errorf("drag: nowhere to drag to (give a target component or a delta)")
	}
	if opts.To != nil && opts.To.Id == "" {
		return fmt.Errorf("drag: target has no id")
	}
	steps := opts.Steps
	if steps == 0 {
		steps = DefaultDragSteps
	}
	if steps < 1 {
		return fmt.Errorf("drag: steps must be at least 1, got %d", steps)
	}

	selJSON, _ := json.Marshal(fmt.Sprintf("[data-sightmap-id=%q]", src.Id))
	off := "null, null"
	if opts.FromOffset != nil {
		off = fmt.Sprintf("%d, %d", opts.FromOffset[0], opts.FromOffset[1])
	}
	raw, err := EvalJSON(ctx, conn, actionsJS+fmt.Sprintf("\n__smDragPrepare(%s, %s)", selJSON, off))
	if err != nil {
		return fmt.Errorf("drag: locate source %q: %w", src.Id, err)
	}
	if len(raw) == 0 || string(raw) == "null" {
		return fmt.Errorf("drag: source %q not found in live DOM (re-run sightmap snapshot)", src.Id)
	}
	var prep dragPrep
	if err := json.Unmarshal(raw, &prep); err != nil {
		return fmt.Errorf("drag: locate source %q: %w", src.Id, err)
	}
	sx, sy := prep.X, prep.Y
	inView := func(x, y int) bool { return x >= 0 && y >= 0 && x < prep.View.Width && y < prep.View.Height }
	if !inView(sx, sy) {
		return fmt.Errorf("drag: start point (%d,%d) is outside the %dx%d viewport", sx, sy, prep.View.Width, prep.View.Height)
	}
	if !prep.Hit {
		return fmt.Errorf("drag: start point (%d,%d) is not over the source %q (covered or clipped); choose another --from-offset", sx, sy, src.Id)
	}

	// The target is read after the source was scrolled, so its bounds are the
	// ones the pointer will meet.
	var toBounds *sightmap.Bounds
	if opts.To != nil {
		to, err := ResolveBySightmapID(ctx, conn, opts.To.Id)
		if err != nil {
			return fmt.Errorf("drag: locate target: %w", err)
		}
		toBounds = to.Bounds
	}
	tx, ty, err := dropPoint(sx, sy, toBounds, opts)
	if err != nil {
		return err
	}
	if !inView(tx, ty) {
		return fmt.Errorf("drag: drop point (%d,%d) is outside the %dx%d viewport; the source and the target must both fit on screen (resize with 'tabs resize', or drag by a smaller delta)", tx, ty, prep.View.Width, prep.View.Height)
	}

	// A point inside the target's box can still be clipped by a scrolling
	// ancestor or covered by a sticky header, so ask the page whether the
	// target is what is top-most at the drop point.
	if opts.To != nil {
		toJSON, _ := json.Marshal(fmt.Sprintf("[data-sightmap-id=%q]", opts.To.Id))
		raw, err := EvalJSON(ctx, conn, actionsJS+fmt.Sprintf("\n__smDropCheck(%s, %d, %d)", toJSON, tx, ty))
		if err != nil {
			return fmt.Errorf("drag: check drop point: %w", err)
		}
		var chk struct {
			Hit     bool   `json:"hit"`
			Covered string `json:"covered"`
		}
		if err := json.Unmarshal(raw, &chk); err != nil {
			return fmt.Errorf("drag: check drop point: %w", err)
		}
		if !chk.Hit {
			what := "nothing"
			if chk.Covered != "" {
				what = chk.Covered
			}
			return fmt.Errorf("drag: drop point (%d,%d) is not over the target %q: %s is there (the target's %s is clipped by a scrolling ancestor or covered by another element; scroll it fully into view first)", tx, ty, opts.To.Id, what, dropEdge(opts.ToPosition))
		}
	}

	mouse := func(typ string, x, y, buttons int) error {
		params := map[string]interface{}{
			"type": typ, "x": x, "y": y, "buttons": buttons,
			"button": "none", "clickCount": 0,
		}
		switch {
		case typ == "mousePressed":
			params["button"], params["clickCount"] = "left", 1
		case typ == "mouseReleased":
			params["button"], params["clickCount"] = "left", 1
		case buttons != 0:
			params["button"] = "left"
		}
		if _, err := conn.call(ctx, "Input.dispatchMouseEvent", params); err != nil {
			return fmt.Errorf("Drag %s: %w", typ, err)
		}
		return nil
	}
	if err := mouse("mouseMoved", sx, sy, 0); err != nil {
		return err
	}
	if err := mouse("mousePressed", sx, sy, 1); err != nil {
		return err
	}
	for i := 1; i <= steps; i++ {
		if dragStepDelay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(dragStepDelay):
			}
		}
		x := sx + int(math.Round(float64(tx-sx)*float64(i)/float64(steps)))
		y := sy + int(math.Round(float64(ty-sy)*float64(i)/float64(steps)))
		if err := mouse("mouseMoved", x, y, 1); err != nil {
			return err
		}
	}
	return mouse("mouseReleased", tx, ty, 0)
}
