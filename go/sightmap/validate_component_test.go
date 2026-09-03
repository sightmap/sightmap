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
				{Name: "label", Extract: "text"},
				{Name: "href", Extract: "attr=href"},
				{Name: "price", Extract: "Price.text"},
				{Name: "sold_out", Extract: "exists:SoldOutBadge"},
				{Name: "amount", Extract: "Row.Price.amount"},
			},
		},
		{
			name: "duplicate name",
			props: []ComponentPropertyDef{
				{Name: "price", Extract: "text"},
				{Name: "price", Extract: "attr=data-price"},
			},
			want: map[string]int{"component-property-duplicate": 1},
		},
		// component-property-invalid-name: the schema pattern for a component
		// property name is ^[a-z][a-z0-9_]*$ (sightmap.schema.json), enforced by
		// ajv. The Go CLI never validates against the schema, so without the
		// pattern check in checkComponentProperties a property named "5" or
		// "Header Text" passes `sightmap validate` while the reference checker
		// rejects it. Mirrors request-property-invalid-name / message-property-
		// invalid-name. The empty string also fails the pattern (a `name:` or
		// `name: ""` is already a schema violation).
		{
			name:  "invalid name: starts with digit",
			props: []ComponentPropertyDef{{Name: "5", Extract: "text"}},
			want:  map[string]int{"component-property-invalid-name": 1},
		},
		{
			name:  "invalid name: uppercase first char",
			props: []ComponentPropertyDef{{Name: "Header", Extract: "text"}},
			want:  map[string]int{"component-property-invalid-name": 1},
		},
		{
			name:  "invalid name: contains space",
			props: []ComponentPropertyDef{{Name: "header text", Extract: "text"}},
			want:  map[string]int{"component-property-invalid-name": 1},
		},
		{
			name:  "invalid name: hyphen",
			props: []ComponentPropertyDef{{Name: "out-come", Extract: "text"}},
			want:  map[string]int{"component-property-invalid-name": 1},
		},
		{
			name:  "invalid name: empty (null or quoted-empty decodes to \"\")",
			props: []ComponentPropertyDef{{Name: "", Extract: "text"}},
			want:  map[string]int{"component-property-invalid-name": 1},
		},
		// The pattern check and the duplicate-dedup check are independent: a
		// pair of identical pattern-invalid names reports invalid-name once per
		// declaration AND the duplicate once. The dedup guard keeps empty names
		// out of the duplicate check, so this only applies to non-empty names.
		{
			name: "invalid name duplicated reports both codes",
			props: []ComponentPropertyDef{
				{Name: "5", Extract: "text"},
				{Name: "5", Extract: "text"},
			},
			want: map[string]int{"component-property-invalid-name": 2, "component-property-duplicate": 1},
		},
		{
			name:  "removed DOM mode: inner_text",
			props: []ComponentPropertyDef{{Name: "x", Extract: "inner_text"}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "removed DOM mode: text_only",
			props: []ComponentPropertyDef{{Name: "x", Extract: "text_only"}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "removed DOM mode: inner_html",
			props: []ComponentPropertyDef{{Name: "x", Extract: "inner_html"}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "bare CSS sub-selector",
			props: []ComponentPropertyDef{{Name: "x", Extract: ".price"}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "bare CSS selector",
			props: []ComponentPropertyDef{{Name: "x", Extract: "[data-testid=x]"}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "mistyped attr (missing =)",
			props: []ComponentPropertyDef{{Name: "x", Extract: "attr"}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "attr with no name",
			props: []ComponentPropertyDef{{Name: "x", Extract: "attr="}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "mistyped exists (missing :)",
			props: []ComponentPropertyDef{{Name: "x", Extract: "exists"}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "exists with empty path",
			props: []ComponentPropertyDef{{Name: "x", Extract: "exists:"}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
		{
			name:  "path with empty prop",
			props: []ComponentPropertyDef{{Name: "x", Extract: "Price."}},
			want:  map[string]int{"component-property-extract-invalid": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := codesFor(checkComponentProperties(compCorpus(tt.props...)))
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
