package browser

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
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
	return evalJSONIn(ctx, conn, "", 0, script)
}

// evalJSONIn is EvalJSON addressed to a child session ("" is the connection's
// own target) and an execution context within it (0 is the session's default,
// the frame's page world).
func evalJSONIn(ctx context.Context, conn *CDPConn, session string, contextID int64, script string) (json.RawMessage, error) {
	params := map[string]interface{}{
		"expression":    script,
		"returnByValue": true,
		"awaitPromise":  true,
	}
	if contextID != 0 {
		params["contextId"] = contextID
	}
	result, err := conn.callSession(ctx, session, "Runtime.evaluate", params)
	if err != nil {
		return nil, err
	}

	// CDP Runtime.evaluate response: {"result":{"type":"...","value":{...}},"exceptionDetails":{...}}
	var evalResp struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(result, &evalResp); err != nil {
		return nil, fmt.Errorf("EvalJSON: unmarshal eval response: %w", err)
	}
	if evalResp.ExceptionDetails != nil {
		// Text is only "Uncaught"; the description carries the error and its message.
		msg := evalResp.ExceptionDetails.Text
		if d := evalResp.ExceptionDetails.Exception.Description; d != "" {
			if first, _, _ := strings.Cut(d, "\n"); first != "" {
				msg = first
			}
		}
		return nil, fmt.Errorf("EvalJSON: JS exception: %s", msg)
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
//
// An id minted in a child frame ("2_17") is looked up in that frame, and its
// bounds come back in top-page viewport coordinates, like every other node's.
func ResolveBySightmapID(ctx context.Context, conn *CDPConn, id string) (*sightmap.ComponentNode, error) {
	b, _, err := resolveInFrame(ctx, conn, id)
	if err != nil {
		return nil, err
	}
	return &sightmap.ComponentNode{Id: id, Bounds: b}, nil
}

// resolveInFrame is ResolveBySightmapID that also says which frame holds the
// element (nil for the top document), for callers that must ask that frame
// something else about it.
func resolveInFrame(ctx context.Context, conn *CDPConn, id string) (*sightmap.Bounds, *Frame, error) {
	script := actionsJS + fmt.Sprintf("\n__smBoundsBySelector(%s)", idSelectorJSON(id))
	raw, f, err := evalForID(ctx, conn, id, script)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve %q: %w", id, err)
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil, notFoundErr(id)
	}
	var b sightmap.Bounds
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, nil, fmt.Errorf("resolve %q: unmarshal bounds: %w", id, err)
	}
	if f != nil {
		ox, oy, err := f.Origin(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve %q: %w", id, err)
		}
		b.X += int(math.Round(ox))
		b.Y += int(math.Round(oy))
	}
	return &b, f, nil
}

// mousePoint is where to send a mouse event: a point in top-page viewport
// coordinates (what Click reports) and the CDP session that receives it.
//
// For an element in the top document, or in a frame of the top document's own
// process, the session is the connection's own and the point goes as is. For an
// element in a cross-process frame, the event is sent on that frame's session,
// in its own viewport's coordinates, not through the top page. Sent through the
// top page, Chrome's browser process routes it by its hit-test data for the
// out-of-process frame, which after a scroll can be missing for good (a click
// there reports "success" and the page never sees it; DOM.getNodeForLocation
// finds no node at the point, however long one waits). The frame's own session
// has no such step.
type mousePoint struct {
	X, Y    int
	session string
	// dx, dy are the origin of the session's viewport on the top page, taken off
	// X and Y to give the coordinates the session expects.
	dx, dy float64
}

// mousePointFor is the point (x, y) in the viewport of the top page, reached
// through f's document (nil for the top document).
func mousePointFor(ctx context.Context, f *Frame, x, y int) (mousePoint, error) {
	p := mousePoint{X: x, Y: y}
	if f == nil {
		return p, nil
	}
	root := f
	for !root.ownRoot() {
		root = root.Parent
	}
	if root.session == "" {
		return p, nil
	}
	ox, oy, err := root.Origin(ctx)
	if err != nil {
		return mousePoint{}, err
	}
	p.session, p.dx, p.dy = root.session, ox, oy
	return p, nil
}

// at is p moved to another point of the top page, on the same session.
func (p mousePoint) at(x, y int) mousePoint {
	p.X, p.Y = x, y
	return p
}

// send dispatches a mouse event of the given type at the point.
func (p mousePoint) send(ctx context.Context, conn *CDPConn, params map[string]interface{}) error {
	params["x"] = float64(p.X) - p.dx
	params["y"] = float64(p.Y) - p.dy
	_, err := conn.callSession(ctx, p.session, "Input.dispatchMouseEvent", params)
	return err
}

func (p mousePoint) hover(ctx context.Context, conn *CDPConn) error {
	return p.send(ctx, conn, map[string]interface{}{"type": "mouseMoved", "button": "none", "clickCount": 0})
}

func (p mousePoint) click(ctx context.Context, conn *CDPConn) error {
	for _, params := range []map[string]interface{}{
		{"type": "mouseMoved", "button": "none", "clickCount": 0},
		{"type": "mousePressed", "button": "left", "clickCount": 1},
		{"type": "mouseReleased", "button": "left", "clickCount": 1},
	} {
		if err := p.send(ctx, conn, params); err != nil {
			return fmt.Errorf("click: %w", err)
		}
	}
	return nil
}

// HoverAt moves the mouse to absolute page coordinates x, y without clicking.
func HoverAt(ctx context.Context, conn *CDPConn, x, y int) error {
	return mousePoint{X: x, Y: y}.hover(ctx, conn)
}

// ClickAt dispatches a left mouse click at absolute page coordinates x, y.
func ClickAt(ctx context.Context, conn *CDPConn, x, y int) error {
	return mousePoint{X: x, Y: y}.click(ctx, conn)
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

	// frame is the child frame holding the element, nil for the top document.
	frame *Frame
}

// locateForClick scrolls the element carrying data-sightmap-id=id to the center
// of the viewport and reports its resulting geometry, including whether its
// center is actually the top-most element there (so a target hidden behind an
// overlay is reported as not-hit rather than clicked through).
//
// The point is in top-page viewport coordinates, which is what the mouse event
// takes whatever frame the element is in. For an element in a child frame it is
// the point found in the frame plus the frame's origin, read after the scroll
// has settled; "hit" then also asks that nothing in an enclosing document is
// drawn over the frame at that point.
func locateForClick(ctx context.Context, conn *CDPConn, id string) (clickTarget, error) {
	script := actionsJS + fmt.Sprintf("\n__smLocateForClick(%s)", idSelectorJSON(id))
	raw, f, err := evalForID(ctx, conn, id, script)
	if err != nil {
		return clickTarget{}, err
	}
	var t clickTarget
	if err := json.Unmarshal(raw, &t); err != nil {
		return clickTarget{}, err
	}
	t.frame = f
	if f == nil || !t.Found || !t.InViewport {
		return t, nil
	}

	ox, oy, err := settleOrigin(ctx, f)
	if err != nil {
		return clickTarget{}, err
	}
	x, y := ox+float64(t.CX), oy+float64(t.CY)
	t.CX, t.CY = int(math.Floor(x)), int(math.Floor(y))

	// The frame's own viewport can extend past what the top page shows (the
	// frame is taller than the window, or clipped by a scrolling parent).
	vp, err := EvalJSON(ctx, conn, `({w: window.innerWidth, h: window.innerHeight})`)
	if err != nil {
		return clickTarget{}, err
	}
	var size struct{ W, H float64 }
	if err := json.Unmarshal(vp, &size); err != nil {
		return clickTarget{}, err
	}
	if x < 0 || y < 0 || x >= size.W || y >= size.H {
		t.InViewport, t.Hit = false, false
		return t, nil
	}
	if t.Hit {
		if t.Hit, err = f.ownersShow(ctx, x, y); err != nil {
			return clickTarget{}, err
		}
	}
	return t, nil
}

// reachablePoint scrolls the element stamped with id into view, measures it
// again and returns the point to send a mouse event to. found is false when the
// element has no live data-sightmap-id or the lookup failed, so the caller can
// fall back to other means; an element that is there but off-screen or covered
// is an error naming verb (the command), rather than an event sent into empty
// space.
func reachablePoint(ctx context.Context, conn *CDPConn, verb, id string) (p mousePoint, found bool, err error) {
	t, lerr := locateForClick(ctx, conn, id)
	if lerr != nil || !t.Found {
		return mousePoint{}, false, nil
	}
	if !t.InViewport {
		return mousePoint{}, false, fmt.Errorf("%s: %q could not be scrolled into the viewport (it may be hidden or have zero size)", verb, id)
	}
	if !t.Hit {
		return mousePoint{}, false, fmt.Errorf("%s: %q is covered by another element at its center — it may be behind an overlay or modal", verb, id)
	}
	p, err = mousePointFor(ctx, t.frame, t.CX, t.CY)
	if err != nil {
		return mousePoint{}, false, err
	}
	return p, true, nil
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
		p, found, err := reachablePoint(ctx, conn, "click", node.Id)
		if err != nil {
			return -1, -1, err
		}
		if found {
			if err := p.click(ctx, conn); err != nil {
				return -1, -1, err
			}
			return p.X, p.Y, nil
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

// Hover moves the mouse to the center of node's bounding box. A node with an id
// is measured again first, in whichever frame holds it: the bounds a snapshot
// recorded go stale as soon as anything scrolls.
func Hover(ctx context.Context, conn *CDPConn, node *sightmap.ComponentNode) error {
	bounds := node.Bounds
	if node.Id != "" {
		if fresh, err := ResolveBySightmapID(ctx, conn, node.Id); err == nil {
			bounds = fresh.Bounds
		}
	}
	if bounds == nil {
		return fmt.Errorf("Hover: node %q has no bounds", node.Id)
	}
	x := bounds.X + bounds.Width/2
	y := bounds.Y + bounds.Height/2
	var f *Frame
	if node.Id != "" {
		f, _ = frameOfID(ctx, conn, node.Id)
	}
	p, err := mousePointFor(ctx, f, x, y)
	if err != nil {
		return err
	}
	return p.hover(ctx, conn)
}

// selectAllIsMeta says which modifier the select-all fallback chord uses: Cmd on
// macOS, Ctrl elsewhere. It follows the host running the CLI, not the browser's:
// a macOS CLI driving a Linux Chrome over --addr sends Cmd+A, which that Chrome
// ignores (the selectAll command is sent either way).
var selectAllIsMeta = runtime.GOOS == "darwin"

// activeSelection is what __smSelectActive reports: whether it selected the
// focused element's content, and that content as it was.
type activeSelection struct {
	Selected bool   `json:"selected"`
	Value    string `json:"value"`
}

// selectAllActive selects everything in the focused element (an input,
// a textarea or a contenteditable) through the DOM, which works the same on
// every platform and whatever the page does with keyboard shortcuts. Selected
// is false when the focused element is none of those.
func selectAllActive(ctx context.Context, conn *CDPConn) (activeSelection, error) {
	raw, err := EvalJSON(ctx, conn, actionsJS+"\n__smSelectActive()")
	if err != nil {
		return activeSelection{}, err
	}
	var sel activeSelection
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &sel); err != nil {
			return activeSelection{}, err
		}
	}
	return sel, nil
}

// selectAllChord presses Ctrl+A (Cmd+A on macOS) with the key, code and key code
// of the A key and the selectAll editing command. Chrome does not derive editing
// commands from key events on macOS (headless included), so the command is sent
// explicitly, as Playwright does. It is the fallback for a focused element the
// DOM cannot select, such as a custom widget that handles the shortcut itself.
func selectAllChord(ctx context.Context, conn *CDPConn) error {
	mods := 2 // Ctrl
	if selectAllIsMeta {
		mods = 4 // Meta
	}
	down := map[string]interface{}{
		"type": "keyDown", "key": "a", "code": "KeyA", "windowsVirtualKeyCode": 65,
		"modifiers": mods, "commands": []string{"selectAll"},
	}
	if _, err := conn.call(ctx, "Input.dispatchKeyEvent", down); err != nil {
		return fmt.Errorf("select-all keyDown: %w", err)
	}
	up := map[string]interface{}{
		"type": "keyUp", "key": "a", "code": "KeyA", "windowsVirtualKeyCode": 65,
		"modifiers": mods,
	}
	if _, err := conn.call(ctx, "Input.dispatchKeyEvent", up); err != nil {
		return fmt.Errorf("select-all keyUp: %w", err)
	}
	return nil
}

// typeText types value into the focused element, one key event pair per rune.
func typeText(ctx context.Context, conn *CDPConn, value string) error {
	for _, r := range value {
		ch := string(r)
		if _, err := conn.call(ctx, "Input.dispatchKeyEvent", map[string]interface{}{
			"type": "keyDown", "key": ch, "text": ch,
		}); err != nil {
			return fmt.Errorf("keyDown %q: %w", ch, err)
		}
		if _, err := conn.call(ctx, "Input.dispatchKeyEvent", map[string]interface{}{
			"type": "keyUp", "key": ch,
		}); err != nil {
			return fmt.Errorf("keyUp %q: %w", ch, err)
		}
	}
	return nil
}

// Fill replaces the value of a form field with value: it clicks the element to
// focus it, selects everything in the focused field through the DOM (falling
// back to a select-all chord with the selectAll command for elements the DOM
// cannot select), then types each rune so the page sees real key events. It
// reads the value back and errors when the old value was kept or the field is
// empty.
func Fill(ctx context.Context, conn *CDPConn, node *sightmap.ComponentNode, value string) error {
	if _, _, err := Click(ctx, conn, node); err != nil {
		return fmt.Errorf("Fill: click to focus: %w", err)
	}
	sel, err := selectAllActive(ctx, conn)
	if err != nil {
		return fmt.Errorf("Fill: select: %w", err)
	}
	before := sel.Value
	if !sel.Selected {
		before, _ = readInputValue(ctx, conn, node.Id)
		if err := selectAllChord(ctx, conn); err != nil {
			return fmt.Errorf("Fill: %w", err)
		}
	}
	if err := typeText(ctx, conn, value); err != nil {
		return fmt.Errorf("Fill: %w", err)
	}
	return verifyFilled(ctx, conn, node.Id, value, before)
}

// verifyFilled reads the target's value back after typing value into a field
// that held before, and reports an error when the fill did not take:
//
//   - the field is empty although a non-empty value was typed (a React-controlled
//     input that dropped the keystrokes), or
//   - the old value was not replaced: the field now reads as the old value with
//     the typed value inserted somewhere in it (the signature of text typed after
//     or inside what was there, as when select-all did not select).
//
// A value that merely differs (a masked, formatted or truncated field, or one
// that adds its own "$" or "%") is accepted. It is best-effort: an unreadable
// value (no id, non-input element) is not an error.
func verifyFilled(ctx context.Context, conn *CDPConn, id, value, before string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	got, ok := readInputValue(ctx, conn, id)
	if !ok {
		return nil
	}
	if strings.TrimSpace(got) == "" {
		return fmt.Errorf("fill: typed %q but the field is still empty afterwards — it may be a React-controlled input; retry with --clear", value)
	}
	if before != "" && got != value && insertedInto(got, value, before) {
		return fmt.Errorf("fill: typed %q but the field reads %q: the old value %q was not replaced; retry with --clear", value, got, before)
	}
	return nil
}

// insertedInto reports whether got is old with ins inserted at some position:
// removing one occurrence of ins from got leaves old.
func insertedInto(got, ins, old string) bool {
	if len(got) != len(old)+len(ins) {
		return false
	}
	for i := 0; i+len(ins) <= len(got); i++ {
		if got[i:i+len(ins)] == ins && got[:i]+got[i+len(ins):] == old {
			return true
		}
	}
	return false
}

// readInputValue returns the current .value of the element carrying
// data-sightmap-id=id. ok is false when the id is empty, the element is gone, or
// it has no value property.
func readInputValue(ctx context.Context, conn *CDPConn, id string) (string, bool) {
	if id == "" {
		return "", false
	}
	raw, _, err := evalForID(ctx, conn, id, actionsJS+fmt.Sprintf("\n__smReadValueBySelector(%s)", idSelectorJSON(id)))
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
// (the way to empty a React-controlled input whose state ignores a selection),
// then types value using keystroke events.
func ClearAndFill(ctx context.Context, conn *CDPConn, node *sightmap.ComponentNode, value string) error {
	if _, _, err := Click(ctx, conn, node); err != nil {
		return fmt.Errorf("ClearAndFill: click to focus: %w", err)
	}
	// Clear the field via the element's native value setter. A failure (nothing
	// focused, an element with no value) is an error: typing on regardless would
	// append to what was there and still report success. The focused element is
	// the one in the document that holds the target: in the top document's own,
	// a frame is merely the <iframe> that hosts it.
	before, err := clearActiveElement(ctx, conn, node.Id)
	if err != nil {
		return fmt.Errorf("ClearAndFill: clear: %w", err)
	}
	if err := typeText(ctx, conn, value); err != nil {
		return fmt.Errorf("ClearAndFill: %w", err)
	}
	return verifyFilled(ctx, conn, node.Id, value, before)
}

// clearActiveElement clears the focused element of the document holding id (the
// top document when id is empty or not found in a frame) and returns the value
// it had.
func clearActiveElement(ctx context.Context, conn *CDPConn, id string) (string, error) {
	script := actionsJS + "\n__smClearActiveElement()"
	var raw json.RawMessage
	var err error
	if id == "" {
		raw, err = EvalJSON(ctx, conn, script)
	} else {
		raw, _, err = evalForID(ctx, conn, id, script)
	}
	if err != nil {
		return "", err
	}
	var before string
	_ = json.Unmarshal(raw, &before)
	return before, nil
}

// ScrollIntoView scrolls node into the visible viewport.
// Tries querySelector(selector) first; falls back to window.scrollTo using the
// node's bounds when no selector is available or querySelector fails.
// querySelector is wrapped in try-catch so a syntactically invalid selector
// (e.g. a bare numeric probe ID) degrades silently to the bounds path rather
// than surfacing as an EvalJSON JS exception.
func ScrollIntoView(ctx context.Context, conn *CDPConn, node *sightmap.ComponentNode) error {
	// An id names its element exactly, and in whichever frame holds it.
	if node.Id != "" {
		if raw, _, err := evalForID(ctx, conn, node.Id, actionsJS+fmt.Sprintf("\n__smLocateForClick(%s)", idSelectorJSON(node.Id))); err == nil {
			var t clickTarget
			if json.Unmarshal(raw, &t) == nil && t.Found {
				return nil
			}
		}
	}
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
// exists in the DOM or ctx is done. The top document is asked first and the
// frames nested in it after, so a selector finds its element wherever it is.
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
			if frameSelectorExists(ctx, conn, selector) {
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
	View struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"view"`
	X   int  `json:"x"`
	Y   int  `json:"y"`
	Hit bool `json:"hit"` // the source is what is top-most at (X, Y)
}

// locateDragSource scrolls the source into view and picks the point to press,
// in top-page viewport coordinates, with the frame that holds the source (nil
// for the top document) and the size of the top page's viewport.
//
// An element in a frame is scrolled only when it is not fully visible: when
// the point found is not on the top page's screen (the frame itself can be
// below the fold, which the frame's own viewport knows nothing about), the
// source is scrolled to the center regardless and measured again. "Hit" for a
// frame's element also asks that nothing drawn over the frame covers the point.
func locateDragSource(ctx context.Context, conn *CDPConn, id string, offset *[2]int) (dragPrep, *Frame, error) {
	off := "null, null"
	if offset != nil {
		off = fmt.Sprintf("%d, %d", offset[0], offset[1])
	}
	prepare := func(force bool) (dragPrep, *Frame, error) {
		extra := ""
		if force {
			extra = ", true"
		}
		raw, f, err := evalForID(ctx, conn, id, actionsJS+fmt.Sprintf("\n__smDragPrepare(%s, %s%s)", idSelectorJSON(id), off, extra))
		if err != nil {
			return dragPrep{}, nil, fmt.Errorf("drag: locate source %q: %w", id, err)
		}
		if len(raw) == 0 || string(raw) == "null" {
			return dragPrep{}, nil, fmt.Errorf("drag: source %q not found in live DOM (re-run sightmap snapshot)", id)
		}
		var prep dragPrep
		if err := json.Unmarshal(raw, &prep); err != nil {
			return dragPrep{}, nil, fmt.Errorf("drag: locate source %q: %w", id, err)
		}
		if f == nil {
			return prep, nil, nil
		}
		// The point is in the frame's viewport: move it to the top page's, once
		// the scroll has settled, and judge it against the top page's screen.
		ox, oy, err := settleOrigin(ctx, f)
		if err != nil {
			return dragPrep{}, nil, fmt.Errorf("drag: locate source %q: %w", id, err)
		}
		vp, err := EvalJSON(ctx, conn, `({width: window.innerWidth, height: window.innerHeight})`)
		if err != nil {
			return dragPrep{}, nil, fmt.Errorf("drag: locate source %q: %w", id, err)
		}
		if err := json.Unmarshal(vp, &prep.View); err != nil {
			return dragPrep{}, nil, fmt.Errorf("drag: locate source %q: %w", id, err)
		}
		x, y := ox+float64(prep.X), oy+float64(prep.Y)
		prep.X, prep.Y = int(math.Floor(x)), int(math.Floor(y))
		if prep.Hit && x >= 0 && y >= 0 && x < float64(prep.View.Width) && y < float64(prep.View.Height) {
			if prep.Hit, err = f.ownersShow(ctx, x, y); err != nil {
				return dragPrep{}, nil, fmt.Errorf("drag: locate source %q: %w", id, err)
			}
		}
		return prep, f, nil
	}
	prep, f, err := prepare(false)
	if err != nil || f == nil {
		return prep, f, err
	}
	if !prep.Hit || prep.X < 0 || prep.Y < 0 || prep.X >= prep.View.Width || prep.Y >= prep.View.Height {
		return prepare(true)
	}
	return prep, f, nil
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
// Pointer-event drags (sortable lists, boards, sliders) ignore a press that
// jumps straight to the target, because they wait for the pointer to travel
// past an activation distance and then follow it.
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
	if opts.To == nil && opts.ToPosition != "" && opts.ToPosition != "center" {
		return fmt.Errorf("drag: drop position %q needs a target component; it does nothing with a delta", opts.ToPosition)
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

	// The source and the target may each be in a child frame. Everything is
	// worked out in top-page viewport coordinates (the point a person would
	// point at), and the mouse events go on the session of the source's frame,
	// the way Click sends them: see mousePoint.
	src0, sf, err := locateDragSource(ctx, conn, src.Id, opts.FromOffset)
	if err != nil {
		return err
	}
	prep := src0
	sx, sy := prep.X, prep.Y
	inView := func(x, y int) bool { return x >= 0 && y >= 0 && x < prep.View.Width && y < prep.View.Height }
	if !inView(sx, sy) {
		return fmt.Errorf("drag: start point (%d,%d) is outside the %dx%d viewport", sx, sy, prep.View.Width, prep.View.Height)
	}
	if !prep.Hit {
		return fmt.Errorf("drag: start point (%d,%d) is not over the source %q (covered or clipped); choose another --from-offset", sx, sy, src.Id)
	}
	from, err := mousePointFor(ctx, sf, sx, sy)
	if err != nil {
		return fmt.Errorf("drag: %w", err)
	}

	// The target is read after the source was scrolled, so its bounds are the
	// ones the pointer will meet.
	var toBounds *sightmap.Bounds
	var tf *Frame
	if opts.To != nil {
		if toBounds, tf, err = resolveInFrame(ctx, conn, opts.To.Id); err != nil {
			return fmt.Errorf("drag: locate target: %w", err)
		}
	}
	if opts.To != nil {
		to, err := mousePointFor(ctx, tf, 0, 0)
		if err != nil {
			return fmt.Errorf("drag: %w", err)
		}
		if to.session != from.session {
			return fmt.Errorf("drag: the source %q and the target %q are in frames of different processes; the pointer cannot be carried from one to the other", src.Id, opts.To.Id)
		}
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
	// target is what is top-most at the drop point: in the target's own
	// document, and, for a target in a frame, in each document enclosing it.
	if opts.To != nil {
		var ox, oy float64
		if tf != nil {
			if ox, oy, err = tf.Origin(ctx); err != nil {
				return fmt.Errorf("drag: check drop point: %w", err)
			}
		}
		toJSON := idSelectorJSON(opts.To.Id)
		raw, _, err := evalForID(ctx, conn, opts.To.Id, actionsJS+fmt.Sprintf("\n__smDropCheck(%s, %d, %d)", toJSON, tx-int(math.Round(ox)), ty-int(math.Round(oy))))
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
		if chk.Hit && tf != nil {
			if chk.Hit, err = tf.ownersShow(ctx, float64(tx), float64(ty)); err != nil {
				return fmt.Errorf("drag: check drop point: %w", err)
			}
			if !chk.Hit {
				chk.Covered = "an element drawn over the frame"
			}
		}
		if !chk.Hit {
			what := "nothing"
			if chk.Covered != "" {
				what = chk.Covered
			}
			return fmt.Errorf("drag: drop point (%d,%d) is not over the target %q: %s is there (the target's %s is clipped by a scrolling ancestor or covered by another element; scroll it fully into view first)", tx, ty, opts.To.Id, what, dropEdge(opts.ToPosition))
		}
	}

	mouseCtx := func(ctx context.Context, typ string, x, y, buttons int) error {
		params := map[string]interface{}{
			"type": typ, "buttons": buttons,
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
		if err := from.at(x, y).send(ctx, conn, params); err != nil {
			return fmt.Errorf("Drag %s: %w", typ, err)
		}
		return nil
	}
	mouse := func(typ string, x, y, buttons int) error {
		return mouseCtx(ctx, typ, x, y, buttons)
	}
	if err := mouse("mouseMoved", sx, sy, 0); err != nil {
		return err
	}
	if err := mouse("mousePressed", sx, sy, 1); err != nil {
		return err
	}
	// Once the button is down, any failure (a dropped connection, a canceled
	// ctx) would leave it held in the page, so the next command would start
	// mid-drag. Release it at the last point reached, on a context that
	// outlives ctx's cancellation, and report the original error.
	lastX, lastY := sx, sy
	fail := func(err error) error {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = mouseCtx(rctx, "mouseReleased", lastX, lastY, 0)
		return err
	}
	for i := 1; i <= steps; i++ {
		if dragStepDelay > 0 {
			select {
			case <-ctx.Done():
				return fail(ctx.Err())
			case <-time.After(dragStepDelay):
			}
		}
		x := sx + int(math.Round(float64(tx-sx)*float64(i)/float64(steps)))
		y := sy + int(math.Round(float64(ty-sy)*float64(i)/float64(steps)))
		if err := mouse("mouseMoved", x, y, 1); err != nil {
			return fail(err)
		}
		lastX, lastY = x, y
	}
	if err := mouse("mouseReleased", tx, ty, 0); err != nil {
		return fail(err)
	}
	return nil
}
