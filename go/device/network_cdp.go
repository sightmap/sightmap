package device

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sightmap/sightmap/go/sightmap"
)

// A React Native Debug build reports its JS fetch/XHR traffic to Metro's
// inspector proxy over the Chrome DevTools Protocol. Metro lists the app's
// pages at /json/list and refuses a debugger WebSocket without an Origin of
// its own. It drops the socket when the app process restarts, does not replay
// requests made before Network.enable, and returns response bodies base64
// encoded.

// metroPage is one /json/list entry.
type metroPage struct {
	ID                   string          `json:"id"`
	AppID                string          `json:"appId"`
	Title                string          `json:"title"`
	WebSocketDebuggerURL string          `json:"webSocketDebuggerUrl"`
	ReactNative          json.RawMessage `json:"reactNative"`
}

// pickMetroPage returns the page for bundleID: one whose appId is the bundle
// id, else (for a Metro that does not report appId) the first React Native page.
func pickMetroPage(pages []metroPage, bundleID string) (metroPage, bool) {
	for _, p := range pages {
		if p.AppID == bundleID && p.WebSocketDebuggerURL != "" {
			return p, true
		}
	}
	for _, p := range pages {
		if p.AppID == "" && len(p.ReactNative) > 0 && p.WebSocketDebuggerURL != "" {
			return p, true
		}
	}
	return metroPage{}, false
}

func metroPages(ctx context.Context, port int) ([]metroPage, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://localhost:%d/json/list", port), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var pages []metroPage
	if err := json.NewDecoder(resp.Body).Decode(&pages); err != nil {
		return nil, err
	}
	return pages, nil
}

type cdpMessage struct {
	ID     int             `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type cdpRequest struct {
	RequestID string  `json:"requestId"`
	Timestamp float64 `json:"timestamp"`
	WallTime  float64 `json:"wallTime"`
	Request   struct {
		URL      string            `json:"url"`
		Method   string            `json:"method"`
		Headers  map[string]string `json:"headers"`
		PostData string            `json:"postData"`
	} `json:"request"`
	Response *struct {
		Status     int               `json:"status"`
		StatusText string            `json:"statusText"`
		Headers    map[string]string `json:"headers"`
		MimeType   string            `json:"mimeType"`
		Protocol   string            `json:"protocol"`
	} `json:"response"`
	EncodedDataLength float64 `json:"encodedDataLength"`
	ErrorText         string  `json:"errorText"`

	seconds float64 // from requestWillBeSent to loadingFinished
}

func mapHeaders(m map[string]string) []sightmap.Header {
	if len(m) == 0 {
		return nil
	}
	out := make([]sightmap.Header, 0, len(m))
	for k, v := range m {
		out = append(out, sightmap.Header{Name: k, Value: v})
	}
	return out
}

func headerValue(hs []sightmap.Header, name string) string {
	for _, h := range hs {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

// cdpTracker assembles Network domain events into entries: an entry is
// complete at loadingFinished (once its body is fetched) or loadingFailed.
type cdpTracker struct {
	inflight map[string]*cdpRequest
}

func newCDPTracker() *cdpTracker { return &cdpTracker{inflight: map[string]*cdpRequest{}} }

// event folds one event in and returns the request it completed, if any.
func (t *cdpTracker) event(method string, params json.RawMessage) (*cdpRequest, bool) {
	var p cdpRequest
	if json.Unmarshal(params, &p) != nil || p.RequestID == "" {
		return nil, false
	}
	switch method {
	case "Network.requestWillBeSent":
		t.inflight[p.RequestID] = &p
	case "Network.responseReceived":
		if r := t.inflight[p.RequestID]; r != nil {
			r.Response = p.Response
		}
	case "Network.loadingFinished", "Network.loadingFailed":
		r := t.inflight[p.RequestID]
		if r == nil {
			return nil, false
		}
		delete(t.inflight, p.RequestID)
		r.EncodedDataLength, r.ErrorText = p.EncodedDataLength, p.ErrorText
		if r.Timestamp > 0 && p.Timestamp > r.Timestamp {
			r.seconds = p.Timestamp - r.Timestamp
		}
		return r, true
	}
	return nil, false
}

// entry converts a completed request.
func (r *cdpRequest) entry(body string) NetworkEntry {
	e := NetworkEntry{Source: SourceRNCDP, Method: r.Request.Method, URL: r.Request.URL, DurationMs: int64(r.seconds * 1000),
		ReqHeaders: mapHeaders(r.Request.Headers), ResponseBytes: int64(r.EncodedDataLength), TaskID: r.RequestID, Error: r.ErrorText}
	if r.WallTime > 0 {
		sec := int64(r.WallTime)
		e.Time = time.Unix(sec, int64((r.WallTime-float64(sec))*1e9)).UTC().Format(time.RFC3339Nano)
	}
	e.ReqBody = newBody(r.Request.PostData, headerValue(e.ReqHeaders, "Content-Type"), 0)
	if rsp := r.Response; rsp != nil {
		e.Status, e.StatusText, e.MimeType, e.Protocol = rsp.Status, rsp.StatusText, rsp.MimeType, rsp.Protocol
		e.RspHeaders = mapHeaders(rsp.Headers)
		e.RspBody = newBody(body, rsp.MimeType, 0)
	}
	return e
}

// decodeCDPBody unpacks a Network.getResponseBody result.
func decodeCDPBody(result json.RawMessage) string {
	var b struct {
		Body          string `json:"body"`
		Base64Encoded bool   `json:"base64Encoded"`
	}
	if json.Unmarshal(result, &b) != nil {
		return ""
	}
	return bodyText(b.Body, map[bool]string{true: "base64"}[b.Base64Encoded])
}

// cdpConn is one debugger connection to a Metro page.
type cdpConn struct {
	ws      *websocket.Conn
	writeMu sync.Mutex
	mu      sync.Mutex
	nextID  int
	waiting map[int]chan cdpMessage
}

func dialMetro(ctx context.Context, port int, page metroPage) (*cdpConn, error) {
	header := http.Header{"Origin": {fmt.Sprintf("http://localhost:%d", port)}}
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	ws, _, err := dialer.DialContext(ctx, page.WebSocketDebuggerURL, header)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(256 << 20)
	return &cdpConn{ws: ws, waiting: map[int]chan cdpMessage{}}, nil
}

// send issues a command without waiting; the reply arrives on the returned
// channel once read delivers it.
func (c *cdpConn) send(method string, params any) (chan cdpMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan cdpMessage, 1)
	c.waiting[id] = ch
	c.mu.Unlock()
	raw, _ := json.Marshal(params)
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return ch, c.ws.WriteJSON(cdpMessage{ID: id, Method: method, Params: raw})
}

// read delivers replies and passes events to events until the socket closes.
func (c *cdpConn) read(events chan<- cdpMessage) error {
	defer close(events)
	for {
		var m cdpMessage
		if err := c.ws.ReadJSON(&m); err != nil {
			return err
		}
		if m.ID != 0 {
			c.mu.Lock()
			ch := c.waiting[m.ID]
			delete(c.waiting, m.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- m
			}
			continue
		}
		events <- m
	}
}

// cdpSource follows the app's page on Metro, reconnecting whenever the app
// restarts, and pushes each completed request.
type cdpSource struct {
	port     int
	bundleID string
	push     func(NetworkEntry)
	setState func(state, detail string)
	log      func(string)
}

func (s *cdpSource) run(ctx context.Context) {
	reported := ""
	for ctx.Err() == nil {
		detail := s.attach(ctx)
		if detail != reported {
			s.setState(stateStandby, detail)
			reported = detail
		}
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
}

// attach serves one connection until it drops, returning why the source is
// not active now.
func (s *cdpSource) attach(ctx context.Context) string {
	pages, err := metroPages(ctx, s.port)
	if err != nil {
		return fmt.Sprintf("no Metro on port %d (a React Native Debug build reports JS traffic there)", s.port)
	}
	page, ok := pickMetroPage(pages, s.bundleID)
	if !ok {
		return fmt.Sprintf("Metro on port %d lists no page for %s", s.port, s.bundleID)
	}
	conn, err := dialMetro(ctx, s.port, page)
	if err != nil {
		return fmt.Sprintf("cannot attach to Metro page %s: %v", page.ID, err)
	}
	defer conn.ws.Close()
	stop := context.AfterFunc(ctx, func() { conn.ws.Close() })
	defer stop()
	events := make(chan cdpMessage, 256)
	done := make(chan error, 1)
	go func() { done <- conn.read(events) }()
	for _, m := range []string{"Runtime.enable", "Network.enable"} {
		if _, err := conn.send(m, struct{}{}); err != nil {
			return "Metro dropped the debugger connection: " + err.Error()
		}
	}
	s.setState(stateActive, fmt.Sprintf("attached to Metro on port %d (page %s)", s.port, page.ID))
	s.log(fmt.Sprintf("network: rn-cdp attached to Metro port %d page %s", s.port, page.ID))
	tracker := newCDPTracker()
	for m := range events {
		r, complete := tracker.event(m.Method, m.Params)
		if !complete {
			continue
		}
		body := ""
		if m.Method == "Network.loadingFinished" {
			body = s.responseBody(conn, r.RequestID)
		}
		s.push(r.entry(body))
	}
	err = <-done
	return fmt.Sprintf("Metro connection closed (%v); reconnecting", err)
}

// responseBody fetches a body; an error (an empty body, say) means none.
func (s *cdpSource) responseBody(conn *cdpConn, requestID string) string {
	ch, err := conn.send("Network.getResponseBody", map[string]string{"requestId": requestID})
	if err != nil {
		return ""
	}
	select {
	case m := <-ch:
		if m.Error != nil {
			return ""
		}
		return decodeCDPBody(m.Result)
	case <-time.After(5 * time.Second):
		return ""
	}
}
