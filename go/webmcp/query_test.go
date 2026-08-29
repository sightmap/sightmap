package webmcp

import (
	"strings"
	"testing"
)

func TestParseQueryBareName(t *testing.T) {
	q, err := parseQuery("ProductCard")
	if err != nil {
		t.Fatal(err)
	}
	if q.Kind != "query" || len(q.Links) != 1 || q.Links[0].Name != "ProductCard" {
		t.Fatalf("got %+v", q)
	}
	if len(q.Links[0].Preds) != 0 || q.Links[0].Index != nil {
		t.Fatalf("got %+v", q.Links[0])
	}
}

func TestParseQueryPredicates(t *testing.T) {
	q, err := parseQuery(`Card[title="Chairs"][price^=$1][text*="weber" i]`)
	if err != nil {
		t.Fatal(err)
	}
	preds := q.Links[0].Preds
	if len(preds) != 3 {
		t.Fatalf("preds = %+v", preds)
	}
	if preds[0] != (queryPred{Prop: "title", Op: "=", Value: "Chairs", CI: false}) {
		t.Fatalf("pred0 = %+v", preds[0])
	}
	if preds[1] != (queryPred{Prop: "price", Op: "^=", Value: "$1", CI: false}) {
		t.Fatalf("pred1 = %+v", preds[1])
	}
	if preds[2] != (queryPred{Prop: "text", Op: "*=", Value: "weber", CI: true}) {
		t.Fatalf("pred2 = %+v", preds[2])
	}
}

func TestParseQueryBareValueIFlag(t *testing.T) {
	q, err := parseQuery("Card[title*=weber i]")
	if err != nil {
		t.Fatal(err)
	}
	got := q.Links[0].Preds[0]
	want := queryPred{Prop: "title", Op: "*=", Value: "weber", CI: true}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestParseQueryDescendantChain(t *testing.T) {
	q, err := parseQuery(`Row[label="Desk"] Buy`)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Links) != 2 || q.Links[0].Name != "Row" || q.Links[1].Name != "Buy" {
		t.Fatalf("links = %+v", q.Links)
	}
	if len(q.Links[0].Preds) != 1 {
		t.Fatalf("preds = %+v", q.Links[0].Preds)
	}
}

func TestParseQueryOccurrenceIndex(t *testing.T) {
	q, err := parseQuery("Tile#1")
	if err != nil {
		t.Fatal(err)
	}
	if q.Links[0].Index == nil || *q.Links[0].Index != 1 {
		t.Fatalf("index = %v", q.Links[0].Index)
	}
}

func TestParseQueryPredicateKeepsSpacesAndTemplates(t *testing.T) {
	q, err := parseQuery(`Row[label="a b {x}"]`)
	if err != nil {
		t.Fatal(err)
	}
	if q.Links[0].Preds[0].Value != "a b {x}" {
		t.Fatalf("value = %q", q.Links[0].Preds[0].Value)
	}
}

func TestParseQueryCSSEscape(t *testing.T) {
	q, err := parseQuery("css:.plp-price-module a")
	if err != nil {
		t.Fatal(err)
	}
	if q.Kind != "css" || q.Selector != ".plp-price-module a" {
		t.Fatalf("got %+v", q)
	}
}

func TestParseQueryRejectsChildCombinator(t *testing.T) {
	_, err := parseQuery("A > B")
	if err == nil || !strings.Contains(err.Error(), "descendant") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseQueryUnterminatedPredicate(t *testing.T) {
	_, err := parseQuery("Card[title=")
	if err == nil || !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("err = %v", err)
	}
}
