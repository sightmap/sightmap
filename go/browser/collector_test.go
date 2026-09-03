package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sightmap/sightmap/go/sightmap"
)

// A retained body must be served without a live CDP fetch. These records carry a
// zero-value conn, so a live getResponseBody/getRequestPostData would fail — a
// pass proves the eagerly-retained copy was used (the #226 reliability fix).
func TestResponseBody_PrefersRetained(t *testing.T) {
	c := &Collector{network: []networkRecord{{
		Request: sightmap.Request{Index: 1, RspBody: &sightmap.Body{Content: "retained-rsp"}},
	}}}
	body, found, err := c.ResponseBody(context.Background(), 1)
	if err != nil || !found {
		t.Fatalf("ResponseBody: found=%v err=%v", found, err)
	}
	if string(body) != "retained-rsp" {
		t.Errorf("ResponseBody = %q, want retained-rsp", body)
	}
}

func TestRequestBody_PrefersRetained(t *testing.T) {
	c := &Collector{network: []networkRecord{{
		Request: sightmap.Request{Index: 2, ReqBody: &sightmap.Body{Content: "retained-req"}},
	}}}
	body, found, err := c.RequestBody(context.Background(), 2)
	if err != nil || !found {
		t.Fatalf("RequestBody: found=%v err=%v", found, err)
	}
	if string(body) != "retained-req" {
		t.Errorf("RequestBody = %q, want retained-req", body)
	}
}

func TestResponseBody_NotFound(t *testing.T) {
	c := &Collector{}
	if _, found, err := c.ResponseBody(context.Background(), 99); found || err != nil {
		t.Errorf("ResponseBody(absent) = found=%v err=%v, want found=false, nil", found, err)
	}
}

func TestParseConsoleAPI(t *testing.T) {
	raw := json.RawMessage(`{"type":"warning","timestamp":1785015495306,"args":[{"type":"string","value":"hello"},{"type":"number","value":42}]}`)
	e, ok := parseConsoleAPI(raw)
	if !ok {
		t.Fatal("parseConsoleAPI returned ok=false")
	}
	if e.Level != "warn" {
		t.Errorf("level = %q, want warn", e.Level)
	}
	if e.Text != "hello 42" {
		t.Errorf("text = %q, want \"hello 42\"", e.Text)
	}
	if e.Ts != 1785015495306 {
		t.Errorf("ts = %d, want 1785015495306", e.Ts)
	}
}

func TestParseException(t *testing.T) {
	raw := json.RawMessage(`{"timestamp":1785015495306,"exceptionDetails":{"text":"Uncaught","exception":{"description":"TypeError: x is not a function"},"stackTrace":{"callFrames":[{"functionName":"syncCart","url":"https://x/cart.js","lineNumber":41,"columnNumber":8},{"functionName":"","url":"https://x/main.js","lineNumber":0,"columnNumber":0}]}}}`)
	e, ok := parseException(raw)
	if !ok {
		t.Fatal("parseException returned ok=false")
	}
	if e.Level != "exception" {
		t.Errorf("level = %q, want exception", e.Level)
	}
	if e.Text != "TypeError: x is not a function" {
		t.Errorf("text = %q", e.Text)
	}
	// The stack is captured, throwing frame first, with 0-based line/column as
	// non-nil pointers (a captured 0 is a real location).
	if len(e.Stack) != 2 {
		t.Fatalf("stack frames = %d, want 2", len(e.Stack))
	}
	top := e.Stack[0]
	if top.Function != "syncCart" || top.File != "https://x/cart.js" || top.Line == nil || *top.Line != 41 {
		t.Errorf("top frame = %+v", top)
	}
	if f2 := e.Stack[1]; f2.Line == nil || *f2.Line != 0 || f2.Column == nil || *f2.Column != 0 {
		t.Errorf("second frame captured-zero line/column not preserved: %+v", f2)
	}
}

func TestParseRequestResponse(t *testing.T) {
	req := json.RawMessage(`{"requestId":"R1","type":"XHR","request":{"url":"https://x/api","method":"POST","headers":{"Authorization":"Bearer t"}}}`)
	e, ok := parseRequest(req)
	if !ok {
		t.Fatal("parseRequest ok=false")
	}
	if e.Method != "POST" || e.URL != "https://x/api" || e.ResourceType != "XHR" || e.requestID != "R1" {
		t.Errorf("request parse = %+v", e)
	}
	if len(e.ReqHeaders) != 1 || e.ReqHeaders[0].Name != "Authorization" || e.ReqHeaders[0].Value != "Bearer t" {
		t.Errorf("request headers = %+v", e.ReqHeaders)
	}

	resp := json.RawMessage(`{"requestId":"R1","type":"XHR","response":{"status":201,"statusText":"Created","headers":{"Content-Type":"application/json","X-RateLimit-Remaining":"0"}}}`)
	info, ok := parseResponse(resp)
	if !ok || info.reqID != "R1" || info.status != 201 || info.statusText != "Created" || info.rtype != "XHR" {
		t.Errorf("response parse = %+v %v", info, ok)
	}
	// Response headers are captured, sorted by name for determinism.
	if len(info.headers) != 2 || info.headers[0].Name != "Content-Type" || info.headers[1].Value != "0" {
		t.Errorf("response headers = %+v", info.headers)
	}
}

func TestRingOverflowAndDropped(t *testing.T) {
	c := NewCollector("localhost:0")
	c.maxEntries = 3
	for i := 0; i < 5; i++ {
		c.addConsole(sightmap.Message{Level: "log", Text: "m"})
	}
	got, dropped := c.Console(ConsoleFilter{})
	if len(got) != 3 {
		t.Fatalf("buffered = %d, want 3 (capped)", len(got))
	}
	if dropped != 2 {
		t.Errorf("dropped = %d, want 2", dropped)
	}
	// Indexes are monotonic and reflect the retained tail (2,3,4).
	if got[0].Index != 2 || got[2].Index != 4 {
		t.Errorf("indexes = %d..%d, want 2..4", got[0].Index, got[2].Index)
	}
}

func TestConsoleFilterAndLimit(t *testing.T) {
	c := NewCollector("localhost:0")
	c.addConsole(sightmap.Message{Level: "log", Text: "a", Tab: "T1"})
	c.addConsole(sightmap.Message{Level: "error", Text: "b", Tab: "T1"})
	c.addConsole(sightmap.Message{Level: "error", Text: "c", Tab: "T2"})

	errs, _ := c.Console(ConsoleFilter{Level: "error"})
	if len(errs) != 2 {
		t.Fatalf("level=error → %d, want 2", len(errs))
	}
	t2, _ := c.Console(ConsoleFilter{Tab: "T2"})
	if len(t2) != 1 || t2[0].Text != "c" {
		t.Errorf("tab=T2 → %+v", t2)
	}
	last, _ := c.Console(ConsoleFilter{Limit: 1})
	if len(last) != 1 || last[0].Text != "c" {
		t.Errorf("limit=1 → %+v (want most recent)", last)
	}
}

func TestNetworkFilterAndResponseMatch(t *testing.T) {
	c := NewCollector("localhost:0")
	c.addNetwork(networkRecord{Request: sightmap.Request{Method: "GET", URL: "https://x/style.css", ResourceType: "Stylesheet"}, requestID: "A"})
	c.addNetwork(networkRecord{Request: sightmap.Request{Method: "POST", URL: "https://x/api/cart", ResourceType: "XHR"}, requestID: "B"})
	c.applyResponse(responseInfo{reqID: "B", status: 500, statusText: "Server Error", rtype: "XHR"})

	xhr, _ := c.Network(NetworkFilter{ResourceType: "xhr"})
	if len(xhr) != 1 || xhr[0].Status != 500 || xhr[0].StatusText != "Server Error" {
		t.Fatalf("xhr filter/response = %+v", xhr)
	}
	byURL, _ := c.Network(NetworkFilter{URLSubstr: "CART"})
	if len(byURL) != 1 || byURL[0].URL != "https://x/api/cart" {
		t.Errorf("url substring (case-insensitive) = %+v", byURL)
	}
}

func TestDecodeBody(t *testing.T) {
	plain, err := decodeBody(json.RawMessage(`{"body":"hello","base64Encoded":false}`))
	if err != nil || string(plain) != "hello" {
		t.Errorf("plain body = %q, %v", plain, err)
	}
	enc, err := decodeBody(json.RawMessage(`{"body":"aGk=","base64Encoded":true}`))
	if err != nil || string(enc) != "hi" {
		t.Errorf("base64 body = %q, %v", enc, err)
	}
}

// TestCollectorStop_NoDeadlockWhileConnected pins Stop's shutdown ordering: a
// drain returns only once its context is cancelled, so Stop must cancel every
// tab context, via the shared parent c.ctx, before wg.Wait. Simulates a
// still-attached tab whose drain is blocked on a healthy CDP connection, the
// case where the browser outlives the collector, and asserts Stop returns
// promptly rather than deadlocking.
func TestCollectorStop_NoDeadlockWhileConnected(t *testing.T) {
	c := NewCollector("unused")
	ctx, cancel := context.WithCancel(c.ctx)
	c.tabsMu.Lock()
	c.tabs["t1"] = &tabColl{conn: nil, cancel: cancel}
	c.tabsMu.Unlock()
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		<-ctx.Done() // exits only when Stop cancels the tab's context
	}()

	done := make(chan struct{})
	go func() { c.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Collector.Stop deadlocked with a healthy (uncancelled) drain")
	}
}

// TestCollector_PersistentScripts drives the real attach path against a fake
// browser that records the CDP commands it receives, then exercises the
// persistent-script registry: adding a script must send
// Page.addScriptToEvaluateOnNewDocument with the source to the attached tab, and
// removing it must send Page.removeScriptToEvaluateOnNewDocument with the
// identifier CDP handed back.
func TestCollector_PersistentScripts(t *testing.T) {
	rec := &cdpRecorder{}
	c := NewCollector(newRecordingBrowser(t, rec))
	c.Start()
	t.Cleanup(c.Stop)
	waitForTabs(t, c, 1, "collector never attached the fake tab")

	ctx := context.Background()
	const src = "window.__x = 1;"
	id, err := c.AddPersistentScript(ctx, src)
	if err != nil || id == "" {
		t.Fatalf("AddPersistentScript: id=%q err=%v", id, err)
	}

	adds := rec.withMethod("Page.addScriptToEvaluateOnNewDocument")
	if len(adds) != 1 {
		t.Fatalf("add: sent %d addScriptToEvaluateOnNewDocument, want 1", len(adds))
	}
	if adds[0].Source != src {
		t.Errorf("add: source = %q, want %q", adds[0].Source, src)
	}

	// Regression guard: Page must be enabled before the add, or the script is
	// registered but never fires on new documents (persist-across-navigation bug).
	enableIdx := rec.firstIndexOf("Page.enable")
	addIdx := rec.firstIndexOf("Page.addScriptToEvaluateOnNewDocument")
	if enableIdx < 0 {
		t.Error("Page.enable was never sent; persisted scripts won't fire on navigation")
	} else if enableIdx > addIdx {
		t.Errorf("Page.enable (idx %d) sent AFTER addScriptToEvaluateOnNewDocument (idx %d); must precede it", enableIdx, addIdx)
	}

	if got := c.PersistentScripts(); len(got) != 1 || got[0].ID != id || got[0].Tabs != 1 || got[0].Bytes != len(src) {
		t.Fatalf("PersistentScripts() = %+v, want one {ID:%s Tabs:1 Bytes:%d}", got, id, len(src))
	}

	removed, err := c.RemovePersistentScript(ctx, id)
	if err != nil || !removed {
		t.Fatalf("RemovePersistentScript: removed=%v err=%v", removed, err)
	}
	rems := rec.withMethod("Page.removeScriptToEvaluateOnNewDocument")
	if len(rems) != 1 {
		t.Fatalf("remove: sent %d removeScriptToEvaluateOnNewDocument, want 1", len(rems))
	}
	// The identifier removed must be the one the fake returned for the add.
	if rems[0].Identifier != "cdp-script-1" {
		t.Errorf("remove: identifier = %q, want cdp-script-1", rems[0].Identifier)
	}
	if got := c.PersistentScripts(); len(got) != 0 {
		t.Errorf("after remove: PersistentScripts() = %+v, want empty", got)
	}

	// Unknown id is a clean no-op.
	if removed, err := c.RemovePersistentScript(ctx, "inj-999"); removed || err != nil {
		t.Errorf("RemovePersistentScript(absent) = removed=%v err=%v, want false, nil", removed, err)
	}
}

// cdpRecorder captures the injection-related CDP commands a fake browser saw.
type cdpRecorder struct {
	mu    sync.Mutex
	calls []recordedCall
}

type recordedCall struct {
	Method     string
	Source     string
	Identifier string
}

func (r *cdpRecorder) add(call recordedCall) {
	r.mu.Lock()
	r.calls = append(r.calls, call)
	r.mu.Unlock()
}

func (r *cdpRecorder) withMethod(method string) []recordedCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []recordedCall
	for _, c := range r.calls {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

// firstIndexOf returns the position of the first recorded call for method, or -1.
func (r *cdpRecorder) firstIndexOf(method string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, c := range r.calls {
		if c.Method == method {
			return i
		}
	}
	return -1
}

// newRecordingBrowser is newFakeBrowser plus command recording: it records the
// injection commands into rec and replies to addScriptToEvaluateOnNewDocument
// with a stable identifier so removal has something to target.
func newRecordingBrowser(t *testing.T, rec *cdpRecorder) string {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewUnstartedServer(mux)

	mux.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		targets := []map[string]string{{
			"id": "tab-1", "type": "page", "url": "http://fake.test/",
			"webSocketDebuggerUrl": "ws://" + r.Host + "/ws",
		}}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(targets); err != nil {
			t.Logf("newRecordingBrowser: encode targets: %v", err)
		}
	})

	var scriptSeq int
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		wsConn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("newRecordingBrowser: ws upgrade: %v", err)
			return
		}
		var wsMu sync.Mutex
		go func() {
			for {
				_, raw, readErr := wsConn.ReadMessage()
				if readErr != nil {
					return
				}
				var req struct {
					ID     int64  `json:"id"`
					Method string `json:"method"`
					Params struct {
						Source     string `json:"source"`
						Identifier string `json:"identifier"`
					} `json:"params"`
				}
				if json.Unmarshal(raw, &req) != nil {
					continue
				}
				result := map[string]any{}
				switch req.Method {
				case "Page.addScriptToEvaluateOnNewDocument":
					scriptSeq++
					ident := fmt.Sprintf("cdp-script-%d", scriptSeq)
					result["identifier"] = ident
					rec.add(recordedCall{Method: req.Method, Source: req.Params.Source, Identifier: ident})
				case "Page.removeScriptToEvaluateOnNewDocument":
					rec.add(recordedCall{Method: req.Method, Identifier: req.Params.Identifier})
				default:
					// Record every other command (e.g. Page.enable) so ordering
					// assertions can see the domain was enabled before the add.
					rec.add(recordedCall{Method: req.Method})
				}
				reply, _ := json.Marshal(map[string]any{"id": req.ID, "result": result})
				wsMu.Lock()
				err := wsConn.WriteMessage(websocket.TextMessage, reply)
				wsMu.Unlock()
				if err != nil {
					return
				}
			}
		}()
	})

	srv.Start()
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().String()
}

// newFakeBrowser starts a fake Chrome CDP endpoint exposing a single page target
// and returns its host:port. The websocket answers every command with an empty
// result, which is all attachTab's Runtime.enable and Network.enable require.
// onList, when non-nil, runs at the head of each /json/list request, letting a
// test stall the tab poll.
func newFakeBrowser(t *testing.T, onList func(r *http.Request)) string {
	t.Helper()

	mux := http.NewServeMux()
	srv := httptest.NewUnstartedServer(mux)

	mux.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		if onList != nil {
			onList(r)
		}
		targets := []map[string]string{{
			"id":                   "tab-1",
			"type":                 "page",
			"url":                  "http://fake.test/",
			"webSocketDebuggerUrl": "ws://" + r.Host + "/ws",
		}}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(targets); err != nil {
			t.Logf("newFakeBrowser: encode targets: %v", err)
		}
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		wsConn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("newFakeBrowser: ws upgrade: %v", err)
			return
		}
		var wsMu sync.Mutex
		go func() {
			for {
				_, raw, readErr := wsConn.ReadMessage()
				if readErr != nil {
					return
				}
				var req struct {
					ID int64 `json:"id"`
				}
				if json.Unmarshal(raw, &req) != nil {
					continue
				}
				reply, _ := json.Marshal(map[string]any{"id": req.ID, "result": map[string]any{}})
				wsMu.Lock()
				err := wsConn.WriteMessage(websocket.TextMessage, reply)
				wsMu.Unlock()
				if err != nil {
					return
				}
			}
		}()
	})

	srv.Start()
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().String()
}

// waitForTabs blocks until the collector has exactly n tabs registered.
func waitForTabs(t *testing.T, c *Collector, n int, why string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		c.tabsMu.Lock()
		got := len(c.tabs)
		c.tabsMu.Unlock()
		if got == n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %d tabs registered, want %d", why, got, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestCollectorStop_LiveTabNoDeadlock drives the real attach path against a fake
// browser and stops the collector while the CDP connection is still healthy, the
// case where the caller owns the browser and it outlives the collector. Stop must
// return promptly and leave no tab registered.
func TestCollectorStop_LiveTabNoDeadlock(t *testing.T) {
	c := NewCollector(newFakeBrowser(t, nil))
	c.Start()
	t.Cleanup(c.Stop) // idempotent; covers the early-Fatal paths below
	waitForTabs(t, c, 1, "collector never attached the fake tab")

	done := make(chan struct{})
	go func() { c.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Collector.Stop deadlocked with a live, healthy tab connection")
	}

	c.tabsMu.Lock()
	remaining := len(c.tabs)
	c.tabsMu.Unlock()
	if remaining != 0 {
		t.Errorf("after Stop: %d tabs still registered, want 0", remaining)
	}
}

// TestCollectorStop_DuringStalledPoll covers the other route to a hung Stop: a
// browser that accepts the connection and then never answers the tab poll.
// ListTabs sets no client timeout, so attachLoop escapes the poll only because it
// runs on the collector context that Stop cancels.
func TestCollectorStop_DuringStalledPoll(t *testing.T) {
	polled := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	addr := newFakeBrowser(t, func(r *http.Request) {
		once.Do(func() { close(polled) })
		select {
		case <-r.Context().Done(): // the collector gave up on the request
		case <-release:
		}
	})
	// Runs before the server's own cleanup, so a stalled handler cannot wedge it.
	t.Cleanup(func() { close(release) })

	c := NewCollector(addr)
	c.Start()
	t.Cleanup(c.Stop)

	select {
	case <-polled:
	case <-time.After(3 * time.Second):
		t.Fatal("collector never polled the fake browser")
	}

	done := make(chan struct{})
	go func() { c.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Collector.Stop deadlocked while the tab poll was stalled")
	}
}
