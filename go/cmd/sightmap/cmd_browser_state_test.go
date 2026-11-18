package main

import (
	"reflect"
	"testing"
)

func TestPathMatches(t *testing.T) {
	cases := []struct {
		url, want string
		ok        bool
	}{
		{"http://localhost:5173/inbox?latency=0", "/inbox", true},
		{"http://localhost:5173/inbox/WEB-144", "/inbox", false},
		{"http://localhost:5173/team/WEB/triage/WEB-153#x", "/team/WEB/triage/*", true},
		{"http://localhost:5173/team/WEB/triage", "/team/*/triage/*", false},
		{"http://localhost:5173", "/", true},
		{"http://localhost:5173/view/v1", "/view/*", true},
		{"http://localhost:5173/a/b/c", "/a/*/c", true},
	}
	for _, c := range cases {
		if got := pathMatches(c.url, c.want); got != c.ok {
			t.Errorf("pathMatches(%q, %q) = %v, want %v", c.url, c.want, got, c.ok)
		}
	}
}

func TestPropCondition(t *testing.T) {
	st := &pageState{Nodes: []componentState{
		{Component: "IssueRow", Props: map[string]string{"group": "status:done", "identifier": "WEB-120"}, Text: "Keyboard navigation"},
	}}
	cases := []struct {
		spec string
		ok   bool
	}{
		{"group=status:done", true},
		{"group=status:todo", false},
		{"group^=status:", true},
		{"group*=done", true},
		{"group!=status:todo", true},
		{"group!=status:done", false},
		{"missing=x", false},
		{"text*=Keyboard", true},
		{"visible=false", true},
	}
	for _, c := range cases {
		cond, err := propCondition("IssueRow", c.spec)
		if err != nil {
			t.Fatalf("%s: %v", c.spec, err)
		}
		if got := cond.ok(st); got != c.ok {
			t.Errorf("%s: ok = %v, want %v (observed %s)", c.spec, got, c.ok, cond.observed(st))
		}
	}
	if _, err := propCondition("IssueRow", "novalue"); err == nil {
		t.Error("propCondition accepted a spec without an operator")
	}
	// No matches never satisfies a property condition.
	cond, _ := propCondition("IssueRow", "group=status:done")
	if cond.ok(&pageState{}) {
		t.Error("a property condition held with no matches")
	}
}

func TestCorpusPropertyShadowsBuiltin(t *testing.T) {
	n := componentState{Props: map[string]string{"focused": ""}, Focused: false}
	if v, _ := field(n, "focused"); v != "" {
		t.Errorf("field focused = %q, want the corpus value \"\"", v)
	}
	if v, _ := field(componentState{Focused: true}, "focused"); v != "true" {
		t.Errorf("built-in focused = %q, want true", v)
	}
	if _, ok := project(n, nil)["focused"]; !ok {
		t.Error("project dropped the corpus focused property")
	}
}

func TestCountCondition(t *testing.T) {
	st := &pageState{Nodes: make([]componentState, 3)}
	for spec, want := range map[string]bool{"3": true, "2": false, ">=2": true, ">=4": false, "<=3": true, "<=2": false} {
		c, err := countCondition("Row", spec)
		if err != nil {
			t.Fatalf("%s: %v", spec, err)
		}
		if got := c.ok(st); got != want {
			t.Errorf("count %s with 3 matches: %v, want %v", spec, got, want)
		}
	}
	if _, err := countCondition("Row", "many"); err == nil {
		t.Error("countCondition accepted a non-number")
	}
}

func TestSplitAroundType(t *testing.T) {
	cases := []struct {
		args, keys, before, after []string
	}{
		{[]string{"g", "m"}, []string{"g", "m"}, []string{"g", "m"}, nil},
		{[]string{"Meta+k", "--type", "prio", "Enter"}, []string{"Meta+k", "Enter"}, []string{"Meta+k"}, []string{"Enter"}},
		{[]string{"--type=hi", "Meta+Enter"}, []string{"Meta+Enter"}, nil, []string{"Meta+Enter"}},
		{[]string{"--focus", "Composer", "--type", "x"}, nil, nil, nil},
	}
	for _, c := range cases {
		b, a := splitAroundType(c.args, c.keys)
		if !reflect.DeepEqual(b, c.before) || !reflect.DeepEqual(a, c.after) {
			t.Errorf("splitAroundType(%q) = %q, %q; want %q, %q", c.args, b, a, c.before, c.after)
		}
	}
}

func TestProject(t *testing.T) {
	n := componentState{Component: "IssueRow", Props: map[string]string{"identifier": "WEB-1", "group": "g"}, Text: "t", Visible: true}
	if got := project(n, []string{"identifier", "visible", "nope"}); !reflect.DeepEqual(got, map[string]any{"identifier": "WEB-1", "visible": "true", "nope": nil}) {
		t.Errorf("project with fields = %v", got)
	}
	if got := project(n, nil); !reflect.DeepEqual(got, map[string]any{"component": "IssueRow", "identifier": "WEB-1", "group": "g", "text": "t"}) {
		t.Errorf("project without fields = %v", got)
	}
}
