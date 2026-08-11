package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/comps"
)

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

// WaitForNetworkIdle waits for Chrome's "networkIdle" lifecycle event, which
// fires when there have been no more than 2 in-flight network requests for at
// least 500 ms. This is the same signal Playwright uses for waitUntil:"networkidle"
// and is the correct post-navigate wait for React/SPA pages: the a11y tree is
// fully hydrated only after all data-fetch requests have settled.
//
// The function enables lifecycle events, then waits up to maxWait. On timeout
// it returns nil rather than an error — a partial render is better than no snap.
// Callers must subscribe BEFORE navigating; use NavigateAndWaitIdle instead.
func WaitForNetworkIdle(ctx context.Context, conn *CDPConn, maxWait time.Duration) error {
	if _, err := conn.call(ctx, "Page.setLifecycleEventsEnabled", map[string]interface{}{
		"enabled": true,
	}); err != nil {
		// Non-fatal: fall through to timeout path.
		_ = err
	}
	ch := conn.Subscribe("Page.lifecycleEvent")
	defer conn.Unsubscribe("Page.lifecycleEvent", ch)

	deadline := time.NewTimer(maxWait)
	defer deadline.Stop()
	for {
		select {
		case raw, ok := <-ch:
			if !ok {
				return nil
			}
			var ev struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(raw, &ev); err == nil && ev.Name == "networkIdle" {
				return nil
			}
		case <-deadline.C:
			return nil // timeout — proceed with whatever rendered so far
		case <-ctx.Done():
			return ctx.Err()
		case <-conn.done:
			return nil
		}
	}
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
func EvalJSON(ctx context.Context, conn *CDPConn, script string) (json.RawMessage, error) {
	result, err := conn.call(ctx, "Runtime.evaluate", map[string]interface{}{
		"expression":    script,
		"returnByValue": true,
		"awaitPromise":  false,
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
		if urlRaw, err := EvalJSON(ctx, conn, `JSON.stringify(location.protocol)`); err == nil {
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
		if raw, err := EvalJSON(ctx, conn, `({x: window.scrollX, y: window.scrollY})`); err == nil {
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
func ResolveBySightmapID(ctx context.Context, conn *CDPConn, id string) (*comps.ComponentNode, error) {
	script := fmt.Sprintf(`(function(){
  var el=document.querySelector('[data-sightmap-id=%q]');
  if(!el) return null;
  var r=el.getBoundingClientRect();
  return {x:Math.round(r.left),y:Math.round(r.top),width:Math.round(r.width),height:Math.round(r.height)};
})()`, id)
	raw, err := EvalJSON(ctx, conn, script)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", id, err)
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, fmt.Errorf(
			"component %q not found in live DOM (data-sightmap-id absent; re-run sightmap snapshot)", id)
	}
	var b comps.Bounds
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("resolve %q: unmarshal bounds: %w", id, err)
	}
	return &comps.ComponentNode{Id: id, Bounds: &b}, nil
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

// Drag moves a component by (deltaX, deltaY) pixels via CDP mouse events.
// Sequence: move to center → press → move to target → release.
func Drag(ctx context.Context, conn *CDPConn, node *comps.ComponentNode, deltaX, deltaY int) error {
	if node.Bounds == nil {
		return fmt.Errorf("drag: node %q has no bounds", node.Id)
	}
	cx := node.Bounds.X + node.Bounds.Width/2
	cy := node.Bounds.Y + node.Bounds.Height/2
	tx, ty := cx+deltaX, cy+deltaY

	for _, ev := range []struct {
		evType  string
		x, y    int
		pressed bool
	}{
		{"mouseMoved", cx, cy, false},
		{"mousePressed", cx, cy, true},
		{"mouseMoved", tx, ty, false},
		{"mouseReleased", tx, ty, true},
	} {
		params := map[string]interface{}{"type": ev.evType, "x": ev.x, "y": ev.y}
		if ev.pressed {
			params["button"] = "left"
			params["clickCount"] = 1
		} else {
			params["button"] = "none"
			params["clickCount"] = 0
		}
		if _, err := conn.call(ctx, "Input.dispatchMouseEvent", params); err != nil {
			return fmt.Errorf("Drag %s: %w", ev.evType, err)
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
	script := fmt.Sprintf(`(function(){
  var el=document.querySelector('[data-sightmap-id=%q]');
  if(!el) return {found:false};
  el.scrollIntoView({block:"center",inline:"center",behavior:"instant"});
  var r=el.getBoundingClientRect();
  var cx=Math.floor(r.left+r.width/2), cy=Math.floor(r.top+r.height/2);
  var iv = r.width>0 && r.height>0 && cx>=0 && cy>=0 && cx<window.innerWidth && cy<window.innerHeight;
  var hit=false;
  if(iv){ var at=document.elementFromPoint(cx,cy); hit=!!at && (at===el||el.contains(at)||at.contains(el)); }
  return {found:true,inViewport:iv,hit:hit,cx:cx,cy:cy};
})()`, id)
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
func Click(ctx context.Context, conn *CDPConn, node *comps.ComponentNode) (int, int, error) {
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
		_, err := EvalJSON(ctx, conn, fmt.Sprintf("document.querySelector(%q)?.click()", sel))
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
func Hover(ctx context.Context, conn *CDPConn, node *comps.ComponentNode) error {
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
func Fill(ctx context.Context, conn *CDPConn, node *comps.ComponentNode, value string) error {
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
	raw, err := EvalJSON(ctx, conn, fmt.Sprintf(
		`(function(){var el=document.querySelector('[data-sightmap-id=%q]');if(!el||el.value==null)return null;return String(el.value)})()`, id))
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
func ClearAndFill(ctx context.Context, conn *CDPConn, node *comps.ComponentNode, value string) error {
	if _, _, err := Click(ctx, conn, node); err != nil {
		return fmt.Errorf("ClearAndFill: click to focus: %w", err)
	}
	// Clear the field via JS native setter — works on React-controlled inputs.
	clearJS := `(function(){
		var el = document.activeElement;
		if (!el) return;
		var setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value') ||
		             Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value');
		if (setter && setter.set) { setter.set.call(el,''); }
		el.dispatchEvent(new Event('input',{bubbles:true}));
		el.dispatchEvent(new Event('change',{bubbles:true}));
	})()`
	if _, err := conn.call(ctx, "Runtime.evaluate", map[string]interface{}{
		"expression": clearJS,
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
func ScrollIntoView(ctx context.Context, conn *CDPConn, node *comps.ComponentNode) error {
	if sel := node.Element.SelectorString(); sel != "" {
		script := fmt.Sprintf(
			`try{const el=document.querySelector(%q);if(el)el.scrollIntoView({block:"center",behavior:"instant"})}catch(_){}`,
			sel,
		)
		if _, err := EvalJSON(ctx, conn, script); err == nil {
			return nil
		}
	}
	// No valid selector or eval failed — scroll to the element's approximate
	// Y position using its bounds.
	if node.Bounds != nil {
		y := int(node.Bounds.Y)
		_, err := EvalJSON(ctx, conn,
			fmt.Sprintf("window.scrollTo(0,Math.max(0,%d-Math.floor(window.innerHeight/2)))", y))
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
	_, err := EvalJSON(ctx, conn, fmt.Sprintf("window.scrollBy(%d,%d)", deltaX, deltaY))
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
		result, err := EvalJSON(ctx, conn, fmt.Sprintf("!!document.querySelector(%q)", selector))
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
