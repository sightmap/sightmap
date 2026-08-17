const fs = require("fs");
const path = require("path");

// __smScanCandidates/__smScanLinks call __smDeepQueryAll, so load
// deepquery.js first — the same prepend order Go uses (browser.DeepQueryJS +
// scanJS). Both are loaded via indirect eval so the test exercises the exact
// files that ship.
(0, eval)(
  fs.readFileSync(
    path.join(__dirname, "..", "browser", "deepquery.js"),
    "utf8",
  ),
);
(0, eval)(fs.readFileSync(path.join(__dirname, "scan.js"), "utf8"));

describe("__smScanCandidates", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("ranks data-testid candidates by match count, most frequent first", () => {
    document.body.innerHTML = `
      <button data-testid="save"></button>
      <button data-testid="save"></button>
      <button data-testid="cancel"></button>
    `;
    const candidates = __smScanCandidates(10);
    expect(candidates[0]).toMatchObject({
      sel: '[data-testid="save"]',
      count: 2,
    });
    expect(candidates[1]).toMatchObject({
      sel: '[data-testid="cancel"]',
      count: 1,
    });
  });

  test("strips the version suffix from data-component before grouping", () => {
    document.body.innerHTML = `
      <div data-component="Card:v1.2.3-abcdef"></div>
      <div data-component="Card:v1.2.4-fedcba"></div>
    `;
    const candidates = __smScanCandidates(10);
    expect(candidates).toEqual([
      expect.objectContaining({ sel: '[data-component^="Card"]', count: 2 }),
    ]);
  });

  test("respects max and finds candidates inside shadow DOM", () => {
    const host = document.createElement("div");
    document.body.appendChild(host);
    host.attachShadow({ mode: "open" }).innerHTML =
      '<button data-testid="shadow-btn"></button>';

    const candidates = __smScanCandidates(10);
    expect(candidates.map((c) => c.sel)).toContain(
      '[data-testid="shadow-btn"]',
    );
  });

  test("records the nearest [data-sightmap-id] ancestor", () => {
    document.body.innerHTML = `
      <div data-sightmap-id="42"><button data-testid="save"></button></div>
    `;
    const [candidate] = __smScanCandidates(10);
    expect(candidate.ancestorId).toBe("42");
  });
});

describe("__smScanLinks", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  test("returns distinct same-host pathnames in document order", () => {
    document.body.innerHTML = `
      <a href="${location.origin}/a">a</a>
      <a href="${location.origin}/b">b</a>
      <a href="${location.origin}/a">a again</a>
    `;
    expect(__smScanLinks()).toEqual(["/a", "/b"]);
  });

  test("skips cross-host links", () => {
    document.body.innerHTML = `
      <a href="${location.origin}/same-host">same</a>
      <a href="https://example.com/other-host">other</a>
    `;
    expect(__smScanLinks()).toEqual(["/same-host"]);
  });
});
