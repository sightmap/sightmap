package webmcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStringifyJSONPreservesInsertionOrder(t *testing.T) {
	om := NewOM().Set("z", 1).Set("a", 2)
	got := StringifyJSON(om)
	want := "{\n  \"z\": 1,\n  \"a\": 2\n}"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStringifyJSONRendersIntegralFloatsLikeJS(t *testing.T) {
	got := StringifyJSON(1.0)
	if got != "1" {
		t.Fatalf("got %q, want 1", got)
	}
}

func TestStringifyJSONNilAndEmpty(t *testing.T) {
	if got := StringifyJSON(nil); got != "null" {
		t.Fatalf("nil: got %q", got)
	}
	if got := StringifyJSON(NewOM()); got != "{}" {
		t.Fatalf("empty object: got %q", got)
	}
	if got := StringifyJSON([]any{}); got != "[]" {
		t.Fatalf("empty array: got %q", got)
	}
}

func TestStringifyJSONLeavesU2028AndHTMLLiteral(t *testing.T) {
	s := "a\u2028b<script>"
	got := StringifyJSON(s)
	if strings.Contains(got, `\u2028`) {
		t.Fatalf("U+2028 was escaped: %q", got)
	}
	if strings.Contains(got, `\u003c`) {
		t.Fatalf("< was HTML-escaped: %q", got)
	}
	enc, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(enc), `\u003c`) {
		t.Fatal("sanity: encoding/json should HTML-escape <; test is wrong if it does not")
	}
}
