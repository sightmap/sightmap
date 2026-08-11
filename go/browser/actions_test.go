package browser

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/sightmap/sightmap/go/sightmap"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// newFakeCDP starts a minimal fake CDP server and returns a connected CDPConn.
// handler is called for every CDP command and must return the result JSON.
// A synthetic Page.loadEventFired event is sent 50 ms after any Page.navigate.
func newFakeCDP(t *testing.T, handler func(method string, params json.RawMessage) json.RawMessage) *CDPConn {
	t.Helper()

	mux := http.NewServeMux()
	srv := httptest.NewUnstartedServer(mux)

	// /json/list — return a single fake "page" target pointing at our WS handler.
	mux.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		wsURL := "ws://" + r.Host + "/ws"
		targets := []map[string]string{
			{"type": "page", "webSocketDebuggerUrl": wsURL, "url": "about:blank"},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(targets); err != nil {
			t.Logf("newFakeCDP: encode targets: %v", err)
		}
	})

	// /ws — WebSocket handler that speaks CDP.
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		wsConn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("newFakeCDP: ws upgrade: %v", err)
			return
		}

		var wsMu sync.Mutex

		writeMsg := func(v interface{}) {
			b, _ := json.Marshal(v)
			wsMu.Lock()
			wsConn.WriteMessage(websocket.TextMessage, b)
			wsMu.Unlock()
		}

		go func() {
			for {
				_, raw, err := wsConn.ReadMessage()
				if err != nil {
					return
				}
				var req struct {
					ID     int64           `json:"id"`
					Method string          `json:"method"`
					Params json.RawMessage `json:"params"`
				}
				if err := json.Unmarshal(raw, &req); err != nil {
					continue
				}

				result := handler(req.Method, req.Params)
				writeMsg(map[string]interface{}{
					"id":     req.ID,
					"result": result,
				})

				// Fire loadEventFired then frameNavigated 50 ms after any navigate.
				if req.Method == "Page.navigate" {
					go func() {
						time.Sleep(50 * time.Millisecond)
						writeMsg(map[string]interface{}{
							"method": "Page.loadEventFired",
							"params": map[string]interface{}{"timestamp": 1.0},
						})
						writeMsg(map[string]interface{}{
							"method": "Page.frameNavigated",
							"params": map[string]interface{}{"frame": map[string]interface{}{"url": "about:blank"}},
						})
					}()
				}
			}
		}()
	})

	srv.Start()
	t.Cleanup(srv.Close)

	conn, err := DialCDP(context.Background(), srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("newFakeCDP: DialCDP: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return conn
}

// TestSubscribeUnsubscribe verifies that Subscribe delivers events and
// Unsubscribe stops delivery.
func TestSubscribeUnsubscribe(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		return json.RawMessage(`{}`)
	})

	ch := conn.Subscribe("Page.loadEventFired")

	ctx := context.Background()
	if err := Navigate(ctx, conn, "about:blank"); err != nil {
		t.Fatal(err)
	}

	// First event must arrive (fake server sends it 50 ms after navigate).
	select {
	case <-ch:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: Page.loadEventFired not received after Subscribe")
	}

	conn.Unsubscribe("Page.loadEventFired", ch)

	// Navigate again; ch must NOT receive another event.
	if err := Navigate(ctx, conn, "about:blank"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
		t.Error("received event on channel after Unsubscribe")
	case <-time.After(300 * time.Millisecond):
		// good: no event delivered
	}
}

// TestEnableDomain verifies that EnableDomain sends the correct CDP command.
func TestEnableDomain(t *testing.T) {
	var mu sync.Mutex
	var methods []string

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		mu.Lock()
		methods = append(methods, method)
		mu.Unlock()
		return json.RawMessage(`{}`)
	})

	if err := conn.EnableDomain(context.Background(), "Page"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, m := range methods {
		if m == "Page.enable" {
			return // pass
		}
	}
	t.Errorf("Page.enable was not sent; methods seen: %v", methods)
}

// TestNavigateAndWait verifies that NavigateAndWait returns once the page fires
// loadEventFired without hanging.
func TestNavigateAndWait(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		return json.RawMessage(`{}`)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := NavigateAndWait(ctx, conn, "about:blank"); err != nil {
		t.Fatal(err)
	}
}

// TestEvalJSON verifies that EvalJSON unwraps the double-envelope correctly.
func TestEvalJSON(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			return json.RawMessage(`{"result":{"type":"number","value":42},"exceptionDetails":null}`)
		}
		return json.RawMessage(`{}`)
	})

	result, err := EvalJSON(context.Background(), conn, "21+21")
	if err != nil {
		t.Fatal(err)
	}

	var n int
	if err := json.Unmarshal(result, &n); err != nil {
		t.Fatalf("unmarshal eval result: %v", err)
	}
	if n != 42 {
		t.Errorf("EvalJSON: got %d, want 42", n)
	}
}

// TestEvalJSONException verifies that EvalJSON returns an error when the
// script throws a JS exception.
func TestEvalJSONException(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			return json.RawMessage(`{"result":{"type":"undefined"},"exceptionDetails":{"text":"ReferenceError: x is not defined"}}`)
		}
		return json.RawMessage(`{}`)
	})

	_, err := EvalJSON(context.Background(), conn, "undeclaredVariable")
	if err == nil {
		t.Fatal("EvalJSON: expected error for JS exception, got nil")
	}
	if !strings.Contains(err.Error(), "ReferenceError") {
		t.Errorf("EvalJSON: error should mention ReferenceError; got: %v", err)
	}
}

// TestClickAt verifies that ClickAt sends the correct three-event mouse sequence.
func TestClickAt(t *testing.T) {
	type mouseEv struct {
		Type       string `json:"type"`
		X          int    `json:"x"`
		Y          int    `json:"y"`
		Button     string `json:"button"`
		ClickCount int    `json:"clickCount"`
	}

	var mu sync.Mutex
	var mouseEvents []mouseEv

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Input.dispatchMouseEvent" {
			var ev mouseEv
			json.Unmarshal(params, &ev)
			mu.Lock()
			mouseEvents = append(mouseEvents, ev)
			mu.Unlock()
		}
		return json.RawMessage(`{}`)
	})

	if err := ClickAt(context.Background(), conn, 100, 200); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(mouseEvents) != 3 {
		t.Fatalf("expected 3 mouse events, got %d", len(mouseEvents))
	}
	wantTypes := []string{"mouseMoved", "mousePressed", "mouseReleased"}
	for i, ev := range mouseEvents {
		if ev.Type != wantTypes[i] {
			t.Errorf("event %d: type=%q, want %q", i, ev.Type, wantTypes[i])
		}
		if ev.X != 100 || ev.Y != 200 {
			t.Errorf("event %d: coords=(%d,%d), want (100,200)", i, ev.X, ev.Y)
		}
	}
	if mouseEvents[0].Button != "none" {
		t.Errorf("mouseMoved button=%q, want \"none\"", mouseEvents[0].Button)
	}
	if mouseEvents[1].Button != "left" || mouseEvents[1].ClickCount != 1 {
		t.Errorf("mousePressed: button=%q clickCount=%d, want left/1", mouseEvents[1].Button, mouseEvents[1].ClickCount)
	}
	if mouseEvents[2].Button != "left" || mouseEvents[2].ClickCount != 1 {
		t.Errorf("mouseReleased: button=%q clickCount=%d, want left/1", mouseEvents[2].Button, mouseEvents[2].ClickCount)
	}
}

// TestClick_WithBounds verifies that Click dispatches a click to the node's center.
func TestClick_WithBounds(t *testing.T) {
	type coordEv struct {
		X int `json:"x"`
		Y int `json:"y"`
	}

	var mu sync.Mutex
	var events []coordEv

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Input.dispatchMouseEvent" {
			var ev coordEv
			json.Unmarshal(params, &ev)
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		}
		return json.RawMessage(`{}`)
	})

	// Center = (100+60/2, 200+40/2) = (130, 220)
	node := &sightmap.ComponentNode{
		Id:     "test",
		Bounds: &sightmap.Bounds{X: 100, Y: 200, Width: 60, Height: 40},
	}
	if _, _, err := Click(context.Background(), conn, node); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(events) != 3 {
		t.Fatalf("expected 3 mouse events, got %d", len(events))
	}
	for i, ev := range events {
		if ev.X != 130 || ev.Y != 220 {
			t.Errorf("event %d: coords=(%d,%d), want (130,220)", i, ev.X, ev.Y)
		}
	}
}

// TestClick_NoBounds verifies that Click falls back to JS click() when Bounds is nil.
func TestClick_NoBounds(t *testing.T) {
	var mu sync.Mutex
	var evalScript string

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			var p struct {
				Expression string `json:"expression"`
			}
			json.Unmarshal(params, &p)
			mu.Lock()
			evalScript = p.Expression
			mu.Unlock()
			return json.RawMessage(`{"result":{"type":"undefined","value":null}}`)
		}
		return json.RawMessage(`{}`)
	})

	node := &sightmap.ComponentNode{
		Id:      "test",
		Element: &sightmap.Element{Tag: "button", Id: "submit"},
	}
	if _, _, err := Click(context.Background(), conn, node); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()

	if !strings.Contains(evalScript, "button#submit") {
		t.Errorf("JS fallback should target button#submit, got: %s", evalScript)
	}
	if !strings.Contains(evalScript, ".click()") {
		t.Errorf("JS fallback should call .click(), got: %s", evalScript)
	}
}

// TestResolveBySightmapID verifies that resolution anchors to the persisted
// data-sightmap-id attribute (not a re-extraction) and returns the element's
// current bounds. This is the core of the race fix: the id minted by a
// prior snapshot must resolve without re-probing the page.
func TestResolveBySightmapID(t *testing.T) {
	var mu sync.Mutex
	var evalScript string

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			var p struct {
				Expression string `json:"expression"`
			}
			json.Unmarshal(params, &p)
			mu.Lock()
			evalScript = p.Expression
			mu.Unlock()
			return json.RawMessage(`{"result":{"type":"object","value":{"x":10,"y":20,"width":30,"height":40}}}`)
		}
		return json.RawMessage(`{}`)
	})

	node, err := ResolveBySightmapID(context.Background(), conn, "805")
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	script := evalScript
	mu.Unlock()

	if !strings.Contains(script, `data-sightmap-id="805"`) {
		t.Errorf("resolution should anchor to data-sightmap-id=\"805\", got: %s", script)
	}
	if !strings.Contains(script, "getBoundingClientRect") {
		t.Errorf("resolution should read live bounds via getBoundingClientRect, got: %s", script)
	}
	if node.Id != "805" {
		t.Errorf("node Id = %q, want 805", node.Id)
	}
	if node.Bounds == nil {
		t.Fatal("node Bounds is nil")
	}
	if got := *node.Bounds; got != (sightmap.Bounds{X: 10, Y: 20, Width: 30, Height: 40}) {
		t.Errorf("bounds = %+v, want {10 20 30 40}", got)
	}
}

// TestResolveBySightmapID_NotFound verifies a clear error when the element carrying
// the id is gone (removed or re-probed away), so callers can prompt for a fresh
// snapshot rather than acting on a stale id.
func TestResolveBySightmapID_NotFound(t *testing.T) {
	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			return json.RawMessage(`{"result":{"type":"object","value":null}}`)
		}
		return json.RawMessage(`{}`)
	})

	_, err := ResolveBySightmapID(context.Background(), conn, "805")
	if err == nil {
		t.Fatal("expected error when data-sightmap-id is absent, got nil")
	}
	if !strings.Contains(err.Error(), "805") || !strings.Contains(err.Error(), "snapshot") {
		t.Errorf("error should name the id and suggest a fresh snapshot, got: %v", err)
	}
}

// TestFill verifies the click + select-all + keystroke sequence.
func TestFill(t *testing.T) {
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
		return json.RawMessage(`{}`)
	})

	node := &sightmap.ComponentNode{
		Id:     "field",
		Bounds: &sightmap.Bounds{X: 0, Y: 0, Width: 100, Height: 30},
	}
	if err := Fill(context.Background(), conn, node, "hi"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()

	mouseCount := 0
	keyDownCount := 0
	keyUpCount := 0
	var selectAllMods int

	for _, c := range calls {
		switch c.method {
		case "Input.dispatchMouseEvent":
			mouseCount++
		case "Input.dispatchKeyEvent":
			var ev struct {
				Type      string `json:"type"`
				Key       string `json:"key"`
				Modifiers int    `json:"modifiers"`
			}
			json.Unmarshal(c.params, &ev)
			if ev.Type == "keyDown" {
				keyDownCount++
				if ev.Key == "a" {
					selectAllMods = ev.Modifiers
				}
			} else if ev.Type == "keyUp" {
				keyUpCount++
			}
		}
	}

	// 3 mouse events (click) + 1 select-all keyDown + 2 char keyDowns = 3 keyDowns
	if mouseCount != 3 {
		t.Errorf("expected 3 mouse events, got %d", mouseCount)
	}
	if keyDownCount != 3 { // select-all + 'h' + 'i'
		t.Errorf("expected 3 keyDown events, got %d", keyDownCount)
	}
	if keyUpCount != 3 { // select-all + 'h' + 'i'
		t.Errorf("expected 3 keyUp events, got %d", keyUpCount)
	}
	if selectAllMods != 10 {
		t.Errorf("select-all modifiers=%d, want 10 (Ctrl|Meta)", selectAllMods)
	}
}

// TestKeyPress_Enter verifies keyDown+keyUp with key="Enter" and text="\r".
func TestKeyPress_Enter(t *testing.T) {
	type keyEv struct {
		Type string `json:"type"`
		Key  string `json:"key"`
		Text string `json:"text"`
	}
	var mu sync.Mutex
	var keyEvents []keyEv

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Input.dispatchKeyEvent" {
			var ev keyEv
			json.Unmarshal(params, &ev)
			mu.Lock()
			keyEvents = append(keyEvents, ev)
			mu.Unlock()
		}
		return json.RawMessage(`{}`)
	})

	if err := KeyPress(context.Background(), conn, "Enter"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(keyEvents) != 2 {
		t.Fatalf("expected 2 key events, got %d", len(keyEvents))
	}
	if keyEvents[0].Type != "keyDown" || keyEvents[0].Key != "Enter" {
		t.Errorf("event 0: type=%q key=%q, want keyDown/Enter", keyEvents[0].Type, keyEvents[0].Key)
	}
	if keyEvents[0].Text != "\r" {
		t.Errorf("keyDown text=%q, want \\r", keyEvents[0].Text)
	}
	if keyEvents[1].Type != "keyUp" || keyEvents[1].Key != "Enter" {
		t.Errorf("event 1: type=%q key=%q, want keyUp/Enter", keyEvents[1].Type, keyEvents[1].Key)
	}
}

// TestScrollBy verifies that ScrollBy sends a mouseWheel event with correct deltas.
func TestScrollBy(t *testing.T) {
	var mu sync.Mutex
	var gotExpr string

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			mu.Lock()
			var p struct {
				Expression string `json:"expression"`
			}
			json.Unmarshal(params, &p)
			gotExpr = p.Expression
			mu.Unlock()
		}
		// Return a valid Runtime.evaluate response (undefined result, no exception).
		return json.RawMessage(`{"result":{"type":"undefined"}}`)
	})

	if err := ScrollBy(context.Background(), conn, 0, 300); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()

	if !strings.Contains(gotExpr, "scrollBy") {
		t.Errorf("expression=%q, want window.scrollBy call", gotExpr)
	}
	if !strings.Contains(gotExpr, "300") {
		t.Errorf("expression=%q, want deltaY 300 in expression", gotExpr)
	}
}

// TestWaitForURL verifies that WaitForURL polls until the URL contains pattern.
func TestWaitForURL(t *testing.T) {
	var mu sync.Mutex
	pollCount := 0

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Target.getTargetInfo" {
			mu.Lock()
			pollCount++
			n := pollCount
			mu.Unlock()
			if n >= 3 {
				return json.RawMessage(`{"targetInfo":{"url":"https://example.com/target-page"}}`)
			}
			return json.RawMessage(`{"targetInfo":{"url":"https://example.com/other"}}`)
		}
		return json.RawMessage(`{}`)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := WaitForURL(ctx, conn, "target-page"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if pollCount < 3 {
		t.Errorf("expected at least 3 polls, got %d", pollCount)
	}
}

// TestWaitForSelector verifies polling until document.querySelector returns true.
func TestWaitForSelector(t *testing.T) {
	var mu sync.Mutex
	evalCount := 0

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Runtime.evaluate" {
			mu.Lock()
			evalCount++
			n := evalCount
			mu.Unlock()
			if n >= 2 {
				return json.RawMessage(`{"result":{"type":"boolean","value":true}}`)
			}
			return json.RawMessage(`{"result":{"type":"boolean","value":false}}`)
		}
		return json.RawMessage(`{}`)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := WaitForSelector(ctx, conn, "#my-element"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if evalCount < 2 {
		t.Errorf("expected at least 2 evals, got %d", evalCount)
	}
}

// TestHandleDialog verifies that HandleDialog calls Page.handleJavaScriptDialog.
func TestHandleDialog(t *testing.T) {
	var mu sync.Mutex
	var dialogParams struct {
		Accept     bool   `json:"accept"`
		PromptText string `json:"promptText"`
	}
	var dialogCalled bool

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Page.handleJavaScriptDialog" {
			mu.Lock()
			json.Unmarshal(params, &dialogParams)
			dialogCalled = true
			mu.Unlock()
		}
		return json.RawMessage(`{}`)
	})

	if err := HandleDialog(context.Background(), conn, "accept", "my input"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()

	if !dialogCalled {
		t.Fatal("Page.handleJavaScriptDialog was not called")
	}
	if !dialogParams.Accept {
		t.Errorf("accept=%v, want true", dialogParams.Accept)
	}
	if dialogParams.PromptText != "my input" {
		t.Errorf("promptText=%q, want \"my input\"", dialogParams.PromptText)
	}
}

// TestScreenshot verifies that Screenshot returns the decoded PNG bytes.
func TestScreenshot(t *testing.T) {
	want := []byte("fake-png-bytes")
	encoded := base64.StdEncoding.EncodeToString(want)

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Page.captureScreenshot" {
			return json.RawMessage(`{"data":"` + encoded + `"}`)
		}
		return json.RawMessage(`{}`)
	})

	got, err := Screenshot(context.Background(), conn)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Screenshot: got %q, want %q", got, want)
	}
}

func TestScreenshotWithOptions_JPEG(t *testing.T) {
	want := []byte("fake-jpeg-bytes")
	encoded := base64.StdEncoding.EncodeToString(want)

	var mu sync.Mutex
	var capturedParams json.RawMessage

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Page.captureScreenshot" {
			mu.Lock()
			capturedParams = params
			mu.Unlock()
			return json.RawMessage(`{"data":"` + encoded + `"}`)
		}
		return json.RawMessage(`{}`)
	})

	got, err := ScreenshotWithOptions(context.Background(), conn, ScreenshotOptions{
		Format: "jpeg",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("ScreenshotWithOptions JPEG: got %q, want %q", got, want)
	}

	mu.Lock()
	p := capturedParams
	mu.Unlock()

	var fields struct {
		Format string `json:"format"`
	}
	if err := json.Unmarshal(p, &fields); err != nil {
		t.Fatalf("unmarshal captureScreenshot params: %v", err)
	}
	if fields.Format != "jpeg" {
		t.Errorf("captureScreenshot format: got %q, want \"jpeg\"", fields.Format)
	}
}

func TestScreenshotWithOptions_OptimizeForSpeed(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("fake"))

	var mu sync.Mutex
	var capturedParams json.RawMessage

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Page.captureScreenshot" {
			mu.Lock()
			capturedParams = params
			mu.Unlock()
			return json.RawMessage(`{"data":"` + encoded + `"}`)
		}
		return json.RawMessage(`{}`)
	})

	_, err := ScreenshotWithOptions(context.Background(), conn, ScreenshotOptions{
		Format:           "jpeg",
		OptimizeForSpeed: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	p := capturedParams
	mu.Unlock()

	var fields struct {
		OptimizeForSpeed bool `json:"optimizeForSpeed"`
	}
	if err := json.Unmarshal(p, &fields); err != nil {
		t.Fatalf("unmarshal captureScreenshot params: %v", err)
	}
	if !fields.OptimizeForSpeed {
		t.Error("captureScreenshot: expected optimizeForSpeed:true in params")
	}
}

func TestScreenshotWithOptions_Clip(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("fake"))

	var mu sync.Mutex
	var capturedParams json.RawMessage

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Page.captureScreenshot" {
			mu.Lock()
			capturedParams = params
			mu.Unlock()
			return json.RawMessage(`{"data":"` + encoded + `"}`)
		}
		return json.RawMessage(`{}`)
	})

	_, err := ScreenshotWithOptions(context.Background(), conn, ScreenshotOptions{
		Clip: &ScreenshotClip{X: 10, Y: 20, Width: 100, Height: 50},
	})
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	p := capturedParams
	mu.Unlock()

	var fields struct {
		CaptureBeyondViewport bool `json:"captureBeyondViewport"`
		Clip                  *struct {
			X      float64 `json:"x"`
			Y      float64 `json:"y"`
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
			Scale  float64 `json:"scale"`
		} `json:"clip"`
	}
	if err := json.Unmarshal(p, &fields); err != nil {
		t.Fatalf("unmarshal captureScreenshot params: %v", err)
	}
	if fields.Clip == nil {
		t.Fatal("captureScreenshot: expected a clip in params")
	}
	// Scroll offset is 0 in the fake harness, so the box passes through unchanged.
	if fields.Clip.X != 10 || fields.Clip.Y != 20 || fields.Clip.Width != 100 || fields.Clip.Height != 50 {
		t.Errorf("clip box = %+v, want {10 20 100 50}", *fields.Clip)
	}
	if fields.Clip.Scale != 1 {
		t.Errorf("clip scale = %v, want 1", fields.Clip.Scale)
	}
	if !fields.CaptureBeyondViewport {
		t.Error("captureScreenshot: expected captureBeyondViewport:true when clipping")
	}
}

func TestScreenshotWithOptions_PauseAnimations(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("fake"))

	var mu sync.Mutex
	var callOrder []string

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		mu.Lock()
		callOrder = append(callOrder, method)
		mu.Unlock()
		if method == "Page.captureScreenshot" {
			return json.RawMessage(`{"data":"` + encoded + `"}`)
		}
		return json.RawMessage(`{}`)
	})

	_, err := ScreenshotWithOptions(context.Background(), conn, ScreenshotOptions{
		PauseAnimations: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	order := append([]string(nil), callOrder...)
	mu.Unlock()

	// Find the first Animation.setPlaybackRate (pause) and Page.captureScreenshot.
	pauseIdx := -1
	captureIdx := -1
	for i, m := range order {
		if m == "Animation.setPlaybackRate" && pauseIdx == -1 {
			pauseIdx = i
		}
		if m == "Page.captureScreenshot" {
			captureIdx = i
		}
	}

	if pauseIdx == -1 {
		t.Fatalf("Animation.setPlaybackRate was never called; calls: %v", order)
	}
	if captureIdx == -1 {
		t.Fatalf("Page.captureScreenshot was never called; calls: %v", order)
	}
	if pauseIdx >= captureIdx {
		t.Errorf("Animation.setPlaybackRate (idx %d) must precede Page.captureScreenshot (idx %d); calls: %v",
			pauseIdx, captureIdx, order)
	}
}
