package sightmap

import (
	"os"
	"path/filepath"
	"testing"
)

func loadYAML(t *testing.T, yaml string) *Corpus {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte("version: 1\n"+yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := loadDir(dir)
	if err != nil {
		t.Fatalf("loadDir: %v", err)
	}
	return c
}

func TestLoad_ExtractObjectAndStringFormsLowerAlike(t *testing.T) {
	c := loadYAML(t, `
components:
  - name: Card
    selector: .card
    properties:
      - name: a
        extract: { from: component, path: 'Tag[].value', join: ',' }
      - name: b
        extract: attr=data-sku
requests:
  - name: Pay
    route: /pay
    properties:
      - name: object
        extract: { from: rsp.headers, path: X-Rate, pattern: '(\d+)' }
      - name: legacy
        source: rsp.headers
        field: X-Rate
        pattern: '(\d+)'
messages:
  - name: Boom
    properties:
      - name: file
        source: stack
        field: top.file
`)
	props := c.GlobalComponents[0].Properties
	if e := props[0].Extract; e.From != FromComponent || e.Path != "Tag[].value" || e.Join != "," || e.IsLegacy() {
		t.Errorf("object component extract = %+v", e)
	}
	if e := props[1].Extract; e.From != FromDOMAttr || e.Path != "data-sku" || e.Legacy != "attr=data-sku" {
		t.Errorf("string component extract = %+v", e)
	}
	rp := c.Requests[0].Properties
	if rp[0].Extract.String() != rp[1].Extract.String() {
		t.Errorf("request forms differ: %s vs %s", rp[0].Extract, rp[1].Extract)
	}
	if !rp[1].Extract.IsLegacy() || rp[0].Extract.IsLegacy() {
		t.Errorf("legacy flags: object=%v legacy=%v", rp[0].Extract.IsLegacy(), rp[1].Extract.IsLegacy())
	}
	if e := c.Messages[0].Properties[0].Extract; e.From != FromStack || e.Path != "top.file" || !e.IsLegacy() {
		t.Errorf("message extract = %+v", e)
	}
}

func TestValidate_ExtractShapeProblems(t *testing.T) {
	c := loadYAML(t, `
components:
  - name: Card
    selector: .card
    properties:
      - name: a
        extract: { from: component, path: 'Tag[].value', join: '' }
      - name: b
        extract: { from: dom.attr, path: x, select: first }
requests:
  - name: Pay
    route: /pay
    properties:
      - name: mixed
        extract: { from: rsp.body, path: status }
        source: rsp.body
      - name: string
        extract: text
      - name: joined
        extract: { from: rsp.body, path: items, join: ',' }
`)
	codes := codesFor(Validate(c))
	for code, n := range map[string]int{"extract-join-invalid": 2, "extract-shape-mixed": 2, "unknown-field": 1} {
		if codes[code] != n {
			t.Errorf("%s: got %d, want %d (all: %v)", code, codes[code], n, codes)
		}
	}
}
