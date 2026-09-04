package browser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestListTabs verifies that ListTabs returns only page-type targets.
func TestListTabs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/list" {
			targets := []map[string]interface{}{
				{"id": "t1", "type": "page", "url": "https://example.com", "title": "Example"},
				{"id": "t2", "type": "worker", "url": "https://example.com/sw.js", "title": ""},
				{"id": "t3", "type": "page", "url": "https://other.com", "title": "Other"},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(targets)
		}
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "http://")
	tabs, err := ListTabs(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(tabs) != 2 {
		t.Fatalf("expected 2 page tabs, got %d", len(tabs))
	}
	if tabs[0].TargetID != "t1" {
		t.Errorf("tab[0].TargetID=%q, want t1", tabs[0].TargetID)
	}
	if tabs[0].URL != "https://example.com" {
		t.Errorf("tab[0].URL=%q, want https://example.com", tabs[0].URL)
	}
	if tabs[0].Title != "Example" {
		t.Errorf("tab[0].Title=%q, want Example", tabs[0].Title)
	}
	if tabs[1].TargetID != "t3" || tabs[1].URL != "https://other.com" {
		t.Errorf("tab[1]=%+v, unexpected", tabs[1])
	}
}

// TestCloseTab verifies that CloseTab calls the correct /json/close/ endpoint.
func TestCloseTab(t *testing.T) {
	var closedID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/json/close/") {
			closedID = strings.TrimPrefix(r.URL.Path, "/json/close/")
			w.Write([]byte("Target is closing"))
		}
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "http://")
	if err := CloseTab(context.Background(), addr, "target-abc"); err != nil {
		t.Fatal(err)
	}
	if closedID != "target-abc" {
		t.Errorf("closedID=%q, want target-abc", closedID)
	}
}

// TestResizeViewport verifies that ResizeViewport sends Emulation.setDeviceMetricsOverride.
func TestResizeViewport(t *testing.T) {
	var mu sync.Mutex
	var viewportParams struct {
		Width             int     `json:"width"`
		Height            int     `json:"height"`
		DeviceScaleFactor float64 `json:"deviceScaleFactor"`
		Mobile            bool    `json:"mobile"`
	}
	called := false

	conn := newFakeCDP(t, func(method string, params json.RawMessage) json.RawMessage {
		if method == "Emulation.setDeviceMetricsOverride" {
			mu.Lock()
			json.Unmarshal(params, &viewportParams)
			called = true
			mu.Unlock()
		}
		return json.RawMessage(`{}`)
	})

	if err := ResizeViewport(context.Background(), conn, 1280, 800); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()

	if !called {
		t.Fatal("Emulation.setDeviceMetricsOverride was not called")
	}
	if viewportParams.Width != 1280 || viewportParams.Height != 800 {
		t.Errorf("viewport=%dx%d, want 1280x800", viewportParams.Width, viewportParams.Height)
	}
	if viewportParams.DeviceScaleFactor != 1 {
		t.Errorf("deviceScaleFactor=%v, want 1", viewportParams.DeviceScaleFactor)
	}
	if viewportParams.Mobile {
		t.Error("mobile=true, want false")
	}
}
