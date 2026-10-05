package sightmap

import "testing"

func compCorpus(props ...ComponentPropertyDef) *Corpus {
	return &Corpus{
		GlobalComponents: []ComponentDef{
			{Name: "Card", Selectors: []string{".card"}, Properties: props},
		},
	}
}

func codesFor(errs []ValidationError) map[string]int {
	m := map[string]int{}
	for _, e := range errs {
		m[e.Code]++
	}
	return m
}

func TestCheckComponentProperties(t *testing.T) {
	tests := []struct {
		name  string
		props []ComponentPropertyDef
		want  map[string]int // error code -> count; nil means no errors
	}{
		{
			name: "valid forms",
			props: []ComponentPropertyDef{
				{Name: "label", Extract: LowerComponentExtract("text")},
				{Name: "tier", Extract: LowerComponentExtract("raw_text")},
				{Name: "href", Extract: LowerComponentExtract("attr=href")},
				{Name: "price", Extract: LowerComponentExtract("Price.text")},
				{Name: "sold_out", Extract: LowerComponentExtract("exists:SoldOutBadge")},
				{Name: "amount", Extract: LowerComponentExtract("Row.Price.amount")},
			},
		},
		{
			name: "duplicate name",
			props: []ComponentPropertyDef{
				{Name: "price", Extract: LowerComponentExtract("text")},
				{Name: "price", Extract: LowerComponentExtract("attr=data-price")},
			},
			want: map[string]int{"component-property-duplicate": 1},
		},
		{
			name:  "removed DOM mode: inner_text",
			props: []ComponentPropertyDef{{Name: "x", Extract: LowerComponentExtract("inner_text")}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "removed DOM mode: text_only",
			props: []ComponentPropertyDef{{Name: "x", Extract: LowerComponentExtract("text_only")}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "removed DOM mode: inner_html",
			props: []ComponentPropertyDef{{Name: "x", Extract: LowerComponentExtract("inner_html")}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "bare CSS sub-selector",
			props: []ComponentPropertyDef{{Name: "x", Extract: LowerComponentExtract(".price")}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "bare CSS selector",
			props: []ComponentPropertyDef{{Name: "x", Extract: LowerComponentExtract("[data-testid=x]")}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "mistyped attr (missing =)",
			props: []ComponentPropertyDef{{Name: "x", Extract: LowerComponentExtract("attr")}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "attr with no name",
			props: []ComponentPropertyDef{{Name: "x", Extract: LowerComponentExtract("attr=")}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "mistyped exists (missing :)",
			props: []ComponentPropertyDef{{Name: "x", Extract: LowerComponentExtract("exists")}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "exists with empty path",
			props: []ComponentPropertyDef{{Name: "x", Extract: LowerComponentExtract("exists:")}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "path with empty prop",
			props: []ComponentPropertyDef{{Name: "x", Extract: LowerComponentExtract("Price.")}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := codesFor(checkComponentProperties(compCorpus(tt.props...)))
			delete(got, "extract-legacy-form") // every case here is a string form
			if len(got) != len(tt.want) {
				t.Fatalf("got error codes %v, want %v", got, tt.want)
			}
			for code, wantCount := range tt.want {
				if got[code] != wantCount {
					t.Errorf("code %q: got %d, want %d", code, got[code], wantCount)
				}
			}
		})
	}
}

func TestCheckComponentProperties_ObjectForm(t *testing.T) {
	tests := []struct {
		name    string
		privacy string
		extract Extract
		want    string // "" means no finding
	}{
		{name: "dom.text", extract: Extract{From: FromDOMText}},
		{name: "dom.raw_text with pattern", extract: Extract{From: FromDOMRawText, Pattern: `\$(\d+)`}},
		{name: "dom.attr", extract: Extract{From: FromDOMAttr, Path: "href"}},
		{name: "dom.state", extract: Extract{From: FromDOMState, Path: "checked"}},
		{name: "component", extract: Extract{From: FromComponent, Path: "Row.Price.amount"}},
		{name: "component join", extract: Extract{From: FromComponent, Path: "Tag[].value", Join: ",", joinSet: true}},
		{name: "component.exists", extract: Extract{From: FromComponentExists, Path: "Row.Badge"}},
		{name: "no from", extract: Extract{}, want: "component-property-extract-invalid"},
		{name: "request source", extract: Extract{From: FromRspBody, Path: "x"}, want: "component-property-extract-invalid"},
		{name: "dom.text with path", extract: Extract{From: FromDOMText, Path: "x"}, want: "component-property-extract-invalid"},
		{name: "dom.attr without path", extract: Extract{From: FromDOMAttr}, want: "component-property-extract-invalid"},
		{name: "dom.state unknown", extract: Extract{From: FromDOMState, Path: "pressed"}, want: "component-property-extract-invalid"},
		{name: "component without prop", extract: Extract{From: FromComponent, Path: "Row"}, want: "component-property-extract-invalid"},
		{name: "exists with pattern", extract: Extract{From: FromComponentExists, Path: "Row", Pattern: "x"}, want: "component-property-extract-invalid"},
		{name: "bad pattern", extract: Extract{From: FromDOMText, Pattern: "("}, want: "component-property-extract-invalid"},
		{name: "join on dom", extract: Extract{From: FromDOMText, Join: ",", joinSet: true}, want: "extract-join-invalid"},
		{name: "empty join", extract: Extract{From: FromComponent, Path: "Tag[].value", joinSet: true}, want: "extract-join-invalid"},
		{name: "join without a multi segment", extract: Extract{From: FromComponent, Path: "Tag.value", Join: ",", joinSet: true}, want: "extract-join-invalid"},
		{name: "multi segment without join", extract: Extract{From: FromComponent, Path: "Tag[].value"}, want: "component-property-extract-invalid"},
		{name: "nested multi segment", extract: Extract{From: FromComponent, Path: "Row[].Tag[].value", Join: ",", joinSet: true}},
		{name: "predicate reserved", extract: Extract{From: FromComponent, Path: "Tab[active=true].label"}, want: "component-property-extract-invalid"},
		{name: "multi property name", extract: Extract{From: FromComponent, Path: "Tag.value[]", Join: ",", joinSet: true}, want: "component-property-extract-invalid"},
		{name: "exists with multi", extract: Extract{From: FromComponentExists, Path: "Tag[]"}, want: "component-property-extract-invalid"},
		{name: "text under mask", privacy: "mask", extract: Extract{From: FromDOMText}, want: "extract-privacy-withheld"},
		{name: "attr under block", privacy: "block", extract: Extract{From: FromDOMAttr, Path: "x"}, want: "extract-privacy-withheld"},
		{name: "attr under mask", privacy: "mask", extract: Extract{From: FromDOMAttr, Path: "data-x"}, want: "extract-privacy-withheld"},
		{name: "state attr under mask", privacy: "mask", extract: Extract{From: FromDOMAttr, Path: "checked"}},
		{name: "state attr under block", privacy: "block", extract: Extract{From: FromDOMAttr, Path: "checked"}, want: "extract-privacy-withheld"},
		{name: "state under mask", privacy: "mask", extract: Extract{From: FromDOMState, Path: "checked"}},
		{name: "state under block", privacy: "block", extract: Extract{From: FromDOMState, Path: "checked"}, want: "extract-privacy-withheld"},
		{name: "component under block", privacy: "block", extract: Extract{From: FromComponent, Path: "Row.x"}},
		{name: "legacy string", extract: LowerComponentExtract("attr=href"), want: "extract-legacy-form"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Corpus{GlobalComponents: []ComponentDef{{
				Name: "Card", Selectors: []string{".card"}, Privacy: tt.privacy,
				Properties: []ComponentPropertyDef{{Name: "x", Extract: tt.extract}},
			}}}
			got := checkComponentProperties(c)
			if tt.want == "" {
				if len(got) != 0 {
					t.Fatalf("want no findings, got %v", codesFor(got))
				}
				return
			}
			if len(got) != 1 || got[0].Code != tt.want {
				t.Fatalf("want exactly %s, got %v", tt.want, codesFor(got))
			}
		})
	}
}

func TestLowerComponentExtract(t *testing.T) {
	for in, want := range map[string]Extract{
		"text":             {From: FromDOMText},
		"raw_text":         {From: FromDOMRawText},
		"attr=checked":     {From: FromDOMAttr, Path: "checked"},
		"exists:Row.Badge": {From: FromComponentExists, Path: "Row.Badge"},
		"Row.Price.amount": {From: FromComponent, Path: "Row.Price.amount"},
		"inner_text":       {},
	} {
		got := LowerComponentExtract(in)
		if got.From != want.From || got.Path != want.Path || got.Legacy != in {
			t.Errorf("LowerComponentExtract(%q) = %+v, want %+v with Legacy %q", in, got, want, in)
		}
	}
}
