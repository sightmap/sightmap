package main

import (
	"reflect"
	"strings"
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
		// Three or more stars need backtracking: the first "aa" is not the one to take.
		{"http://x/abaaab", "/*aa*aa*b", false},
		{"http://x/abaaaab", "/*aa*aa*b", true},
		{"http://x/team/WEB/cycles/14/x", "/team/*/cycles/*/*", true},
		{"http://x/team/WEB/cycles", "/team/*/cycles/*/*", false},
		{"http://x/inbox", "/inbox*", true},
		{"http://x/", "/*", true},
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
		// The first operator wins; a later "!=" belongs to the value.
		{"text=a!=b", false},
		{"text!=a=b", true},
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

func TestListCondition(t *testing.T) {
	row := func(id string) componentState {
		return componentState{Component: "InboxRow", Props: map[string]string{"identifier": id}}
	}
	project := componentState{Component: "InboxRow", Props: map[string]string{"project": "plant-id-v2"}}
	st := &pageState{Nodes: []componentState{row("WEB-144"), row("WEB-130"), project, row("WEB-160")}}
	cases := []struct {
		kind listKind
		spec string
		ok   bool
	}{
		{listSeq, "identifier=WEB-144,WEB-130,WEB-160", true},
		{listSeq, "identifier=WEB-130,WEB-144,WEB-160", false},
		{listSeq, "identifier=WEB-144,WEB-130", false},
		{listSet, "identifier=WEB-160,WEB-144,WEB-130", true},
		{listSet, "identifier=WEB-160,WEB-144", false},
		{listContains, "identifier=WEB-160,WEB-144", true},
		{listContains, "identifier=WEB-160,WEB-999", false},
		{listContains, "identifier=WEB-144,WEB-144", false}, // a repeat must be there twice
		{listOrder, "identifier=WEB-144,WEB-160", true},
		{listOrder, "identifier=WEB-160,WEB-144", false},
		{listSeq, "title=", true}, // no match has a title: the empty list
		{listSeq, "identifier|project=WEB-144,WEB-130,plant-id-v2,WEB-160", true},
		{listSeq, "identifier|project=WEB-144,WEB-130,WEB-160", false},
	}
	for _, c := range cases {
		cond, err := listCondition("InboxRow", c.kind, c.spec)
		if err != nil {
			t.Fatalf("--%s %s: %v", c.kind, c.spec, err)
		}
		if got := cond.ok(st); got != c.ok {
			t.Errorf("--%s %s: ok = %v, want %v (observed %s)", c.kind, c.spec, got, c.ok, cond.observed(st))
		}
	}
	cond, _ := listCondition("InboxRow", listContains, "identifier=WEB-160,WEB-999")
	if got := cond.observed(st); !strings.Contains(got, "missing WEB-999") {
		t.Errorf("observed = %q, want it to name the missing value", got)
	}
	if _, err := listCondition("InboxRow", listContains, "identifier="); err == nil {
		t.Error("--contains with no values: want an error")
	}
	if got := splitValues(`a\,b,c`); !reflect.DeepEqual(got, []string{"a,b", "c"}) {
		t.Errorf("splitValues = %q", got)
	}
}

func TestQueryCondition(t *testing.T) {
	st := &pageState{URL: "http://localhost:5173/my-issues/assigned?peek=WEB-120&latency=0"}
	cases := []struct {
		spec string
		ok   bool
	}{
		{"peek=WEB-120", true},
		{"peek=WEB-130", false},
		{"peek!=WEB-130", true},
		{"peek!=WEB-120", false},
		{"tab=", true},   // absent
		{"peek=", false}, // present
		{"peek!=", true}, // present
		{"tab!=", false},
	}
	for _, c := range cases {
		cond, err := queryCondition(c.spec)
		if err != nil {
			t.Fatalf("%s: %v", c.spec, err)
		}
		if got := cond.ok(st); got != c.ok {
			t.Errorf("%s: ok = %v, want %v (observed %s)", c.spec, got, c.ok, cond.observed(st))
		}
	}
	if _, err := queryCondition("peek"); err == nil {
		t.Error("--query peek: want an error")
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

func TestKeySteps(t *testing.T) {
	k := func(s string) keyStep { return keyStep{text: s} }
	ty := func(s string) keyStep { return keyStep{text: s, typed: true} }
	cases := []struct {
		args, keys []string
		want       []keyStep
	}{
		{[]string{"g", "m"}, []string{"g", "m"}, []keyStep{k("g"), k("m")}},
		{[]string{"Meta+k", "--type", "prio", "Enter"}, []string{"Meta+k", "Enter"}, []keyStep{k("Meta+k"), ty("prio"), k("Enter")}},
		// Plumb i2: two --type flags in one call; the second must not swallow the first.
		{[]string{"f", "--type", "Labels", "Enter", "--type", "Bug", "Enter"}, []string{"f", "Enter", "Enter"},
			[]keyStep{k("f"), ty("Labels"), k("Enter"), ty("Bug"), k("Enter")}},
		{[]string{"--type=hi", "Meta+Enter"}, []string{"Meta+Enter"}, []keyStep{ty("hi"), k("Meta+Enter")}},
		{[]string{"--focus", "Composer", "--type", "x", "--settle", "Enter"}, []string{"Enter"}, []keyStep{ty("x"), k("Enter")}},
		{[]string{"--delay-ms", "80", "j", "--sightmap-dir", "d", "s"}, []string{"j", "s"}, []keyStep{k("j"), k("s")}},
	}
	for _, c := range cases {
		if got := keySteps(c.args, c.keys); !reflect.DeepEqual(got, c.want) {
			t.Errorf("keySteps(%q) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestEvalHint(t *testing.T) {
	cases := map[string]string{
		`document.querySelector("[data-component=IssueRow]").dataset.group`:            "browser get",
		`[...document.querySelectorAll("[data-component=Toast]")].map(e=>e.innerText)`: "browser get",
		`new Promise(r => setTimeout(r, 500))`:                                         "browser expect",
		`document.querySelector("[data-action=apply]").click()`:                        "browser click",
		`location.pathname`:                   "",
		`localStorage.getItem("plumb.draft")`: "",
	}
	for script, want := range cases {
		got := evalHint(script)
		if want == "" && got != "" || want != "" && !strings.Contains(got, want) {
			t.Errorf("evalHint(%q) = %q, want it to mention %q", script, got, want)
		}
	}
}

func TestTakeSettle(t *testing.T) {
	s, rest := takeSettle([]string{"http://x/", "--settle", "--sightmap-dir", "d"})
	if !s || !reflect.DeepEqual(rest, []string{"http://x/", "--sightmap-dir", "d"}) {
		t.Errorf("takeSettle = %v, %q", s, rest)
	}
	if s, _ := takeSettle([]string{"http://x/"}); s {
		t.Error("takeSettle found a --settle that wasn't there")
	}
	// The flag package's other forms of a bool flag, before or after the URL.
	for _, c := range []struct {
		args []string
		want bool
	}{
		{[]string{"http://x/", "--settle=true"}, true},
		{[]string{"--settle=true", "http://x/"}, true},
		{[]string{"-settle=1", "http://x/"}, true},
		{[]string{"http://x/", "--settle=false"}, false},
		{[]string{"--settle", "http://x/", "--settle=false"}, false},
	} {
		s, rest := takeSettle(c.args)
		if s != c.want || !reflect.DeepEqual(rest, []string{"http://x/"}) {
			t.Errorf("takeSettle(%q) = %v, %q; want %v, [http://x/]", c.args, s, rest, c.want)
		}
	}
	// Not the flag: a URL or value that merely contains the word.
	if s, rest := takeSettle([]string{"http://x/settle", "--settled"}); s || len(rest) != 2 {
		t.Errorf("takeSettle matched a non-flag: %v %q", s, rest)
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
