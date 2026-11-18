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
	"os"
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
// active element, read in the same round trip as the URL and title.
func readState(ctx context.Context, conn *browser.CDPConn, sightmapDir string, q *compquery.Query) (*pageState, error) {
	root, matches, props, err := extractMatchedComponents(ctx, conn, sightmapDir)
	if err != nil {
		return nil, err
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
		cs := componentState{Props: props[n.Id], Text: n.Text, Visible: n.IsVisible}
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
	propsFlag := fs.String("props", "", "Comma-separated properties to print (default: all; 'text', 'visible' and 'focused' work too)")
	countFlag := fs.Bool("count", false, "Print only how many components match")
	queries, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return err
	}
	if len(queries) == 0 {
		return fmt.Errorf("usage: browser get QUERY... [--props a,b] [--count]\n" +
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
		for _, n := range st.Nodes {
			printJSONLine(project(n, fields))
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
			out["text"] = n.Text
		}
		if n.Focused {
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

// field reads one named value off a component: a corpus property, or one of the
// built-ins text, visible, focused.
func field(n componentState, name string) (string, bool) {
	switch name {
	case "text":
		return n.Text, true
	case "visible":
		return strconv.FormatBool(n.Visible), true
	case "focused":
		return strconv.FormatBool(n.Focused), true
	}
	v, ok := n.Props[name]
	return v, ok
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
	var propConds, textConds multiFlag
	fs.Var(&propConds, "prop", "NAME=VALUE (also NAME^=prefix, NAME*=substring, NAME!=value): every match's property (repeatable)")
	fs.Var(&textConds, "text", "Substring of the component's text (repeatable)")
	countFlag := fs.String("count", "", "How many components match: N, >=N or <=N")
	goneFlag := fs.Bool("gone", false, "No component matches")
	focusedFlag := fs.Bool("focused", false, "The (single) match holds keyboard focus")
	urlFlag := fs.String("url", "", "The page path (no origin or query) equals this, or matches it as a glob with *")
	titleFlag := fs.String("title", "", "document.title equals this")
	queries, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return err
	}
	usage := "usage: browser expect [QUERY] [--prop k=v]... [--text S]... [--count N|>=N|<=N] [--gone] [--focused] [--url PATH] [--title T] [--timeout-ms N]\n" +
		"  waits until every condition holds, then prints the matches; on timeout prints what it saw and exits 1\n" +
		"  with a QUERY and no other condition it waits for the component to be present\n" +
		"  examples: expect 'IssueRow[identifier=WEB-120]' --prop group='status:done'\n" +
		"            expect Toast --text 'Reverted.'   expect PropertyPicker --gone   expect --url /inbox --title 'Inbox (6) · Plumb'"
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
	componentConds := len(propConds) > 0 || len(textConds) > 0 || *countFlag != "" || *goneFlag || *focusedFlag
	if componentConds && q == nil {
		return errors.New("expect: --prop, --text, --count, --gone and --focused need a QUERY\n" + usage)
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
				for _, n := range st.Nodes {
					printJSONLine(project(n, nil))
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
// must satisfy, using the component-query predicate semantics.
func propCondition(qs, spec string) (condition, error) {
	for _, op := range []string{"!=", "^=", "*=", "="} {
		i := strings.Index(spec, op)
		if i <= 0 {
			continue
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
					if name == "text" || name == "visible" || name == "focused" {
						v, _ := field(n, name)
						return pred.Match(map[string]string{name: v}) != neg
					}
					return pred.Match(n.Props) != neg
				})
			},
			observed: func(st *pageState) string { return observedField(st, name) },
		}, nil
	}
	return condition{}, fmt.Errorf("expect: --prop wants NAME=VALUE (or ^= *= !=), got %q", spec)
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
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for i, p := range parts[1:] {
		if i == len(parts)-2 {
			return strings.HasSuffix(s, p)
		}
		j := strings.Index(s, p)
		if j < 0 {
			return false
		}
		s = s[j+len(p):]
	}
	return true
}

// ── keys ──────────────────────────────────────────────────────────────────────

func runKeys(args []string) error {
	fs := flag.NewFlagSet("keys", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (for --focus queries)")
	focusFlag := fs.String("focus", "", "Component query (or probe id) to give keyboard focus first")
	typeFlag := fs.String("type", "", "Text to type after the keys, one character at a time")
	delayMs := fs.Int("delay-ms", 50, "Pause between keys, so key handlers and sequence timers see them apart")
	keys, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return err
	}
	if len(keys) == 0 && *typeFlag == "" {
		return fmt.Errorf("usage: browser keys KEY... [--type TEXT] [--focus QUERY] [--delay-ms N]\n" +
			"  presses each KEY (a key or chord, as for keypress) in order, then types TEXT\n" +
			"  examples: keys g m   keys Meta+k --type prio   keys s 3   keys --focus CommentComposer --type 'Looks good' Meta+Enter")
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
	pause := time.Duration(*delayMs) * time.Millisecond
	// Keys before --type and keys after it: `keys Meta+k --type prio Enter` is parsed
	// by the flag package as keys [Meta+k Enter], so a key given after the text is
	// told apart by its position in the original arguments.
	before, after := splitAroundType(args, keys)
	for _, k := range before {
		if err := browser.KeyPress(ctx, conn, k); err != nil {
			return err
		}
		time.Sleep(pause)
	}
	if *typeFlag != "" {
		if err := browser.TypeText(ctx, conn, *typeFlag); err != nil {
			return err
		}
		time.Sleep(pause)
	}
	for _, k := range after {
		if err := browser.KeyPress(ctx, conn, k); err != nil {
			return err
		}
		time.Sleep(pause)
	}
	parts := append([]string{}, before...)
	if *typeFlag != "" {
		parts = append(parts, strconv.Quote(*typeFlag))
	}
	parts = append(parts, after...)
	fmt.Fprintf(os.Stderr, "pressed %s\n", strings.Join(parts, " "))
	return nil
}

// splitAroundType splits the positional keys into those given before --type and
// those given after it, by walking the original arguments.
func splitAroundType(args, keys []string) (before, after []string) {
	seenType := false
	ki := 0
	for i := 0; i < len(args) && ki < len(keys); i++ {
		a := args[i]
		if a == "--type" || a == "-type" {
			seenType = true
			i++ // skip the value
			continue
		}
		if strings.HasPrefix(a, "--type=") || strings.HasPrefix(a, "-type=") {
			seenType = true
			continue
		}
		if a == keys[ki] {
			if seenType {
				after = append(after, a)
			} else {
				before = append(before, a)
			}
			ki++
		}
	}
	return before, append(after, keys[ki:]...)
}

// ── helpers ───────────────────────────────────────────────────────────────────

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
