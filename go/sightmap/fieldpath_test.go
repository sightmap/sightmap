package sightmap

import (
	"reflect"
	"testing"
)

func TestSplitFieldPath(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		want    []string
		wantErr bool
	}{
		{name: "empty", field: "", want: nil},
		{name: "single segment", field: "status", want: []string{"status"}},
		{name: "simple dotted path", field: "a.b.c", want: []string{"a", "b", "c"}},
		{name: "array index segment", field: "items.0.name", want: []string{"items", "0", "name"}},
		{name: "escaped literal dot", field: `a\.b.c`, want: []string{"a.b", "c"}},
		{name: "escaped literal backslash", field: `a\\.b`, want: []string{`a\`, "b"}},
		{name: "escaped dot at start of segment", field: `\.a.b`, want: []string{".a", "b"}},
		{name: "escaped dot at end of field", field: `a.b\.`, want: []string{"a", "b."}},
		{name: "malformed: empty segment", field: "a..b", want: []string{"a", "", "b"}},
		{name: "malformed: leading dot", field: ".a.b", want: []string{"", "a", "b"}},
		{name: "invalid escape", field: `a\xb`, wantErr: true},
		{name: "trailing unterminated escape", field: `a\`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SplitFieldPath(tt.field)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("SplitFieldPath(%q) = %v, nil; want an error", tt.field, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("SplitFieldPath(%q) unexpected error: %v", tt.field, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SplitFieldPath(%q) = %#v, want %#v", tt.field, got, tt.want)
			}
		})
	}
}

// walkJSONPath must resolve an escaped-dot key correctly: a JSON object whose
// key itself contains a literal "." can only be addressed via the escape.
func TestWalkJSONPath_EscapedDotKey(t *testing.T) {
	got, ok := walkJSONPath(`{"a.b":{"c":"value"}}`, `a\.b.c`)
	if !ok {
		t.Fatal("walkJSONPath: want ok=true")
	}
	if got != "value" {
		t.Errorf("walkJSONPath = %q, want %q", got, "value")
	}
}

// Before the escape existed, "a.b.c" against an object keyed "a.b" (nested
// "c") was inherently unaddressable — plain splitting always read it as three
// segments. Confirm the unescaped form still does NOT resolve the same
// document (i.e. this is genuinely a new capability, not a silent behavior
// change for existing unescaped paths).
func TestWalkJSONPath_UnescapedDotStillSplitsNormally(t *testing.T) {
	_, ok := walkJSONPath(`{"a.b":{"c":"value"}}`, `a.b.c`)
	if ok {
		t.Fatal("walkJSONPath: want ok=false — unescaped \"a.b.c\" must not resolve a key literally named \"a.b\"")
	}
}

func TestWalkJSONPath_InvalidEscapeFailsResolution(t *testing.T) {
	_, ok := walkJSONPath(`{"a":"value"}`, `a\xb`)
	if ok {
		t.Fatal("walkJSONPath: want ok=false for an invalid escape sequence")
	}
}
