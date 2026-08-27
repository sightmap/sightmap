package webmcp

// Tool-manifest loader — a direct port of webmcp/src/manifest.js: structural
// validation of a webmcp.tools.yaml. Cross-validation against the corpus
// lives in compile.go. Structural problems are errors; unknown fields warn.

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	siteRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	toolNameRe  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	paramNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	methodRe    = regexp.MustCompile(`^[A-Z]+$`)
)

var paramTypes = []string{"string", "number", "integer", "boolean"}
var stepActions = []string{"navigate", "wait_for", "fill", "click", "press", "sleep", "scroll", "read"}
var stepOptionKeys = map[string][]string{"press": {"target"}}
var rootKeys = []string{"version", "site", "base_url", "description", "tool_version", "match", "sightmap", "tools"}
var toolKeys = []string{"name", "title", "description", "read_only", "require_view", "view", "params", "api", "flow", "mode"}
var paramKeys = []string{"name", "type", "description", "required", "enum", "default"}
var apiKeys = []string{"request", "method", "url", "query", "headers", "body", "result", "max_body_chars"}
var resultKeys = []string{"name", "source", "field", "pattern", "transform"}
var resultSources = []string{"req.body", "rsp.body", "req.headers", "rsp.headers"}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

type diags struct {
	errors   []string
	warnings []string
}

func (d *diags) errf(format string, a ...any) { d.errors = append(d.errors, fmt.Sprintf(format, a...)) }
func (d *diags) warnf(format string, a ...any) {
	d.warnings = append(d.warnings, fmt.Sprintf(format, a...))
}

func warnUnknown(om *OM, known []string, where string, d *diags) {
	if om == nil {
		return
	}
	for _, k := range om.Keys() {
		if !contains(known, k) {
			d.warnf("%s: unknown field %q", where, k)
		}
	}
}

func validateParams(params any, where string, d *diags) {
	list, ok := params.([]any)
	if !ok {
		d.errf("%s: \"params\" must be a list", where)
		return
	}
	seen := map[string]bool{}
	for _, p := range list {
		pom := asOM(p)
		if pom == nil {
			d.errf("%s: each param must be a mapping", where)
			continue
		}
		name := asString(omGet(pom, "name"))
		warnUnknown(pom, paramKeys, fmt.Sprintf("%s param %q", where, name), d)
		if name == "" || !paramNameRe.MatchString(name) {
			d.errf("%s: param name %q must match %s", where, name, paramNameRe)
		} else if seen[name] {
			d.errf("%s: duplicate param %q", where, name)
		} else {
			seen[name] = true
		}
		ptype := asString(omGet(pom, "type"))
		if !contains(paramTypes, ptype) {
			d.errf("%s param %q: type must be one of %s", where, name, strings.Join(paramTypes, "|"))
		}
		if asString(omGet(pom, "description")) == "" {
			d.warnf("%s param %q: missing description (agents rely on it)", where, name)
		}
	}
}

func validateAPI(api any, where string, d *diags) {
	aom := asOM(api)
	if aom == nil {
		d.errf("%s: \"api\" must be a mapping", where)
		return
	}
	warnUnknown(aom, apiKeys, where, d)
	if asString(omGet(aom, "url")) == "" && asString(omGet(aom, "request")) == "" {
		d.errf("%s: api needs a \"url\" template (or a corpus \"request\" whose route is literal)", where)
	}
	if m := asString(omGet(aom, "method")); m != "" && !methodRe.MatchString(m) {
		d.errf("%s: api method %q must be an upper-case HTTP method", where, m)
	}
	for _, r := range asList(omGet(aom, "result")) {
		rom := asOM(r)
		if rom == nil {
			d.errf("%s: each result entry must be a mapping", where)
			continue
		}
		name := asString(omGet(rom, "name"))
		rw := fmt.Sprintf("%s result %q", where, name)
		warnUnknown(rom, resultKeys, rw, d)
		if name == "" || !paramNameRe.MatchString(name) {
			d.errf("%s: name must match %s", rw, paramNameRe)
		}
		source := asString(omGet(rom, "source"))
		if !contains(resultSources, source) {
			d.errf("%s: source must be one of %s", rw, strings.Join(resultSources, "|"))
		}
		field := asString(omGet(rom, "field"))
		pattern := asString(omGet(rom, "pattern"))
		if field == "" && pattern == "" {
			d.errf("%s: needs \"field\" and/or \"pattern\"", rw)
		}
		if (source == "req.headers" || source == "rsp.headers") && field == "" {
			d.errf("%s: a headers source requires \"field\"", rw)
		}
		if pattern != "" {
			if _, err := regexp.Compile(pattern); err != nil {
				d.errf("%s: invalid pattern: %v", rw, err)
			}
		}
	}
}

func stepAction(step *OM) string {
	var found []string
	for _, k := range step.Keys() {
		if contains(stepActions, k) {
			found = append(found, k)
		}
	}
	if len(found) == 1 {
		return found[0]
	}
	return ""
}

func validateFlow(flow any, mode, where string, d *diags) {
	steps, ok := flow.([]any)
	if !ok || len(steps) == 0 {
		d.errf("%s: \"flow\" must be a non-empty list of steps", where)
		return
	}
	for i, s := range steps {
		sw := fmt.Sprintf("%s step %d", where, i+1)
		som := asOM(s)
		if som == nil {
			d.errf("%s: each step must be a mapping with one action key", sw)
			continue
		}
		action := stepAction(som)
		if action == "" {
			d.errf("%s: expected exactly one action key (%s), got: %s", sw,
				strings.Join(stepActions, "|"), strings.Join(som.Keys(), ", "))
			continue
		}
		known := append([]string{action}, stepOptionKeys[action]...)
		warnUnknown(som, known, sw, d)
		if action == "navigate" {
			if mode == "fetch" && i != 0 {
				d.errf("%s: in a fetch flow \"navigate\" must be the first step", sw)
			}
			if mode == "live" && i != len(steps)-1 {
				d.errf("%s: in a live flow \"navigate\" is only allowed as the final step — "+
					"a WebMCP tool call dies with the document it runs in, so a mid-flow "+
					"navigation can never return a result. Split the tool, or use \"mode: fetch\" for read-only flows.", sw)
			}
		}
		if mode == "fetch" && action != "navigate" && action != "read" {
			d.errf("%s: a fetch flow only supports \"navigate\" and \"read\" (it runs on a detached fetched document)", sw)
		}
		switch action {
		case "wait_for":
			w := asOM(omGet(som, "wait_for"))
			if w == nil || (asString(omGet(w, "component")) == "" && asString(omGet(w, "selector")) == "" && asString(omGet(w, "url_includes")) == "") {
				d.errf("%s: wait_for needs one of component|selector|url_includes", sw)
			}
		case "fill":
			f := asOM(omGet(som, "fill"))
			var value any
			if f != nil {
				value, _ = f.Get("value")
			}
			if f == nil || asString(omGet(f, "target")) == "" || value == nil {
				d.errf("%s: fill needs \"target\" and \"value\"", sw)
			}
		case "read":
			if asOM(omGet(som, "read")) == nil {
				d.errf("%s: read must be a mapping of result keys to value specs", sw)
			}
		}
	}
	if first := asOM(steps[0]); mode == "fetch" && (first == nil || stepAction(first) != "navigate") {
		d.errf("%s: a fetch flow must start with \"navigate\"", where)
	}
}

func validateManifest(doc any, d *diags) {
	dom := asOM(doc)
	if dom == nil {
		d.errf("manifest is empty or not a mapping")
		return
	}
	warnUnknown(dom, rootKeys, "manifest", d)
	if v, _ := asInt(omGet(dom, "version")); v != 1 {
		d.errf("manifest must declare \"version: 1\"")
	}
	site := asString(omGet(dom, "site"))
	if site == "" || !siteRe.MatchString(site) {
		d.errf("\"site\" must be a slug matching %s (got %q)", siteRe, site)
	}
	baseURL := asString(omGet(dom, "base_url"))
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		d.errf("\"base_url\" must be an absolute http(s) URL (got %q)", baseURL)
	}
	tools := asList(omGet(dom, "tools"))
	if len(tools) == 0 {
		d.errf("\"tools\" must be a non-empty list")
		return
	}
	seen := map[string]bool{}
	for i, t := range tools {
		tom := asOM(t)
		name := asString(omGet(tom, "name"))
		where := fmt.Sprintf("tool %d (%q)", i+1, name)
		if tom == nil {
			d.errf("%s: must be a mapping", where)
			continue
		}
		warnUnknown(tom, toolKeys, where, d)
		if name == "" || !toolNameRe.MatchString(name) {
			d.errf("%s: \"name\" must match %s", where, toolNameRe)
		} else if seen[name] {
			d.errf("%s: duplicate tool name", where)
		} else {
			seen[name] = true
		}
		if strings.TrimSpace(asString(omGet(tom, "description"))) == "" {
			d.errf("%s: \"description\" is required — it is what the agent decides by", where)
		}
		if params, ok := tom.Get("params"); ok && params != nil {
			validateParams(params, where, d)
		}
		_, hasAPI := tom.Get("api")
		_, hasFlow := tom.Get("flow")
		if hasAPI == hasFlow {
			d.errf("%s: exactly one of \"api\" or \"flow\" is required", where)
			continue
		}
		mode := asString(omGet(tom, "mode"))
		if tom.Has("mode") && !hasFlow {
			d.warnf("%s: \"mode\" only applies to flow tools", where)
		}
		if tom.Has("mode") && mode != "live" && mode != "fetch" {
			d.errf("%s: \"mode\" must be live|fetch", where)
		}
		effMode := mode
		if effMode == "" {
			effMode = "live"
		}
		if tom.Has("require_view") && (!hasFlow || effMode != "live") {
			d.warnf("%s: \"require_view\" only applies to live flow tools", where)
		}
		if tom.Has("view") && !hasFlow {
			d.warnf("%s: \"view\" only affects flow tools (it scopes component resolution)", where)
		}
		if hasAPI {
			api, _ := tom.Get("api")
			validateAPI(api, where, d)
		}
		if hasFlow {
			flow, _ := tom.Get("flow")
			validateFlow(flow, effMode, where, d)
		}
	}
}

// LoadManifest parses and structurally validates a manifest file.
func LoadManifest(file string) (any, []string, []string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, nil, nil, err
	}
	doc, err := parseYAMLOrdered(data)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: %v", file, err)}, nil, nil
	}
	var d diags
	validateManifest(doc, &d)
	return doc, d.errors, d.warnings, nil
}
