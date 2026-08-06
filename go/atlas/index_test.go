package atlas

import (
	"strings"
	"testing"
)

// The version gate has to run before the document's shape is assumed. `find`
// and `list` are the only readers of the index now, so this is where a future
// schema bump has to say "upgrade sightmap" rather than dying on a field.
func TestParseIndex_refusesAFutureSchemaBeforeDecodingEntries(t *testing.T) {
	// Entries restructured into an object: a v1 decoder cannot read a line of
	// it, which is exactly why the gate reads schema_version on its own first.
	data := []byte(`{"schema_version": 2, "entries": {"square-pos": {"name": "Square POS"}}}`)
	_, err := ParseIndex(data)
	if err == nil {
		t.Fatal("expected a schema_version refusal")
	}
	mustContain(t, err.Error(), "schema_version 2", "upgrade sightmap")
	if strings.Contains(err.Error(), "cannot unmarshal") {
		t.Errorf("error = %v, want the upgrade message rather than a JSON type error", err)
	}
}

func TestParseIndex_acceptsTheCurrentSchemaAndIgnoresUnknownFields(t *testing.T) {
	data := []byte(`{
	  "schema_version": 1,
	  "generated_at": "2026-08-01T00:00:00Z",
	  "entries": [{
	    "slug": "square-pos",
	    "name": "Square POS",
	    "domains": ["squareup.com"],
	    "categories": ["payments"],
	    "stats": {"views": 12, "components": 48},
	    "last_verified": "2026-07-14",
	    "stars": 41
	  }]
	}`)
	idx, err := ParseIndex(data)
	if err != nil {
		t.Fatalf("ParseIndex: %v", err)
	}
	if len(idx.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(idx.Entries))
	}
	e := idx.Entries[0]
	if e.Slug != "square-pos" || e.Stats.Views != 12 || e.Stats.Components != 48 || e.LastVerified != "2026-07-14" {
		t.Errorf("entry = %+v", e)
	}
	if len(e.Domains) != 1 || e.Domains[0] != "squareup.com" {
		t.Errorf("domains = %v", e.Domains)
	}
}

// An index older than this CLI is fine — that is the whole point of the gate
// being one-sided.
func TestParseIndex_acceptsAnOlderSchema(t *testing.T) {
	if _, err := ParseIndex([]byte(`{"schema_version": 0, "entries": []}`)); err != nil {
		t.Fatalf("ParseIndex(v0): %v", err)
	}
}

// Index.Validate is the publisher CI's half of the contract: it reports every
// problem rather than the first, so one PR fixes them all.
func TestIndexValidate_reportsEveryProblem(t *testing.T) {
	ix := &Index{
		SchemaVersion: 1,
		Entries: []Entry{
			{Slug: "ok"},
			{Slug: "ok"},
			{Slug: "../escape"},
			{Slug: "fine", Name: "bell\x07"},
		},
	}
	errs := ix.Validate()
	if len(errs) != 3 {
		t.Fatalf("Validate() = %v, want 3 problems", errs)
	}
	joined := strings.Join([]string{errs[0].Error(), errs[1].Error(), errs[2].Error()}, " | ")
	mustContain(t, joined, "duplicate slug", "unsafe slug", "unsafe name")
}
