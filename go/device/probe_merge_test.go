package device

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// probe-catalog.json.gz is a real probe dump of the sample app's Catalog tab
// (testdata/SampleApp), trimmed to the three row hosts.
func loadProbeDump(t *testing.T) *ProbeDump {
	t.Helper()
	f, err := os.Open("testdata/probe-catalog.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var d ProbeDump
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		t.Fatal(err)
	}
	return &d
}

func catalogDescribe() *Describe {
	row := func(id, label string, y float64) *RawElement {
		return &RawElement{Type: "Button", Identifier: id, Label: label, Traits: []string{"Button"}, Frame: Frame{X: 16, Y: y, W: 358, H: 90.3}}
	}
	return &Describe{Screen: Size{W: 390, H: 844}, Root: &RawElement{Type: "Application", Label: "Sample", Frame: Frame{W: 390, H: 844}, Children: []*RawElement{
		{Type: "Heading", Label: "Catalog", Frame: Frame{X: 16, Y: 100, W: 200, H: 40}},
		row("catalog-row-1", "Socks, Wool, Pair of 2, Acme, Inc., $9.99", 153),
		row("catalog-row-2", "Desk Lamp, Bright, Light & Co., $24.50", 243.3),
		row("catalog-row-3", "Mug, Large, Kiln, $12.00", 333.7),
	}}}
}

func TestProbeSplitsJoinedLabels(t *testing.T) {
	items, err := ProbeItems(loadProbeDump(t))
	if err != nil {
		t.Fatal(err)
	}
	d := catalogDescribe()
	MergeProbe(d, items, MergeOptions{Fields: true})
	s, err := Annotate(Normalize(d), loadCorpus(t, "SampleApp/.sightmap"), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	out := s.Render(RenderOpts{})
	for _, want := range []string{
		`[View: Catalog]`,
		`[CatalogRow maker="Acme, Inc." price="9.99" summary="Socks, Wool, Pair of 2, Acme, Inc., $9.99" title="Socks, Wool, Pair of 2"]`,
		`view[type="CatalogRow"]`,
		`[CatalogRow maker="Bright, Light & Co." price="24.5" summary="Desk Lamp, Bright, Light & Co., $24.50" title="Desk Lamp"]`,
		`[CatalogRow maker="Kiln" price="12" summary="Mug, Large, Kiln, $12.00" title="Mug, Large"]`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
	view := d.Root.Children[1].Children[0]
	if view.Attrs["item.title"] != "Socks, Wool, Pair of 2" || view.Attrs["item.price"] != "9.99" || view.Attrs["maker"] != "Acme, Inc." {
		t.Errorf("view attrs = %v", view.Attrs)
	}
	if price := view.Children[2]; price.Attrs["format"] != "currency USD" || price.Value != "9.99" {
		t.Errorf("price = %+v", price)
	}
}

func TestProbeLeavesSeparateTextsAlone(t *testing.T) {
	items, err := ProbeItems(loadProbeDump(t))
	if err != nil {
		t.Fatal(err)
	}
	d := catalogDescribe()
	row := d.Root.Children[1]
	row.Children = []*RawElement{
		{Type: "StaticText", Label: "Socks, Wool, Pair of 2"},
		{Type: "StaticText", Label: "Acme, Inc."},
	}
	MergeProbe(d, items, MergeOptions{Fields: true})
	var texts []string
	var walk func(*RawElement)
	walk = func(e *RawElement) {
		if e.Type == "StaticText" {
			texts = append(texts, e.Label)
		}
		for _, c := range e.Children {
			walk(c)
		}
	}
	walk(row)
	if got := strings.Join(texts, "|"); got != "Socks, Wool, Pair of 2|Acme, Inc.|9.99" {
		t.Errorf("texts = %s", got)
	}
}

func TestAppType(t *testing.T) {
	for in, want := range map[string]string{
		"SimSample.CatalogRow":                    "CatalogRow",
		"SimSample.CatalogRow<SwiftUI.EmptyView>": "CatalogRow",
		"SwiftUI.Text":                            "",
		"Swift.Optional<SimSample.CatalogRow>":    "",
		"SwiftUI.(unknown context at $1).Thing":   "",
	} {
		if got := appType(in); got != want {
			t.Errorf("appType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProbeRedactsAppViewFields(t *testing.T) {
	view := &probeView{Name: "ProfileRow", Fields: [][2]string{
		{"user.name", "Ada"},
		{"user.apiKey", "k-123"},
		{"session.token", "t-456"},
		{"avatar", "https://cdn.example.com/a.png?size=2&sig=abc&token=xyz"},
		{"type", "admin"},
	}}
	items := []*probeItem{
		{Anchor: "profile", View: view},
		{Anchor: "profile", View: view, Text: "Ada", Frame: Frame{X: 10, Y: 10, W: 50, H: 20}},
		{Anchor: "profile", View: view, Text: "t-456", Frame: Frame{X: 10, Y: 40, W: 50, H: 20}},
	}
	row := &RawElement{Type: "Button", Identifier: "profile", Label: "Ada, t-456", Frame: Frame{W: 300, H: 80}}
	d := &Describe{Root: &RawElement{Type: "Application", Frame: Frame{W: 390, H: 844}, Children: []*RawElement{row}}}
	MergeProbe(d, items, MergeOptions{Fields: true})
	v := row.Children[0]
	want := map[string]string{
		"type":          "ProfileRow",
		"user.name":     "Ada",
		"user.apiKey":   Redacted,
		"session.token": Redacted,
		"avatar":        "https://cdn.example.com/a.png?size=2&sig=abc&token=" + Redacted,
	}
	for k, w := range want {
		if v.Attrs[k] != w {
			t.Errorf("%s = %q, want %q", k, v.Attrs[k], w)
		}
	}
	if len(v.Children) != 2 || v.Children[1].Label != Redacted || v.Children[1].Attrs["field"] != "session.token" {
		t.Errorf("texts = %+v", v.Children)
	}

	row.Children = nil
	MergeProbe(d, items, MergeOptions{})
	if v := row.Children[0]; len(v.Attrs) != 1 || v.Attrs["type"] != "ProfileRow" || len(v.Children) != 2 {
		t.Errorf("with fields off: attrs %v, %d texts", v.Attrs, len(v.Children))
	}
}

func TestProbeSkipsSecureFields(t *testing.T) {
	items := []*probeItem{{Anchor: "pw", Text: "hunter2", Frame: Frame{W: 10, H: 10}}}
	field := &RawElement{Type: "SecureTextField", Identifier: "pw", Label: "hunter2 x", Frame: Frame{W: 300, H: 40}}
	d := &Describe{Root: &RawElement{Type: "Application", Frame: Frame{W: 390, H: 844}, Children: []*RawElement{field}}}
	MergeProbe(d, items, MergeOptions{Fields: true})
	if len(field.Children) != 0 {
		t.Errorf("secure field got children %+v", field.Children)
	}
}
