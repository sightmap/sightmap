package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// InjectExtensionServerPort finds the sightmap extension's service worker
// target via the Chrome DevTools Protocol and injects the sightmap HTTP server
// port into chrome.storage.local. Each Chrome profile has its own isolated
// chrome.storage.local, so this replaces the shared sightmap-config.json file
// approach without any file-system races.
//
// The function retries for up to 5 seconds to allow the service worker to
// finish starting. Returns nil if no extension service worker is found — the
// extension will fall back to port-range probing.
func InjectExtensionServerPort(ctx context.Context, addr string, serverPort int) error {
	swWSURL, err := findExtensionSWWS(ctx, addr)
	if err != nil {
		return fmt.Errorf("inject extension port: %w", err)
	}
	if swWSURL == "" {
		// No extension service worker found — non-fatal; extension will probe ports.
		return nil
	}

	// Connect directly to the service worker's debugger endpoint.
	dialer := websocket.Dialer{}
	ws, _, err := dialer.DialContext(ctx, swWSURL, nil)
	if err != nil {
		return fmt.Errorf("inject extension port: dial service worker: %w", err)
	}
	defer ws.Close()

	type cdpReq struct {
		ID     int    `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}
	expr := fmt.Sprintf("chrome.storage.local.set({serverPort: %d})", serverPort)
	if err := ws.WriteJSON(cdpReq{
		ID:     1,
		Method: "Runtime.evaluate",
		Params: map[string]any{
			"expression":   expr,
			"awaitPromise": true,
		},
	}); err != nil {
		return fmt.Errorf("inject extension port: send evaluate: %w", err)
	}

	// Read and discard the response (we only care that the call succeeded).
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, raw, err := ws.ReadMessage()
	if err != nil {
		return fmt.Errorf("inject extension port: read response: %w", err)
	}
	var resp struct {
		Error *struct{ Message string } `json:"error"`
	}
	if json.Unmarshal(raw, &resp) == nil && resp.Error != nil {
		return fmt.Errorf("inject extension port: CDP error: %s", resp.Error.Message)
	}
	return nil
}

// findExtensionSWWS returns the WebSocket debugger URL of the sightmap extension's
// service worker, retrying for up to 5 seconds while it starts. Returns "" if no
// extension service worker is found.
func findExtensionSWWS(ctx context.Context, addr string) (string, error) {
	type target struct {
		Type  string `json:"type"`
		URL   string `json:"url"`
		WSURL string `json:"webSocketDebuggerUrl"`
	}
	findSW := func() (string, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/json/list", nil)
		if err != nil {
			return "", err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		var targets []target
		if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
			return "", err
		}
		for _, t := range targets {
			if t.Type == "service_worker" && strings.HasPrefix(t.URL, "chrome-extension://") {
				return t.WSURL, nil
			}
		}
		return "", nil
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		wsURL, err := findSW()
		if err != nil {
			return "", fmt.Errorf("list targets: %w", err)
		}
		if wsURL != "" {
			return wsURL, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return "", nil
}

// ReloadExtension finds the sightmap extension's service worker and triggers
// chrome.runtime.reload(), forcing Chrome to re-register the worker from the
// freshly extracted on-disk files. This defeats Chrome's habit of restoring a
// cached/stale service worker after an extension version bump, even across a
// full restart. The reload tears the worker down, so a missing
// response is expected and not an error.
func ReloadExtension(ctx context.Context, addr string) error {
	swWSURL, err := findExtensionSWWS(ctx, addr)
	if err != nil {
		return err
	}
	if swWSURL == "" {
		return nil // no extension service worker; nothing to reload
	}
	dialer := websocket.Dialer{}
	ws, _, err := dialer.DialContext(ctx, swWSURL, nil)
	if err != nil {
		return fmt.Errorf("reload extension: dial service worker: %w", err)
	}
	defer ws.Close()
	type cdpReq struct {
		ID     int    `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}
	if err := ws.WriteJSON(cdpReq{
		ID:     1,
		Method: "Runtime.evaluate",
		Params: map[string]any{"expression": "chrome.runtime.reload()"},
	}); err != nil {
		return fmt.Errorf("reload extension: send evaluate: %w", err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, _, _ = ws.ReadMessage() // worker is torn down by reload; response optional
	return nil
}
