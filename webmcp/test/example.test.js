/**
 * Every committed example must stay in sync with its manifest + corpus
 * (drift check), and each generated snippet must actually boot: evaluated in
 * jsdom it installs the shim and registers its tools with a (faked)
 * document.modelContext.
 */

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

const ROOT = path.join(__dirname, "..", "..");
const BIN = path.join(ROOT, "webmcp", "bin", "sightmap-webmcp.js");
const EXAMPLES_DIR = path.join(ROOT, "webmcp", "examples");

const examples = fs
  .readdirSync(EXAMPLES_DIR, { withFileTypes: true })
  .filter((e) => e.isDirectory())
  .map((e) => e.name)
  .sort();

test("there are examples to check", () => {
  expect(examples.length).toBeGreaterThan(0);
});

describe.each(examples)("example %s", (site) => {
  const dir = path.join(EXAMPLES_DIR, site);

  test("committed bundles match a fresh generation (drift check)", () => {
    const out = execFileSync(
      process.execPath,
      [
        BIN,
        "generate",
        "--tools",
        path.join(dir, "webmcp.tools.yaml"),
        "--format",
        "all",
        "--check",
      ],
      { cwd: ROOT, encoding: "utf8" },
    );
    expect(out).toMatch(/up to date/);
    expect(out).not.toMatch(/drift/);
  });

  test("generated snippet boots and registers every tool", () => {
    const src = fs.readFileSync(path.join(dir, `${site}.webmcp.js`), "utf8");
    const registered = [];
    delete window.__sightmapWebMCP;
    document.modelContext = {
      registerTool: async (d) => registered.push(d.name),
    };

    // eslint-disable-next-line no-eval
    (0, eval)(src);

    expect(window.__sightmapWebMCP).toBeDefined();
    expect(window.__sightmapWebMCP.site).toBe(site);
    const tools = window.__sightmapWebMCP.listTools();
    expect(tools.length).toBeGreaterThan(0);
    expect(registered).toEqual(tools.map((t) => t.name));
    for (const t of tools) {
      expect(t.description.length).toBeGreaterThan(20);
      expect(t.inputSchema.type).toBe("object");
      expect(typeof t.annotations.readOnlyHint).toBe("boolean");
    }
  });
});
