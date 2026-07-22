package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"

	"github.com/sightmap/sightmap/go/extract"
	"github.com/sightmap/sightmap/go/probe"
)

// CDPConn is a connection to a Chrome remote debugging endpoint.
// It manages the WebSocket message loop and dispatches responses to callers.
type CDPConn struct {
	ws        *websocket.Conn
	writeMu   sync.Mutex // serialises ws writes
	nextID    atomic.Int64
	pendMu    sync.Mutex
	pending   map[int64]chan cdpResponse
	subMu     sync.RWMutex
	subs      map[string][]chan json.RawMessage
	closeOnce sync.Once
	done      chan struct{}
}

// DialCDP connects to a Chrome remote debugging address (e.g. "localhost:7892")
// and returns a CDPConn ready to use. The first available "page" target is
// selected; call Close when done.
func DialCDP(ctx context.Context, addr string) (*CDPConn, error) {
	// Discover available targets.
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/json/list", nil)
	if err != nil {
		return nil, fmt.Errorf("cdp: build targets request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cdp: get targets from %s: %w", addr, err)
	}
	defer resp.Body.Close()

	var targets []struct {
		Type    string `json:"type"`
		WSURL   string `json:"webSocketDebuggerUrl"`
		PageURL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("cdp: decode targets: %w", err)
	}

	// Prefer a real content page over internal surfaces (extension side panel,
	// DevTools), which CDP also reports as type=="page" — picking one of those by
	// default is the silent wrong-tab footgun (go-tabg). Fall back to any page
	// target only if no content tab exists.
	var wsURL, fallbackWS string
	for _, t := range targets {
		if t.Type != "page" {
			continue
		}
		if fallbackWS == "" {
			fallbackWS = t.WSURL
		}
		if IsContentTabURL(t.PageURL) {
			wsURL = t.WSURL
			break
		}
	}
	if wsURL == "" {
		wsURL = fallbackWS
	}
	if wsURL == "" {
		return nil, fmt.Errorf("cdp: no page target found at %s", addr)
	}

	dialer := websocket.Dialer{}
	ws, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cdp: websocket dial %s: %w", wsURL, err)
	}

	conn := &CDPConn{
		ws:      ws,
		pending: make(map[int64]chan cdpResponse),
		subs:    make(map[string][]chan json.RawMessage),
		done:    make(chan struct{}),
	}
	go conn.readLoop()
	return conn, nil
}

// DefaultPage returns a Page backed by the CDPConn's connected target.
func (c *CDPConn) DefaultPage() (Page, error) {
	return &cdpPage{conn: c}, nil
}

// Close releases the WebSocket connection and drains pending requests.
func (c *CDPConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		err = c.ws.Close()
	})
	return err
}

// ---- CDP protocol internals -------------------------------------------------

type cdpRequest struct {
	ID     int64       `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params,omitempty"`
}

type cdpResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *cdpError       `json:"error,omitempty"`
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *CDPConn) readLoop() {
	defer close(c.done)
	for {
		_, raw, err := c.ws.ReadMessage()
		if err != nil {
			return
		}

		// Unified peek: detect response (has "id") vs event (no "id", has "method").
		var peek struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *cdpError       `json:"error"`
		}
		if err := json.Unmarshal(raw, &peek); err != nil {
			continue
		}

		if peek.ID != nil {
			// CDP command response.
			id := *peek.ID
			resp := cdpResponse{ID: id, Result: peek.Result, Error: peek.Error}
			c.pendMu.Lock()
			ch := c.pending[id]
			delete(c.pending, id)
			c.pendMu.Unlock()
			if ch != nil {
				ch <- resp
			}
		} else if peek.Method != "" {
			// CDP event — fan out to subscribers.
			c.subMu.RLock()
			chs := c.subs[peek.Method]
			c.subMu.RUnlock()
			for _, ch := range chs {
				select {
				case ch <- peek.Params:
				default: // slow consumer; drop
				}
			}
		}
	}
}

// Subscribe registers a buffered channel (cap=8) to receive CDP events
// for the given method name (e.g. "Page.loadEventFired").
// Slow consumers: events are dropped via non-blocking send.
// Call Unsubscribe when done to avoid leaks.
func (c *CDPConn) Subscribe(method string) <-chan json.RawMessage {
	ch := make(chan json.RawMessage, 8)
	c.subMu.Lock()
	c.subs[method] = append(c.subs[method], ch)
	c.subMu.Unlock()
	return ch
}

// Unsubscribe removes ch from the subscribers for method.
func (c *CDPConn) Unsubscribe(method string, ch <-chan json.RawMessage) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	old := c.subs[method]
	var newSubs []chan json.RawMessage
	for _, s := range old {
		if (<-chan json.RawMessage)(s) != ch {
			newSubs = append(newSubs, s)
		}
	}
	if len(newSubs) == 0 {
		delete(c.subs, method)
	} else {
		c.subs[method] = newSubs
	}
}

// EnableDomain sends the {domain}.enable CDP command
// (e.g. "Page" → sends {"method":"Page.enable"}).
func (c *CDPConn) EnableDomain(ctx context.Context, domain string) error {
	_, err := c.call(ctx, domain+".enable", map[string]interface{}{})
	return err
}

// call sends a CDP command and blocks until the response arrives or ctx is cancelled.
func (c *CDPConn) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan cdpResponse, 1)

	c.pendMu.Lock()
	c.pending[id] = ch
	c.pendMu.Unlock()

	req := cdpRequest{ID: id, Method: method, Params: params}
	c.writeMu.Lock()
	err := c.ws.WriteJSON(req)
	c.writeMu.Unlock()
	if err != nil {
		c.pendMu.Lock()
		delete(c.pending, id)
		c.pendMu.Unlock()
		return nil, fmt.Errorf("cdp: write %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		c.pendMu.Lock()
		delete(c.pending, id)
		c.pendMu.Unlock()
		return nil, fmt.Errorf("cdp: %s: %w", method, ctx.Err())
	case <-c.done:
		return nil, fmt.Errorf("cdp: connection closed while waiting for %s", method)
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("cdp: %s error %d: %s", method, resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

// ---- cdpPage implements Page ------------------------------------------------

type cdpPage struct {
	conn    *CDPConn
	pageURL string
}

func (p *cdpPage) URL() string { return p.pageURL }

func (p *cdpPage) RunProbe(ctx context.Context, isLogicalRoot, useScrollOffset bool) (*extract.ProbeResult, error) {
	// Wrap probe.js in an IIFE that calls computeCompProps with the given args.
	script := fmt.Sprintf(
		"(function(){\n%s\nreturn computeCompProps(%v,%v);\n})()",
		probe.ProbeJS, isLogicalRoot, useScrollOffset,
	)

	// EvalJSON handles the Runtime.evaluate double-envelope unwrap and JS exception check.
	value, err := EvalJSON(ctx, p.conn, script)
	if err != nil {
		return nil, fmt.Errorf("runtime.evaluate probe: %w", err)
	}

	var probeResult extract.ProbeResult
	if err := json.Unmarshal(value, &probeResult); err != nil {
		return nil, fmt.Errorf("probe: unmarshal result: %w", err)
	}
	return &probeResult, nil
}

func (p *cdpPage) GetA11YTree(ctx context.Context) ([]extract.A11YNode, error) {
	result, err := p.conn.call(ctx, "Accessibility.getFullAXTree", map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("a11y tree: %w", err)
	}

	var resp struct {
		Nodes []extract.A11YNode `json:"nodes"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("a11y tree: unmarshal: %w", err)
	}
	return resp.Nodes, nil
}

func (p *cdpPage) GetDOMTree(ctx context.Context) (*extract.DOMNode, error) {
	result, err := p.conn.call(ctx, "DOM.getDocument", map[string]interface{}{
		"depth":  -1,
		"pierce": true,
	})
	if err != nil {
		return nil, fmt.Errorf("dom tree: %w", err)
	}

	var resp struct {
		Root *extract.DOMNode `json:"root"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("dom tree: unmarshal: %w", err)
	}
	return resp.Root, nil
}
