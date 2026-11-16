package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
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

func TestIsOwnRequest(t *testing.T) {
	c := NewCollector("unused")
	c.IgnoreServerPort(7891)
	for url, want := range map[string]bool{
		// The tool's own traffic.
		"http://localhost:7891/sightmap/version":    true,
		"http://127.0.0.1:7891/sightmap":            true,
		"http://localhost:7891/anything/else":       true, // the session's server port
		"http://localhost:7893/sightmap/version":    true, // the extension probing another port
		"http://127.0.0.1:7892/sightmap":            true,
		"chrome-extension://abcdef/side-panel.html": true,
		"http://[::1]:7891/sightmap/version":        true,
		// The page's.
		"http://localhost:5291/api/issues":               false,
		"http://localhost:5291/sightmap/version-history": false,
		"https://example.com/sightmap/version":           false, // another host's own path
		"http://localhost:5291/":                         false,
		"http://localhost:3000/sightmap":                 false, // the app's own route, outside the probe range
		"http://localhost:3000/sightmap/version":         false,
		"http://localhost:7901/sightmap/version":         false, // just past the probe range
		"http://localhost/sightmap":                      false, // no port
		"http://localhost:78910/api":                     false,
		"https://api.example.com:7891/v1/things":         false, // same port, not loopback
		"not a url \x7f":                                 false,
		"data:text/plain,hi":                             false,
	} {
		if got := c.isOwnRequest(url); got != want {
			t.Errorf("isOwnRequest(%q) = %v, want %v", url, got, want)
		}
	}
	// Without a known port only the endpoint paths are recognised.
	c = NewCollector("unused")
	if c.isOwnRequest("http://localhost:7891/other") {
		t.Error("an unknown server port must not swallow arbitrary loopback requests")
	}
	if !c.isOwnRequest("http://localhost:7891/sightmap/version") {
		t.Error("the version endpoint is the tool's on any port the extension probes")
	}
}

// The extension's poll must not displace the page's requests: after far more
// poll requests than the ring holds, the app's calls are all still there with
// nothing reported dropped, and the polls never appear.
func TestDrainKeepsOwnRequestsOutOfTheBuffer(t *testing.T) {
	c := NewCollector("unused")
	c.IgnoreServerPort(7891)
	c.maxEntries = 10

	reqCh := make(chan json.RawMessage, 8)
	respCh := make(chan json.RawMessage, 8)
	otherCh := make(chan json.RawMessage)
	ctx, cancel := context.WithCancel(context.Background())
	c.wg.Add(1)
	go c.drain(ctx, "T1", nil, otherCh, otherCh, reqCh, respCh, otherCh, otherCh)
	t.Cleanup(func() { cancel(); c.wg.Wait() })

	req := func(id, url string) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"requestId":%q,"type":"Fetch","request":{"url":%q,"method":"GET","headers":{}}}`, id, url))
	}
	resp := func(id string) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"requestId":%q,"type":"Fetch","response":{"status":200,"statusText":"OK","headers":{}}}`, id))
	}

	apiURLs := []string{"http://localhost:5291/api/issues", "http://localhost:5291/api/projects", "http://localhost:5291/api/me"}
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("poll-%d", i)
		reqCh <- req(id, "http://localhost:7891/sightmap/version")
		respCh <- resp(id)
		if i%70 == 0 { // i = 0, 70, 140
			k := i / 70
			reqCh <- req(fmt.Sprintf("api-%d", k), apiURLs[k])
			// drain selects between its channels at random, so let the request
			// land before its response is sent, as it does on a real connection.
			waitFor(t, func() bool { got, _ := c.Network(NetworkFilter{}); return len(got) > k }, "app request buffered")
			respCh <- resp(fmt.Sprintf("api-%d", k))
		}
	}

	deadline := time.After(3 * time.Second)
	for {
		got, dropped := c.Network(NetworkFilter{})
		if len(got) == len(apiURLs) && got[len(got)-1].Status == 200 {
			if dropped.Main != 0 {
				t.Errorf("dropped = %d, want 0: own requests overflowed the buffer", dropped.Main)
			}
			for i, r := range got {
				if r.URL != apiURLs[i] {
					t.Errorf("entry %d = %s, want %s", i, r.URL, apiURLs[i])
				}
				if r.Status != 200 {
					t.Errorf("entry %d status = %d, want 200 (its response was applied)", i, r.Status)
				}
			}
			return
		}
		select {
		case <-deadline:
			t.Fatalf("buffer holds %d entries (dropped %d), want %d app requests", len(got), dropped.Main, len(apiURLs))
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for: %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestInStaticRing covers the one rule for which ring a record is in, and the
// type list it rests on: an unknown or empty type is not static (better to show
// it than hide it), matching is case-insensitive, and a failed load never is.
func TestInStaticRing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rtype  string
		status int
		failed bool
		want   bool
	}{
		{"script ok", "Script", 200, false, true},
		{"script cached", "Script", 304, false, true},
		{"script 204", "Script", 204, false, true},
		{"image pending", "Image", 0, false, true},
		{"stylesheet ok", "Stylesheet", 200, false, true},
		{"font ok", "Font", 200, false, true},
		{"media ok", "Media", 206, false, true},
		{"manifest ok", "Manifest", 200, false, true},
		{"text track ok", "TextTrack", 200, false, true},
		{"lower case", "script", 200, false, true},
		{"upper case", "FONT", 200, false, true},
		{"script missing", "Script", 404, false, false},
		{"stylesheet server error", "Stylesheet", 500, false, false},
		{"script redirect", "Script", 302, false, false},
		{"script failed while pending", "Script", 0, true, false},
		{"script failed after a 200", "Script", 200, true, false},
		{"fetch ok", "Fetch", 200, false, false},
		{"xhr ok", "XHR", 200, false, false},
		{"document ok", "Document", 200, false, false},
		{"event source", "EventSource", 200, false, false},
		{"websocket", "WebSocket", 101, false, false},
		{"other", "Other", 200, false, false},
		{"ping", "Ping", 200, false, false},
		{"preflight", "Preflight", 204, false, false},
		{"unknown type ok", "Hologram", 200, false, false},
		{"no type", "", 200, false, false},
		{"no type pending", "", 0, false, false},
	} {
		r := networkRecord{Request: sightmap.Request{ResourceType: tc.rtype, Status: tc.status}, failed: tc.failed}
		if got := inStaticRing(r); got != tc.want {
			t.Errorf("%s: inStaticRing(%q, %d, failed=%v) = %v, want %v", tc.name, tc.rtype, tc.status, tc.failed, got, tc.want)
		}
	}
}

func TestParseLoadingFailed(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{`{"requestId":"r","errorText":"net::ERR_CONNECTION_REFUSED"}`, true},
		{`{"requestId":"r","errorText":"net::ERR_ABORTED","canceled":true}`, false},
		{`{"requestId":"r","canceled":true,"blockedReason":"csp"}`, true},
	} {
		id, failed := parseLoadingFailed(json.RawMessage(tc.raw))
		if id != "r" || failed != tc.want {
			t.Errorf("parseLoadingFailed(%s) = %q, %v; want r, %v", tc.raw, id, failed, tc.want)
		}
	}
	if id, failed := parseLoadingFailed(json.RawMessage(`{}`)); id != "" || failed {
		t.Errorf("no requestId = %q, %v", id, failed)
	}
}

// settled adds a request and applies its response, as the drain loop does.
func settled(c *Collector, id, url, rtype string, status int) {
	// The body is the URL, so a lookup by index can tell which record it reached.
	c.addNetwork(networkRecord{Request: sightmap.Request{Method: "GET", URL: url, ResourceType: rtype, RspBody: &sightmap.Body{Content: url}}, requestID: id})
	c.applyResponse(responseInfo{reqID: id, status: status, rtype: rtype})
}

func urls(rs []sightmap.Request) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.URL)
	}
	return out
}

func TestStaticRequestsHiddenUnlessAsked(t *testing.T) {
	c := NewCollector("unused")
	settled(c, "a", "/src/main.tsx", "Script", 200)
	settled(c, "b", "/api/me", "Fetch", 200)
	settled(c, "c", "/src/app.css", "Stylesheet", 304)
	settled(c, "d", "/src/gone.js", "Script", 404)
	settled(c, "e", "/", "Document", 200)

	got, _ := c.Network(NetworkFilter{})
	if want := []string{"/api/me", "/src/gone.js", "/"}; fmt.Sprint(urls(got)) != fmt.Sprint(want) {
		t.Errorf("default = %v, want %v (a failed script stays visible)", urls(got), want)
	}
	got, _ = c.Network(NetworkFilter{IncludeStatic: true})
	if want := []string{"/src/main.tsx", "/api/me", "/src/app.css", "/src/gone.js", "/"}; fmt.Sprint(urls(got)) != fmt.Sprint(want) {
		t.Errorf("--static = %v, want all five in index order %v", urls(got), want)
	}
	got, _ = c.Network(NetworkFilter{IncludeStatic: true, ResourceType: "script"})
	if want := []string{"/src/main.tsx", "/src/gone.js"}; fmt.Sprint(urls(got)) != fmt.Sprint(want) {
		t.Errorf("--static --type script = %v, want %v", urls(got), want)
	}
	// Asking for a static type by name searches the static ring without --static.
	got, drops := c.Network(NetworkFilter{ResourceType: "Script"})
	if want := []string{"/src/main.tsx", "/src/gone.js"}; fmt.Sprint(urls(got)) != fmt.Sprint(want) {
		t.Errorf("--type Script = %v, want %v (a static type implies --static)", urls(got), want)
	}
	c.staticDropped = 5
	if _, drops = c.Network(NetworkFilter{ResourceType: "Script"}); drops.Static != 5 {
		t.Errorf("--type Script drops = %+v, want the static ring's 5 reported", drops)
	}
	if _, drops = c.Network(NetworkFilter{ResourceType: "Fetch"}); drops.Static != 0 {
		t.Errorf("--type Fetch drops = %+v, want the static ring's drops left out", drops)
	}
	got, _ = c.Network(NetworkFilter{IncludeStatic: true, Limit: 2})
	if want := []string{"/src/gone.js", "/"}; fmt.Sprint(urls(got)) != fmt.Sprint(want) {
		t.Errorf("--static --limit 2 = %v, want the newest two %v", urls(got), want)
	}
}

func TestNetworkURLPattern(t *testing.T) {
	c := NewCollector("unused")
	settled(c, "a", "http://x/api/issues", "Fetch", 200)
	settled(c, "b", "http://x/api/me", "Fetch", 200)
	settled(c, "c", "http://x/src/api.ts", "Script", 200)
	settled(c, "d", "http://x/static/logo.png", "Image", 200)

	got, _ := c.Network(NetworkFilter{URLPattern: regexp.MustCompile(`/api/(issues|me)$`)})
	if want := []string{"http://x/api/issues", "http://x/api/me"}; fmt.Sprint(urls(got)) != fmt.Sprint(want) {
		t.Errorf("filter = %v, want %v", urls(got), want)
	}
	// The pattern narrows the static ring too once it is asked for.
	got, _ = c.Network(NetworkFilter{URLPattern: regexp.MustCompile(`api`), IncludeStatic: true})
	if len(got) != 3 {
		t.Errorf("filter+static = %v, want 3 api URLs", urls(got))
	}
	got, _ = c.Network(NetworkFilter{URLPattern: regexp.MustCompile(`(?i)LOGO`)})
	if len(got) != 0 {
		t.Errorf("a hidden static request matched without --static: %v", urls(got))
	}
}

// The reason for the second ring: a dev server's module scripts fill only their
// own ring, the page's calls all survive, and the drops are charged to the ring
// that dropped.
func TestStaticBurstDoesNotEvictAppRequests(t *testing.T) {
	c := NewCollector("unused")
	c.maxEntries = 10
	c.maxStatic = 20
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("s-%d", i)
		settled(c, id, fmt.Sprintf("/src/m%d.ts", i), "Script", 200)
		if i%70 == 0 {
			settled(c, fmt.Sprintf("api-%d", i), fmt.Sprintf("/api/%d", i), "Fetch", 200)
		}
	}

	got, drops := c.Network(NetworkFilter{})
	if want := []string{"/api/0", "/api/70", "/api/140"}; fmt.Sprint(urls(got)) != fmt.Sprint(want) {
		t.Errorf("default = %v, want all 3 Fetch requests %v", urls(got), want)
	}
	if drops != (NetworkDrops{Main: 0}) {
		t.Errorf("default drops = %+v, want none: the static ring's drops are not reported without static", drops)
	}
	got, drops = c.Network(NetworkFilter{IncludeStatic: true})
	if len(got) != 3+20 {
		t.Errorf("--static returned %d entries, want 3 Fetch + 20 newest scripts", len(got))
	}
	if drops != (NetworkDrops{Main: 0, Static: 180}) {
		t.Errorf("--static drops = %+v, want {Main:0 Static:180}", drops)
	}
	// The main ring still drops on its own overflow.
	for i := 0; i < 15; i++ {
		settled(c, fmt.Sprintf("x-%d", i), fmt.Sprintf("/api/x%d", i), "XHR", 200)
	}
	if _, drops = c.Network(NetworkFilter{IncludeStatic: true}); drops != (NetworkDrops{Main: 8, Static: 180}) {
		t.Errorf("after main overflow drops = %+v, want {Main:8 Static:180}", drops)
	}
}

func TestNetworkIndexesUniqueAcrossRings(t *testing.T) {
	c := NewCollector("unused")
	settled(c, "a", "/a.js", "Script", 200)
	settled(c, "b", "/api/b", "Fetch", 200)
	settled(c, "c", "/c.css", "Stylesheet", 200)
	settled(c, "d", "/missing.js", "Script", 404) // moves static→main after its response
	settled(c, "e", "/api/e", "XHR", 200)

	all, _ := c.Network(NetworkFilter{IncludeStatic: true})
	seen := map[int]string{}
	for i, r := range all {
		if prev, dup := seen[r.Index]; dup {
			t.Errorf("index %d used by %s and %s", r.Index, prev, r.URL)
		}
		seen[r.Index] = r.URL
		if r.Index != i {
			t.Errorf("entry %d (%s) has index %d, want the order requests were made", i, r.URL, r.Index)
		}
	}
	// `network get N` works for either ring, including a record that moved.
	for _, r := range all {
		if _, _, found := c.lookupRequest(r.Index); !found {
			t.Errorf("lookupRequest(%d) (%s) not found", r.Index, r.URL)
		}
		if body, found, err := c.ResponseBody(context.Background(), r.Index); !found || err != nil || string(body) != r.URL {
			t.Errorf("ResponseBody(%d) = %q, found=%v, err=%v; want %q", r.Index, body, found, err, r.URL)
		}
	}
}

// A script is static while pending, becomes main when it fails, and a cancelled
// load (navigated away) stays out of the main ring.
func TestFailedStaticRequestMovesToMainRing(t *testing.T) {
	c := NewCollector("unused")
	add := func(id, url string) {
		c.addNetwork(networkRecord{Request: sightmap.Request{Method: "GET", URL: url, ResourceType: "Script"}, requestID: id})
	}
	add("a", "/refused.js")
	add("b", "/ok.js")
	add("c", "/aborted.js")
	c.applyResponse(responseInfo{reqID: "b", status: 200, rtype: "Script"})

	if got, _ := c.Network(NetworkFilter{}); len(got) != 0 {
		t.Fatalf("pending scripts visible by default: %v", urls(got))
	}
	c.failRequest("a")
	got, _ := c.Network(NetworkFilter{})
	if want := []string{"/refused.js"}; fmt.Sprint(urls(got)) != fmt.Sprint(want) {
		t.Errorf("after failure default = %v, want %v", urls(got), want)
	}
	if got[0].Index != 0 {
		t.Errorf("moved record index = %d, want 0 (indexes survive a move)", got[0].Index)
	}
	got, _ = c.Network(NetworkFilter{IncludeStatic: true})
	if want := []string{"/refused.js", "/ok.js", "/aborted.js"}; fmt.Sprint(urls(got)) != fmt.Sprint(want) {
		t.Errorf("--static = %v, want index order %v", urls(got), want)
	}
}

// drain reads responseReceived and loadingFailed from separate channels, so a
// response can be applied after the failure of the same request (headers, then
// net::ERR_CONTENT_LENGTH_MISMATCH). Either order leaves the record failed and
// visible by default.
func TestFailureSticksWhateverOrder(t *testing.T) {
	for _, order := range []string{"response first", "failure first"} {
		t.Run(order, func(t *testing.T) {
			c := NewCollector("unused")
			c.addNetwork(networkRecord{Request: sightmap.Request{Method: "GET", URL: "/cut.js", ResourceType: "Script"}, requestID: "a"})
			respond := func() { c.applyResponse(responseInfo{reqID: "a", status: 200, rtype: "Script"}) }
			if order == "response first" {
				respond()
				c.failRequest("a")
			} else {
				c.failRequest("a")
				respond()
			}
			got, _ := c.Network(NetworkFilter{})
			if fmt.Sprint(urls(got)) != "[/cut.js]" || got[0].Status != 200 {
				t.Errorf("default = %v, want the failed /cut.js with its 200 status", got)
			}
			if len(c.static) != 0 {
				t.Errorf("static ring holds %d records, want 0", len(c.static))
			}
		})
	}
}

// requestWillBeSent often carries no type; the response brings it. A typeless
// pending request starts in the main ring, then moves by what its response says.
func TestResourceTypeArrivesWithResponse(t *testing.T) {
	c := NewCollector("unused")
	add := func(id, url string) {
		c.addNetwork(networkRecord{Request: sightmap.Request{Method: "GET", URL: url}, requestID: id})
	}
	add("a", "/ok.js")
	add("b", "/api/me")
	add("c", "/gone.js")
	if got, _ := c.Network(NetworkFilter{}); len(got) != 3 {
		t.Fatalf("typeless pending requests hidden: %v (an unknown type is shown)", urls(got))
	}
	c.applyResponse(responseInfo{reqID: "a", status: 200, rtype: "Script"})
	c.applyResponse(responseInfo{reqID: "b", status: 200, rtype: "Fetch"})
	c.applyResponse(responseInfo{reqID: "c", status: 404, rtype: "Script"})

	got, _ := c.Network(NetworkFilter{})
	if want := []string{"/api/me", "/gone.js"}; fmt.Sprint(urls(got)) != fmt.Sprint(want) {
		t.Errorf("default = %v, want %v (the 200 script moved to the static ring, the 404 stayed)", urls(got), want)
	}
	got, _ = c.Network(NetworkFilter{IncludeStatic: true})
	if want := []string{"/ok.js", "/api/me", "/gone.js"}; fmt.Sprint(urls(got)) != fmt.Sprint(want) {
		t.Errorf("--static = %v, want index order %v", urls(got), want)
	}
}

// A redirect reuses its requestId for each hop. When the hops sit in different
// rings, a response must land on the newest one.
func TestRecordByReqIDPrefersNewestHopAcrossRings(t *testing.T) {
	tests := []struct {
		name       string
		hop1Status int // response applied to hop 1 before hop 2 arrives; 0 leaves it pending
		hop2Type   string
		wantStatic bool
	}{
		{"hop 1 main, newest hop static", 302, "Script", true},
		{"hop 1 static, newest hop main", 0, "Fetch", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCollector("unused")
			add := func(url, rtype string) {
				c.addNetwork(networkRecord{Request: sightmap.Request{Method: "GET", URL: url, ResourceType: rtype}, requestID: "r"})
			}
			add("/hop1.js", "Script")
			if tt.hop1Status != 0 {
				c.applyResponse(responseInfo{reqID: "r", status: tt.hop1Status, rtype: "Script"})
			}
			add("/hop2", tt.hop2Type)
			if len(c.network) != 1 || len(c.static) != 1 {
				t.Fatalf("hops share a ring: main %d, static %d; want one in each", len(c.network), len(c.static))
			}

			static, i, ok := c.recordByReqID("r")
			if !ok || static != tt.wantStatic {
				t.Fatalf("recordByReqID = static %v, ok %v; want static %v", static, ok, tt.wantStatic)
			}
			if got := c.record(static, i).URL; got != "/hop2" {
				t.Errorf("found %q, want the newest hop /hop2", got)
			}
			if _, _, ok := c.recordByReqID("missing"); ok {
				t.Error("found a record for an unknown requestId")
			}
		})
	}
}

// The same flow through drain, with loadingFailed delivered as CDP sends it.
func TestDrainRoutesLoadingFailed(t *testing.T) {
	c := NewCollector("unused")
	reqCh := make(chan json.RawMessage, 8)
	failedCh := make(chan json.RawMessage, 8)
	otherCh := make(chan json.RawMessage)
	ctx, cancel := context.WithCancel(context.Background())
	c.wg.Add(1)
	go c.drain(ctx, "T1", nil, otherCh, otherCh, reqCh, otherCh, otherCh, failedCh)
	t.Cleanup(func() { cancel(); c.wg.Wait() })

	reqCh <- json.RawMessage(`{"requestId":"a","type":"Script","request":{"url":"/refused.js","method":"GET","headers":{}}}`)
	waitFor(t, func() bool { got, _ := c.Network(NetworkFilter{IncludeStatic: true}); return len(got) == 1 }, "request buffered")
	failedCh <- json.RawMessage(`{"requestId":"a","errorText":"net::ERR_CONNECTION_REFUSED"}`)
	waitFor(t, func() bool { got, _ := c.Network(NetworkFilter{}); return len(got) == 1 }, "failed script visible by default")
}

// BenchmarkRecordByReqID measures the lookup every response and finished event
// does under c.mu, against two full rings as a Vite page load leaves them: the
// static ring holds older scripts, the main ring 1000 API calls, and a burst of
// newer scripts sits on top.
func BenchmarkRecordByReqID(b *testing.B) {
	c := NewCollector("unused")
	add := func(prefix string, n int, rtype string) {
		for i := range n {
			c.addNetwork(networkRecord{
				Request:   sightmap.Request{Method: "GET", URL: "/x", ResourceType: rtype},
				requestID: fmt.Sprintf("%s%05d", prefix, i),
			})
		}
	}
	add("old", DefaultMaxStatic, "Script")
	add("api", DefaultMaxEntries, "Fetch")
	add("new", 200, "Script")

	for _, bc := range []struct{ name, id string }{
		{"newest static request", "new00199"},
		{"newest API call behind a 200-script burst", "api00999"},
		{"oldest API call", "api00000"},
		{"unknown requestId", "gone"},
	} {
		b.Run(bc.name, func(b *testing.B) {
			if _, _, ok := c.recordByReqID(bc.id); ok != (bc.id != "gone") {
				b.Fatalf("lookup of %q: ok = %v", bc.id, ok)
			}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				c.recordByReqID(bc.id)
			}
		})
	}
}
