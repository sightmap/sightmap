// Browser state commands: read what the corpus says about the live page, wait
// for it to say something, and send key sequences. They exist because agents
// driving an app with sightmap were writing `browser eval` scripts to read
// component attributes (57-69% of their evals in the Plumb bake-off, F18), and
// `sleep` + eval loops to check results. `get` answers from the corpus's
// extracted properties, `expect` is a wait and an assertion in one command, and
// `keys` sends a whole sequence where they sent one keypress per call.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/compquery"
	"github.com/sightmap/sightmap/go/sightmap"
)

// componentState is one matched component as `get` and `expect` report it: its
// corpus name, its extracted properties, and the text and focus a check most
// often needs. Properties are exactly the corpus's `properties:` for the
// component, so what an agent can read is what the corpus defines.
type componentState struct {
	Component string            `json:"component"`
	Props     map[string]string `json:"props,omitempty"`
	Text      string            `json:"text,omitempty"`
	Visible   bool              `json:"visible"`
	Focused   bool              `json:"focused,omitempty"`
	// Box is the viewport bounding box in CSS pixels, nil when the tree carried
	// none. It feeds the built-in x, y, width, height, right and bottom.
	Box *sightmap.Bounds `json:"-"`
}

// pageState is the live page as the state commands see it: the URL, the title,
// and every node matching a query, in document order.
type pageState struct {
	URL   string
	Title string
	Nodes []componentState
}

// readState extracts the live tree once, applies the corpus and returns every
// candidate for q (all of them; `#N` picks one). Focus comes from the page's
// active element, read in the same round trip as the URL and title. A nil q
// reads only the URL and title: no extraction, so `get page` and `expect --url`
// work on a page the corpus does not match and poll cheaply.
//
// The extraction runs first because it stamps the data-sightmap-id the focus
// lookup reads.
func readState(ctx context.Context, conn *browser.CDPConn, sightmapDir string, q *compquery.Query) (*pageState, error) {
	var (
		root    *sightmap.ComponentNode
		matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch
		props   map[string]map[string]string
	)
	if q != nil {
		var err error
		if root, matches, props, err = extractMatchedComponents(ctx, conn, sightmapDir); err != nil {
			return nil, err
		}
	}
	var meta struct {
		URL     string `json:"url"`
		Title   string `json:"title"`
		FocusID string `json:"focus"`
	}
	raw, err := browser.EvalJSON(ctx, conn, `(() => { let a = document.activeElement; while (a && a.shadowRoot && a.shadowRoot.activeElement) a = a.shadowRoot.activeElement; const host = a && a.closest ? a.closest('[data-sightmap-id]') : null; return { url: location.href, title: document.title, focus: host ? host.getAttribute('data-sightmap-id') : '' }; })()`)
	if err == nil {
		_ = json.Unmarshal(raw, &meta)
	}
	st := &pageState{URL: meta.URL, Title: meta.Title}
	if q == nil {
		return st, nil
	}
	cands := compquery.FindCandidates(root, matches, props, q)
	if q.Index >= 0 {
		if q.Index < len(cands) {
			cands = cands[q.Index : q.Index+1]
		} else {
			cands = nil
		}
	}
	for _, n := range cands {
		cs := componentState{Props: props[n.Id], Text: n.Text, Visible: n.IsVisible, Box: n.Bounds}
		if m := matches[n]; m != nil {
			cs.Component = m.Name
		}
		cs.Focused = meta.FocusID != "" && containsID(n, meta.FocusID)
		st.Nodes = append(st.Nodes, cs)
	}
	return st, nil
}

// containsID reports whether id is n or one of its descendants: focus inside a
// component (its input, say) counts as the component being focused.
func containsID(n *sightmap.ComponentNode, id string) bool {
	if n.Id == id {
		return true
	}
	for _, c := range n.Children {
		if containsID(c, id) {
			return true
		}
	}
	return false
}

// ── get ───────────────────────────────────────────────────────────────────────

func runGet(args []string) error {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	propsFlag := fs.String("props", "", "Comma-separated properties to print (default: all, with text cut to 80 characters; 'text', 'visible' and 'focused' work too)")
	countFlag := fs.Bool("count", false, "Print only how many components match")
	limitFlag := fs.Int("limit", 25, "Print at most N matches (0 = all); the rest are counted")
	asFlag := fs.String("as", "", "In a `run` script: keep the value as $NAME for later steps (needs --count, or one match and one --props field)")
	queries, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return err
	}
	if *asFlag != "" && !validVarName(*asFlag) {
		return fmt.Errorf("get: --as wants a variable name (letters, digits, _), got %q", *asFlag)
	}
	if *asFlag != "" && len(queries) != 1 {
		return errors.New("get: --as takes one query")
	}
	if len(queries) == 0 {
		return fmt.Errorf("usage: browser get QUERY... [--props a,b] [--count] [--limit N]\n" +
			"  prints each match's corpus properties (and text, visibility, focus) as JSON, one line per match\n" +
			"  QUERY: a component query (IssueRow, IssueRow[identifier=WEB-143], ViewBar Tab#1); 'page' prints url and title\n" +
			"  examples: get 'IssueRow[identifier=WEB-143]' --props status,priority   get IssueRow --props identifier,group\n" +
			"            get IssueRow --count   get page")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := browser.Connect(resolveCDPAddr(*addrFlag, *sightmapDirFlag), *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	fields := splitList(*propsFlag)
	for _, qs := range queries {
		if qs == "page" {
			st, err := readState(ctx, conn, *sightmapDirFlag, nil)
			if err != nil {
				return err
			}
			printJSONLine(map[string]string{"url": st.URL, "title": st.Title})
			continue
		}
		q, err := compquery.ParseQuery(qs)
		if err != nil {
			return err
		}
		st, err := readState(ctx, conn, *sightmapDirFlag, q)
		if err != nil {
			return err
		}
		if *asFlag != "" {
			v, err := singleValue(st, *countFlag, fields)
			if err != nil {
				return fmt.Errorf("get %s --as %s: %w", qs, *asFlag, err)
			}
			setRunVar(*asFlag, v)
			fmt.Printf("%s=%s\n", *asFlag, v)
			continue
		}
		if *countFlag {
			if len(queries) > 1 {
				fmt.Printf("%s\t%d\n", qs, len(st.Nodes))
			} else {
				fmt.Println(len(st.Nodes))
			}
			continue
		}
		if len(st.Nodes) == 0 {
			return fmt.Errorf("get: no component matches %s%s", q, nearMiss(ctx, conn, *sightmapDirFlag, q))
		}
		shown := st.Nodes
		if *limitFlag > 0 && len(shown) > *limitFlag {
			shown = shown[:*limitFlag]
		}
		for _, n := range shown {
			printJSONLine(project(n, fields))
		}
		if len(shown) < len(st.Nodes) {
			fmt.Fprintf(os.Stderr, "... %d more %s (--limit 0 for all, --count for the number, or narrow the query)\n", len(st.Nodes)-len(shown), q)
		}
	}
	return nil
}

// project picks fields out of a component's state for printing. With no fields
// it prints the component, every property, and the text.
func project(n componentState, fields []string) map[string]any {
	out := map[string]any{}
	if len(fields) == 0 {
		out["component"] = n.Component
		for k, v := range n.Props {
			out[k] = v
		}
		if n.Text != "" {
			out["text"] = clip(n.Text, 80)
		}
		if _, own := n.Props["focused"]; n.Focused && !own {
			out["focused"] = true
		}
		return out
	}
	for _, f := range fields {
		if v, ok := field(n, f); ok {
			out[f] = v
		} else {
			out[f] = nil
		}
	}
	return out
}

// singleValue is what `get --as` keeps: the match count, or the one field of
// the one match.
func singleValue(st *pageState, count bool, fields []string) (string, error) {
	if count {
		return strconv.Itoa(len(st.Nodes)), nil
	}
	if len(fields) != 1 {
		return "", errors.New("name one field with --props (or use --count)")
	}
	if len(st.Nodes) != 1 {
		return "", fmt.Errorf("want one match, saw %d", len(st.Nodes))
	}
	v, ok := field(st.Nodes[0], fields[0])
	if !ok {
		return "", fmt.Errorf("the match has no %q", fields[0])
	}
	return v, nil
}

// field reads one named value off a component: a corpus property, or one of the
// built-ins text, visible, focused and the box (x or left, y or top, width,
// height, right, bottom: viewport CSS pixels). The corpus wins: an app whose rows carry their
// own `focused` (a data-focused hook) keeps its meaning.
func field(n componentState, name string) (string, bool) {
	if v, ok := n.Props[name]; ok {
		return v, true
	}
	switch name {
	case "text":
		return n.Text, true
	case "visible":
		return strconv.FormatBool(n.Visible), true
	case "focused":
		return strconv.FormatBool(n.Focused), true
	}
	if n.Box != nil {
		b := n.Box
		switch name {
		case "x", "left":
			return strconv.Itoa(b.X), true
		case "y", "top":
			return strconv.Itoa(b.Y), true
		case "width":
			return strconv.Itoa(b.Width), true
		case "height":
			return strconv.Itoa(b.Height), true
		case "right":
			return strconv.Itoa(b.X + b.Width), true
		case "bottom":
			return strconv.Itoa(b.Y + b.Height), true
		}
	}
	return "", false
}

// nearMiss says what the page has instead, for a query that matched nothing:
// how many of the target component there are and which values its predicate
// properties take, so the caller can fix the query without another round trip.
func nearMiss(ctx context.Context, conn *browser.CDPConn, sightmapDir string, q *compquery.Query) string {
	target := q.Parts[len(q.Parts)-1]
	bare := &compquery.Query{Parts: []compquery.Part{{Name: target.Name}}, Index: -1}
	st, err := readState(ctx, conn, sightmapDir, bare)
	if err != nil {
		return ""
	}
	if len(st.Nodes) == 0 {
		return fmt.Sprintf(" (no %s on the page at %s)", target.Name, st.URL)
	}
	var b strings.Builder
	fmt.Fprintf(&b, " (%d %s on the page", len(st.Nodes), target.Name)
	for _, pr := range target.Preds {
		seen := map[string]bool{}
		var vals []string
		for _, n := range st.Nodes {
			if v, ok := n.Props[pr.Prop]; ok && !seen[v] {
				seen[v] = true
				vals = append(vals, strconv.Quote(v))
			}
		}
		if len(vals) == 0 {
			fmt.Fprintf(&b, "; none has a %q property", pr.Prop)
			continue
		}
		if len(vals) > 8 {
			vals = append(vals[:8], "...")
		}
		fmt.Fprintf(&b, "; %s is %s", pr.Prop, strings.Join(vals, ", "))
	}
	return b.String() + ")"
}

// ── expect ────────────────────────────────────────────────────────────────────

// condition is one thing `expect` waits for. ok reports whether the page
// satisfies it; observed describes what the page shows instead.
type condition struct {
	desc     string
	ok       func(st *pageState) bool
	observed func(st *pageState) string
}

func runExpect(args []string) error {
	fs := flag.NewFlagSet("expect", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir")
	timeoutMs := fs.Int("timeout-ms", 5000, "How long to wait for the conditions (default 5 000)")
	var propConds, textConds, noTextConds, queryConds multiFlag
	fs.Var(&propConds, "prop", "NAME=VALUE (also NAME^=prefix, NAME*=substring, NAME!=value; numbers: NAME>=N, NAME<N, NAME=N~T): every match's property or box (left, top, width, height, right, bottom) (repeatable)")
	fs.Var(&textConds, "text", "Substring of the component's text (repeatable)")
	fs.Var(&noTextConds, "no-text", "A substring no match's text contains (repeatable)")
	fs.Var(&queryConds, "query", "NAME=VALUE: the page URL's query parameter equals VALUE; NAME= means absent, NAME!=VALUE differs (repeatable)")
	var seqConds, setConds, containsConds, orderConds multiFlag
	fs.Var(&seqConds, "seq", "NAME=a,b,c: the matches' NAME values, in document order, are exactly this list (repeatable; \\, for a literal comma; NAME|OTHER takes each match's first)")
	fs.Var(&setConds, "set", "NAME=a,b,c: the matches' NAME values are exactly these, in any order (repeatable)")
	fs.Var(&containsConds, "contains", "NAME=a,b: each value is among the matches' NAME values (repeatable)")
	fs.Var(&orderConds, "order", "NAME=a,b: these values occur among the matches in this order, others may come between (repeatable)")
	countFlag := fs.String("count", "", "How many components match: N, >=N or <=N")
	goneFlag := fs.Bool("gone", false, "No component matches")
	focusedFlag := fs.Bool("focused", false, "The (single) match holds keyboard focus")
	urlFlag := fs.String("url", "", "The page path (no origin or query) equals this, or matches it as a glob with *")
	titleFlag := fs.String("title", "", "document.title equals this")
	queries, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return err
	}
	usage := "usage: browser expect [QUERY] [--prop k=v]... [--text S]... [--no-text S]... [--seq|--set|--contains|--order k=a,b]... [--count N|>=N|<=N] [--gone] [--focused] [--url PATH] [--query k=v]... [--title T] [--timeout-ms N]\n" +
		"  waits until every condition holds, then prints the matches; on timeout prints what it saw and exits 1\n" +
		"  with a QUERY and no other condition it waits for the component to be present\n" +
		"  examples: expect 'IssueRow[identifier=WEB-120]' --prop group='status:done'\n" +
		"            expect Toast --text 'Reverted.'   expect PropertyPicker --gone   expect --url /inbox --title 'Inbox (6) · Plumb'\n" +
		"            expect InboxRow --seq identifier=WEB-144,WEB-130,WEB-160   expect IssueRow --order identifier=WEB-130,WEB-144\n" +
		"            expect --url /my-issues/assigned --query peek=WEB-120   expect Toast --no-text Error"
	if len(queries) > 1 {
		return errors.New("expect: one query at a time\n" + usage)
	}
	var q *compquery.Query
	qs := ""
	if len(queries) == 1 {
		qs = queries[0]
		if q, err = compquery.ParseQuery(qs); err != nil {
			return err
		}
	}

	var conds []condition
	listConds := len(seqConds) + len(setConds) + len(containsConds) + len(orderConds)
	componentConds := len(propConds) > 0 || len(textConds) > 0 || len(noTextConds) > 0 || listConds > 0 || *countFlag != "" || *goneFlag || *focusedFlag
	if componentConds && q == nil {
		return errors.New("expect: --prop, --text, --no-text, --seq, --set, --contains, --order, --count, --gone and --focused need a QUERY\n" + usage)
	}
	if q != nil && !componentConds {
		conds = append(conds, condition{
			desc:     qs + " present",
			ok:       func(st *pageState) bool { return len(st.Nodes) > 0 },
			observed: func(st *pageState) string { return "0 matches" },
		})
	}
	for _, pc := range propConds {
		c, err := propCondition(qs, pc)
		if err != nil {
			return err
		}
		conds = append(conds, c)
	}
	for _, t := range textConds {
		t := t
		conds = append(conds, condition{
			desc: fmt.Sprintf("%s text contains %q", qs, t),
			ok: func(st *pageState) bool {
				return len(st.Nodes) > 0 && allNodes(st, func(n componentState) bool { return strings.Contains(n.Text, t) })
			},
			observed: func(st *pageState) string { return observedField(st, "text") },
		})
	}
	for _, t := range noTextConds {
		t := t
		conds = append(conds, condition{
			desc: fmt.Sprintf("%s text lacks %q", qs, t),
			ok: func(st *pageState) bool {
				return len(st.Nodes) > 0 && allNodes(st, func(n componentState) bool { return !strings.Contains(n.Text, t) })
			},
			observed: func(st *pageState) string { return observedField(st, "text") },
		})
	}
	for _, spec := range queryConds {
		c, err := queryCondition(spec)
		if err != nil {
			return err
		}
		conds = append(conds, c)
	}
	for _, l := range []struct {
		kind  listKind
		specs multiFlag
	}{{listSeq, seqConds}, {listSet, setConds}, {listContains, containsConds}, {listOrder, orderConds}} {
		for _, spec := range l.specs {
			c, err := listCondition(qs, l.kind, spec)
			if err != nil {
				return err
			}
			conds = append(conds, c)
		}
	}
	if *countFlag != "" {
		c, err := countCondition(qs, *countFlag)
		if err != nil {
			return err
		}
		conds = append(conds, c)
	}
	if *goneFlag {
		conds = append(conds, condition{
			desc:     qs + " gone",
			ok:       func(st *pageState) bool { return len(st.Nodes) == 0 },
			observed: func(st *pageState) string { return fmt.Sprintf("%d match(es)", len(st.Nodes)) },
		})
	}
	if *focusedFlag {
		conds = append(conds, condition{
			desc:     qs + " focused",
			ok:       func(st *pageState) bool { return len(st.Nodes) == 1 && st.Nodes[0].Focused },
			observed: func(st *pageState) string { return observedField(st, "focused") },
		})
	}
	if *urlFlag != "" {
		want := *urlFlag
		conds = append(conds, condition{
			desc:     "url " + want,
			ok:       func(st *pageState) bool { return pathMatches(st.URL, want) },
			observed: func(st *pageState) string { return "url " + st.URL },
		})
	}
	if *titleFlag != "" {
		want := *titleFlag
		conds = append(conds, condition{
			desc:     fmt.Sprintf("title %q", want),
			ok:       func(st *pageState) bool { return st.Title == want },
			observed: func(st *pageState) string { return fmt.Sprintf("title %q", st.Title) },
		})
	}
	if len(conds) == 0 {
		return errors.New(usage)
	}

	conn, err := browser.Connect(resolveCDPAddr(*addrFlag, *sightmapDirFlag), *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()

	deadline := time.Now().Add(time.Duration(*timeoutMs) * time.Millisecond)
	var last *pageState
	var lastErr error
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		st, err := readState(ctx, conn, *sightmapDirFlag, q)
		cancel()
		if err == nil {
			last, lastErr = st, nil
			if allHold(conds, st) {
				// The matches, when there are a few: a check that holds needs no list.
				if len(st.Nodes) <= 3 {
					for _, n := range st.Nodes {
						printJSONLine(project(n, nil))
					}
				}
				fmt.Fprintf(os.Stderr, "ok: %s\n", describe(conds))
				return nil
			}
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if last == nil {
		return fmt.Errorf("expect: timed out after %dms: %v", *timeoutMs, lastErr)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "expect: timed out after %dms", *timeoutMs)
	for _, c := range conds {
		if !c.ok(last) {
			fmt.Fprintf(&b, "\n  want %s; saw %s", c.desc, c.observed(last))
		}
	}
	if q != nil && len(last.Nodes) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		b.WriteString(nearMiss(ctx, conn, *sightmapDirFlag, q))
		cancel()
	}
	return errors.New(b.String())
}

func allHold(conds []condition, st *pageState) bool {
	for _, c := range conds {
		if !c.ok(st) {
			return false
		}
	}
	return true
}

func describe(conds []condition) string {
	parts := make([]string, len(conds))
	for i, c := range conds {
		parts[i] = c.desc
	}
	return strings.Join(parts, ", ")
}

func allNodes(st *pageState, f func(componentState) bool) bool {
	for _, n := range st.Nodes {
		if !f(n) {
			return false
		}
	}
	return true
}

// observedField lists the values a field takes across the matches, for a
// failed expectation's message.
func observedField(st *pageState, name string) string {
	if len(st.Nodes) == 0 {
		return "0 matches"
	}
	vals := make([]string, 0, len(st.Nodes))
	for _, n := range st.Nodes {
		v, ok := field(n, name)
		if !ok {
			vals = append(vals, "(no "+name+")")
			continue
		}
		if len(v) > 80 {
			v = v[:77] + "..."
		}
		vals = append(vals, strconv.Quote(v))
	}
	if len(vals) > 6 {
		vals = append(vals[:6], fmt.Sprintf("... (%d matches)", len(st.Nodes)))
	}
	return name + " " + strings.Join(vals, ", ")
}

// propCondition parses NAME=VALUE (or ^=, *=, !=) into a condition every match
// must satisfy, using the component-query predicate semantics. Numeric forms
// compare the value as a number: NAME>=N, NAME<=N, NAME>N, NAME<N, and
// NAME=N~T (within T of N), for the box built-ins or any numeric property.
func propCondition(qs, spec string) (condition, error) {
	if c, ok, err := numericCondition(qs, spec); ok || err != nil {
		return c, err
	}
	// The operator is the first one in the spec: a value may itself contain
	// "!=" or "^=" (`text=Count != 3` is an equality on "Count != 3").
	op, i := "", -1
	for _, o := range []string{"!=", "^=", "*=", "="} {
		if j := strings.Index(spec, o); j > 0 && (i < 0 || j < i) {
			op, i = o, j
		}
	}
	if i < 0 {
		return condition{}, fmt.Errorf("expect: --prop wants NAME=VALUE (or ^= *= !=), got %q", spec)
	}
	name, val := spec[:i], spec[i+len(op):]
	neg := op == "!="
	pred := compquery.Predicate{Prop: name, Op: op, Val: val}
	if neg {
		pred.Op = "="
	}
	return condition{
		desc: fmt.Sprintf("%s %s%s%q", qs, name, op, val),
		ok: func(st *pageState) bool {
			return len(st.Nodes) > 0 && allNodes(st, func(n componentState) bool {
				v, ok := field(n, name)
				if !ok {
					return neg // a missing property is never equal, so != holds
				}
				return pred.Match(map[string]string{name: v}) != neg
			})
		},
		observed: func(st *pageState) string { return observedField(st, name) },
	}, nil
}

// numericCondition recognises the numeric --prop forms; ok is false for any
// other spec, so propCondition handles it.
func numericCondition(qs, spec string) (c condition, ok bool, err error) {
	var name, op, rhs string
	// Like propCondition, the first operator in the spec decides: in
	// `text=a > b` the "=" comes first, so it is a text equality, not a number.
	eq := strings.Index(spec, "=")
	for _, o := range []string{">=", "<=", ">", "<"} {
		if i := strings.Index(spec, o); i > 0 && (eq < 0 || i <= eq) {
			name, op, rhs = spec[:i], o, spec[i+len(o):]
			break
		}
	}
	if op == "" {
		i := strings.Index(spec, "=")
		if i <= 0 || !strings.Contains(spec[i:], "~") || strings.ContainsAny(spec[:i], "!^*") {
			return condition{}, false, nil
		}
		name, op, rhs = spec[:i], "~", spec[i+1:]
	}
	var want, tol float64
	if op == "~" {
		v, t, _ := strings.Cut(rhs, "~")
		if want, err = strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			tol, err = strconv.ParseFloat(strings.TrimSpace(t), 64)
		}
	} else {
		want, err = strconv.ParseFloat(strings.TrimSpace(rhs), 64)
	}
	if err != nil {
		return condition{}, true, fmt.Errorf("expect: --prop %q: want a number (NAME>=N, NAME=N~T)", spec)
	}
	holds := func(v float64) bool {
		switch op {
		case ">=":
			return v >= want
		case "<=":
			return v <= want
		case ">":
			return v > want
		case "<":
			return v < want
		}
		return math.Abs(v-want) <= tol
	}
	return condition{
		desc: fmt.Sprintf("%s %s%s%s", qs, name, strings.Replace(op, "~", "=", 1), strings.Replace(rhs, "~", "±", 1)),
		ok: func(st *pageState) bool {
			return len(st.Nodes) > 0 && allNodes(st, func(n componentState) bool {
				s, ok := field(n, name)
				if !ok {
					return false
				}
				v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
				return err == nil && holds(v)
			})
		},
		observed: func(st *pageState) string { return observedField(st, name) },
	}, true, nil
}

// queryCondition parses NAME=VALUE or NAME!=VALUE into a condition on the page
// URL's query string. NAME= (an empty value) means the parameter is absent;
// NAME!= means it is present.
func queryCondition(spec string) (condition, error) {
	neg := false
	name, want, ok := strings.Cut(spec, "!=")
	if ok {
		neg = true
	} else if name, want, ok = strings.Cut(spec, "="); !ok {
		return condition{}, fmt.Errorf("expect: --query wants NAME=VALUE or NAME!=VALUE, got %q", spec)
	}
	if name == "" {
		return condition{}, fmt.Errorf("expect: --query wants a parameter name, got %q", spec)
	}
	param := func(st *pageState) (string, bool) {
		u, err := url.Parse(st.URL)
		if err != nil {
			return "", false
		}
		vs, has := u.Query()[name]
		if !has || len(vs) == 0 {
			return "", false
		}
		return vs[0], true
	}
	op := "="
	if neg {
		op = "!="
	}
	desc := fmt.Sprintf("url ?%s%s%s", name, op, want)
	if want == "" {
		desc = fmt.Sprintf("url without ?%s", name)
		if neg {
			desc = fmt.Sprintf("url with ?%s", name)
		}
	}
	return condition{
		desc: desc,
		ok: func(st *pageState) bool {
			v, has := param(st)
			if want == "" {
				return has == neg
			}
			return (has && v == want) != neg
		},
		observed: func(st *pageState) string {
			if v, has := param(st); has {
				return fmt.Sprintf("?%s=%s", name, v)
			}
			return fmt.Sprintf("no ?%s (url %s)", name, st.URL)
		},
	}, nil
}

// listKind is how a list condition compares the matches' values with its own.
type listKind string

const (
	listSeq      listKind = "seq"      // exactly this list, in order
	listSet      listKind = "set"      // exactly these values, any order
	listContains listKind = "contains" // each value present
	listOrder    listKind = "order"    // these values, in this relative order
)

// listCondition parses NAME=a,b,c into a condition on the sequence of NAME
// values across the matches, in document order. A match without the property
// contributes no value. Values split on commas; `\,` is a literal comma.
func listCondition(qs string, kind listKind, spec string) (condition, error) {
	i := strings.IndexByte(spec, '=')
	if i <= 0 {
		return condition{}, fmt.Errorf("expect: --%s wants NAME=a,b,c, got %q", kind, spec)
	}
	name, want := spec[:i], splitValues(spec[i+1:])
	if len(want) == 0 && kind != listSeq && kind != listSet {
		return condition{}, fmt.Errorf("expect: --%s %s= needs at least one value", kind, name)
	}
	return condition{
		desc: fmt.Sprintf("%s %s --%s %s", qs, name, kind, strings.Join(want, ",")),
		ok:   func(st *pageState) bool { return listHolds(kind, values(st, name), want) },
		observed: func(st *pageState) string {
			got := values(st, name)
			if len(got) == 0 {
				return fmt.Sprintf("no %s values (%d match(es))", name, len(st.Nodes))
			}
			s := name + " " + strings.Join(got, ",")
			if kind == listContains || kind == listOrder {
				if miss := missing(got, want); len(miss) > 0 {
					s += "; missing " + strings.Join(miss, ",")
				}
			}
			return s
		},
	}, nil
}

// values lists a field across the matches, in document order, skipping a match
// that lacks it. NAME may list alternatives, `identifier|project`: each match
// contributes the first it has, for a list that mixes kinds of rows.
func values(st *pageState, name string) []string {
	var out []string
	for _, n := range st.Nodes {
		for _, alt := range strings.Split(name, "|") {
			if v, ok := field(n, alt); ok {
				out = append(out, v)
				break
			}
		}
	}
	return out
}

func listHolds(kind listKind, got, want []string) bool {
	switch kind {
	case listSeq:
		return slices.Equal(got, want)
	case listSet:
		a, b := slices.Clone(got), slices.Clone(want)
		sort.Strings(a)
		sort.Strings(b)
		return slices.Equal(a, b)
	case listContains:
		return len(missing(got, want)) == 0
	case listOrder:
		j := 0
		for _, v := range got {
			if j < len(want) && v == want[j] {
				j++
			}
		}
		return j == len(want)
	}
	return false
}

// missing lists the wanted values got lacks, counting repeats: a value wanted
// twice must be there twice.
func missing(got, want []string) []string {
	have := map[string]int{}
	for _, v := range got {
		have[v]++
	}
	var out []string
	for _, v := range want {
		if have[v] > 0 {
			have[v]--
		} else {
			out = append(out, v)
		}
	}
	return out
}

// splitValues splits a comma-separated list, where `\,` is a literal comma.
// Values are kept as written (no trimming), since a property value can carry
// spaces; an empty list is written as nothing after the `=`.
func splitValues(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s) && s[i+1] == ',':
			cur.WriteByte(',')
			i++
		case s[i] == ',':
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(s[i])
		}
	}
	return append(out, cur.String())
}

func countCondition(qs, spec string) (condition, error) {
	op, num := "=", spec
	switch {
	case strings.HasPrefix(spec, ">="):
		op, num = ">=", spec[2:]
	case strings.HasPrefix(spec, "<="):
		op, num = "<=", spec[2:]
	}
	n, err := strconv.Atoi(strings.TrimSpace(num))
	if err != nil {
		return condition{}, fmt.Errorf("expect: --count wants N, >=N or <=N, got %q", spec)
	}
	return condition{
		desc: fmt.Sprintf("%s count %s%d", qs, strings.TrimPrefix(op, "="), n),
		ok: func(st *pageState) bool {
			switch op {
			case ">=":
				return len(st.Nodes) >= n
			case "<=":
				return len(st.Nodes) <= n
			}
			return len(st.Nodes) == n
		},
		observed: func(st *pageState) string { return fmt.Sprintf("%d match(es)", len(st.Nodes)) },
	}, nil
}

// pathMatches compares a page URL's path with want: equal, or a glob where *
// matches any run of characters. The query string and origin are ignored, since
// fixture parameters like ?latency=0 ride along on a page's URL.
func pathMatches(pageURL, want string) bool {
	path := pageURL
	if i := strings.Index(path, "://"); i >= 0 {
		if j := strings.IndexByte(path[i+3:], '/'); j >= 0 {
			path = path[i+3+j:]
		} else {
			path = "/"
		}
	}
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	if !strings.Contains(want, "*") {
		return path == want
	}
	return globMatch(want, path)
}

func globMatch(pattern, s string) bool {
	// Iterative wildcard matching with one backtrack point: on a mismatch after a
	// *, retry with the * taking one more character. Linear in practice, and right
	// for any number of stars (a first-occurrence scan is not: *aa*aa*b vs abaaab).
	p, i, star, mark := 0, 0, -1, 0
	for i < len(s) {
		switch {
		case p < len(pattern) && pattern[p] == '*':
			star, mark = p, i
			p++
		case p < len(pattern) && pattern[p] == s[i]:
			p++
			i++
		case star >= 0:
			p, mark = star+1, mark+1
			i = mark
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}

// ── keys ──────────────────────────────────────────────────────────────────────

func runKeys(args []string) error {
	fs := flag.NewFlagSet("keys", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (for --focus queries)")
	focusFlag := fs.String("focus", "", "Component query (or probe id) to give keyboard focus first")
	var typeFlags multiFlag
	fs.Var(&typeFlags, "type", "Text to type, one character at a time, at its place among the keys (repeatable)")
	delayMs := fs.Int("delay-ms", 50, "Pause between keys, so key handlers and sequence timers see them apart")
	settle := addSettleFlag(fs)
	keys, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return err
	}
	if len(keys) == 0 && len(typeFlags) == 0 {
		return fmt.Errorf("usage: browser keys KEY... [--type TEXT]... [--focus QUERY] [--delay-ms N] [--settle]\n" +
			"  presses each KEY (a key or chord, as for keypress) and types each --type TEXT, in the order given\n" +
			"  examples: keys g m   keys Meta+k --type prio Enter   keys f --type Labels Enter --type Bug Enter\n" +
			"            keys --focus CommentComposer --type 'Looks good' Meta+Enter")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, err := browser.Connect(resolveCDPAddr(*addrFlag, *sightmapDirFlag), *tabFlag)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := foreground(ctx, conn); err != nil {
		return err
	}
	if *focusFlag != "" {
		node, err := resolveTarget(ctx, conn, *sightmapDirFlag, *focusFlag)
		if err != nil {
			return err
		}
		if err := browser.Focus(ctx, conn, node); err != nil {
			return err
		}
	}
	if err := armSettle(ctx, conn, *settle); err != nil {
		return err
	}
	pause := time.Duration(*delayMs) * time.Millisecond
	var parts []string
	for _, s := range keySteps(args, keys) {
		var err error
		if s.typed {
			err = browser.TypeText(ctx, conn, s.text)
			parts = append(parts, strconv.Quote(s.text))
		} else {
			err = browser.KeyPress(ctx, conn, s.text)
			parts = append(parts, s.text)
		}
		if err != nil {
			return err
		}
		time.Sleep(pause)
	}
	fmt.Fprintf(os.Stderr, "pressed %s\n", strings.Join(parts, " "))
	return settleAfter(ctx, conn, *settle)
}

// keyStep is one step of a `keys` call: a key to press, or text to type.
type keyStep struct {
	text  string
	typed bool
}

// keySteps puts the keys and the --type texts back in the order they were
// given. The flag package separates them (`keys f --type Labels Enter --type Bug
// Enter` parses as keys [f Enter Enter] and two type values), so the order is
// read from the original arguments. Other flags and their values are skipped.
func keySteps(args, keys []string) []keyStep {
	var steps []keyStep
	ki := 0
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if strings.HasPrefix(a, "-") && a != "-" {
			switch name {
			case "type":
				if !hasVal && i+1 < len(args) {
					i++
					val = args[i]
				}
				steps = append(steps, keyStep{text: val, typed: true})
			case "settle":
				// a bool flag: no value follows
			default:
				if !hasVal {
					i++ // skip the value of --focus, --delay-ms, --addr, ...
				}
			}
			continue
		}
		if ki < len(keys) && a == keys[ki] {
			steps = append(steps, keyStep{text: a})
			ki++
		}
	}
	for ; ki < len(keys); ki++ {
		steps = append(steps, keyStep{text: keys[ki]})
	}
	return steps
}

// ── settle ─────────────────────────────────────────────────────────────────────

// settleQuiet is how long the DOM must stay unchanged, with no fetch in
// flight, for the page to count as settled; settleMax caps the wait.
// redirectFloor is the least time navigate watches for a client-side redirect
// after load, even on a page that renders and goes quiet sooner.
const (
	settleQuiet   = 300 * time.Millisecond
	settleMax     = 5 * time.Second
	redirectFloor = 1 * time.Second
)

// addSettleFlag registers --settle on an action's flag set. Waiting after an
// action stays the caller's explicit choice (an action never waits on its own),
// but it is one flag instead of a separate sleep.
func addSettleFlag(fs *flag.FlagSet) *bool {
	return fs.Bool("settle", false, "After acting, wait until the DOM has been quiet for 300 ms with no fetch in flight (at most 5 s), instead of a sleep")
}

// armSettle starts counting the page's requests in flight when --settle was
// given. Call it just before the action, so a request the action starts is
// counted from the moment it is sent.
func armSettle(ctx context.Context, conn *browser.CDPConn, settle bool) error {
	if !settle {
		return nil
	}
	return browser.ArmSettle(ctx, conn)
}

// settleAfter waits for the page to settle when asked, and says how long it
// took on stderr, so a caller sees both that it waited and that it can stop
// sleeping.
func settleAfter(ctx context.Context, conn *browser.CDPConn, settle bool) error {
	if !settle {
		return nil
	}
	r, err := browser.WaitSettled(ctx, conn, settleQuiet, settleMax)
	if err != nil {
		return err
	}
	if r.Quiet {
		fmt.Fprintf(os.Stderr, "settled in %dms\n", r.Elapsed.Milliseconds())
	} else {
		fmt.Fprintf(os.Stderr, "still changing after %dms (an animation or a poll?); expect a specific result instead\n", r.Elapsed.Milliseconds())
	}
	return nil
}

// takeSettle removes a --settle flag from args for the commands that parse
// their arguments by hand (navigate). It accepts every form the flag package
// does for a bool: --settle, -settle, and --settle=VALUE / -settle=VALUE with a
// strconv.ParseBool value; the last occurrence wins, as with flag.
func takeSettle(args []string) (bool, []string) {
	out := args[:0:0]
	settle := false
	for _, a := range args {
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !strings.HasPrefix(a, "-") || name != "settle" {
			out = append(out, a)
			continue
		}
		settle = true
		if hasValue {
			if b, err := strconv.ParseBool(value); err == nil {
				settle = b
			}
		}
	}
	return settle, out
}

// ── helpers ───────────────────────────────────────────────────────────────────

// clip shortens s to at most n runes, marking the cut.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// multiFlag collects a repeatable string flag.
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// printJSONLine prints v as one line of JSON with sorted keys, so output is
// stable across runs and easy to grep.
func printJSONLine(v any) {
	if m, ok := v.(map[string]any); ok {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		// "component" first reads better.
		sort.SliceStable(keys, func(i, j int) bool { return keys[i] == "component" && keys[j] != "component" })
		var b strings.Builder
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			vb, _ := json.Marshal(m[k])
			b.Write(kb)
			b.WriteByte(':')
			b.Write(vb)
		}
		b.WriteByte('}')
		fmt.Println(b.String())
		return
	}
	out, _ := json.Marshal(v)
	fmt.Println(string(out))
}
