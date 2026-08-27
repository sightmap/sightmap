const path = require("path");
const { loadCorpus } = require("../src/corpus");
const { loadManifest } = require("../src/manifest");
const { compile } = require("../src/compile");

const FIXTURE = path.join(__dirname, "fixtures", "site");

function fixtureCompile() {
  const corpus = loadCorpus(path.join(FIXTURE, ".sightmap"));
  const { manifest, errors } = loadManifest(
    path.join(FIXTURE, "webmcp.tools.yaml"),
  );
  expect(errors).toEqual([]);
  return { corpus, ...compile(corpus, manifest) };
}

describe("corpus loading", () => {
  test("indexes globals, view components, children, and $ref expansions", () => {
    const corpus = loadCorpus(path.join(FIXTURE, ".sightmap"));
    expect([...corpus.components.keys()].sort()).toEqual([
      "Card",
      "Header",
      "Header SearchInput",
      "Row",
      "Row Buy",
      "Summary",
    ]);
    // Summary is defined per-view with different selectors.
    const summaries = corpus.components.get("Summary");
    expect(summaries).toHaveLength(2);
    expect(summaries.map((s) => s.scope).sort()).toEqual(["Detail", "Results"]);
    // $ref expansion of Header inside Results dedupes against the global.
    expect(corpus.components.get("Header")).toHaveLength(1);
    // Child chains prepend ancestor selectors; alternatives survive.
    expect(corpus.components.get("Header SearchInput")[0].chainLevels).toEqual([
      ["header.site"],
      ["input.search-v2", "input.search"],
    ]);
    expect(corpus.requests.get("SearchApi").method).toBe("POST");
    expect(corpus.views.get("Detail").route).toBe("/items/:id");
  });
});

describe("compile", () => {
  test("fixture manifest compiles without errors", () => {
    const { errors, warnings, ir } = fixtureCompile();
    expect(errors).toEqual([]);
    expect(warnings).toEqual([]);
    expect(ir.tools.map((t) => t.name)).toEqual([
      "search",
      "search_api",
      "stock",
      "buy_first_match",
    ]);
  });

  test("view scoping picks the right Summary", () => {
    const { ir } = fixtureCompile();
    const search = ir.tools.find((t) => t.name === "search");
    const readStep = search.flow.steps.find((s) => s.do === "read");
    expect(readStep.spec.summary.one.target.links[0].chain).toEqual([
      [".summary"],
    ]);
  });

  test("for_each fields resolve relative to the iterated component", () => {
    const { ir } = fixtureCompile();
    const search = ir.tools.find((t) => t.name === "search");
    const rows = ir.tools
      .find((t) => t.name === "search")
      .flow.steps.find((s) => s.do === "read").spec.rows;
    expect(rows.list.target.links[0].chain).toEqual([["li.result"]]);
    // label uses the iterated element's own corpus property.
    expect(rows.list.fields.label).toEqual({ extract: ".label" });
    expect(rows.list.fields.href).toEqual({
      extract: "attr=href",
      target: { kind: "css", selector: "a" },
    });
    expect(rows.list.max).toBe("{max}");
    expect(search.readOnly).toBe(true);
  });

  test("api tool inherits corpus request method and properties", () => {
    const { ir } = fixtureCompile();
    const api = ir.tools.find((t) => t.name === "search_api");
    expect(api.api.method).toBe("POST");
    expect(api.api.result.map((r) => r.name)).toEqual(["total", "first_title"]);
    expect(api.readOnly).toBe(false);
    const stock = ir.tools.find((t) => t.name === "stock");
    expect(stock.api.method).toBe("GET");
    expect(stock.readOnly).toBe(true);
    expect(stock.api.result).toEqual([]);
  });

  test("live flow compiles queries with predicates and require_view", () => {
    const { ir } = fixtureCompile();
    const buy = ir.tools.find((t) => t.name === "buy_first_match");
    expect(buy.flow.requireView.view).toBe("Results");
    expect(new RegExp(buy.flow.requireView.pathRegex).test("/search")).toBe(
      true,
    );
    const click = buy.flow.steps.find((s) => s.do === "click");
    expect(click.target.links.map((l) => l.name)).toEqual(["Row", "Row Buy"]);
    // Buy's chain is relative to Row.
    expect(click.target.links[1].chain).toEqual([["button.buy"]]);
    expect(click.target.links[0].preds[0]).toEqual({
      prop: "label",
      op: "=",
      value: "{label}",
      ci: false,
    });
    // fill resolves the SearchInput child chain under Header.
    const fill = buy.flow.steps.find((s) => s.do === "fill");
    expect(fill.target.links[0].chain).toEqual([
      ["header.site"],
      ["input.search-v2", "input.search"],
    ]);
    expect(buy.readOnly).toBe(false);
    expect(buy.inputSchema).toEqual({
      type: "object",
      properties: {
        label: { type: "string", description: "Row label to buy." },
      },
      required: ["label"],
    });
  });

  test("undeclared template param is an error", () => {
    const corpus = loadCorpus(path.join(FIXTURE, ".sightmap"));
    const { errors } = compile(corpus, {
      site: "x",
      base_url: "https://x.example",
      tools: [
        {
          name: "bad",
          description: "d",
          mode: "fetch",
          flow: [{ navigate: "/search?q={nope}" }],
        },
      ],
    });
    expect(errors.join("\n")).toMatch(/undeclared param "\{nope\}"/);
  });

  test("unknown component and property produce actionable errors", () => {
    const corpus = loadCorpus(path.join(FIXTURE, ".sightmap"));
    const { errors } = compile(corpus, {
      site: "x",
      base_url: "https://x.example",
      tools: [
        {
          name: "bad",
          description: "d",
          flow: [
            { click: "Nope" },
            { read: { v: { component: "Card", property: "missing" } } },
          ],
        },
      ],
    });
    expect(errors.join("\n")).toMatch(/no component named "Nope"/);
    expect(errors.join("\n")).toMatch(
      /has no property "missing" \(properties: title, price, fancy\)/,
    );
  });

  test("a view-scoped component referenced without view: gets a set-view hint", () => {
    const corpus = loadCorpus(path.join(FIXTURE, ".sightmap"));
    const { errors } = compile(corpus, {
      site: "x",
      base_url: "https://x.example",
      tools: [
        {
          name: "bad",
          description: "d",
          flow: [{ read: { s: { component: "Summary", property: "text" } } }],
        },
      ],
    });
    expect(errors.join("\n")).toMatch(
      /exists only view-scoped; set the tool's "view:"/,
    );
  });

  test("view: picks that view's definition of a shared name", () => {
    const corpus = loadCorpus(path.join(FIXTURE, ".sightmap"));
    const { errors, ir } = compile(corpus, {
      site: "x",
      base_url: "https://x.example",
      tools: [
        {
          name: "detail",
          description: "d",
          view: "Detail",
          flow: [{ read: { s: { component: "Summary", property: "text" } } }],
        },
      ],
    });
    expect(errors).toEqual([]);
    const read = ir.tools[0].flow.steps[0];
    expect(read.spec.s.one.target.links[0].chain).toEqual([
      [".detail-summary"],
    ]);
  });

  test("unknown read-spec keys and undeclared css templates are errors", () => {
    const corpus = loadCorpus(path.join(FIXTURE, ".sightmap"));
    const { errors } = compile(corpus, {
      site: "x",
      base_url: "https://x.example",
      tools: [
        {
          name: "bad",
          description: "d",
          flow: [
            { read: { v: { component: "Card", proprety: "title" } } },
            { click: 'css:[data-id="{nope}"]' },
          ],
        },
      ],
    });
    expect(errors.join("\n")).toMatch(
      /unknown key "proprety" in a read value spec/,
    );
    expect(errors.join("\n")).toMatch(/undeclared param "\{nope\}"/);
  });

  test("a parameterized api origin warns", () => {
    const corpus = loadCorpus(path.join(FIXTURE, ".sightmap"));
    const { warnings } = compile(corpus, {
      site: "x",
      base_url: "https://x.example",
      tools: [
        {
          name: "w",
          description: "d",
          params: [
            { name: "host", type: "string", required: true, description: "h" },
          ],
          api: { method: "GET", url: "https://{host}/api" },
        },
      ],
    });
    expect(warnings.join("\n")).toMatch(/origin is parameterized/);
  });

  test("manifest shape errors: match string, result mapping, bad timeouts", () => {
    const { validateManifest } = require("../src/manifest");
    const errors = [];
    validateManifest(
      {
        version: 1,
        site: "x",
        base_url: "https://x.example",
        match: "https://x.example/*",
        tools: [
          {
            name: "a",
            description: "d",
            api: { url: "/x", result: { name: "n" } },
          },
          {
            name: "b",
            description: "d",
            flow: [
              { wait_for: { selector: ".x", timeout_ms: "5s" } },
              { sleep: "long" },
              { read: { v: { selector: ".x" } } },
            ],
          },
        ],
      },
      errors,
      [],
    );
    const all = errors.join("\n");
    expect(all).toMatch(/"match" must be a list/);
    expect(all).toMatch(/"result" must be a list/);
    expect(all).toMatch(/"timeout_ms" must be a positive integer/);
    expect(all).toMatch(/sleep must be a positive integer/);
  });

  test("api url mismatching the corpus route warns", () => {
    const corpus = loadCorpus(path.join(FIXTURE, ".sightmap"));
    const { warnings } = compile(corpus, {
      site: "x",
      base_url: "https://x.example",
      tools: [
        {
          name: "w",
          description: "d",
          api: { request: "SearchApi", url: "/api/other" },
        },
      ],
    });
    expect(warnings.join("\n")).toMatch(
      /does not match corpus request "SearchApi" route/,
    );
  });

  test("mid-flow navigate in a live flow is rejected at manifest level", () => {
    const { validateManifest } = require("../src/manifest");
    const errors = [];
    validateManifest(
      {
        version: 1,
        site: "x",
        base_url: "https://x.example",
        tools: [
          {
            name: "bad",
            description: "d",
            flow: [{ navigate: "/a" }, { click: "Card" }],
          },
        ],
      },
      errors,
      [],
    );
    expect(errors.join("\n")).toMatch(/dies with the document/);
  });
});
