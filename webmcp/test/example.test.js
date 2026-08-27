/**
 * The committed IKEA example must stay in sync with its manifest + the
 * vendored atlas corpus (drift check), and the generated bundle must actually
 * boot: evaluated in jsdom it installs the shim and registers its tools with
 * a (faked) document.modelContext.
 */

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

const ROOT = path.join(__dirname, "..", "..");
const BIN = path.join(ROOT, "webmcp", "bin", "sightmap-webmcp.js");
const EXAMPLE = path.join(ROOT, "webmcp", "examples", "ikea");

test("committed IKEA bundles match a fresh generation (drift check)", () => {
  const out = execFileSync(
    process.execPath,
    [
      BIN,
      "generate",
      "--tools",
      path.join(EXAMPLE, "webmcp.tools.yaml"),
      "--format",
      "all",
      "--check",
    ],
    { cwd: ROOT, encoding: "utf8" },
  );
  expect(out).toMatch(/up to date/);
});

test("generated IKEA snippet boots in a plain page", () => {
  const src = fs.readFileSync(path.join(EXAMPLE, "ikea.webmcp.js"), "utf8");
  const registered = [];
  delete window.__sightmapWebMCP;
  document.modelContext = {
    registerTool: async (d) => registered.push(d.name),
  };

  // eslint-disable-next-line no-eval
  (0, eval)(src);

  expect(window.__sightmapWebMCP).toBeDefined();
  expect(window.__sightmapWebMCP.site).toBe("ikea");
  const names = window.__sightmapWebMCP.listTools().map((t) => t.name);
  expect(names).toEqual([
    "search_products",
    "browse_category",
    "get_product",
    "get_buyback_offers",
    "add_to_cart",
  ]);
  expect(registered).toEqual(names);
  const search = window.__sightmapWebMCP.listTools()[0];
  expect(search.annotations.readOnlyHint).toBe(true);
  expect(search.inputSchema.properties.query.type).toBe("string");
});
