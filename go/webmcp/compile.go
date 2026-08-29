package webmcp

// Compiler. Resolves a validated manifest against a loaded corpus and
// produces the ordered-JSON IR the emitter embeds. Objects are built with
// insertion-ordered keys so the embedded JSON (and thus generated bundles)
// stays deterministic.

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var templateRe = regexp.MustCompile(`\{([a-z][a-z0-9_]*)(\|raw)?\}`)
var apiOriginRe = regexp.MustCompile(`^https?://[^/]*`)
var fieldSpecKeys = []string{"component", "selector", "extract", "transform", "property", "max_chars"}
var braceEscapeRe = regexp.MustCompile(`\{\{|\}\}`)

func templateRefs(s string) []string {
	var refs []string
	clean := braceEscapeRe.ReplaceAllString(s, "")
	for _, m := range templateRe.FindAllStringSubmatch(clean, -1) {
		refs = append(refs, m[1])
	}
	return refs
}

// refSet is an insertion-ordered string set so diagnostics come out in the
// same order every run.
type refSet struct {
	order []string
	seen  map[string]bool
}

func newRefSet() *refSet { return &refSet{seen: map[string]bool{}} }

func (r *refSet) add(ref string) {
	if !r.seen[ref] {
		r.seen[ref] = true
		r.order = append(r.order, ref)
	}
}

func (r *refSet) has(ref string) bool { return r.seen[ref] }

func (r *refSet) addString(v any) {
	switch t := v.(type) {
	case string:
		for _, ref := range templateRefs(t) {
			r.add(ref)
		}
	case []any:
		for _, item := range t {
			r.addString(item)
		}
	case *OM:
		for _, k := range t.Keys() {
			val, _ := t.Get(k)
			r.addString(val)
		}
	}
}

// resolvedEntry is a corpus component after name lookup, with its selector
// chain and properties ready to inline.
type resolvedEntry struct {
	Crumb     string
	Levels    [][]string
	LevelsLen int
	PropOrder []string
	Props     map[string]propSpec
}

func entriesFor(c *Corpus, crumb, viewScope string) []*componentEntry {
	all := c.Components[crumb]
	var applicable, scoped []*componentEntry
	for _, e := range all {
		if e.Scope == "" || e.Scope == viewScope {
			applicable = append(applicable, e)
			if e.Scope != "" {
				scoped = append(scoped, e)
			}
		}
	}
	if len(scoped) > 0 {
		return scoped
	}
	return applicable
}

func pickEntry(c *Corpus, crumb string, prev *resolvedEntry, viewScope string) (*resolvedEntry, error) {
	defs := entriesFor(c, crumb, viewScope)
	if len(defs) == 0 {
		return nil, fmt.Errorf("no component at %q", crumb)
	}
	if len(defs) > 1 {
		var scopes []string
		for _, d := range defs {
			if d.Scope == "" {
				scopes = append(scopes, "global")
			} else {
				scopes = append(scopes, d.Scope)
			}
		}
		return nil, fmt.Errorf("%q has %d definitions (%s) — set the tool's \"view:\" to pick the one that applies",
			crumb, len(defs), strings.Join(scopes, ", "))
	}
	def := defs[0]
	levels := def.ChainLevels
	if prev != nil && strings.HasPrefix(def.Path, prev.Crumb+" ") {
		levels = def.ChainLevels[prev.LevelsLen:]
	}
	return &resolvedEntry{
		Crumb: def.Path, Levels: levels, LevelsLen: len(def.ChainLevels),
		PropOrder: def.PropOrder, Props: def.Props,
	}, nil
}

func resolveName(c *Corpus, name string, prev *resolvedEntry, viewScope string) (*resolvedEntry, error) {
	var crumbs []string
	for _, crumb := range c.ByName[name] {
		if len(entriesFor(c, crumb, viewScope)) > 0 {
			crumbs = append(crumbs, crumb)
		}
	}
	if len(crumbs) == 0 {
		if len(entriesFor(c, name, viewScope)) > 0 {
			return pickEntry(c, name, prev, viewScope)
		}
		msg := fmt.Sprintf("no component named %q in the corpus", name)
		if viewScope != "" {
			msg += fmt.Sprintf(" (in view %s or globals)", viewScope)
		}
		if len(c.ByName[name]) > 0 {
			msg += " — it exists only view-scoped; set the tool's \"view:\""
		}
		return nil, fmt.Errorf("%s", msg)
	}
	candidates := crumbs
	if prev != nil {
		var under []string
		for _, crumb := range crumbs {
			if strings.HasPrefix(crumb, prev.Crumb+" ") {
				under = append(under, crumb)
			}
		}
		if len(under) > 0 {
			candidates = under
		}
	}
	if len(candidates) > 1 {
		return nil, fmt.Errorf("component name %q is ambiguous — qualify it with its parent. Candidates: %s",
			name, strings.Join(candidates, ", "))
	}
	return pickEntry(c, candidates[0], prev, viewScope)
}

func chainToJSON(levels [][]string) []any {
	out := make([]any, len(levels))
	for i, alts := range levels {
		lvl := make([]any, len(alts))
		for j, s := range alts {
			lvl[j] = s
		}
		out[i] = lvl
	}
	return out
}

func propsToJSON(order []string, props map[string]propSpec) *OM {
	om := NewOM()
	for _, name := range order {
		p := props[name]
		pv := NewOM().Set("extract", p.Extract)
		if p.Transform != "" {
			pv.Set("transform", p.Transform)
		}
		om.Set(name, pv)
	}
	return om
}

// resolveTarget compiles a component reference into a runtime TargetIR.
func resolveTarget(c *Corpus, ref string, scopeEntry *resolvedEntry, viewScope string) (*OM, *resolvedEntry, error) {
	parsed, err := parseQuery(ref)
	if err != nil {
		return nil, nil, err
	}
	if parsed.Kind == "css" {
		return NewOM().Set("kind", "css").Set("selector", parsed.Selector), nil, nil
	}
	links := []any{}
	prev := scopeEntry
	for _, link := range parsed.Links {
		entry, err := resolveName(c, link.Name, prev, viewScope)
		if err != nil {
			return nil, nil, err
		}
		for _, pred := range link.Preds {
			if _, ok := entry.Props[pred.Prop]; !ok {
				have := entry.PropOrder
				detail := " (it declares no properties)"
				if len(have) > 0 {
					detail = fmt.Sprintf(" (properties: %s)", strings.Join(have, ", "))
				}
				return nil, nil, fmt.Errorf("%q has no property %q%s", entry.Crumb, pred.Prop, detail)
			}
		}
		preds := []any{}
		for _, p := range link.Preds {
			preds = append(preds, NewOM().Set("prop", p.Prop).Set("op", p.Op).Set("value", p.Value).Set("ci", p.CI))
		}
		var index any
		if link.Index != nil {
			index = *link.Index
		}
		links = append(links, NewOM().
			Set("name", entry.Crumb).
			Set("chain", chainToJSON(entry.Levels)).
			Set("props", propsToJSON(entry.PropOrder, entry.Props)).
			Set("preds", preds).
			Set("index", index))
		prev = entry
	}
	return NewOM().Set("kind", "chain").Set("links", links), prev, nil
}

func collectPredRefs(target *OM, refs *refSet) {
	if kind, _ := target.Get("kind"); kind == "css" {
		sel, _ := target.Get("selector")
		refs.addString(sel)
		return
	} else if kind != "chain" {
		return
	}
	linksV, _ := target.Get("links")
	for _, l := range asList(linksV) {
		lom := l.(*OM)
		predsV, _ := lom.Get("preds")
		for _, p := range asList(predsV) {
			pom := p.(*OM)
			v, _ := pom.Get("value")
			refs.addString(v)
		}
	}
}

func compileFieldSpec(c *Corpus, spec any, where string, scopeEntry *resolvedEntry, refs *refSet, viewScope string) (*OM, error) {
	som := asOM(spec)
	if som == nil {
		return nil, fmt.Errorf("%s: must be a mapping", where)
	}
	for _, k := range som.Keys() {
		if !contains(fieldSpecKeys, k) {
			return nil, fmt.Errorf("%s: unknown key %q in a read value spec (%s)",
				where, k, strings.Join(fieldSpecKeys, ", "))
		}
	}
	var target *OM
	lastEntry := scopeEntry
	if comp := asString(omGet(som, "component")); comp != "" {
		t, entry, err := resolveTarget(c, comp, scopeEntry, viewScope)
		if err != nil {
			return nil, err
		}
		target = t
		lastEntry = entry
		collectPredRefs(target, refs)
	} else if sel := asString(omGet(som, "selector")); sel != "" {
		target = NewOM().Set("kind", "css").Set("selector", sel)
		collectPredRefs(target, refs)
		lastEntry = nil
	}
	extract := asString(omGet(som, "extract"))
	transform := asString(omGet(som, "transform"))
	if prop := asString(omGet(som, "property")); prop != "" {
		if extract != "" {
			return nil, fmt.Errorf("%s: give \"property\" or \"extract\", not both", where)
		}
		if lastEntry == nil {
			return nil, fmt.Errorf("%s: \"property\" lookup needs a component target (got a raw selector)", where)
		}
		ps, ok := lastEntry.Props[prop]
		if !ok {
			detail := ""
			if len(lastEntry.PropOrder) > 0 {
				detail = fmt.Sprintf(" (properties: %s)", strings.Join(lastEntry.PropOrder, ", "))
			}
			return nil, fmt.Errorf("%s: %q has no property %q%s", where, lastEntry.Crumb, prop, detail)
		}
		extract = ps.Extract
		if transform == "" && ps.Transform != "" {
			transform = ps.Transform
		}
	}
	if extract == "" {
		extract = "text"
	}
	out := NewOM().Set("extract", extract)
	if target != nil {
		out.Set("target", target)
	}
	if transform != "" {
		out.Set("transform", transform)
	}
	if mc, ok := som.Get("max_chars"); ok && mc != nil {
		cap, isInt := asInt(mc)
		if !isInt || cap < 1 {
			return nil, fmt.Errorf("%s: \"max_chars\" must be a positive integer", where)
		}
		out.Set("cap", cap)
	}
	return out, nil
}

func compileValueSpec(c *Corpus, spec any, where string, scopeEntry *resolvedEntry, refs *refSet, viewScope string) (*OM, error) {
	som := asOM(spec)
	if som == nil {
		return nil, fmt.Errorf("%s: a read value must be a mapping", where)
	}
	if fe := asString(omGet(som, "for_each")); fe != "" {
		for _, k := range som.Keys() {
			if k != "for_each" && k != "max" && k != "fields" {
				return nil, fmt.Errorf("%s: unknown key %q in a for_each spec (for_each, max, fields)", where, k)
			}
		}
		target, lastEntry, err := resolveTarget(c, fe, scopeEntry, viewScope)
		if err != nil {
			return nil, err
		}
		collectPredRefs(target, refs)
		fieldsOM := asOM(omGet(som, "fields"))
		if fieldsOM == nil {
			return nil, fmt.Errorf("%s: for_each needs a \"fields\" mapping", where)
		}
		fields := NewOM()
		for _, key := range fieldsOM.Keys() {
			fv, _ := fieldsOM.Get(key)
			fir, err := compileFieldSpec(c, fv, where+".fields."+key, lastEntry, refs, viewScope)
			if err != nil {
				return nil, err
			}
			fields.Set(key, fir)
		}
		var max any
		if mv, ok := som.Get("max"); ok && mv != nil {
			if n, isInt := asInt(mv); isInt {
				max = n
			} else {
				s := asString(mv)
				max = s
				for _, r := range templateRefs(s) {
					refs.add(r)
				}
			}
		}
		list := NewOM().Set("target", target).Set("max", max).Set("fields", fields)
		return NewOM().Set("list", list), nil
	}
	one, err := compileFieldSpec(c, spec, where, scopeEntry, refs, viewScope)
	if err != nil {
		return nil, err
	}
	return NewOM().Set("one", one), nil
}

func compileParams(tool *OM) (*OM, []any) {
	properties := NewOM()
	required := []any{}
	params := []any{}
	for _, p := range asList(omGet(tool, "params")) {
		pom := asOM(p)
		if pom == nil {
			continue
		}
		name := asString(omGet(pom, "name"))
		prop := NewOM().
			Set("type", asString(omGet(pom, "type"))).
			Set("description", asString(omGet(pom, "description")))
		if enum, ok := pom.Get("enum"); ok && enum != nil {
			prop.Set("enum", enum)
		}
		if def, ok := pom.Get("default"); ok {
			prop.Set("default", def)
		}
		properties.Set(name, prop)
		if asBool(omGet(pom, "required")) {
			required = append(required, name)
		}
		var defVal any
		if dv, ok := pom.Get("default"); ok {
			defVal = dv
		}
		params = append(params, NewOM().
			Set("name", name).
			Set("type", asString(omGet(pom, "type"))).
			Set("required", asBool(omGet(pom, "required"))).
			Set("default", defVal))
	}
	schema := NewOM().Set("type", "object").Set("properties", properties)
	if len(required) > 0 {
		schema.Set("required", required)
	}
	return schema, params
}

type compiledTool struct {
	kind     string
	body     *OM // the api or flow object
	refs     *refSet
	readOnly bool
}

func compileAPITool(c *Corpus, tool *OM, d *diags) *compiledTool {
	api := asOM(omGet(tool, "api"))
	name := asString(omGet(tool, "name"))
	refs := newRefSet()
	method := asString(omGet(api, "method"))
	var requestName any
	resultSpecs := asList(omGet(api, "result"))
	if reqName := asString(omGet(api, "request")); reqName != "" {
		req, ok := c.Requests[reqName]
		if !ok {
			names := strings.Join(c.ReqOrder, ", ")
			if names == "" {
				names = "none"
			}
			d.errf("tool %q: no corpus request named %q (requests: %s)", name, reqName, names)
			return nil
		}
		requestName = req.Name
		if method == "" {
			method = req.Method
			if method == "" {
				method = "GET"
			}
		}
		if u := asString(omGet(api, "url")); u != "" && req.Route != "" {
			probe := templateRe.ReplaceAllString(u, "x")
			if base, err := url.Parse("https://x.invalid"); err == nil {
				if parsed, err := base.Parse(probe); err == nil {
					if !pathMatchesRoute(parsed.Path, req.Route) {
						d.warnf("tool %q: url path %q does not match corpus request %q route %q",
							name, parsed.Path, req.Name, req.Route)
					}
				}
			}
		}
		if len(resultSpecs) == 0 && len(req.Properties) > 0 {
			resultSpecs = req.Properties
		}
	}
	if method == "" {
		method = "GET"
	}
	apiURL := asString(omGet(api, "url"))
	if apiURL == "" && requestName != nil {
		// A corpus request whose route is literal (no wildcards, no :params)
		// IS a usable URL — the fallback the validator promises.
		route := c.Requests[requestName.(string)].Route
		if route != "" && !strings.Contains(route, "*") && !strings.Contains(route, "/:") {
			apiURL = route
		}
	}
	if apiURL == "" {
		d.errf("tool %q: api \"url\" template is required (the referenced request's route has wildcards, so it cannot serve as one)", name)
		return nil
	}
	if requestName != nil && c.DuplicateRequests[requestName.(string)] {
		d.warnf("tool %q: the corpus defines more than one request named %q — the first definition's route/method/properties are used", name, requestName.(string))
	}
	refs.addString(apiURL)
	if origin := apiOriginRe.FindString(apiURL); strings.HasPrefix(apiURL, "{") || strings.Contains(origin, "{") {
		d.warnf("tool %q: the api url's origin is parameterized — agent-supplied params choose the host that receives a credentialed request; prefer a fixed origin", name)
	}
	refs.addString(omGet(api, "query"))
	refs.addString(omGet(api, "headers"))
	refs.addString(omGet(api, "body"))

	readOnly := method == "GET"
	if ro, ok := tool.Get("read_only"); ok && ro != nil {
		readOnly = asBool(ro)
	}
	if ro, ok := tool.Get("read_only"); ok && asBool(ro) && method != "GET" {
		d.warnf("tool %q: read_only: true on a %s request — double-check", name, method)
	}

	result := []any{}
	for _, r := range resultSpecs {
		rom := asOM(r)
		if rom == nil {
			continue
		}
		entry := NewOM().
			Set("name", asString(omGet(rom, "name"))).
			Set("source", asString(omGet(rom, "source")))
		for _, k := range []string{"field", "pattern", "transform"} {
			if v := asString(omGet(rom, k)); v != "" {
				entry.Set(k, v)
			} else {
				entry.Set(k, nil)
			}
		}
		result = append(result, entry)
	}

	maxBody := 20000
	if mb, ok := asInt(omGet(api, "max_body_chars")); ok && mb > 0 {
		maxBody = mb
	}
	var queryV, headersV, bodyV any
	if v, ok := api.Get("query"); ok && v != nil {
		queryV = v
	}
	if v, ok := api.Get("headers"); ok && v != nil {
		headersV = v
	}
	if v, ok := api.Get("body"); ok && v != nil {
		bodyV = v
	}
	body := NewOM().
		Set("method", method).
		Set("url", apiURL).
		Set("query", queryV).
		Set("headers", headersV).
		Set("body", bodyV).
		Set("result", result).
		Set("maxBodyChars", maxBody).
		Set("request", requestName)
	return &compiledTool{kind: "api", body: body, refs: refs, readOnly: readOnly}
}

func compileFlowTool(c *Corpus, tool *OM, d *diags) *compiledTool {
	name := asString(omGet(tool, "name"))
	refs := newRefSet()
	mode := asString(omGet(tool, "mode"))
	if mode == "" {
		mode = "live"
	}
	viewScope := asString(omGet(tool, "view"))
	if viewScope == "" {
		viewScope = asString(omGet(tool, "require_view"))
	}
	if v := asString(omGet(tool, "view")); v != "" {
		if _, ok := c.Views[v]; !ok {
			d.errf("tool %q: no corpus view named %q (views: %s)", name, v, strings.Join(c.ViewOrder, ", "))
		}
	}

	var requireView any
	if rv := asString(omGet(tool, "require_view")); rv != "" {
		view, ok := c.Views[rv]
		if !ok {
			d.errf("tool %q: no corpus view named %q (views: %s)", name, rv, strings.Join(c.ViewOrder, ", "))
		} else {
			var urlV any
			if view.URL != "" {
				urlV = view.URL
			}
			requireView = NewOM().
				Set("view", view.Name).
				Set("route", view.Route).
				Set("pathRegex", routeGlobRegexSource(view.Route)).
				Set("url", urlV)
		}
	}

	steps := []any{}
	hasRead := false
	hasNavigate := false
	hasInteraction := false
	for i, s := range asList(omGet(tool, "flow")) {
		som := asOM(s)
		if som == nil {
			continue
		}
		where := fmt.Sprintf("tool %q step %d", name, i+1)
		step, err := compileStep(c, som, refs, viewScope, &hasRead, &hasNavigate, &hasInteraction)
		if err != nil {
			d.errf("%s: %v", where, err)
			continue
		}
		if step != nil {
			steps = append(steps, step)
		}
	}

	if !hasRead && !hasNavigate {
		d.warnf("tool %q: flow has no \"read\" step — the tool returns only {ok: true}", name)
	}
	readOnly := mode == "fetch" || !hasInteraction
	if ro, ok := tool.Get("read_only"); ok && ro != nil {
		readOnly = asBool(ro)
	}
	flow := NewOM().Set("mode", mode).Set("requireView", requireView).Set("steps", steps)
	return &compiledTool{kind: "flow", body: flow, refs: refs, readOnly: readOnly}
}

func compileStep(c *Corpus, som *OM, refs *refSet, viewScope string, hasRead, hasNavigate, hasInteraction *bool) (*OM, error) {
	if v, ok := som.Get("navigate"); ok && v != nil {
		*hasNavigate = true
		u := asString(v)
		refs.addString(u)
		return NewOM().Set("do", "navigate").Set("url", u), nil
	}
	if v, ok := som.Get("wait_for"); ok && v != nil {
		w := asOM(v)
		timeout := 10000
		if t, isInt := asInt(omGet(w, "timeout_ms")); isInt && t > 0 {
			timeout = t
		}
		poll := 200
		if p, isInt := asInt(omGet(w, "poll_ms")); isInt && p > 0 {
			poll = p
		}
		ir := NewOM().Set("do", "wait_for").Set("timeoutMs", timeout).Set("pollMs", poll)
		if comp := asString(omGet(w, "component")); comp != "" {
			t, _, err := resolveTarget(c, comp, nil, viewScope)
			if err != nil {
				return nil, err
			}
			collectPredRefs(t, refs)
			ir.Set("target", t)
		} else if sel := asString(omGet(w, "selector")); sel != "" {
			ir.Set("selector", sel)
		} else if ui := asString(omGet(w, "url_includes")); ui != "" {
			ir.Set("urlIncludes", ui)
			refs.addString(ui)
		}
		return ir, nil
	}
	if v, ok := som.Get("fill"); ok && v != nil {
		*hasInteraction = true
		f := asOM(v)
		t, _, err := resolveTarget(c, asString(omGet(f, "target")), nil, viewScope)
		if err != nil {
			return nil, err
		}
		collectPredRefs(t, refs)
		value, _ := f.Get("value")
		refs.addString(value)
		return NewOM().Set("do", "fill").Set("target", t).Set("value", asString(value)), nil
	}
	if v, ok := som.Get("click"); ok && v != nil {
		*hasInteraction = true
		t, _, err := resolveTarget(c, asString(v), nil, viewScope)
		if err != nil {
			return nil, err
		}
		collectPredRefs(t, refs)
		return NewOM().Set("do", "click").Set("target", t), nil
	}
	if v, ok := som.Get("press"); ok && v != nil {
		*hasInteraction = true
		ir := NewOM().Set("do", "press").Set("key", asString(v))
		if tv := asString(omGet(som, "target")); tv != "" {
			t, _, err := resolveTarget(c, tv, nil, viewScope)
			if err != nil {
				return nil, err
			}
			collectPredRefs(t, refs)
			ir.Set("target", t)
		}
		return ir, nil
	}
	if v, ok := som.Get("sleep"); ok && v != nil {
		ms, _ := asInt(v)
		return NewOM().Set("do", "sleep").Set("ms", ms), nil
	}
	if v, ok := som.Get("scroll"); ok && v != nil {
		ir := NewOM().Set("do", "scroll")
		s := asOM(v)
		if to := asString(omGet(s, "to")); to != "" {
			t, _, err := resolveTarget(c, to, nil, viewScope)
			if err != nil {
				return nil, err
			}
			collectPredRefs(t, refs)
			ir.Set("target", t)
		} else if dy, isInt := asInt(omGet(s, "delta_y")); isInt {
			ir.Set("deltaY", dy)
		}
		return ir, nil
	}
	if v, ok := som.Get("read"); ok && v != nil {
		*hasRead = true
		rom := asOM(v)
		spec := NewOM()
		for _, key := range rom.Keys() {
			vv, _ := rom.Get(key)
			vir, err := compileValueSpec(c, vv, fmt.Sprintf("read.%s", key), nil, refs, viewScope)
			if err != nil {
				return nil, err
			}
			spec.Set(key, vir)
		}
		return NewOM().Set("do", "read").Set("spec", spec), nil
	}
	return nil, nil
}

// Compile resolves the whole manifest into the ordered-JSON IR.
func Compile(c *Corpus, manifest any) (*OM, []string, []string) {
	var d diags
	mom := asOM(manifest)
	tools := []any{}
	for _, t := range asList(omGet(mom, "tools")) {
		tom := asOM(t)
		if tom == nil {
			continue
		}
		name := asString(omGet(tom, "name"))
		declared := map[string]bool{}
		var declaredOrder []string
		for _, p := range asList(omGet(tom, "params")) {
			pname := asString(omGet(asOM(p), "name"))
			if !declared[pname] {
				declared[pname] = true
				declaredOrder = append(declaredOrder, pname)
			}
		}
		var compiled *compiledTool
		if _, hasAPI := tom.Get("api"); hasAPI {
			compiled = compileAPITool(c, tom, &d)
		} else {
			compiled = compileFlowTool(c, tom, &d)
		}
		if compiled == nil {
			continue
		}
		for _, r := range compiled.refs.order {
			if !declared[r] {
				d.errf("tool %q: template references undeclared param \"{%s}\"", name, r)
			}
		}
		for _, p := range declaredOrder {
			if !compiled.refs.has(p) {
				d.warnf("tool %q: param %q is declared but never referenced", name, p)
			}
		}
		schema, params := compileParams(tom)
		var title any
		if tt := asString(omGet(tom, "title")); tt != "" {
			title = tt
		}
		toolIR := NewOM().
			Set("name", name).
			Set("title", title).
			Set("description", strings.TrimSpace(asString(omGet(tom, "description")))).
			Set("readOnly", compiled.readOnly).
			Set("inputSchema", schema).
			Set("params", params).
			Set("kind", compiled.kind).
			Set(compiled.kind, compiled.body)
		tools = append(tools, toolIR)
	}

	var descV, matchV any
	if desc := asString(omGet(mom, "description")); desc != "" {
		descV = desc
	}
	if m, ok := mom.Get("match"); ok && m != nil {
		matchV = m
	}
	toolVersion := asString(omGet(mom, "tool_version"))
	if toolVersion == "" {
		toolVersion = "0.1.0"
	}
	meta := NewOM().
		Set("site", asString(omGet(mom, "site"))).
		Set("baseUrl", asString(omGet(mom, "base_url"))).
		Set("description", descV).
		Set("toolVersion", toolVersion).
		Set("match", matchV)
	ir := NewOM().Set("meta", meta).Set("tools", tools)
	return ir, d.errors, d.warnings
}
