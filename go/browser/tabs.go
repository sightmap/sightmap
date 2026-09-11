package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

// TabInfo describes an open browser page/tab.
type TabInfo struct {
	TargetID string
	URL      string
	Title    string
}

// IsContentTabURL reports whether a page target's URL is real navigable content
// an agent would drive, as opposed to an internal surface like the extension
// side panel or DevTools. Those internal targets are reported by CDP with
// type=="page" too, so filtering on type alone is not enough — selecting one by
// default is the silent wrong-tab footgun.
func IsContentTabURL(url string) bool {
	switch {
	case strings.HasPrefix(url, "chrome-extension://"),
		strings.HasPrefix(url, "devtools://"),
		strings.HasPrefix(url, "chrome://"),
		strings.HasPrefix(url, "chrome-untrusted://"):
		return false
	}
	return true
}

// ListTabs returns the open CONTENT page targets for the browser at addr (e.g.
// "localhost:7892"). Internal surfaces (extension side panel, DevTools) are
// excluded — see IsContentTabURL.
func ListTabs(ctx context.Context, addr string) ([]TabInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/json/list", nil)
	if err != nil {
		return nil, fmt.Errorf("ListTabs: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ListTabs: request: %w", err)
	}
	defer resp.Body.Close()

	var targets []struct {
		ID    string `json:"id"`
		Type  string `json:"type"`
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("ListTabs: decode: %w", err)
	}

	var tabs []TabInfo
	for _, t := range targets {
		if t.Type == "page" && IsContentTabURL(t.URL) {
			tabs = append(tabs, TabInfo{
				TargetID: t.ID,
				URL:      t.URL,
				Title:    t.Title,
			})
		}
	}
	return tabs, nil
}

// ConnectTab returns a *CDPConn connected to an existing tab by TargetID.
// addr is the browser address (host:port).
func ConnectTab(ctx context.Context, addr, targetID string) (*CDPConn, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/json/list", nil)
	if err != nil {
		return nil, fmt.Errorf("ConnectTab: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ConnectTab: request: %w", err)
	}
	defer resp.Body.Close()

	var targets []struct {
		ID    string `json:"id"`
		WSURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("ConnectTab: decode: %w", err)
	}

	for _, t := range targets {
		if t.ID == targetID {
			if t.WSURL == "" {
				return nil, fmt.Errorf("ConnectTab: target %q has no webSocketDebuggerUrl", targetID)
			}
			return dialCDPWebSocket(ctx, t.WSURL)
		}
	}
	return nil, fmt.Errorf("ConnectTab: target %q not found", targetID)
}

// CreateTab opens a new browser tab and returns its target ID and a CDPConn.
// url may be empty (opens about:blank).
func CreateTab(ctx context.Context, addr, url string) (targetID string, conn *CDPConn, err error) {
	newURL := "http://" + addr + "/json/new"
	if url != "" {
		newURL += "?" + url
	}
	// Chrome 111+ requires PUT (not GET) for /json/new; a GET is rejected with a
	// plaintext "Using unsafe HTTP verb GET..." body that fails JSON decoding.
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, newURL, nil)
	if err != nil {
		return "", nil, fmt.Errorf("CreateTab: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("CreateTab: request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("CreateTab: %s returned %d: %s", newURL, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var target struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &target); err != nil {
		return "", nil, fmt.Errorf("CreateTab: decode %q: %w", strings.TrimSpace(string(body)), err)
	}
	if target.ID == "" {
		return "", nil, fmt.Errorf("CreateTab: no target ID in response")
	}
	conn, err = ConnectTab(ctx, addr, target.ID)
	return target.ID, conn, err
}

// CloseTab closes the tab identified by targetID.
func CloseTab(ctx context.Context, addr, targetID string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/json/close/"+targetID, nil)
	if err != nil {
		return fmt.Errorf("CloseTab: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("CloseTab: request: %w", err)
	}
	resp.Body.Close()
	return nil
}

// ResizeViewport sets the current page's viewport size using
// Emulation.setDeviceMetricsOverride.
func ResizeViewport(ctx context.Context, conn *CDPConn, width, height int) error {
	_, err := conn.call(ctx, "Emulation.setDeviceMetricsOverride", map[string]interface{}{
		"width":             width,
		"height":            height,
		"deviceScaleFactor": 1,
		"mobile":            false,
	})
	return err
}

// dialCDPWebSocket creates a CDPConn by dialing a WebSocket URL directly,
// bypassing the /json/list target discovery used by DialCDP.
func dialCDPWebSocket(ctx context.Context, wsURL string) (*CDPConn, error) {
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
