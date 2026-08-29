package webmcp

import (
	"fmt"
	"sort"
	"testing"
)

func TestCorpusIndexesGlobalsViewsChildrenAndRefs(t *testing.T) {
	c := fixtureCorpus(t)
	var names []string
	for k := range c.Components {
		names = append(names, k)
	}
	sort.Strings(names)
	want := []string{"Card", "Header", "Header SearchInput", "Row", "Row Buy", "Summary"}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatalf("components = %v, want %v", names, want)
	}

	summaries := c.Components["Summary"]
	if len(summaries) != 2 {
		t.Fatalf("Summary entries = %d, want 2", len(summaries))
	}
	scopes := []string{summaries[0].Scope, summaries[1].Scope}
	sort.Strings(scopes)
	if fmt.Sprint(scopes) != "[Detail Results]" {
		t.Fatalf("Summary scopes = %v", scopes)
	}

	if len(c.Components["Header"]) != 1 {
		t.Fatalf("Header should dedupe $ref against the global, got %d", len(c.Components["Header"]))
	}

	search := c.Components["Header SearchInput"][0]
	if fmt.Sprint(search.ChainLevels) != "[[header.site] [input.search-v2 input.search]]" {
		t.Fatalf("Header SearchInput chain = %#v", search.ChainLevels)
	}
	if c.Requests["SearchApi"].Method != "POST" {
		t.Fatalf("SearchApi method = %q", c.Requests["SearchApi"].Method)
	}
	if c.Views["Detail"].Route != "/items/:id" {
		t.Fatalf("Detail route = %q", c.Views["Detail"].Route)
	}
}

func TestViewScopingPicksTheRightSummary(t *testing.T) {
	ir := fixtureCompile(t)
	search := toolNamed(ir, "search")
	read := stepDo(search, "read")
	summary := asOM(omGet(asOM(omGet(read, "spec")), "summary"))
	one := asOM(omGet(summary, "one"))
	target := asOM(omGet(one, "target"))
	chain := omGet(firstLink(target), "chain")
	if fmt.Sprint(chain) != "[[.summary]]" {
		t.Fatalf("Summary chain = %#v (%s)", chain, StringifyJSON(chain))
	}
}

func TestForEachFieldsResolveRelativeToIteratedComponent(t *testing.T) {
	ir := fixtureCompile(t)
	search := toolNamed(ir, "search")
	read := stepDo(search, "read")
	rows := asOM(omGet(asOM(omGet(read, "spec")), "rows"))
	list := asOM(omGet(rows, "list"))
	chain := omGet(firstLink(asOM(omGet(list, "target"))), "chain")
	if fmt.Sprint(chain) != "[[li.result]]" {
		t.Fatalf("Row chain = %#v", chain)
	}
	fields := asOM(omGet(list, "fields"))
	label := asOM(omGet(fields, "label"))
	if asString(omGet(label, "extract")) != ".label" {
		t.Fatalf("label extract = %s", StringifyJSON(label))
	}
	href := asOM(omGet(fields, "href"))
	if asString(omGet(href, "extract")) != "attr=href" {
		t.Fatalf("href extract = %s", StringifyJSON(href))
	}
	hrefTarget := asOM(omGet(href, "target"))
	if asString(omGet(hrefTarget, "kind")) != "css" || asString(omGet(hrefTarget, "selector")) != "a" {
		t.Fatalf("href target = %s", StringifyJSON(hrefTarget))
	}
	if asString(omGet(list, "max")) != "{max}" {
		t.Fatalf("max = %#v", omGet(list, "max"))
	}
	if v, _ := search.Get("readOnly"); v != true {
		t.Fatalf("search.readOnly = %#v", v)
	}
}

func TestAPIToolInheritsCorpusRequest(t *testing.T) {
	ir := fixtureCompile(t)
	api := asOM(omGet(toolNamed(ir, "search_api"), "api"))
	if asString(omGet(api, "method")) != "POST" {
		t.Fatalf("method = %q", asString(omGet(api, "method")))
	}
	var names []string
	for _, r := range asList(omGet(api, "result")) {
		names = append(names, asString(omGet(asOM(r), "name")))
	}
	if fmt.Sprint(names) != "[total first_title]" {
		t.Fatalf("result names = %v", names)
	}
	if v, _ := toolNamed(ir, "search_api").Get("readOnly"); v != false {
		t.Fatalf("search_api.readOnly = %#v", v)
	}

	stock := toolNamed(ir, "stock")
	stockAPI := asOM(omGet(stock, "api"))
	if asString(omGet(stockAPI, "method")) != "GET" {
		t.Fatalf("stock method = %q", asString(omGet(stockAPI, "method")))
	}
	if v, _ := stock.Get("readOnly"); v != true {
		t.Fatalf("stock.readOnly = %#v", v)
	}
	if len(asList(omGet(stockAPI, "result"))) != 0 {
		t.Fatalf("stock result = %s", StringifyJSON(omGet(stockAPI, "result")))
	}
}

func TestLiveFlowCompilesQueriesAndRequireView(t *testing.T) {
	ir := fixtureCompile(t)
	buy := toolNamed(ir, "buy_first_match")
	flow := asOM(omGet(buy, "flow"))
	rv := asOM(omGet(flow, "requireView"))
	if asString(omGet(rv, "view")) != "Results" {
		t.Fatalf("requireView.view = %s", StringifyJSON(rv))
	}
	click := stepDo(buy, "click")
	var linkNames []string
	for _, l := range asList(omGet(asOM(omGet(click, "target")), "links")) {
		linkNames = append(linkNames, asString(omGet(asOM(l), "name")))
	}
	if fmt.Sprint(linkNames) != "[Row Row Buy]" {
		t.Fatalf("click links = %v", linkNames)
	}
	links := asList(omGet(asOM(omGet(click, "target")), "links"))
	buyChain := omGet(asOM(links[1]), "chain")
	if fmt.Sprint(buyChain) != "[[button.buy]]" {
		t.Fatalf("Buy chain = %#v", buyChain)
	}
	rowPreds := asList(omGet(asOM(links[0]), "preds"))
	pred := asOM(rowPreds[0])
	if asString(omGet(pred, "prop")) != "label" || asString(omGet(pred, "op")) != "=" || asString(omGet(pred, "value")) != "{label}" {
		t.Fatalf("pred = %s", StringifyJSON(pred))
	}
	if v, _ := pred.Get("ci"); v != false {
		t.Fatalf("ci = %#v", v)
	}

	fill := stepDo(buy, "fill")
	fillChain := omGet(firstLink(asOM(omGet(fill, "target"))), "chain")
	if fmt.Sprint(fillChain) != "[[header.site] [input.search-v2 input.search]]" {
		t.Fatalf("fill chain = %#v", fillChain)
	}
	if v, _ := buy.Get("readOnly"); v != false {
		t.Fatalf("buy.readOnly = %#v", v)
	}
	schema := asOM(omGet(buy, "inputSchema"))
	props := asOM(omGet(schema, "properties"))
	label := asOM(omGet(props, "label"))
	if asString(omGet(schema, "type")) != "object" {
		t.Fatalf("schema type = %s", StringifyJSON(schema))
	}
	if asString(omGet(label, "type")) != "string" || asString(omGet(label, "description")) != "Row label to buy." {
		t.Fatalf("label prop = %s", StringifyJSON(label))
	}
	req := asList(omGet(schema, "required"))
	if fmt.Sprint(req) != "[label]" {
		t.Fatalf("required = %#v", req)
	}
}

func TestUndeclaredTemplateParamIsAnError(t *testing.T) {
	_, errs, _ := compileYAML(t, fixtureCorpus(t), `
version: 1
site: x
base_url: https://x.example
tools:
  - name: bad
    description: d
    mode: fetch
    flow:
      - navigate: "/search?q={nope}"
`)
	wantContains(t, joinErrs(errs), `undeclared param "{nope}"`)
}

func TestUnknownComponentAndPropertyErrors(t *testing.T) {
	_, errs, _ := compileYAML(t, fixtureCorpus(t), `
version: 1
site: x
base_url: https://x.example
tools:
  - name: bad
    description: d
    flow:
      - click: Nope
      - read:
          v:
            component: Card
            property: missing
`)
	all := joinErrs(errs)
	wantContains(t, all, `no component named "Nope"`)
	wantContains(t, all, `has no property "missing" (properties: title, price, fancy)`)
}

func TestViewScopedComponentWithoutViewGetsHint(t *testing.T) {
	_, errs, _ := compileYAML(t, fixtureCorpus(t), `
version: 1
site: x
base_url: https://x.example
tools:
  - name: bad
    description: d
    flow:
      - read:
          s:
            component: Summary
            property: text
`)
	wantContains(t, joinErrs(errs), `exists only view-scoped; set the tool's "view:"`)
}

func TestViewPicksThatViewsDefinitionOfASharedName(t *testing.T) {
	ir, errs, _ := compileYAML(t, fixtureCorpus(t), `
version: 1
site: x
base_url: https://x.example
tools:
  - name: detail
    description: d
    view: Detail
    flow:
      - read:
          s:
            component: Summary
            property: text
`)
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	read := stepDo(toolNamed(ir, "detail"), "read")
	spec := asOM(omGet(read, "spec"))
	s := asOM(omGet(spec, "s"))
	one := asOM(omGet(s, "one"))
	chain := omGet(firstLink(asOM(omGet(one, "target"))), "chain")
	if fmt.Sprint(chain) != "[[.detail-summary]]" {
		t.Fatalf("chain = %#v", chain)
	}
}

func TestUnknownReadSpecKeysAndUndeclaredCSSTemplates(t *testing.T) {
	_, errs, _ := compileYAML(t, fixtureCorpus(t), `
version: 1
site: x
base_url: https://x.example
tools:
  - name: bad
    description: d
    flow:
      - read:
          v:
            component: Card
            proprety: title
      - click: 'css:[data-id="{nope}"]'
`)
	all := joinErrs(errs)
	wantContains(t, all, `unknown key "proprety" in a read value spec`)
	wantContains(t, all, `undeclared param "{nope}"`)
}

func TestLiteralCorpusRouteServesAsAPIURLFallback(t *testing.T) {
	ir, errs, _ := compileYAML(t, fixtureCorpus(t), `
version: 1
site: x
base_url: https://x.example
tools:
  - name: t
    description: d
    params:
      - name: query
        type: string
        required: true
        description: q
    api:
      request: SearchApi
      body:
        q: "{query}"
`)
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	api := asOM(omGet(toolNamed(ir, "t"), "api"))
	if asString(omGet(api, "url")) != "/api/search" {
		t.Fatalf("url = %q", asString(omGet(api, "url")))
	}
	if asString(omGet(api, "method")) != "POST" {
		t.Fatalf("method = %q", asString(omGet(api, "method")))
	}
}

func TestParameterizedAPIOriginWarns(t *testing.T) {
	_, _, warns := compileYAML(t, fixtureCorpus(t), `
version: 1
site: x
base_url: https://x.example
tools:
  - name: w
    description: d
    params:
      - name: host
        type: string
        required: true
        description: h
    api:
      method: GET
      url: "https://{host}/api"
`)
	wantContains(t, joinErrs(warns), "origin is parameterized")
}

func TestManifestShapeErrors(t *testing.T) {
	doc := parseManifestYAML(t, `
version: 1
site: x
base_url: https://x.example
match: "https://x.example/*"
tools:
  - name: a
    description: d
    api:
      url: /x
      result:
        name: n
  - name: b
    description: d
    flow:
      - wait_for:
          selector: .x
          timeout_ms: 5s
      - sleep: long
      - read:
          v:
            selector: .x
`)
	var d diags
	validateManifest(doc, &d)
	all := joinErrs(d.errors)
	wantContains(t, all, `"match" must be a list`)
	wantContains(t, all, `"result" must be a list`)
	wantContains(t, all, `"timeout_ms" must be a positive integer`)
	wantContains(t, all, "sleep must be a positive integer")
}

func TestAPIURLMismatchingCorpusRouteWarns(t *testing.T) {
	_, _, warns := compileYAML(t, fixtureCorpus(t), `
version: 1
site: x
base_url: https://x.example
tools:
  - name: w
    description: d
    api:
      request: SearchApi
      url: /api/other
`)
	wantContains(t, joinErrs(warns), `does not match corpus request "SearchApi" route`)
}

func TestMidFlowNavigateRejectedAtManifestLevel(t *testing.T) {
	doc := parseManifestYAML(t, `
version: 1
site: x
base_url: https://x.example
tools:
  - name: bad
    description: d
    flow:
      - navigate: /a
      - click: Card
`)
	var d diags
	validateManifest(doc, &d)
	wantContains(t, joinErrs(d.errors), "dies with the document")
}
