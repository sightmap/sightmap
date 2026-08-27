/**
 * Runtime tests — the generated bundle's browser interpreter, exercised in
 * jsdom against the fixture corpus + manifest compiled by the real pipeline.
 *
 * @jest-environment-options {"url": "https://shop.example/search?q=chairs"}
 */

const path = require("path");
const { loadCorpus } = require("../src/corpus");
const { loadManifest } = require("../src/manifest");
const { compile } = require("../src/compile");
const rt = require("../src/runtime/runtime");

const FIXTURE = path.join(__dirname, "fixtures", "site");

function compiledIR() {
  const corpus = loadCorpus(path.join(FIXTURE, ".sightmap"));
  const { manifest } = loadManifest(path.join(FIXTURE, "webmcp.tools.yaml"));
  const { ir, errors } = compile(corpus, manifest);
  expect(errors).toEqual([]);
  return ir;
}

const PAGE = `
  <header class="site"><input class="search" value="preset"></header>
  <div class="summary">50 items for "chairs"</div>
  <ul>
    <li class="result"><span class="label">Desk</span><a href="/items/1">go</a><button class="buy">Buy</button></li>
    <li class="result"><span class="label">Sofa</span><a href="/items/2">go</a><button class="buy">Buy</button></li>
  </ul>
  <div class="confirmation"></div>
`;

beforeEach(() => {
  document.body.innerHTML = PAGE;
  delete window.__sightmapWebMCP;
  delete document.modelContext;
});

describe("deep query", () => {
  test("finds elements across shadow roots", () => {
    const host = document.createElement("div");
    document.body.appendChild(host);
    const shadow = host.attachShadow({ mode: "open" });
    shadow.innerHTML =
      '<article class="card"><h2 class="card-title">Hidden</h2></article>';
    const found = rt.__smwDeepQueryAll(
      document.documentElement,
      "article.card",
    );
    expect(found).toHaveLength(1);
    expect(rt.__smwExtractValue(found[0], ".card-title")).toBe("Hidden");
  });

  test("invalid selector throws a named error", () => {
    expect(() =>
      rt.__smwDeepQueryAll(document.documentElement, ":::nope"),
    ).toThrow(/invalid selector/);
  });
});

describe("extraction + transforms", () => {
  test("modes and transforms match the sightmap spec", () => {
    document.body.innerHTML = `
      <article class="card" data-sku="A1">
        <h2 class="card-title">Chair</h2> from $19.99
        <span class="badge"></span>
      </article>`;
    const el = document.querySelector("article.card");
    expect(rt.__smwReadProp(el, { extract: ".card-title" })).toBe("Chair");
    expect(
      rt.__smwReadProp(el, { extract: "text", transform: "first_dollar" }),
    ).toBe("$19.99");
    expect(rt.__smwReadProp(el, { extract: "attr=data-sku" })).toBe("A1");
    expect(rt.__smwReadProp(el, { extract: "exists:.badge" })).toBe("true");
    expect(rt.__smwReadProp(el, { extract: "exists:.nope" })).toBeNull();
    expect(
      rt.__smwReadProp(el, {
        extract: "text",
        transform: "match:from (\\$[\\d.]+)",
      }),
    ).toBe("$19.99");
    expect(rt.__smwApplyTransform("Garden Center!", "slug")).toBe(
      "garden-center",
    );
    expect(rt.__smwApplyTransform("50 items", "first_number")).toBe("50");
  });
});

describe("target resolution", () => {
  const ir = compiledIR();
  const buy = ir.tools.find((t) => t.name === "buy_first_match");

  test("selector alternatives fall through in order", () => {
    const fill = buy.flow.steps.find((s) => s.do === "fill");
    const els = rt.__smwResolveTarget(
      document.documentElement,
      fill.target,
      {},
    );
    expect(els).toHaveLength(1);
    expect(els[0].className).toBe("search");
  });

  test("predicate filters by extracted property with template args", () => {
    const click = buy.flow.steps.find((s) => s.do === "click");
    const els = rt.__smwResolveTarget(document.documentElement, click.target, {
      label: "Sofa",
    });
    expect(els).toHaveLength(1);
    expect(els[0].closest("li").textContent).toContain("Sofa");
  });

  test("requireOne errors are actionable", () => {
    const click = buy.flow.steps.find((s) => s.do === "click");
    expect(() =>
      rt.__smwRequireOne(
        document.documentElement,
        click.target,
        { label: "Nope" },
        "click",
      ),
    ).toThrow(/matched nothing/);
    const rows = { kind: "css", selector: "li.result" };
    expect(() =>
      rt.__smwRequireOne(document.documentElement, rows, {}, "click"),
    ).toThrow(/matched 2 elements/);
  });
});

describe("live flow execution", () => {
  const ir = compiledIR();
  const buy = ir.tools.find((t) => t.name === "buy_first_match");

  test("fill + click + read against the live document", async () => {
    const sofaBuy = document.querySelectorAll("button.buy")[1];
    sofaBuy.addEventListener("click", () => {
      document.querySelector(".confirmation").textContent = "Added Sofa!";
    });
    const input = document.querySelector("input.search");
    const events = [];
    input.addEventListener("input", () => events.push("input"));

    const out = await rt.__smwExecuteTool(buy, ir.meta, { label: "Sofa" });
    expect(out).toEqual({ ok: true, confirmed: "Added Sofa!" });
    expect(input.value).toBe("Sofa");
    expect(events).toEqual(["input"]);
  });

  test("missing required param rejects", async () => {
    await expect(rt.__smwExecuteTool(buy, ir.meta, {})).rejects.toThrow(
      /missing required param "label"/,
    );
  });

  test("require_view mismatch returns a structured error with a navigation hint", async () => {
    const tool = {
      ...buy,
      flow: {
        ...buy.flow,
        requireView: {
          view: "Other",
          route: "/other",
          pathRegex: "^/other$",
          url: "https://shop.example/other",
        },
      },
    };
    const out = await rt.__smwExecuteTool(tool, ir.meta, { label: "Sofa" });
    expect(out.error).toMatch(/runs on the Other view/);
    expect(out.navigate_to).toBe("https://shop.example/other");
  });
});

describe("fetch flow execution", () => {
  const ir = compiledIR();
  const search = ir.tools.find((t) => t.name === "search");

  test("fetches the page and reads the detached document", async () => {
    global.fetch = jest.fn(async () => ({
      status: 200,
      text: async () => `<html><body>${PAGE}</body></html>`,
      headers: new Map(),
    }));
    const out = await rt.__smwExecuteTool(search, ir.meta, {
      query: "desk chair",
      max: 1,
    });
    expect(global.fetch).toHaveBeenCalledWith(
      "https://shop.example/search?q=desk%20chair",
      expect.objectContaining({ credentials: "include" }),
    );
    expect(out.summary).toBe('50 items for "chairs"');
    expect(out.rows).toEqual([{ label: "Desk", href: "/items/1" }]);
    expect(out.status).toBe(200);
  });
});

describe("api execution", () => {
  const ir = compiledIR();

  test("POST body keeps typed params; corpus result properties extract", async () => {
    global.fetch = jest.fn(async () => ({
      status: 200,
      text: async () =>
        JSON.stringify({ meta: { total: 12 }, results: [{ title: "First" }] }),
      headers: { forEach: (fn) => fn("application/json", "content-type") },
    }));
    const api = ir.tools.find((t) => t.name === "search_api");
    const out = await rt.__smwExecuteTool(api, ir.meta, { query: "chairs" });
    const [url, init] = global.fetch.mock.calls[0];
    expect(url).toBe("https://shop.example/api/search");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body)).toEqual({ q: "chairs", limit: 5 });
    expect(init.headers["content-type"]).toBe("application/json");
    expect(out).toEqual({ status: 200, total: "12", first_title: "First" });
  });

  test("no result spec echoes status, url, and parsed body", async () => {
    global.fetch = jest.fn(async () => ({
      status: 404,
      text: async () => JSON.stringify({ error: "no such sku" }),
      headers: { forEach: () => {} },
    }));
    const stock = ir.tools.find((t) => t.name === "stock");
    const out = await rt.__smwExecuteTool(stock, ir.meta, { sku: "B-2" });
    expect(global.fetch.mock.calls[0][0]).toBe(
      "https://shop.example/api/stock/B-2",
    );
    expect(out).toEqual({
      status: 404,
      url: "https://shop.example/api/stock/B-2",
      body: { error: "no such sku" },
    });
  });
});

describe("boot + registration", () => {
  const ir = compiledIR();

  test("registers with document.modelContext and installs the shim", async () => {
    const registered = [];
    document.modelContext = {
      registerTool: jest.fn(async (d) => registered.push(d)),
    };
    const shim = rt.__smwBoot(ir.meta, ir.tools);
    expect(window.__sightmapWebMCP).toBe(shim);
    expect(document.modelContext.registerTool).toHaveBeenCalledTimes(4);
    const desc = registered.find((d) => d.name === "search");
    expect(desc.description).toMatch(/Search the fixture shop/);
    expect(desc.inputSchema.required).toEqual(["query"]);
    expect(desc.annotations).toEqual({ readOnlyHint: true });
    expect(typeof desc.execute).toBe("function");
    expect(shim.listTools().map((t) => t.name)).toEqual([
      "search",
      "search_api",
      "stock",
      "buy_first_match",
    ]);
  });

  test("boot is idempotent per site", () => {
    const first = rt.__smwBoot(ir.meta, ir.tools);
    const second = rt.__smwBoot(ir.meta, ir.tools);
    expect(second).toBe(first);
  });

  test("callToolAndStore parks the outcome for eval-bridge polling", async () => {
    const shim = rt.__smwBoot(ir.meta, ir.tools);
    global.fetch = jest.fn(async () => ({
      status: 200,
      text: async () => "{}",
      headers: { forEach: () => {} },
    }));
    expect(shim.callToolAndStore("stock", { sku: "A" })).toBe("started");
    expect(shim.last.done).toBe(false);
    await new Promise((r) => setTimeout(r, 0));
    expect(shim.last).toEqual({
      tool: "stock",
      done: true,
      result: {
        status: 200,
        url: "https://shop.example/api/stock/A",
        body: {},
      },
    });
    shim.callToolAndStore("no_such_tool", {});
    await new Promise((r) => setTimeout(r, 0));
    expect(shim.last.error).toMatch(/no tool named "no_such_tool"/);
  });
});
