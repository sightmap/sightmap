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

	stop chan struct{}
	wg   sync.WaitGroup
}

type tabColl struct {
	conn   *CDPConn
	cancel context.CancelFunc
}

// NewCollector creates a collector for the Chrome session at addr (host:port).
// Call Start to begin capturing and Stop to tear down.
func NewCollector(addr string) *Collector {
	return &Collector{
		addr:       addr,
		maxEntries: DefaultMaxEntries,
		tabs:       make(map[string]*tabColl),
		stop:       make(chan struct{}),
	}
}

// Start begins the tab-attach loop. It returns immediately; capture runs in the
// background until Stop.
func (c *Collector) Start() {
	c.wg.Add(1)
	go c.attachLoop()
}

// Stop halts capture and closes every per-tab connection.
func (c *Collector) Stop() {
	close(c.stop)
	c.wg.Wait()
	c.tabsMu.Lock()
	for id, tc := range c.tabs {
		tc.cancel()
		tc.conn.Close()
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
		case <-c.stop:
			return
		case <-t.C:
			c.syncTabs()
		}
	}
}

func (c *Collector) syncTabs() {
	tabs, err := ListTabs(context.Background(), c.addr)
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
	conn, err := ConnectTab(context.Background(), c.addr, tabID)
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())

	consoleCh := conn.Subscribe("Runtime.consoleAPICalled")
	excCh := conn.Subscribe("Runtime.exceptionThrown")
	reqCh := conn.Subscribe("Network.requestWillBeSent")
	respCh := conn.Subscribe("Network.responseReceived")

	// Enable the domains after subscribing so buffered replays aren't missed.
	if err := conn.EnableDomain(ctx, "Runtime"); err != nil {
		fmt.Fprintf(os.Stderr, "[collector] tab %s Runtime.enable: %v\n", tabID, err)
	}
	if err := conn.EnableDomain(ctx, "Network"); err != nil {
		fmt.Fprintf(os.Stderr, "[collector] tab %s Network.enable: %v\n", tabID, err)
	}

	c.tabsMu.Lock()
	c.tabs[tabID] = &tabColl{conn: conn, cancel: cancel}
	c.tabsMu.Unlock()

	c.wg.Add(1)
	go c.drain(ctx, tabID, conn, consoleCh, excCh, reqCh, respCh)
}

// drain funnels one tab's CDP events into the shared buffers until ctx is
// cancelled. It does minimal work per event so the cap-8 subscription channels
// don't drop during bursts.
func (c *Collector) drain(ctx context.Context, tabID string, conn *CDPConn, consoleCh, excCh, reqCh, respCh <-chan json.RawMessage) {
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

// applyResponse matches a response back to its pending request (scanning from
// the tail, where the request almost always is) and fills in status, type,
// response headers, and the observed request→response duration.
func (c *Collector) applyResponse(info responseInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.network) - 1; i >= 0; i-- {
		if c.network[i].requestID == info.reqID {
			c.network[i].Status = info.status
			c.network[i].StatusText = info.statusText
			if info.rtype != "" {
				c.network[i].ResourceType = info.rtype
			}
			c.network[i].RspHeaders = info.headers
			if ts := c.network[i].Ts; ts > 0 {
				c.network[i].DurationMs = time.Now().UnixMilli() - ts
			}
			return
		}
	}
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
	reqID, conn, ok := c.lookupRequest(index)
	if !ok {
		return nil, false, nil
	}
	body, err := getResponseBody(ctx, conn, reqID)
	return body, true, err
}

// RequestBody fetches the request post-data for the network entry with the given
// index. Returns (body, found, err).
func (c *Collector) RequestBody(ctx context.Context, index int) ([]byte, bool, error) {
	reqID, conn, ok := c.lookupRequest(index)
	if !ok {
		return nil, false, nil
	}
	body, err := getRequestPostData(ctx, conn, reqID)
	return body, true, err
}

func (c *Collector) lookupRequest(index int) (string, *CDPConn, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.network {
		if e.Index == index {
			return e.requestID, e.conn, true
		}
	}
	return "", nil, false
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
			URL     string            `json:"url"`
			Method  string            `json:"method"`
			Headers map[string]string `json:"headers"`
		} `json:"request"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil || ev.RequestID == "" {
		return networkRecord{}, false
	}
	return networkRecord{
		Request: sightmap.Request{
			Method:       ev.Request.Method,
			URL:          ev.Request.URL,
			ResourceType: ev.Type,
			Ts:           time.Now().UnixMilli(),
			ReqHeaders:   headersFromMap(ev.Request.Headers),
		},
		requestID: ev.RequestID,
	}, true
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
