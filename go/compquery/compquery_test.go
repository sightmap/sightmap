package compquery

import (
	"strings"
	"testing"

	"github.com/sightmap/sightmap/go/sightmap"
)

// ── Parser ────────────────────────────────────────────────────────────────────

func TestParseQuery(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want *Query
	}{
		{
			name: "bare name",
			in:   "ProductCard",
			want: &Query{Parts: []Part{{Name: "ProductCard"}}, Index: -1},
		},
		{
			name: "name with exact predicate",
			in:   "FulfillmentTileButton[label=deliveryTile]",
			want: &Query{Parts: []Part{{Name: "FulfillmentTileButton", Preds: []Predicate{{Prop: "label", Op: "=", Val: "deliveryTile"}}}}, Index: -1},
		},
		{
			name: "prefix op + quoted value with space",
			in:   `ProductCard[name^="Webber grill"]`,
			want: &Query{Parts: []Part{{Name: "ProductCard", Preds: []Predicate{{Prop: "name", Op: "^=", Val: "Webber grill"}}}}, Index: -1},
		},
		{
			name: "substring op + case-insensitive flag",
			in:   `ProductCard[name*="weber" i]`,
			want: &Query{Parts: []Part{{Name: "ProductCard", Preds: []Predicate{{Prop: "name", Op: "*=", Val: "weber", CI: true}}}}, Index: -1},
		},
		{
			name: "descendant chain",
			in:   "LoginForm UserNameInput",
			want: &Query{Parts: []Part{{Name: "LoginForm"}, {Name: "UserNameInput"}}, Index: -1},
		},
		{
			name: "occurrence index, no space",
			in:   "FulfillmentTileButton#1",
			want: &Query{Parts: []Part{{Name: "FulfillmentTileButton"}}, Index: 1},
		},
		{
			name: "occurrence index after predicate",
			in:   "Tile[k=v]#2",
			want: &Query{Parts: []Part{{Name: "Tile", Preds: []Predicate{{Prop: "k", Op: "=", Val: "v"}}}}, Index: 2},
		},
		{
			name: "multiple predicates on one component",
			in:   "Card[sku=12345][brand^=Web]",
			want: &Query{Parts: []Part{{Name: "Card", Preds: []Predicate{{Prop: "sku", Op: "=", Val: "12345"}, {Prop: "brand", Op: "^=", Val: "Web"}}}}, Index: -1},
		},
		{
			name: "chain with predicate on ancestor and index",
			in:   `MainPanel LoginForm[state=ready] UserNameInput#0`,
			want: &Query{Parts: []Part{{Name: "MainPanel"}, {Name: "LoginForm", Preds: []Predicate{{Prop: "state", Op: "=", Val: "ready"}}}, {Name: "UserNameInput"}}, Index: 0},
		},
		{
			name: "hyphenated property name",
			in:   "Tile[data-method=delivery]",
			want: &Query{Parts: []Part{{Name: "Tile", Preds: []Predicate{{Prop: "data-method", Op: "=", Val: "delivery"}}}}, Index: -1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseQuery(tt.in)
			if err != nil {
				t.Fatalf("ParseQuery(%q) error: %v", tt.in, err)
			}
			if !queriesEqual(got, tt.want) {
				t.Errorf("ParseQuery(%q)\n  got  %#v\n  want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseQuery_QuotedEscapes(t *testing.T) {
	q, err := ParseQuery(`Card[name="say \"hi\""]`)
	if err != nil {
		t.Fatal(err)
	}
	got := q.Parts[0].Preds[0].Val
	if want := `say "hi"`; got != want {
		t.Errorf("escaped quoted value = %q, want %q", got, want)
	}
}

func TestParseQuery_Errors(t *testing.T) {
	bad := []string{
		"",                        // empty
		"   ",                     // whitespace only
		"#1",                      // index without component
		"Card[]",                  // empty predicate
		"Card[name]",              // missing operator
		"Card[name=]",             // missing value
		`Card[name="unterminated`, // unterminated quote
		"Card[name~=x]",           // unsupported operator
		"Card#",                   // '#' without digits
		"Card#1 Extra",            // trailing content after index
	}
	for _, in := range bad {
		t.Run(in, func(t *testing.T) {
			if _, err := ParseQuery(in); err == nil {
				t.Errorf("ParseQuery(%q) expected error, got nil", in)
			}
		})
	}
}

// ── Predicate matching ──────────────────────────────────────────────────────

func TestPredicateMatch(t *testing.T) {
	props := map[string]string{"label": "Delivery", "sku": "12345"}
	tests := []struct {
		pred Predicate
		want bool
	}{
		{Predicate{Prop: "label", Op: "=", Val: "Delivery"}, true},
		{Predicate{Prop: "label", Op: "=", Val: "delivery"}, false},
		{Predicate{Prop: "label", Op: "=", Val: "delivery", CI: true}, true},
		{Predicate{Prop: "label", Op: "^=", Val: "Del"}, true},
		{Predicate{Prop: "label", Op: "^=", Val: "del"}, false},
		{Predicate{Prop: "label", Op: "^=", Val: "del", CI: true}, true},
		{Predicate{Prop: "label", Op: "*=", Val: "live"}, true},
		{Predicate{Prop: "sku", Op: "=", Val: "12345"}, true},
		{Predicate{Prop: "missing", Op: "=", Val: ""}, false},
	}
	for _, tt := range tests {
		if got := tt.pred.Match(props); got != tt.want {
			t.Errorf("%+v.Match(%v) = %v, want %v", tt.pred, props, got, tt.want)
		}
	}
}

// ── Resolution against a tree ────────────────────────────────────────────────

// buildFixture returns a small matched tree plus matches/props maps:
//
//	root
//	├─ MainPanel (m1)
//	│   └─ LoginForm (m2)
//	│       ├─ UserNameInput (u1)
//	│       └─ PasswordInput (p1)
//	└─ TileGroup
//	    ├─ FulfillmentTileButton (t1) label=Pickup
//	    └─ FulfillmentTileButton (t2) label=Delivery
func buildFixture() (*sightmap.ComponentNode, map[*sightmap.ComponentNode]*sightmap.ComponentMatch, map[string]map[string]string) {
	u1 := &sightmap.ComponentNode{Id: "u1"}
	p1 := &sightmap.ComponentNode{Id: "p1"}
	loginForm := &sightmap.ComponentNode{Id: "m2", Children: []*sightmap.ComponentNode{u1, p1}}
	mainPanel := &sightmap.ComponentNode{Id: "m1", Children: []*sightmap.ComponentNode{loginForm}}

	t1 := &sightmap.ComponentNode{Id: "t1"}
	t2 := &sightmap.ComponentNode{Id: "t2"}
	tileGroup := &sightmap.ComponentNode{Id: "g1", Children: []*sightmap.ComponentNode{t1, t2}}

	root := &sightmap.ComponentNode{Id: "root", Children: []*sightmap.ComponentNode{mainPanel, tileGroup}}

	matches := map[*sightmap.ComponentNode]*sightmap.ComponentMatch{
		mainPanel: {Name: "MainPanel"},
		loginForm: {Name: "LoginForm"},
		u1:        {Name: "UserNameInput"},
		p1:        {Name: "PasswordInput"},
		t1:        {Name: "FulfillmentTileButton"},
		t2:        {Name: "FulfillmentTileButton"},
	}
	props := map[string]map[string]string{
		"t1": {"label": "Pickup"},
		"t2": {"label": "Delivery"},
	}
	return root, matches, props
}

func mustResolve(t *testing.T, queryStr string) *sightmap.ComponentNode {
	t.Helper()
	root, matches, props := buildFixture()
	q, err := ParseQuery(queryStr)
	if err != nil {
		t.Fatalf("parse %q: %v", queryStr, err)
	}
	node, err := Resolve(root, matches, props, q)
	if err != nil {
		t.Fatalf("resolve %q: %v", queryStr, err)
	}
	return node
}

func TestResolve_PropertyDisambiguation(t *testing.T) {
	if got := mustResolve(t, "FulfillmentTileButton[label=Delivery]"); got.Id != "t2" {
		t.Errorf("got id %q, want t2", got.Id)
	}
	if got := mustResolve(t, "FulfillmentTileButton[label=Pickup]"); got.Id != "t1" {
		t.Errorf("got id %q, want t1", got.Id)
	}
}

func TestResolve_CaseInsensitivePrefix(t *testing.T) {
	if got := mustResolve(t, `FulfillmentTileButton[label^="deliv" i]`); got.Id != "t2" {
		t.Errorf("got id %q, want t2", got.Id)
	}
}

func TestResolve_DescendantChain(t *testing.T) {
	if got := mustResolve(t, "LoginForm UserNameInput"); got.Id != "u1" {
		t.Errorf("got id %q, want u1", got.Id)
	}
	// Deep (non-direct) ancestor still satisfies descendant semantics.
	if got := mustResolve(t, "MainPanel UserNameInput"); got.Id != "u1" {
		t.Errorf("got id %q, want u1", got.Id)
	}
	if got := mustResolve(t, "MainPanel LoginForm PasswordInput"); got.Id != "p1" {
		t.Errorf("got id %q, want p1", got.Id)
	}
}

func TestResolve_OccurrenceIndex(t *testing.T) {
	if got := mustResolve(t, "FulfillmentTileButton#0"); got.Id != "t1" {
		t.Errorf("#0 got id %q, want t1", got.Id)
	}
	if got := mustResolve(t, "FulfillmentTileButton#1"); got.Id != "t2" {
		t.Errorf("#1 got id %q, want t2", got.Id)
	}
}

func TestResolve_AmbiguityError(t *testing.T) {
	root, matches, props := buildFixture()
	q, _ := ParseQuery("FulfillmentTileButton")
	_, err := Resolve(root, matches, props, q)
	if err == nil {
		t.Fatal("expected ambiguity error, got nil")
	}
	msg := err.Error()
	// Candidate list must surface both ids and their distinguishing props.
	for _, want := range []string{"ambiguous", "t1", "t2", "Pickup", "Delivery"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ambiguity error missing %q; got:\n%s", want, msg)
		}
	}
}

// TestPresent is the wait-for --component predicate: presence, not single-node
// resolution. A query matching several nodes must count as PRESENT (the whole
// point of waiting for a component is that it has appeared), even though the
// very same query is an ambiguity ERROR for Resolve, which the interaction
// commands use because they need one target. This is the regression guard for
// issue #387, where multi-match was mistaken for "not matched yet" and polled to
// a timeout.
func TestPresent(t *testing.T) {
	root, matches, props := buildFixture()

	// Multi-match: Resolve errors (ambiguous), but Present is true.
	multi, _ := ParseQuery("FulfillmentTileButton")
	if _, err := Resolve(root, matches, props, multi); err == nil {
		t.Fatal("precondition: expected FulfillmentTileButton to be ambiguous for Resolve")
	}
	if !Present(root, matches, props, multi) {
		t.Error("Present(FulfillmentTileButton) = false; multiple matches must count as present")
	}

	// Single match and predicate-narrowed queries are present too.
	for _, qs := range []string{
		"UserNameInput",
		`FulfillmentTileButton[label=Delivery]`,
		"LoginForm UserNameInput",
	} {
		q, _ := ParseQuery(qs)
		if !Present(root, matches, props, q) {
			t.Errorf("Present(%q) = false, want true", qs)
		}
	}

	// Absent queries are not present (so wait-for keeps polling / times out).
	for _, qs := range []string{
		"NoSuchComponent",
		`FulfillmentTileButton[label=Nope]`,
	} {
		q, _ := ParseQuery(qs)
		if Present(root, matches, props, q) {
			t.Errorf("Present(%q) = true, want false", qs)
		}
	}

	// An occurrence index is present only when that occurrence exists (mirrors
	// Resolve's bounds check, without erroring on an as-yet-unreached index).
	in0, _ := ParseQuery("FulfillmentTileButton#0")
	if !Present(root, matches, props, in0) {
		t.Error("Present(FulfillmentTileButton#0) = false, want true")
	}
	oob, _ := ParseQuery("FulfillmentTileButton#5")
	if Present(root, matches, props, oob) {
		t.Error("Present(FulfillmentTileButton#5) = true, want false (occurrence out of range)")
	}
}

func TestResolve_NoMatch(t *testing.T) {
	root, matches, props := buildFixture()
	for _, qs := range []string{
		"NoSuchComponent",
		"FulfillmentTileButton[label=Nope]",
		"PasswordInput LoginForm", // wrong chain order (target not under ancestor)
	} {
		q, _ := ParseQuery(qs)
		if _, err := Resolve(root, matches, props, q); err == nil {
			t.Errorf("Resolve(%q) expected no-match error, got nil", qs)
		}
	}
}

func TestResolve_IndexOutOfRange(t *testing.T) {
	root, matches, props := buildFixture()
	q, _ := ParseQuery("FulfillmentTileButton#5")
	if _, err := Resolve(root, matches, props, q); err == nil {
		t.Error("expected out-of-range error, got nil")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func queriesEqual(a, b *Query) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Index != b.Index || len(a.Parts) != len(b.Parts) {
		return false
	}
	for i := range a.Parts {
		pa, pb := a.Parts[i], b.Parts[i]
		if pa.Name != pb.Name || len(pa.Preds) != len(pb.Preds) {
			return false
		}
		for j := range pa.Preds {
			if pa.Preds[j] != pb.Preds[j] {
				return false
			}
		}
	}
	return true
}
