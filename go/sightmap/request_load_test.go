package sightmap

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// requests: is parsed at the file root (global) and inside a view (view-scoped),
// including the nested request/response payload fields and headers.
func TestLoadDir_Requests(t *testing.T) {
	dir := t.TempDir()
	yaml := `
version: 1
requests:
  - name: SearchFlights
    route: /api/flights/search
    method: POST
    request:
      fields:
        - name: origin
          type: string
    response:
      fields:
        - name: results
          type: array
    headers: [x-request-id]
    memory:
      - rate limited to 10/min
views:
  - name: Admin
    route: /admin
    requests:
      - name: AdminPing
        route: /api/admin/ping
`
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	corpus, err := loadDir(dir)
	if err != nil {
		t.Fatalf("loadDir: %v", err)
	}

	if len(corpus.Requests) != 1 {
		t.Fatalf("want 1 global request, got %d: %+v", len(corpus.Requests), corpus.Requests)
	}
	r := corpus.Requests[0]
	if r.Name != "SearchFlights" || r.Route != "/api/flights/search" || r.Method != "POST" {
		t.Errorf("global request = %+v, want SearchFlights POST /api/flights/search", r)
	}
	if r.Request == nil || len(r.Request.Fields) != 1 ||
		r.Request.Fields[0].Name != "origin" || r.Request.Fields[0].Type != "string" {
		t.Errorf("request payload = %+v, want one field origin:string", r.Request)
	}
	if r.Response == nil || len(r.Response.Fields) != 1 || r.Response.Fields[0].Name != "results" {
		t.Errorf("response payload = %+v, want one field results", r.Response)
	}
	if !reflect.DeepEqual(r.Headers, []string{"x-request-id"}) {
		t.Errorf("headers = %v, want [x-request-id]", r.Headers)
	}
	if !reflect.DeepEqual(r.Memory, []string{"rate limited to 10/min"}) {
		t.Errorf("memory = %v, want [rate limited to 10/min]", r.Memory)
	}

	if len(corpus.Views) != 1 || len(corpus.Views[0].Requests) != 1 ||
		corpus.Views[0].Requests[0].Name != "AdminPing" {
		t.Errorf("view requests = %+v, want one AdminPing", corpus.Views)
	}
}
