package sightmap

import (
	"reflect"
	"testing"
)

func excStack() []Frame {
	return []Frame{
		{Function: "syncCart", File: "src/cart/sync.ts", Line: 42, Column: 9},
		{Function: "onClick", File: "src/checkout/pay.ts", Line: 118, Column: 4},
	}
}

func TestMessageExtract_TopFrameAttributes(t *testing.T) {
	d := &MessageDef{
		Name: "UncaughtCheckoutError", Level: "exception",
		Properties: []MessagePropertyDef{
			{Name: "origin_file", Source: "stack", Field: "top.file"},
			{Name: "origin_fn", Source: "stack", Field: "top.function"},
			{Name: "origin_line", Source: "stack", Field: "top.line"},
		},
	}
	rec := Message{Level: "exception", Text: "boom", Stack: excStack()}

	got := d.ExtractProperties(rec)
	want := []PropertyValue{
		{Name: "origin_file", Value: "src/cart/sync.ts"},
		{Name: "origin_fn", Value: "syncCart"},
		{Name: "origin_line", Value: "42"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestMessageExtract_NumericFrameIndex(t *testing.T) {
	d := &MessageDef{
		Properties: []MessagePropertyDef{
			{Name: "caller_file", Source: "stack", Field: "1.file"},
		},
	}
	got := d.ExtractProperties(Message{Stack: excStack()})
	want := []PropertyValue{{Name: "caller_file", Value: "src/checkout/pay.ts"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestMessageExtract_Pattern(t *testing.T) {
	// Extract just the basename via a pattern's capture group.
	d := &MessageDef{
		Properties: []MessagePropertyDef{
			{Name: "file_base", Source: "stack", Field: "top.file", Pattern: `([^/]+)$`},
		},
	}
	got := d.ExtractProperties(Message{Stack: excStack()})
	want := []PropertyValue{{Name: "file_base", Value: "sync.ts"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestMessageExtract_SilentOmission(t *testing.T) {
	d := &MessageDef{
		Properties: []MessagePropertyDef{
			{Name: "oob_frame", Source: "stack", Field: "9.file"},      // index out of range
			{Name: "bad_attr", Source: "stack", Field: "top.garbage"},  // unknown attribute
			{Name: "malformed", Source: "stack", Field: "topfile"},     // no "." separator
			{Name: "empty_fn", Source: "stack", Field: "top.function"}, // frame has no function name
			{Name: "no_match", Source: "stack", Field: "top.file", Pattern: `zzz`},
		},
	}
	// A single frame with only a file — no function, so empty_fn omits; every
	// other property is out-of-range / malformed / non-matching.
	rec := Message{Stack: []Frame{{File: "a.ts"}}}
	if got := d.ExtractProperties(rec); got != nil {
		t.Fatalf("want nil (all omitted), got %+v", got)
	}
}

func TestMessageExtract_PlainConsoleRecordNoStack(t *testing.T) {
	d := &MessageDef{Properties: []MessagePropertyDef{{Name: "x", Source: "stack", Field: "top.file"}}}
	if got := d.ExtractProperties(Message{Level: "error", Text: "plain log"}); got != nil {
		t.Fatalf("plain console record should extract nothing, got %+v", got)
	}
}

func TestMessagesForRecord_FoldsStackProperties(t *testing.T) {
	c := &Corpus{Messages: []MessageDef{
		{
			Name: "UncaughtCheckoutError", Level: "exception",
			Message:     `Cannot read propert`,
			Description: "null deref in checkout",
			Properties:  []MessagePropertyDef{{Name: "origin_file", Source: "stack", Field: "top.file"}},
		},
	}}
	c.Messages[0].precompile()

	rec := Message{Level: "exception", Text: "Cannot read property 'x' of undefined", Stack: excStack()}
	got := c.MessagesForRecord(rec)
	if len(got) != 1 {
		t.Fatalf("want 1 match, got %d", len(got))
	}
	if got[0].Name != "UncaughtCheckoutError" || got[0].Description != "null deref in checkout" {
		t.Errorf("projection wrong: %+v", got[0])
	}
	if !reflect.DeepEqual(got[0].Properties, []PropertyValue{{Name: "origin_file", Value: "src/cart/sync.ts"}}) {
		t.Errorf("stack properties = %+v", got[0].Properties)
	}
}

func TestMessageProperties_Validation(t *testing.T) {
	c := &Corpus{Messages: []MessageDef{
		{Name: "M", Level: "exception", Properties: []MessagePropertyDef{
			{Name: "Bad-Name", Source: "stack", Field: "top.file"},    // invalid name
			{Name: "wrong_src", Source: "console", Field: "top.file"}, // bad source
			{Name: "no_field", Source: "stack"},                       // stack requires field
			{Name: "bad_re", Source: "stack", Field: "top.file", Pattern: `(unclosed`},
		}},
	}}
	diags := Validate(c)

	want := map[string]bool{
		"message-property-invalid-name":    false,
		"message-property-source-invalid":  false,
		"message-property-no-field":        false,
		"message-property-pattern-invalid": false,
	}
	for _, d := range diags {
		if _, ok := want[d.Code]; ok {
			want[d.Code] = true
		}
	}
	for code, seen := range want {
		if !seen {
			t.Errorf("expected diagnostic %q, not emitted; got %+v", code, diags)
		}
	}
}
