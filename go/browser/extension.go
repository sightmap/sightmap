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

// sightmapExtensionMarker is matched (case-insensitively, as a substring)
// against a candidate extension's manifest `name` to identify the sightmap
// overlay extension among any others installed in the browser. Matching the
// manifest name — rather than a hardcoded extension ID — is what lets this work
// across both the published extension and an unpacked `--load-extension` build,
// whose IDs differ.
const sightmapExtensionMarker = "sightmap"

// InjectExtensionServerPort finds the sightmap extension's service worker
// target via the Chrome DevTools Protocol and injects the sightmap HTTP server
// port into chrome.storage.local. Each Chrome profile has its own isolated
// chrome.storage.local, so this replaces the shared sightmap-config.json file
// approach without any file-system races.
//
// The function retries for up to 5 seconds to allow the service worker to
// finish starting. Returns nil if the sightmap extension's service worker is not
// found — the extension will fall back to port-range probing.
func InjectExtensionServerPort(ctx context.Context, addr string, serverPort int) error {
	swWSURL, err := findExtensionSWWS(ctx, addr)
	if err != nil {
		return fmt.Errorf("inject extension port: %w", err)
	}
	if swWSURL == "" {
		// Sightmap extension service worker not found — non-fatal; the extension
		// will fall back to port-range probing.
		return nil
	}

	expr := fmt.Sprintf("chrome.storage.local.set({serverPort: %d})", serverPort)
	if _, err := swEval(ctx, swWSURL, expr); err != nil {
		return fmt.Errorf("inject extension port: %w", err)
	}
	return nil
}

// findExtensionSWWS returns the WebSocket debugger URL of the *sightmap*
// extension's service worker, retrying for up to 5 seconds while it starts.
// Returns "" if no sightmap extension service worker is found.
//
// It must not just take the first extension service worker it sees: a real
// browser (the --attach case) commonly has several extensions installed, so the
// candidate is confirmed by evaluating its manifest `name` and matching
// sightmapExtensionMarker. (An owned launch has an isolated profile with only
// the sightmap extension, so this only ever mattered once attach let us point at
// a user's everyday browser.)
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
			if t.Type != "service_worker" || !strings.HasPrefix(t.URL, "chrome-extension://") || t.WSURL == "" {
				continue
			}
			if isSightmapSW(ctx, t.WSURL) {
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

// isSightmapSW reports whether the extension service worker at wsURL belongs to
// the sightmap extension, by reading its manifest name over CDP. Any dial/eval
// error is treated as "not it" so an unrelated or uncooperative extension is
// simply skipped.
func isSightmapSW(ctx context.Context, wsURL string) bool {
	val, err := swEval(ctx, wsURL, "chrome.runtime.getManifest().name")
	if err != nil {
		return false
	}
	var name string
	if json.Unmarshal(val, &name) != nil {
		return false
	}
	return strings.Contains(strings.ToLower(name), sightmapExtensionMarker)
}

// swEval dials a service-worker debugger endpoint, runs Runtime.evaluate for
// expr (awaiting promises, returning the value by value), and returns the raw
// JSON of the result value. Shared by the extension-identity check and the
// server-port injection.
func swEval(ctx context.Context, wsURL, expr string) (json.RawMessage, error) {
	dialer := websocket.Dialer{}
	ws, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("dial service worker: %w", err)
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
		Params: map[string]any{
			"expression":    expr,
			"awaitPromise":  true,
			"returnByValue": true,
		},
	}); err != nil {
		return nil, fmt.Errorf("send evaluate: %w", err)
	}

	// Bound the read by the caller's deadline when it has one, else a short
	// default. Loop until our command reply (id==1) arrives, skipping any
	// protocol events the worker emits first.
	if dl, ok := ctx.Deadline(); ok {
		_ = ws.SetReadDeadline(dl)
	} else {
		_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	}
	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		var resp struct {
			ID     int `json:"id"`
			Result struct {
				Result struct {
					Value json.RawMessage `json:"value"`
				} `json:"result"`
			} `json:"result"`
			Error *struct{ Message string } `json:"error"`
		}
		if json.Unmarshal(raw, &resp) != nil || resp.ID != 1 {
			continue // malformed, or an event rather than our reply
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("CDP error: %s", resp.Error.Message)
		}
		return resp.Result.Result.Value, nil
	}
}
