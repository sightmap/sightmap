package atlas

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestParseIndex_ignoresUnknownFields(t *testing.T) {
	// The atlas must be able to grow metadata without breaking shipped CLIs.
	data := []byte(`{
	  "schema_version": 1,
	  "generated_at": "2026-08-01T00:00:00Z",
	  "entries": [
	    {"slug": "shop", "name": "Shop", "description": "demo", "stars": 12,
	     "files": [".sightmap/config.yaml"]}
	  ]
	}`)
	idx, err := ParseIndex(data)
	if err != nil {
		t.Fatalf("ParseIndex: %v", err)
	}
	if len(idx.Entries) != 1 || idx.Entries[0].Slug != "shop" || idx.Entries[0].Name != "Shop" {
		t.Fatalf("entries = %+v", idx.Entries)
	}
}

// The schema gate has to run before the document's shape is assumed, or a
// restructured v2 index dies on a field-level JSON error and the actionable
// "upgrade sightmap" message never reaches the user.
func TestParseIndex_schemaGateBeatsShape(t *testing.T) {
	cases := []struct{ name, body string }{
		{"same-shape", `{"schema_version": 2, "entries": [{"slug": "shop", "files": [".sightmap/config.yaml"]}]}`},
		{"restructured", `{"schema_version": 2, "entries": {"shop": {"files": [".sightmap/config.yaml"]}}}`},
		{"far-future", `{"schema_version": 99, "corpora": []}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseIndex([]byte(tc.body))
			if err == nil {
				t.Fatal("expected a schema_version rejection")
			}
			for _, want := range []string{"schema_version", "upgrade sightmap"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %v, want it to mention %q", err, want)
				}
			}
		})
	}
}

func TestParseIndex_acceptsCurrentAndUnset(t *testing.T) {
	for _, body := range []string{
		`{"schema_version": 1, "entries": []}`,
		`{"entries": []}`, // pre-schema_version index
	} {
		if _, err := ParseIndex([]byte(body)); err != nil {
			t.Errorf("ParseIndex(%s) = %v, want ok", body, err)
		}
	}
}

func TestParseIndex_malformedJSON(t *testing.T) {
	if _, err := ParseIndex([]byte(`{"schema_version": 1,`)); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestIndexSuggestSlugs(t *testing.T) {
	idx := &Index{Entries: []Entry{
		{Slug: "square-pos"},
		{Slug: "square-web"},
		{Slug: "pinned-shop"},
		{Slug: "cafe-demo"},
		{Slug: "crm-suite"},
	}}
	cases := []struct {
		name string
		arg  string
		want []string
	}{
		// Containment in either direction, alphabetical.
		{"prefix-of-two", "square", []string{"square-pos", "square-web"}},
		{"longer-than-slug", "cafe-demo-2", []string{"cafe-demo"}},
		{"middle", "shop", []string{"pinned-shop"}},
		// A shared leading character is not a resemblance — the old scoring
		// tier answered `checkout` with `cafe-demo` and `crm-suite`.
		{"shared-first-letter-only", "checkout", nil},
		{"no-match", "zzz", nil},
		{"empty", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := idx.SuggestSlugs(tc.arg); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SuggestSlugs(%q) = %v, want %v", tc.arg, got, tc.want)
			}
		})
	}
}

func TestIndexSuggestSlugs_capsAtFive(t *testing.T) {
	var idx Index
	for i := 0; i < 8; i++ {
		idx.Entries = append(idx.Entries, Entry{Slug: fmt.Sprintf("shop-%d", i)})
	}
	got := idx.SuggestSlugs("shop")
	if len(got) != maxSuggestions {
		t.Fatalf("got %d suggestions, want %d: %v", len(got), maxSuggestions, got)
	}
	// Deterministic: the same five, in the same order, every time.
	want := []string{"shop-0", "shop-1", "shop-2", "shop-3", "shop-4"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SuggestSlugs = %v, want %v", got, want)
	}
}

// Suggestions are built from slugs no validator has looked at yet, so a slug
// that would be refused at install time must never be offered as a next step.
func TestIndexSuggestSlugs_skipsUnsafeSlugs(t *testing.T) {
	idx := &Index{Entries: []Entry{
		{Slug: "shop-good"},
		{Slug: "shop\x1b[2J-evil"},
		{Slug: "shop/../../etc"},
		{Slug: ""},
	}}
	got := idx.SuggestSlugs("shop")
	if !reflect.DeepEqual(got, []string{"shop-good"}) {
		t.Errorf("SuggestSlugs = %q, want only the installable slug", got)
	}
}

func TestIndexValidate(t *testing.T) {
	idx := &Index{
		SchemaVersion: 1,
		Entries: []Entry{
			{Slug: "shop", Files: []string{".sightmap/config.yaml"}},
			{Slug: "shop", Files: []string{".sightmap/config.yaml"}},
			{Slug: "bad", Files: []string{"../etc/passwd"}},
		},
	}
	errs := idx.Validate()
	if len(errs) != 2 {
		t.Fatalf("got %d problems, want 2: %v", len(errs), errs)
	}
	joined := fmt.Sprint(errs)
	for _, want := range []string{"duplicate slug", "unsafe file path"} {
		if !strings.Contains(joined, want) {
			t.Errorf("problems %v, want one mentioning %q", errs, want)
		}
	}

	clean := &Index{SchemaVersion: 1, Entries: []Entry{{Slug: "shop", Files: []string{".sightmap/config.yaml"}}}}
	if errs := clean.Validate(); len(errs) != 0 {
		t.Errorf("clean index reported %v", errs)
	}
}

func TestIndexEntry(t *testing.T) {
	idx := &Index{Entries: []Entry{{Slug: "a"}, {Slug: "b"}}}
	if e := idx.Entry("b"); e == nil || e.Slug != "b" {
		t.Errorf("Entry(\"b\") = %+v", e)
	}
	if e := idx.Entry("missing"); e != nil {
		t.Errorf("Entry(\"missing\") = %+v, want nil", e)
	}
}
