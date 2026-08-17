package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
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
			if reqID, st, stText, rtype, ok := parseResponse(raw); ok {
				c.applyResponse(reqID, st, stText, rtype)
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
// the tail, where the request almost always is) and fills in status + type.
func (c *Collector) applyResponse(reqID string, status int, statusText, rtype string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.network) - 1; i >= 0; i-- {
		if c.network[i].requestID == reqID {
			c.network[i].Status = status
			c.network[i].StatusText = statusText
			if rtype != "" {
				c.network[i].ResourceType = rtype
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

func parseException(raw json.RawMessage) (sightmap.Message, bool) {
	var ev struct {
		Timestamp        float64 `json:"timestamp"`
		ExceptionDetails struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		return sightmap.Message{}, false
	}
	text := ev.ExceptionDetails.Exception.Description
	if text == "" {
		text = ev.ExceptionDetails.Text
	}
	return sightmap.Message{Level: "exception", Text: text, Ts: int64(ev.Timestamp)}, true
}

func parseRequest(raw json.RawMessage) (networkRecord, bool) {
	var ev struct {
		RequestID string `json:"requestId"`
		Type      string `json:"type"`
		Request   struct {
			URL    string `json:"url"`
			Method string `json:"method"`
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
		},
		requestID: ev.RequestID,
	}, true
}

func parseResponse(raw json.RawMessage) (reqID string, status int, statusText, rtype string, ok bool) {
	var ev struct {
		RequestID string `json:"requestId"`
		Type      string `json:"type"`
		Response  struct {
			Status     int    `json:"status"`
			StatusText string `json:"statusText"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil || ev.RequestID == "" {
		return "", 0, "", "", false
	}
	return ev.RequestID, ev.Response.Status, ev.Response.StatusText, ev.Type, true
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
