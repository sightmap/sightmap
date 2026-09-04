package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sightmap/sightmap/go/sightmap"
)

// DefaultMaxEntries is the per-stream ring-buffer cap (console and network are
// buffered independently). Sized so a normal session never overflows; only a
// long burst — or a client that stays away for a while — drops the oldest
// entries, which the query surface then reports as "N earlier entries dropped".
const DefaultMaxEntries = 1000

// networkRecord is one buffered request/response: the tool-free sightmap.Request
// the query surface returns, plus the CDP handle this tool keeps for lazy body
// fetch. Bodies are NOT buffered; they are fetched from the collector connection
// that saw the request (only that connection can, via Network.getResponseBody /
// getRequestPostData). The handle fields are unexported so they never cross the
// public surface or the wire.
type networkRecord struct {
	sightmap.Request

	requestID string   // CDP requestId, for lazy body fetch
	conn      *CDPConn // the collector connection that observed this request
}

// Collector attaches to every tab of a running Chrome session and buffers
// console and network activity into bounded ring buffers. It is the session-
// lifetime component the browser daemon hosts so that per-command CLI queries
// can read history the transient per-command connections never saw. Safe for
// concurrent use.
type Collector struct {
	addr       string
	maxEntries int

	mu             sync.Mutex
	console        []sightmap.Message
	network        []networkRecord
	nextConsole    int
	nextNetwork    int
	consoleDropped int
	networkDropped int

	tabsMu sync.Mutex
	tabs   map[string]*tabColl

	// ctx spans the collector's entire capture lifetime and is cancelled by Stop.
	// Every per-tab context derives from it, so a single cancel stops every drain
	// and interrupts any in-flight tab poll or dial.
	//
	// Holding a context in a struct is a deliberate deviation from the usual
	// "pass it as the first argument" rule. The scope here is this object's
	// lifetime rather than one request, and fixing both fields at construction
	// keeps that lifetime immutable: Start and Stop then need no locking, no nil
	// check, and no guard against a second Start replacing the first cancel func
	// and stranding wg.Wait forever. Long-lived clients hold the same pair for
	// the same reason (e.g. grpc.ClientConn).
	ctx    context.Context
	cancel context.CancelFunc

	wg sync.WaitGroup

	// scriptsMu guards the persistent-script registry: sources registered via
	// AddPersistentScript that the collector re-applies to every attached tab
	// (current and future) so they survive navigations and new tabs. It is held
	// across the CDP (un)register calls so a tab attaching concurrently with an
	// Add can neither double-apply a script nor miss one (the per-tab guard in
	// applyScriptsToTab / AddPersistentScript keys off ps.cdpIDs).
	scriptsMu sync.Mutex
	scripts   []*persistentScript
	scriptSeq int
}

// persistentScript is one source registered for auto-injection at the start of
// every new document. id is the logical handle handed to callers; cdpIDs maps a
// tabID to the identifier CDP returned on that tab, which removal needs (and
// whose presence marks the script as already applied to that tab).
type persistentScript struct {
	id     string
	source string
	cdpIDs map[string]string
}

// PersistentScriptInfo is a public, source-free snapshot of one registered
// script for the daemon's list surface.
type PersistentScriptInfo struct {
	ID    string `json:"id"`
	Bytes int    `json:"bytes"`
	Tabs  int    `json:"tabs"`
}

type tabColl struct {
	conn   *CDPConn
	cancel context.CancelFunc
}

// NewCollector creates a collector for the Chrome session at addr (host:port).
// Call Start to begin capturing and Stop to tear down.
func NewCollector(addr string) *Collector {
	ctx, cancel := context.WithCancel(context.Background())
	return &Collector{
		addr:       addr,
		maxEntries: DefaultMaxEntries,
		tabs:       make(map[string]*tabColl),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start begins the tab-attach loop. It returns immediately; capture runs in the
// background until Stop.
func (c *Collector) Start() {
	c.wg.Add(1)
	go c.attachLoop()
}

// Stop halts capture and closes every per-tab connection. It is safe to call
// more than once.
//
// Order matters: a drain goroutine returns only once its context is cancelled,
// so cancellation MUST precede wg.Wait(). Waiting first deadlocks whenever the
// CDP connections are still healthy at Stop time, which is the normal case
// whenever the caller owns the browser and it outlives the collector, as under
// --attach. One cancel covers every tab, since all tab contexts descend from
// c.ctx.
//
// A tab attaching concurrently with Stop is safe on three counts: its context
// descends from c.ctx, so it is born cancelled and its drain returns on its
// first select; its conn is registered in c.tabs before that drain is counted,
// so the sweep below still closes it; and its wg.Add cannot race wg.Wait,
// because attachTab runs only on the attachLoop goroutine, which holds a count
// of its own until it returns, keeping the counter above zero for as long as a
// further Add is possible.
func (c *Collector) Stop() {
	c.cancel()
	c.wg.Wait()
	c.tabsMu.Lock()
	for id, tc := range c.tabs {
		if tc.conn != nil {
			tc.conn.Close()
		}
		delete(c.tabs, id)
	}
	c.tabsMu.Unlock()
}

// attachLoop polls the session's tab list, attaching to new tabs and dropping
// closed ones. Polling (rather than Target auto-attach) keeps the collector on
// the same per-target CDPConn primitive the rest of the browser package uses.
func (c *Collector) attachLoop() {
	defer c.wg.Done()
	c.syncTabs()
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-t.C:
			c.syncTabs()
		}
	}
}

func (c *Collector) syncTabs() {
	// Neither this poll nor the ConnectTab below sets a client timeout, so the
	// collector context is the only thing bounding them: a browser that accepts
	// the socket and then never answers would otherwise block Stop indefinitely.
	tabs, err := ListTabs(c.ctx, c.addr)
	if err != nil {
		return
	}
	seen := make(map[string]bool, len(tabs))
	for _, t := range tabs {
		seen[t.TargetID] = true
		c.tabsMu.Lock()
		_, attached := c.tabs[t.TargetID]
		c.tabsMu.Unlock()
		if !attached {
			c.attachTab(t.TargetID)
		}
	}
	// Detach tabs that have closed.
	c.tabsMu.Lock()
	for id, tc := range c.tabs {
		if !seen[id] {
			tc.cancel()
			tc.conn.Close()
			delete(c.tabs, id)
		}
	}
	c.tabsMu.Unlock()
}

func (c *Collector) attachTab(tabID string) {
	conn, err := ConnectTab(c.ctx, c.addr, tabID)
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(c.ctx)

	consoleCh := conn.Subscribe("Runtime.consoleAPICalled")
	excCh := conn.Subscribe("Runtime.exceptionThrown")
	reqCh := conn.Subscribe("Network.requestWillBeSent")
	respCh := conn.Subscribe("Network.responseReceived")
	finishedCh := conn.Subscribe("Network.loadingFinished")

	// Enable the domains after subscribing so buffered replays aren't missed.
	if err := conn.EnableDomain(ctx, "Runtime"); err != nil {
		fmt.Fprintf(os.Stderr, "[collector] tab %s Runtime.enable: %v\n", tabID, err)
	}
	if err := conn.EnableDomain(ctx, "Network"); err != nil {
		fmt.Fprintf(os.Stderr, "[collector] tab %s Network.enable: %v\n", tabID, err)
	}

	c.tabsMu.Lock()
	c.tabs[tabID] = &tabColl{conn: conn, cancel: cancel}
	c.wg.Add(1)
	c.tabsMu.Unlock()

	go c.drain(ctx, tabID, conn, consoleCh, excCh, reqCh, respCh, finishedCh)

	// A newly-attached tab (a fresh tab, or the initial one) must carry every
	// persistent script so injection survives new tabs, not just navigations
	// within one tab. No-op when nothing is registered.
	c.applyScriptsToTab(ctx, tabID, conn)
}

// drain funnels one tab's CDP events into the shared buffers until ctx is
// cancelled. It does minimal work per event so the cap-8 subscription channels
// don't drop during bursts.
func (c *Collector) drain(ctx context.Context, tabID string, conn *CDPConn, consoleCh, excCh, reqCh, respCh, finishedCh <-chan json.RawMessage) {
	defer c.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case raw := <-consoleCh:
			if e, ok := parseConsoleAPI(raw); ok {
				e.Tab = tabID
				c.addConsole(e)
			}
		case raw := <-excCh:
			if e, ok := parseException(raw); ok {
				e.Tab = tabID
				c.addConsole(e)
			}
		case raw := <-reqCh:
			if e, ok := parseRequest(raw); ok {
				e.Tab = tabID
				e.conn = conn
				c.addNetwork(e)
			}
		case raw := <-respCh:
			if info, ok := parseResponse(raw); ok {
				c.applyResponse(info)
			}
		case raw := <-finishedCh:
			// The response body is reliably fetchable only once loading finished.
			// Fetch it OFF the drain loop (a getResponseBody round-trip would
			// otherwise stall these cap-8 channels and drop events).
			if reqID := parseLoadingFinished(raw); reqID != "" {
				c.wg.Add(1)
				go c.retainResponseBody(ctx, conn, reqID)
			}
		}
	}
}

func (c *Collector) addConsole(e sightmap.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e.Index = c.nextConsole
	c.nextConsole++
	c.console = append(c.console, e)
	if over := len(c.console) - c.maxEntries; over > 0 {
		c.console = c.console[over:]
		c.consoleDropped += over
	}
}

func (c *Collector) addNetwork(e networkRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e.Index = c.nextNetwork
	c.nextNetwork++
	c.network = append(c.network, e)
	if over := len(c.network) - c.maxEntries; over > 0 {
		c.network = c.network[over:]
		c.networkDropped += over
	}
}

// applyResponse matches a response back to its pending request and fills in
// status, type, response headers, and the observed request→response duration.
func (c *Collector) applyResponse(info responseInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	i := c.networkIndexByReqID(info.reqID)
	if i < 0 {
		return
	}
	c.network[i].Status = info.status
	c.network[i].StatusText = info.statusText
	if info.rtype != "" {
		c.network[i].ResourceType = info.rtype
	}
	c.network[i].RspHeaders = info.headers
	if ts := c.network[i].Ts; ts > 0 {
		c.network[i].DurationMs = time.Now().UnixMilli() - ts
	}
}

// networkIndexByReqID returns the index of the network record with the given
// CDP requestId, or -1 if none is buffered. Scans from the tail, where a match
// almost always is. Caller must hold c.mu.
func (c *Collector) networkIndexByReqID(reqID string) int {
	for i := len(c.network) - 1; i >= 0; i-- {
		if c.network[i].requestID == reqID {
			return i
		}
	}
	return -1
}

// ConsoleFilter narrows a Console query.
type ConsoleFilter struct {
	Level string // exact level match, or "" for all
	Tab   string // exact tab match, or "" for all
	Limit int    // keep only the most recent N (0 = all)
}

// Console returns the buffered console entries matching f (oldest→newest) plus
// the number of entries dropped from the front of the ring.
func (c *Collector) Console(f ConsoleFilter) ([]sightmap.Message, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]sightmap.Message, 0, len(c.console))
	for _, e := range c.console {
		if f.Level != "" && e.Level != f.Level {
			continue
		}
		if f.Tab != "" && e.Tab != f.Tab {
			continue
		}
		out = append(out, e)
	}
	out = tailLimit(out, f.Limit)
	return out, c.consoleDropped
}

// NetworkFilter narrows a Network query.
type NetworkFilter struct {
	ResourceType string // exact resourceType match (case-insensitive), or "" for all
	URLSubstr    string // case-insensitive URL substring, or "" for all
	Tab          string // exact tab match, or "" for all
	Limit        int    // keep only the most recent N (0 = all)
}

// Network returns the buffered network entries matching f (oldest→newest) plus
// the number dropped from the front of the ring.
func (c *Collector) Network(f NetworkFilter) ([]sightmap.Request, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rt := strings.ToLower(f.ResourceType)
	sub := strings.ToLower(f.URLSubstr)
	out := make([]sightmap.Request, 0, len(c.network))
	for _, e := range c.network {
		if rt != "" && strings.ToLower(e.ResourceType) != rt {
			continue
		}
		if sub != "" && !strings.Contains(strings.ToLower(e.URL), sub) {
			continue
		}
		if f.Tab != "" && e.Tab != f.Tab {
			continue
		}
		out = append(out, e.Request)
	}
	out = tailLimit(out, f.Limit)
	return out, c.networkDropped
}

// ResponseBody fetches the response body for the network entry with the given
// index, from the collector connection that observed it. Returns (body, found,
// err); found is false when no such entry is buffered.
func (c *Collector) ResponseBody(ctx context.Context, index int) ([]byte, bool, error) {
	retained, reqID, conn, found := c.bodyLookup(index, func(r networkRecord) *sightmap.Body { return r.RspBody })
	if !found {
		return nil, false, nil
	}
	if retained != nil {
		return retained, true, nil
	}
	body, err := getResponseBody(ctx, conn, reqID)
	return body, true, err
}

// RequestBody fetches the request post-data for the network entry with the given
// index. Returns (body, found, err).
func (c *Collector) RequestBody(ctx context.Context, index int) ([]byte, bool, error) {
	retained, reqID, conn, found := c.bodyLookup(index, func(r networkRecord) *sightmap.Body { return r.ReqBody })
	if !found {
		return nil, false, nil
	}
	if retained != nil {
		return retained, true, nil
	}
	body, err := getRequestPostData(ctx, conn, reqID)
	return body, true, err
}

// bodyLookup finds the buffered record for index and returns EITHER the body
// already retained on it (pick selects ReqBody/RspBody) or the reqID+conn to
// fetch it live. Preferring the retained copy is what makes `network get`
// reliable: the eager loadingFinished capture keeps the body in memory, so a
// query after Chrome has evicted it from its own network cache still succeeds
// (a live getResponseBody would fail). Falls back to a live fetch only when
// nothing was retained (e.g. a non-XHR/Fetch type, which wantsBody skips).
func (c *Collector) bodyLookup(index int, pick func(networkRecord) *sightmap.Body) (retained []byte, reqID string, conn *CDPConn, found bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.network {
		if c.network[i].Index == index {
			if b := pick(c.network[i]); b != nil {
				return []byte(b.Content), "", nil, true
			}
			return nil, c.network[i].requestID, c.network[i].conn, true
		}
	}
	return nil, "", nil, false
}

// retainResponseBody fetches a finished response's body and stores it on the
// buffered record, so property extraction runs against a complete record without
// the query path making its own CDP round-trip. Only XHR/Fetch bodies are
// retained — the API-call types a request property addresses — so page assets
// (images, fonts, stylesheets) aren't pulled into memory. Best-effort: a body
// that can't be fetched (evicted, no body, a redirect) is simply left absent,
// matching the SEP's silent omission.
func (c *Collector) retainResponseBody(ctx context.Context, conn *CDPConn, reqID string) {
	defer c.wg.Done()

	c.mu.Lock()
	i := c.networkIndexByReqID(reqID)
	var rtype, ctype string
	if i >= 0 {
		rtype = c.network[i].ResourceType
		ctype = contentTypeFromHeaders(c.network[i].RspHeaders)
	}
	c.mu.Unlock()
	if i < 0 || !wantsBody(rtype) {
		return
	}

	body, err := getResponseBody(ctx, conn, reqID)
	if err != nil || len(body) == 0 {
		return
	}

	c.mu.Lock()
	if i := c.networkIndexByReqID(reqID); i >= 0 {
		c.network[i].RspBody = &sightmap.Body{
			Content:     string(body),
			Size:        len(body),
			ContentType: ctype,
		}
	}
	c.mu.Unlock()
}

// wantsBody reports whether a response body is worth retaining for a given CDP
// resource type: the API-call types whose bodies a request property extracts.
func wantsBody(resourceType string) bool {
	switch strings.ToLower(resourceType) {
	case "xhr", "fetch":
		return true
	}
	return false
}

// contentTypeFromMap / contentTypeFromHeaders read the Content-Type header
// (case-insensitive) from CDP's header map and from a captured Header slice.
func contentTypeFromMap(m map[string]string) string {
	for k, v := range m {
		if strings.EqualFold(k, "content-type") {
			return v
		}
	}
	return ""
}

func contentTypeFromHeaders(hs []sightmap.Header) string {
	for _, h := range hs {
		if strings.EqualFold(h.Name, "content-type") {
			return h.Value
		}
	}
	return ""
}

// ── persistent script injection ─────────────────────────────────────────────
//
// Scripts registered with Page.addScriptToEvaluateOnNewDocument live only as
// long as the CDP session that added them, so a transient per-command connection
// cannot persist one. The collector is the session-lifetime CDP owner, so it
// holds the registry and re-applies each script to every tab it attaches. Because
// addScriptToEvaluateOnNewDocument re-runs the script at the start of every new
// document, a script registered here survives navigations, and re-applying on
// attach extends that to new tabs.

// AddPersistentScript registers source to auto-run at the start of every new
// document, in every tab of the session (now and future), returning a logical id
// for RemovePersistentScript. It applies the script to every attached tab
// immediately; because CDP only injects into FUTURE documents, a caller that also
// wants it in the currently-loaded document must eval it once separately (the
// `inject` command does).
func (c *Collector) AddPersistentScript(ctx context.Context, source string) (string, error) {
	c.scriptsMu.Lock()
	defer c.scriptsMu.Unlock()

	c.scriptSeq++
	ps := &persistentScript{
		id:     fmt.Sprintf("inj-%d", c.scriptSeq),
		source: source,
		cdpIDs: make(map[string]string),
	}
	// Apply to every currently-attached tab. A tab attaching concurrently is
	// covered by attachTab's own apply pass (also under scriptsMu); the cdpIDs
	// guard there makes the two idempotent so neither double-applies.
	for tabID, conn := range c.tabConns() {
		cdpID, err := addScriptOnNewDocument(ctx, conn, source)
		if err != nil {
			continue // tab may be navigating/closing; a later attach re-applies
		}
		ps.cdpIDs[tabID] = cdpID
	}
	c.scripts = append(c.scripts, ps)
	return ps.id, nil
}

// RemovePersistentScript unregisters the script with the given id and best-effort
// removes it from every tab it was applied to. It reports whether such an id
// existed.
func (c *Collector) RemovePersistentScript(ctx context.Context, id string) (bool, error) {
	c.scriptsMu.Lock()
	defer c.scriptsMu.Unlock()

	idx := -1
	for i, ps := range c.scripts {
		if ps.id == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false, nil
	}
	ps := c.scripts[idx]
	conns := c.tabConns()
	for tabID, cdpID := range ps.cdpIDs {
		if conn, ok := conns[tabID]; ok {
			_ = removeScriptOnNewDocument(ctx, conn, cdpID) // best-effort; tab may be gone
		}
	}
	c.scripts = append(c.scripts[:idx], c.scripts[idx+1:]...)
	return true, nil
}

// PersistentScripts returns a source-free snapshot of the registry.
func (c *Collector) PersistentScripts() []PersistentScriptInfo {
	c.scriptsMu.Lock()
	defer c.scriptsMu.Unlock()
	out := make([]PersistentScriptInfo, 0, len(c.scripts))
	for _, ps := range c.scripts {
		out = append(out, PersistentScriptInfo{ID: ps.id, Bytes: len(ps.source), Tabs: len(ps.cdpIDs)})
	}
	return out
}

// applyScriptsToTab registers every not-yet-applied persistent script on conn.
// Called from attachTab for each new tab; the ps.cdpIDs guard skips a script a
// concurrent AddPersistentScript already put on this tab, keeping the two paths
// idempotent.
func (c *Collector) applyScriptsToTab(ctx context.Context, tabID string, conn *CDPConn) {
	c.scriptsMu.Lock()
	defer c.scriptsMu.Unlock()
	for _, ps := range c.scripts {
		if _, done := ps.cdpIDs[tabID]; done {
			continue
		}
		cdpID, err := addScriptOnNewDocument(ctx, conn, ps.source)
		if err != nil {
			continue
		}
		ps.cdpIDs[tabID] = cdpID
	}
}

// tabConns snapshots the attached tabs' connections. Taken under tabsMu only;
// callers already hold scriptsMu, so the lock order is always scriptsMu→tabsMu.
func (c *Collector) tabConns() map[string]*CDPConn {
	c.tabsMu.Lock()
	defer c.tabsMu.Unlock()
	out := make(map[string]*CDPConn, len(c.tabs))
	for id, tc := range c.tabs {
		out[id] = tc.conn
	}
	return out
}

// addScriptOnNewDocument registers source to run at the start of every future
// document on conn's target, returning CDP's identifier for later removal.
//
// The Page domain MUST be enabled first: without it,
// Page.addScriptToEvaluateOnNewDocument still returns an identifier (so the
// script looks registered) but is never actually evaluated on new documents —
// the persist-across-navigation bug. Enabling Page is idempotent, so calling it
// per registration is safe; the collector connection subscribes to Runtime and
// Network only, so the Page lifecycle events it turns on are dispatched to no
// subscribers and dropped.
func addScriptOnNewDocument(ctx context.Context, conn *CDPConn, source string) (string, error) {
	if err := conn.EnableDomain(ctx, "Page"); err != nil {
		return "", fmt.Errorf("addScriptToEvaluateOnNewDocument: enable Page: %w", err)
	}
	raw, err := conn.call(ctx, "Page.addScriptToEvaluateOnNewDocument", map[string]interface{}{
		"source": source,
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		Identifier string `json:"identifier"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("addScriptToEvaluateOnNewDocument: unmarshal: %w", err)
	}
	return resp.Identifier, nil
}

// removeScriptOnNewDocument unregisters a script previously added on conn.
func removeScriptOnNewDocument(ctx context.Context, conn *CDPConn, identifier string) error {
	_, err := conn.call(ctx, "Page.removeScriptToEvaluateOnNewDocument", map[string]interface{}{
		"identifier": identifier,
	})
	return err
}

// tailLimit keeps only the most recent n entries when n > 0.
func tailLimit[T any](s []T, n int) []T {
	if n > 0 && len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// ── CDP event parsing (pure; unit-tested) ──────────────────────────────────

func parseConsoleAPI(raw json.RawMessage) (sightmap.Message, bool) {
	var ev struct {
		Type      string  `json:"type"`
		Timestamp float64 `json:"timestamp"`
		Args      []struct {
			Type        string          `json:"type"`
			Value       json.RawMessage `json:"value"`
			Description string          `json:"description"`
		} `json:"args"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		return sightmap.Message{}, false
	}
	parts := make([]string, 0, len(ev.Args))
	for _, a := range ev.Args {
		parts = append(parts, renderArg(a.Value, a.Description))
	}
	return sightmap.Message{
		Level: consoleLevel(ev.Type),
		Text:  strings.Join(parts, " "),
		Ts:    int64(ev.Timestamp),
	}, true
}

// cdpCallFrame is one CDP Runtime.CallFrame (0-based line/column).
type cdpCallFrame struct {
	FunctionName string `json:"functionName"`
	URL          string `json:"url"`
	LineNumber   int    `json:"lineNumber"`
	ColumnNumber int    `json:"columnNumber"`
}

func parseException(raw json.RawMessage) (sightmap.Message, bool) {
	var ev struct {
		Timestamp        float64 `json:"timestamp"`
		ExceptionDetails struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
			StackTrace struct {
				CallFrames []cdpCallFrame `json:"callFrames"`
			} `json:"stackTrace"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		return sightmap.Message{}, false
	}
	text := ev.ExceptionDetails.Exception.Description
	if text == "" {
		text = ev.ExceptionDetails.Text
	}
	return sightmap.Message{
		Level: "exception",
		Text:  text,
		Ts:    int64(ev.Timestamp),
		Stack: framesFromCallFrames(ev.ExceptionDetails.StackTrace.CallFrames),
	}, true
}

// framesFromCallFrames converts CDP stackTrace.callFrames into sightmap.Frame
// records, throwing frame first. lineNumber/columnNumber are always present on a
// CDP call frame and are 0-based, so they are captured as pointers (never nil
// here); a downstream producer that lacks them leaves the pointer nil instead.
func framesFromCallFrames(cfs []cdpCallFrame) []sightmap.Frame {
	if len(cfs) == 0 {
		return nil
	}
	out := make([]sightmap.Frame, 0, len(cfs))
	for _, cf := range cfs {
		line, col := cf.LineNumber, cf.ColumnNumber
		out = append(out, sightmap.Frame{
			Function: cf.FunctionName,
			File:     cf.URL,
			Line:     &line,
			Column:   &col,
		})
	}
	return out
}

func parseRequest(raw json.RawMessage) (networkRecord, bool) {
	var ev struct {
		RequestID string `json:"requestId"`
		Type      string `json:"type"`
		Request   struct {
			URL      string            `json:"url"`
			Method   string            `json:"method"`
			Headers  map[string]string `json:"headers"`
			PostData string            `json:"postData"`
		} `json:"request"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil || ev.RequestID == "" {
		return networkRecord{}, false
	}
	rec := networkRecord{
		Request: sightmap.Request{
			Method:       ev.Request.Method,
			URL:          ev.Request.URL,
			ResourceType: ev.Type,
			Ts:           time.Now().UnixMilli(),
			ReqHeaders:   headersFromMap(ev.Request.Headers),
		},
		requestID: ev.RequestID,
	}
	// The request body is delivered inline on requestWillBeSent (no round-trip),
	// so retain it directly. The response body is not — it's fetched on
	// loadingFinished (retainResponseBody).
	if ev.Request.PostData != "" {
		rec.ReqBody = &sightmap.Body{
			Content:     ev.Request.PostData,
			Size:        len(ev.Request.PostData),
			ContentType: contentTypeFromMap(ev.Request.Headers),
		}
	}
	return rec, true
}

// parseLoadingFinished extracts the requestId from a Network.loadingFinished
// event, or "" if it doesn't parse.
func parseLoadingFinished(raw json.RawMessage) string {
	var ev struct {
		RequestID string `json:"requestId"`
	}
	if json.Unmarshal(raw, &ev) != nil {
		return ""
	}
	return ev.RequestID
}

// responseInfo is the parsed Network.responseReceived event, applied back onto
// the pending request record by applyResponse.
type responseInfo struct {
	reqID      string
	status     int
	statusText string
	rtype      string
	headers    []sightmap.Header
}

func parseResponse(raw json.RawMessage) (responseInfo, bool) {
	var ev struct {
		RequestID string `json:"requestId"`
		Type      string `json:"type"`
		Response  struct {
			Status     int               `json:"status"`
			StatusText string            `json:"statusText"`
			Headers    map[string]string `json:"headers"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil || ev.RequestID == "" {
		return responseInfo{}, false
	}
	return responseInfo{
		reqID:      ev.RequestID,
		status:     ev.Response.Status,
		statusText: ev.Response.StatusText,
		rtype:      ev.Type,
		headers:    headersFromMap(ev.Response.Headers),
	}, true
}

// headersFromMap converts CDP's header object into an ordered Header slice,
// sorted by name for determinism. CDP delivers headers as a JSON object, so
// duplicates are already collapsed and order is not preserved — a producer with
// the raw header text (e.g. a different capture layer) can populate duplicates,
// which the []Header type supports.
func headersFromMap(m map[string]string) []sightmap.Header {
	if len(m) == 0 {
		return nil
	}
	out := make([]sightmap.Header, 0, len(m))
	for k, v := range m {
		out = append(out, sightmap.Header{Name: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// renderArg turns one console argument into display text: the JSON value when
// present (unquoting plain strings), else the remote-object description.
func renderArg(value json.RawMessage, description string) string {
	if len(value) > 0 {
		var s string
		if json.Unmarshal(value, &s) == nil {
			return s
		}
		return string(value)
	}
	return description
}

func consoleLevel(t string) string {
	switch t {
	case "error", "assert":
		return "error"
	case "warning":
		return "warn"
	case "debug":
		return "debug"
	case "info":
		return "info"
	case "log":
		return "log"
	default:
		return "info"
	}
}

func getResponseBody(ctx context.Context, conn *CDPConn, reqID string) ([]byte, error) {
	raw, err := conn.call(ctx, "Network.getResponseBody", map[string]interface{}{"requestId": reqID})
	if err != nil {
		return nil, err
	}
	return decodeBody(raw)
}

func getRequestPostData(ctx context.Context, conn *CDPConn, reqID string) ([]byte, error) {
	raw, err := conn.call(ctx, "Network.getRequestPostData", map[string]interface{}{"requestId": reqID})
	if err != nil {
		return nil, err
	}
	var resp struct {
		PostData string `json:"postData"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return []byte(resp.PostData), nil
}

func decodeBody(raw json.RawMessage) ([]byte, error) {
	var resp struct {
		Body          string `json:"body"`
		Base64Encoded bool   `json:"base64Encoded"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	if resp.Base64Encoded {
		return base64.StdEncoding.DecodeString(resp.Body)
	}
	return []byte(resp.Body), nil
}
